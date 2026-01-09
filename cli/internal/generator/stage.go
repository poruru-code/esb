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
	"unicode"
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

// stageFunction prepares the function source, layers, and sitecustomize file
// under the output directory so downstream steps can render Dockerfiles.
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

	stagedLayers, err := stageLayers(fn.Layers, ctx, fn.Name, functionDir, fn.Runtime)
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

// stageLayers stages each referenced layer inside the function directory,
// applying smart nesting for Python runtimes and sanitizing names.
func stageLayers(layers []LayerSpec, ctx stageContext, functionName, functionDir, runtime string) ([]LayerSpec, error) {
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

		targetName := layerTargetName(layer, source)
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
			if shouldNestPython(runtime, finalSrc) {
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

// layerTargetName derives a filesystem-safe directory name for a layer.
func layerTargetName(layer LayerSpec, source string) string {
	if sanitized := sanitizeLayerName(layer.Name); sanitized != "" {
		return sanitized
	}
	base := filepath.Base(source)
	if strings.HasSuffix(strings.ToLower(base), ".zip") {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if sanitized := sanitizeLayerName(base); sanitized != "" {
		return sanitized
	}
	return "layer"
}

// sanitizeLayerName keeps only alphanumeric, dot, underscore, and dash characters.
func sanitizeLayerName(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// shouldNestPython returns true when a Python layer lacks an explicit python/
// layout and therefore must be nested to satisfy the runtime expectation.
func shouldNestPython(runtime, sourceDir string) bool {
	if !isPythonRuntime(runtime) {
		return false
	}
	if sourceDir == "" {
		return false
	}
	return !containsPythonLayout(sourceDir)
}

// isPythonRuntime detects if the runtime string references Python.
func isPythonRuntime(runtime string) bool {
	return strings.Contains(strings.ToLower(runtime), "python")
}

// containsPythonLayout checks for python/ or site-packages/ at the root level.
func containsPythonLayout(dir string) bool {
	return dirExists(filepath.Join(dir, "python")) || dirExists(filepath.Join(dir, "site-packages"))
}

func ensureSlash(value string) string {
	if strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}
