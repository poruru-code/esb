// Where: cli/internal/infra/templategen/generate.go
// What: Generator entrypoint orchestration.
// Why: Coordinate parse, staging, and render phases for deploy artifacts.
package templategen

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/config"
	samparser "github.com/poruru/edge-serverless-box/cli/internal/infra/sam"
)

// GenerateOptions configures Generator.GenerateFiles behavior.
type GenerateOptions struct {
	ProjectRoot         string
	DryRun              bool
	Verbose             bool
	Out                 io.Writer
	Registry            string
	BuildRegistry       string
	RuntimeRegistry     string
	Tag                 string
	Parameters          map[string]string
	SitecustomizeSource string
	Parser              samparser.Parser
}

// GenerateFiles runs the generator pipeline: parse, stage assets, and render configs.
func GenerateFiles(cfg config.GeneratorConfig, opts GenerateOptions) ([]template.FunctionSpec, error) {
	out := resolveGenerateOutput(opts.Out)
	errOut := resolveGenerateErrOutput(opts.Out)

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
	outputDir := resolveOutputDir(cfg.Paths.OutputDir, baseDir)

	contents, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}

	parameters := mergeParameters(cfg.Parameters, opts.Parameters)
	parser := opts.Parser
	if parser == nil {
		parser = samparser.DefaultParser{}
	}

	if opts.Verbose {
		_, _ = fmt.Fprintln(out, "Parsing template...")
	}

	parsed, err := parser.Parse(string(contents), parameters)
	if err != nil {
		return nil, err
	}
	for _, warning := range parsed.Warnings {
		_, _ = fmt.Fprintf(errOut, "Warning: %s\n", warning)
	}
	if err := template.ApplyImageNames(parsed.Functions); err != nil {
		return nil, err
	}

	resolvedTag := resolveTag(opts.Tag, "")
	buildRegistry := opts.BuildRegistry
	if strings.TrimSpace(buildRegistry) == "" {
		buildRegistry = opts.Registry
	}
	runtimeRegistry := opts.RuntimeRegistry
	if strings.TrimSpace(runtimeRegistry) == "" {
		runtimeRegistry = opts.Registry
	}
	imageImports, err := resolveImageImports(parsed.Functions, runtimeRegistry)
	if err != nil {
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

	functions := make([]template.FunctionSpec, 0, len(parsed.Functions))
	javaRuntimeBuild := &javaRuntimeBuildState{}

	for _, fn := range parsed.Functions {
		if opts.Verbose {
			_, _ = fmt.Fprintf(out, "Processing function: %s\n", fn.Name)
		}
		if strings.TrimSpace(fn.ImageSource) != "" {
			functions = append(functions, fn)
			continue
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
				Out:               out,
				JavaRuntimeBuild:  javaRuntimeBuild,
				ProjectRoot:       projectRoot,
				SitecustomizePath: opts.SitecustomizeSource,
			},
		)
		if err != nil {
			return nil, err
		}

		dockerConfig := template.DockerConfig{
			SitecustomizeSource: staged.SitecustomizeRef,
		}
		dockerfile, err := template.RenderDockerfile(staged.Function, dockerConfig, buildRegistry, resolvedTag)
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
	functionsContent, err := template.RenderFunctionsYml(functions, runtimeRegistry, resolvedTag)
	if err != nil {
		return nil, err
	}
	if !opts.DryRun {
		if err := writeConfigFile(functionsYmlPath, functionsContent); err != nil {
			return nil, err
		}
	}

	routingYmlPath := resolveConfigPath(cfg.Paths.RoutingYml, baseDir, outputDir, "routing.yml")
	routingContent, err := template.RenderRoutingYml(functions)
	if err != nil {
		return nil, err
	}
	if !opts.DryRun {
		if err := writeConfigFile(routingYmlPath, routingContent); err != nil {
			return nil, err
		}
	}

	resourcesYmlPath := resolveConfigPath(cfg.Paths.ResourcesYml, baseDir, outputDir, "resources.yml")
	resourcesContent, err := template.RenderResourcesYml(parsed.Resources)
	if err != nil {
		return nil, err
	}
	if !opts.DryRun {
		if err := writeConfigFile(resourcesYmlPath, resourcesContent); err != nil {
			return nil, err
		}
	}

	imageImportPath := filepath.Join(outputDir, "config", "image-import.json")
	if !opts.DryRun {
		if err := writeImageImportManifest(imageImportPath, imageImports); err != nil {
			return nil, err
		}
	}

	return functions, nil
}
