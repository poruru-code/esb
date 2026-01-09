// Where: cli/internal/generator/stage.go
// What: File staging helpers for generator output.
// Why: Keep GenerateFiles readable and testable.
package generator

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type stageContext struct {
	BaseDir           string
	OutputDir         string
	FunctionsDir      string
	ProjectRoot       string
	SitecustomizePath string
	LayerCacheDir     string
	DryRun            bool
	Verbose           bool
}

type stagedFunction struct {
	Function         FunctionSpec
	FunctionDir      string
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

	stagedLayers, err := stageLayers(fn.Layers, ctx, fn.Name, functionDir)
	if err != nil {
		return stagedFunction{}, err
	}
	fn.Layers = stagedLayers

	siteRef := path.Join("functions", fn.Name, "sitecustomize.py")
	siteRef = filepath.ToSlash(siteRef)
	if !ctx.DryRun {
		siteSrc := resolveSitecustomizeSource(ctx)
		if siteSrc != "" {
			if info, err := os.Stat(siteSrc); err == nil {
				if err := linkOrCopyFile(siteSrc, filepath.Join(functionDir, "sitecustomize.py"), info.Mode()); err != nil {
					return stagedFunction{}, err
				}
			}
		}
	}

	return stagedFunction{
		Function:         fn,
		FunctionDir:      functionDir,
		SitecustomizeRef: siteRef,
	}, nil
}

func stageLayers(layers []LayerSpec, ctx stageContext, functionName, functionDir string) ([]LayerSpec, error) {
	if len(layers) == 0 {
		return nil, nil
	}

	staged := make([]LayerSpec, 0, len(layers))
	layersDir := filepath.Join(functionDir, "layers")
	for _, layer := range layers {
		source := resolveResourcePath(ctx.BaseDir, layer.ContentURI)
		if !fileOrDirExists(source) {
			continue
		}

		targetName := layerTargetName(source)
		if targetName == "" {
			continue
		}

		layerRef := filepath.ToSlash(filepath.Join("functions", functionName, "layers", targetName))
		if !ctx.DryRun {
			targetDir := filepath.Join(layersDir, targetName)
			if err := removeDir(targetDir); err != nil {
				return nil, err
			}

			var finalSrc string
			if fileExists(source) && strings.HasSuffix(strings.ToLower(source), ".zip") {
				extracted, err := extractZipLayer(source, ctx.LayerCacheDir)
				if err != nil {
					return nil, err
				}
				finalSrc = extracted
			} else if dirExists(source) {
				finalSrc = source
			} else {
				continue
			}

			finalDest := targetDir
			if dirExists(source) && filepath.Base(source) == "python" {
				finalDest = filepath.Join(targetDir, "python")
			}

			if err := copyDirLinkOrCopy(finalSrc, finalDest); err != nil {
				return nil, err
			}
		}

		layer.ContentURI = layerRef
		staged = append(staged, layer)
	}

	return staged, nil
}

func resolveResourcePath(baseDir, raw string) string {
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

func layerTargetName(source string) string {
	base := filepath.Base(source)
	if strings.HasSuffix(strings.ToLower(base), ".zip") {
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
	return base
}

func ensureSlash(value string) string {
	if strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}
