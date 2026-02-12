// Where: cli/internal/infra/templategen/stage_layers.go
// What: Layer staging utilities for generator output.
// Why: Keep layer-specific copy/naming rules isolated from stage entry flow.
package templategen

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/runtime"
)

// stageLayers stages each referenced layer inside the function directory,
// applying smart nesting for Python runtimes and sanitizing names.
func stageLayers(
	layers []manifest.LayerSpec,
	ctx stageContext,
	functionName,
	functionDir string,
	profile runtime.Profile,
) ([]manifest.LayerSpec, error) {
	if len(layers) == 0 {
		return nil, nil
	}

	staged := make([]manifest.LayerSpec, 0, len(layers))
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

		ctx.verbosef("  Staging layer: %s -> %s\n", layer.Name, targetName)

		layerRef := filepath.ToSlash(filepath.Join("functions", functionName, "layers", targetName))
		if !ctx.DryRun {
			targetDir := filepath.Join(layersDir, targetName)
			if err := removeDir(targetDir); err != nil {
				return nil, err
			}

			var finalSrc string
			switch {
			case fileExists(source) && strings.HasSuffix(strings.ToLower(source), ".zip"):
				extracted, err := extractZipLayer(source, ctx.LayerCacheDir)
				if err != nil {
					return nil, err
				}
				finalSrc = extracted
			case dirExists(source):
				finalSrc = source
			default:
				continue
			}

			finalDest := targetDir
			if shouldNestPython(profile.NestPythonLayers, finalSrc) {
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

// layerTargetName derives a filesystem-safe directory name for a layer.
func layerTargetName(layer manifest.LayerSpec, source string) string {
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
