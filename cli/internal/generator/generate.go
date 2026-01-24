// Where: cli/internal/generator/generate.go
// What: Generator entrypoints for file generation.
// Why: Provide a Go-based implementation of the Python generator workflow.
package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/meta"
)

// GenerateOptions configures Generator.GenerateFiles behavior.
type GenerateOptions struct {
	ProjectRoot         string
	DryRun              bool
	Verbose             bool
	Registry            string
	Tag                 string
	Parameters          map[string]string
	SitecustomizeSource string
	Parser              Parser
}

// GenerateFiles runs the generator pipeline: parse, stage assets, and render configs.
func GenerateFiles(cfg config.GeneratorConfig, opts GenerateOptions) ([]FunctionSpec, error) {
	projectRoot := opts.ProjectRoot
	if projectRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		projectRoot = wd
	}

	projectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}

	templatePath, err := resolveTemplatePath(cfg.Paths.SamTemplate, projectRoot)
	if err != nil {
		return nil, err
	}
	baseDir := filepath.Dir(templatePath)
	outputDir, err := resolveOutputDir(cfg.Paths.OutputDir, baseDir)
	if err != nil {
		return nil, err
	}

	contents, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}

	parameters := mergeParameters(cfg.Parameters, opts.Parameters)
	parser := opts.Parser
	if parser == nil {
		parser = DefaultParser{}
	}

	if opts.Verbose {
		fmt.Println("Parsing template...")
	}

	parsed, err := parser.Parse(string(contents), parameters)
	if err != nil {
		return nil, err
	}
	if err := applyImageNames(parsed.Functions); err != nil {
		return nil, err
	}

	functionsDir := filepath.Join(outputDir, "functions")
	layerCacheDir := filepath.Join(outputDir, ".layers_cache")

	if !opts.DryRun {
		if err := removeDir(functionsDir); err != nil {
			return nil, err
		}
		if err := ensureDir(layerCacheDir); err != nil {
			return nil, err
		}
	}

	resolvedTag := resolveTag(opts.Tag, "")
	functions := make([]FunctionSpec, 0, len(parsed.Functions))

	for _, fn := range parsed.Functions {
		if opts.Verbose {
			fmt.Printf("Processing function: %s\n", fn.Name)
		}
		staged, err := stageFunction(
			fn,
			stageContext{
				BaseDir:           baseDir,
				OutputDir:         outputDir,
				FunctionsDir:      functionsDir,
				LayerCacheDir:     layerCacheDir,
				DryRun:            opts.DryRun,
				Verbose:           opts.Verbose,
				ProjectRoot:       projectRoot,
				SitecustomizePath: opts.SitecustomizeSource,
			},
		)
		if err != nil {
			return nil, err
		}

		dockerConfig := DockerConfig{
			SitecustomizeSource: staged.SitecustomizeRef,
		}
		dockerfile, err := RenderDockerfile(staged.Function, dockerConfig, opts.Registry, resolvedTag)
		if err != nil {
			return nil, err
		}
		if !opts.DryRun {
			if err := writeFile(filepath.Join(staged.FunctionDir, "Dockerfile"), dockerfile); err != nil {
				return nil, err
			}
		}

		functions = append(functions, staged.Function)
	}

	sortFunctionsByName(functions)

	functionsYmlPath := resolveConfigPath(cfg.Paths.FunctionsYml, baseDir, outputDir, "functions.yml")
	functionsContent, err := RenderFunctionsYml(functions, opts.Registry, resolvedTag)
	if err != nil {
		return nil, err
	}
	if !opts.DryRun {
		if err := writeConfigFile(functionsYmlPath, functionsContent); err != nil {
			return nil, err
		}
	}

	routingYmlPath := resolveConfigPath(cfg.Paths.RoutingYml, baseDir, outputDir, "routing.yml")
	routingContent, err := RenderRoutingYml(functions)
	if err != nil {
		return nil, err
	}
	if !opts.DryRun {
		if err := writeConfigFile(routingYmlPath, routingContent); err != nil {
			return nil, err
		}
	}

	resourcesYmlPath := resolveConfigPath(cfg.Paths.ResourcesYml, baseDir, outputDir, "resources.yml")
	resourcesContent, err := RenderResourcesYml(parsed.Resources)
	if err != nil {
		return nil, err
	}
	if !opts.DryRun {
		if err := writeConfigFile(resourcesYmlPath, resourcesContent); err != nil {
			return nil, err
		}
	}

	return functions, nil
}

// resolveTemplatePath determines the absolute path to the SAM template.
func resolveTemplatePath(samTemplate, projectRoot string) (string, error) {
	if strings.TrimSpace(samTemplate) == "" {
		return "", fmt.Errorf("sam_template is required")
	}
	path := samTemplate
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectRoot, path)
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

// resolveOutputDir returns the absolute output directory where artifacts will be staged.
func resolveOutputDir(outputDir, baseDir string) (string, error) {
	normalized := normalizeOutputDir(outputDir)
	path := normalized
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Clean(path), nil
}

// resolveTag picks the Docker image tag from opts first, then generator config, then "latest".
func resolveTag(tag, fallback string) string {
	if strings.TrimSpace(tag) != "" {
		return tag
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "latest"
}

func sortFunctionsByName(functions []FunctionSpec) {
	sort.Slice(functions, func(i, j int) bool {
		return functions[i].Name < functions[j].Name
	})
}

// mergeParameters merges generator config parameters with runtime overrides.
func mergeParameters(cfgParams map[string]any, overrides map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range cfgParams {
		if value == nil {
			continue
		}
		out[key] = fmt.Sprint(value)
	}
	for key, value := range overrides {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out[key] = value
	}
	return out
}

// resolveConfigPath chooses where to write config files (functions/routing).
func resolveConfigPath(explicit, baseDir, outputDir, name string) string {
	if strings.TrimSpace(explicit) == "" {
		return filepath.Join(outputDir, "config", name)
	}
	path := explicit
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Clean(path)
}

func normalizeOutputDir(outputDir string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(outputDir), "/\\")
	if trimmed == "" {
		return meta.OutputDir
	}
	return trimmed
}
