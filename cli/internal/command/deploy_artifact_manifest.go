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

	domaincfg "github.com/poruru-code/esb/cli/internal/domain/config"
	"github.com/poruru-code/esb/cli/internal/meta"
	"github.com/poruru-code/esb/cli/internal/version"
	"github.com/poruru-code/esb/pkg/artifactcore"
)

const artifactManifestFileName = "artifact.yml"

func writeDeployArtifactManifest(
	inputs deployInputs,
	bundleEnabled bool,
	manifestOverride string,
	esbVersion string,
) (string, error) {
	manifestPath := resolveDeployArtifactManifestPath(
		inputs.ProjectDir,
		inputs.Project,
		inputs.Env,
		manifestOverride,
	)
	manifestDir := filepath.Dir(manifestPath)
	entries := make([]artifactcore.ArtifactEntry, 0, len(inputs.Templates))

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

		source := artifactcore.ArtifactSourceTemplate{
			Path:       normalizeSourceTemplatePath(inputs.ProjectDir, tpl.TemplatePath),
			SHA256:     templateSHA,
			Parameters: cloneStringValues(tpl.Parameters),
		}
		entry := artifactcore.ArtifactEntry{
			ArtifactRoot:     artifactRoot,
			RuntimeConfigDir: filepath.ToSlash("config"),
			BundleManifest:   bundleManifest,
			SourceTemplate:   source,
		}
		entries = append(entries, entry)
	}

	manifest := artifactcore.ArtifactManifest{
		SchemaVersion: artifactcore.ArtifactSchemaVersionV1,
		Project:       strings.TrimSpace(inputs.Project),
		Env:           strings.TrimSpace(inputs.Env),
		Mode:          strings.TrimSpace(inputs.Mode),
		RuntimeStack:  defaultRuntimeStack(inputs.Mode, esbVersion),
		Artifacts:     entries,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Generator: artifactcore.ArtifactGenerator{
			Name:    meta.AppName,
			Version: version.GetVersion(),
		},
	}
	if err := artifactcore.WriteArtifactManifest(manifestPath, manifest); err != nil {
		return "", err
	}
	return manifestPath, nil
}

func defaultRuntimeStack(mode, esbVersion string) artifactcore.RuntimeStackMeta {
	version := strings.TrimSpace(esbVersion)
	if version == "" {
		version = "latest"
	}
	return artifactcore.RuntimeStackMeta{
		APIVersion: artifactcore.RuntimeStackAPIVersion,
		Mode:       strings.TrimSpace(mode),
		ESBVersion: version,
	}
}

func resolveDeployArtifactManifestPath(projectDir, project, env string, overridePath ...string) string {
	if len(overridePath) > 0 {
		trimmed := strings.TrimSpace(overridePath[0])
		if trimmed != "" {
			candidate := filepath.Clean(trimmed)
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(projectDir, candidate)
			}
			return filepath.Clean(candidate)
		}
	}
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

func normalizeSourceTemplatePath(projectDir, templatePath string) string {
	trimmed := strings.TrimSpace(templatePath)
	if trimmed == "" {
		return ""
	}
	templateAbs := filepath.Clean(trimmed)
	if !filepath.IsAbs(templateAbs) {
		if resolved, err := filepath.Abs(templateAbs); err == nil {
			templateAbs = resolved
		}
	}

	root := strings.TrimSpace(projectDir)
	if root == "" {
		return templateAbs
	}
	rootAbs := filepath.Clean(root)
	if !filepath.IsAbs(rootAbs) {
		if resolved, err := filepath.Abs(rootAbs); err == nil {
			rootAbs = resolved
		}
	}
	rel, err := filepath.Rel(rootAbs, templateAbs)
	if err != nil {
		return templateAbs
	}
	if rel == "." || rel == "" {
		return "."
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return templateAbs
	}
	return filepath.ToSlash(rel)
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
