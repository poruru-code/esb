// Where: tools-go/internal/generator/stage.go
// What: File staging helpers for generator output.
// Why: Keep GenerateFiles readable and testable.
package generator

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type stageContext struct {
	BaseDir           string
	OutputDir         string
	FunctionsDir      string
	LayersDir         string
	ProjectRoot       string
	SitecustomizePath string
	LayerCache        map[string]string
	DryRun            bool
	Verbose           bool
}

type stagedFunction struct {
	Function        FunctionSpec
	FunctionDir     string
	SitecustomizeRef string
}

func stageFunction(fn FunctionSpec, ctx stageContext) (stagedFunction, error) {
	if fn.Name == "" {
		return stagedFunction{}, fmt.Errorf("function name is required")
	}

	functionDir := filepath.Join(ctx.FunctionsDir, fn.Name)
	if !ctx.DryRun {
		if err := ensureDir(functionDir); err != nil {
			return stagedFunction{}, err
		}
	}

	sourceDir := resolveResourcePath(ctx.BaseDir, fn.CodeURI)
	stagingSrc := filepath.Join(functionDir, "src")
	if !ctx.DryRun && dirExists(sourceDir) {
		if err := copyDir(sourceDir, stagingSrc); err != nil {
			return stagedFunction{}, err
		}
	}

	fn.CodeURI = ensureSlash(path.Join("functions", fn.Name, "src"))
	fn.HasRequirements = fileExists(filepath.Join(stagingSrc, "requirements.txt"))

	stagedLayers, err := stageLayers(fn.Layers, ctx)
	if err != nil {
		return stagedFunction{}, err
	}
	fn.Layers = stagedLayers

	siteRef := path.Join("functions", fn.Name, "sitecustomize.py")
	siteRef = filepath.ToSlash(siteRef)
	if !ctx.DryRun {
		siteSrc := resolveSitecustomizeSource(ctx)
		if siteSrc != "" {
			if err := copyFile(siteSrc, filepath.Join(functionDir, "sitecustomize.py")); err != nil {
				return stagedFunction{}, err
			}
		}
	}

	return stagedFunction{
		Function:        fn,
		FunctionDir:     functionDir,
		SitecustomizeRef: siteRef,
	}, nil
}

func stageLayers(layers []LayerSpec, ctx stageContext) ([]LayerSpec, error) {
	if len(layers) == 0 {
		return nil, nil
	}

	staged := make([]LayerSpec, 0, len(layers))
	for _, layer := range layers {
		source := resolveResourcePath(ctx.BaseDir, layer.ContentURI)
		if !fileOrDirExists(source) {
			continue
		}

		cacheKey := filepath.Clean(source)
		if cached, ok := ctx.LayerCache[cacheKey]; ok {
			layer.ContentURI = cached
			staged = append(staged, layer)
			continue
		}

		targetName := layerStagingName(source, ctx.BaseDir)
		targetDir := filepath.Join(ctx.LayersDir, targetName)
		if !ctx.DryRun {
			if err := removeDir(targetDir); err != nil {
				return nil, err
			}
			if err := ensureDir(targetDir); err != nil {
				return nil, err
			}
			if fileExists(source) && strings.HasSuffix(strings.ToLower(source), ".zip") {
				if err := unzipFile(source, targetDir); err != nil {
					return nil, err
				}
			} else if dirExists(source) {
				if err := copyDir(source, targetDir); err != nil {
					return nil, err
				}
			} else {
				continue
			}
		}

		layerRef := filepath.ToSlash(filepath.Join("layers", targetName))
		layer.ContentURI = layerRef
		ctx.LayerCache[cacheKey] = layerRef
		staged = append(staged, layer)
	}

	return staged, nil
}

func resolveResourcePath(baseDir string, raw string) string {
	trimmed := strings.TrimLeft(raw, "/\\")
	if trimmed == "" {
		trimmed = raw
	}
	return filepath.Clean(filepath.Join(baseDir, trimmed))
}

func resolveSitecustomizeSource(ctx stageContext) string {
	source := ctx.SitecustomizePath
	if strings.TrimSpace(source) == "" {
		source = defaultSitecustomizeSource
	}

	if filepath.IsAbs(source) {
		if fileExists(source) {
			return source
		}
		return ""
	}

	candidate := filepath.Clean(filepath.Join(ctx.BaseDir, source))
	if fileExists(candidate) {
		return candidate
	}

	candidate = filepath.Clean(filepath.Join(ctx.ProjectRoot, source))
	if fileExists(candidate) {
		return candidate
	}
	return ""
}

var layerNamePattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func layerStagingName(source string, baseDir string) string {
	rel, err := filepath.Rel(baseDir, source)
	name := ""
	if err == nil {
		name = filepath.ToSlash(strings.Trim(rel, "/"))
	}
	if name == "" || strings.HasPrefix(name, "..") {
		name = filepath.Base(source)
	}
	safe := layerNamePattern.ReplaceAllString(name, "_")
	if safe == "" {
		return filepath.Base(source)
	}
	return safe
}

func ensureSlash(value string) string {
	if strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}
