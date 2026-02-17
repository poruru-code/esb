// Where: cli/internal/command/deploy_artifact_manifest.go
// What: Artifact manifest generation for deploy command.
// Why: Emit artifact.yml as the canonical output for apply/non-CLI flows.
package command

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
	"github.com/poruru/edge-serverless-box/cli/internal/meta"
	"github.com/poruru/edge-serverless-box/cli/internal/usecase/deploy"
	"github.com/poruru/edge-serverless-box/cli/internal/version"
)

const artifactManifestFileName = "artifact.yml"

func writeDeployArtifactManifest(inputs deployInputs, imagePrewarm string, bundleEnabled bool) (string, error) {
	manifestPath := resolveDeployArtifactManifestPath(inputs.ProjectDir, inputs.Project, inputs.Env)
	manifestDir := filepath.Dir(manifestPath)
	entries := make([]deploy.ArtifactEntry, 0, len(inputs.Templates))

	for _, tpl := range inputs.Templates {
		artifactRootAbs, err := resolveTemplateArtifactRoot(tpl.TemplatePath, tpl.OutputDir, inputs.Env)
		if err != nil {
			return "", err
		}
		artifactRoot := toManifestPath(manifestDir, artifactRootAbs)

		templateSHA, err := fileSHA256(tpl.TemplatePath)
		if err != nil {
			return "", fmt.Errorf("hash template %s: %w", tpl.TemplatePath, err)
		}

		bundleManifest := ""
		bundlePath := filepath.Join(artifactRootAbs, "bundle", "manifest.json")
		if pathExists(bundlePath) {
			bundleManifest = filepath.ToSlash(filepath.Join("bundle", "manifest.json"))
		} else if bundleEnabled {
			return "", fmt.Errorf("bundle manifest not found: %s", bundlePath)
		}

		source := deploy.ArtifactSourceTemplate{
			Path:       filepath.Clean(strings.TrimSpace(tpl.TemplatePath)),
			SHA256:     templateSHA,
			Parameters: cloneStringValues(tpl.Parameters),
		}
		entry := deploy.ArtifactEntry{
			ID:               deploy.ComputeArtifactID(source.Path, source.Parameters, source.SHA256),
			ArtifactRoot:     artifactRoot,
			RuntimeConfigDir: filepath.ToSlash("config"),
			BundleManifest:   bundleManifest,
			ImagePrewarm:     imagePrewarm,
			SourceTemplate:   source,
			RuntimeMeta: deploy.ArtifactRuntimeMeta{
				Renderer: deploy.RendererMeta{
					Name:       "esb-cli-embedded-templates",
					APIVersion: "1.0",
				},
			},
		}
		entries = append(entries, entry)
	}

	manifest := deploy.ArtifactManifest{
		SchemaVersion: deploy.ArtifactSchemaVersionV1,
		Project:       strings.TrimSpace(inputs.Project),
		Env:           strings.TrimSpace(inputs.Env),
		Mode:          strings.TrimSpace(inputs.Mode),
		Artifacts:     entries,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Generator: deploy.ArtifactGenerator{
			Name:    meta.AppName,
			Version: version.GetVersion(),
		},
	}
	if err := deploy.WriteArtifactManifest(manifestPath, manifest); err != nil {
		return "", err
	}
	return manifestPath, nil
}

func resolveDeployArtifactManifestPath(projectDir, project, env string) string {
	return filepath.Join(
		projectDir,
		meta.HomeDir,
		"artifacts",
		sanitizePathSegment(project),
		sanitizePathSegment(env),
		artifactManifestFileName,
	)
}

func sanitizePathSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "default"
	}
	trimmed = strings.ReplaceAll(trimmed, "/", "-")
	trimmed = strings.ReplaceAll(trimmed, "\\", "-")
	trimmed = strings.Trim(trimmed, " ")
	if trimmed == "" || trimmed == "." || trimmed == ".." {
		return "default"
	}
	return trimmed
}

func resolveTemplateArtifactRoot(templatePath, outputDir, env string) (string, error) {
	resolved := domaincfg.ResolveOutputSummary(templatePath, outputDir, env)
	if filepath.IsAbs(resolved) {
		return filepath.Clean(resolved), nil
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve artifact root %s: %w", resolved, err)
	}
	return filepath.Clean(abs), nil
}

func toManifestPath(manifestDir, targetPath string) string {
	cleanTarget := filepath.Clean(targetPath)
	rel, err := filepath.Rel(manifestDir, cleanTarget)
	if err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(cleanTarget)
}

func cloneStringValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
