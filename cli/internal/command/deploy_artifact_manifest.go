// Where: cli/internal/command/deploy_artifact_manifest.go
// What: Artifact manifest generation for deploy command.
// Why: Emit artifact.yml as the canonical output for apply/non-CLI flows.
package command

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
	"github.com/poruru/edge-serverless-box/cli/internal/meta"
	"github.com/poruru/edge-serverless-box/cli/internal/usecase/deploy"
	"github.com/poruru/edge-serverless-box/cli/internal/version"
)

const artifactManifestFileName = "artifact.yml"

const (
	runtimeHooksAPIVersion     = "1.0"
	templateRendererName       = "esb-cli-embedded-templates"
	templateRendererAPIVersion = "1.0"
)

func writeDeployArtifactManifest(inputs deployInputs, imagePrewarm string, bundleEnabled bool) (string, error) {
	manifestPath := resolveDeployArtifactManifestPath(inputs.ProjectDir, inputs.Project, inputs.Env)
	manifestDir := filepath.Dir(manifestPath)
	entries := make([]deploy.ArtifactEntry, 0, len(inputs.Templates))
	runtimeMeta, err := resolveRuntimeMeta(inputs.ProjectDir)
	if err != nil {
		return "", err
	}

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
			RuntimeMeta:      runtimeMeta,
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

func resolveRuntimeMeta(projectDir string) (deploy.ArtifactRuntimeMeta, error) {
	pythonSitecustomizeDigest, err := fileSHA256(filepath.Join(projectDir, "runtime-hooks", "python", "sitecustomize", "site-packages", "sitecustomize.py"))
	if err != nil {
		return deploy.ArtifactRuntimeMeta{}, fmt.Errorf("hash runtime hook python sitecustomize: %w", err)
	}
	javaAgentDigest, err := fileSHA256(filepath.Join(projectDir, "runtime-hooks", "java", "agent", "lambda-java-agent.jar"))
	if err != nil {
		return deploy.ArtifactRuntimeMeta{}, fmt.Errorf("hash runtime hook java agent: %w", err)
	}
	javaWrapperDigest, err := fileSHA256(filepath.Join(projectDir, "runtime-hooks", "java", "wrapper", "lambda-java-wrapper.jar"))
	if err != nil {
		return deploy.ArtifactRuntimeMeta{}, fmt.Errorf("hash runtime hook java wrapper: %w", err)
	}
	templateDigest, err := directoryDigest(filepath.Join(projectDir, "cli", "assets", "runtime-templates"))
	if err != nil {
		return deploy.ArtifactRuntimeMeta{}, fmt.Errorf("hash runtime templates: %w", err)
	}
	return deploy.ArtifactRuntimeMeta{
		Hooks: deploy.RuntimeHooksMeta{
			APIVersion:                runtimeHooksAPIVersion,
			PythonSitecustomizeDigest: pythonSitecustomizeDigest,
			JavaAgentDigest:           javaAgentDigest,
			JavaWrapperDigest:         javaWrapperDigest,
		},
		Renderer: deploy.RendererMeta{
			Name:           templateRendererName,
			APIVersion:     templateRendererAPIVersion,
			TemplateDigest: templateDigest,
		},
	}, nil
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

func directoryDigest(root string) (string, error) {
	entries := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		digest, err := fileSHA256(path)
		if err != nil {
			return err
		}
		entries = append(entries, filepath.ToSlash(rel)+":"+digest)
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("no files found under %s", root)
	}
	sort.Strings(entries)

	h := sha256.New()
	for _, entry := range entries {
		_, _ = h.Write([]byte(entry))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
