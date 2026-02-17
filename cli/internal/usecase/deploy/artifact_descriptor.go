// Where: cli/internal/usecase/deploy/artifact_descriptor.go
// What: Descriptor contract for artifact-first deploy inputs.
// Why: Define a stable artifact boundary before wiring generate/apply commands.
package deploy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const ArtifactSchemaVersionV1 = "1"

type ArtifactDescriptor struct {
	SchemaVersion     string              `json:"schema_version"`
	Project           string              `json:"project"`
	Env               string              `json:"env"`
	Mode              string              `json:"mode"`
	RuntimeConfigDir  string              `json:"runtime_config_dir"`
	BundleManifest    string              `json:"bundle_manifest,omitempty"`
	ImagePrewarm      string              `json:"image_prewarm,omitempty"`
	RequiredSecretEnv []string            `json:"required_secret_env,omitempty"`
	Templates         []ArtifactTemplate  `json:"templates,omitempty"`
	RuntimeMeta       ArtifactRuntimeMeta `json:"runtime_meta,omitempty"`
}

type ArtifactTemplate struct {
	Path       string            `json:"path"`
	SHA256     string            `json:"sha256,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

type ArtifactRuntimeMeta struct {
	Hooks    RuntimeHooksMeta `json:"runtime_hooks,omitempty"`
	Renderer RendererMeta     `json:"template_renderer,omitempty"`
}

type RuntimeHooksMeta struct {
	APIVersion                string `json:"api_version,omitempty"`
	PythonSitecustomizeDigest string `json:"python_sitecustomize_digest,omitempty"`
	JavaAgentDigest           string `json:"java_agent_digest,omitempty"`
	JavaWrapperDigest         string `json:"java_wrapper_digest,omitempty"`
}

type RendererMeta struct {
	Name           string `json:"name,omitempty"`
	APIVersion     string `json:"api_version,omitempty"`
	TemplateDigest string `json:"template_digest,omitempty"`
}

func (d ArtifactDescriptor) Validate() error {
	if strings.TrimSpace(d.SchemaVersion) == "" {
		return fmt.Errorf("schema_version is required")
	}
	if strings.TrimSpace(d.Project) == "" {
		return fmt.Errorf("project is required")
	}
	if strings.TrimSpace(d.Env) == "" {
		return fmt.Errorf("env is required")
	}
	if strings.TrimSpace(d.Mode) == "" {
		return fmt.Errorf("mode is required")
	}
	if err := validateRelativePath("runtime_config_dir", d.RuntimeConfigDir); err != nil {
		return err
	}
	if strings.TrimSpace(d.BundleManifest) != "" {
		if err := validateRelativePath("bundle_manifest", d.BundleManifest); err != nil {
			return err
		}
	}
	for i, tmpl := range d.Templates {
		name := fmt.Sprintf("templates[%d].path", i)
		if err := validateRelativePath(name, tmpl.Path); err != nil {
			return err
		}
	}
	for _, key := range d.RequiredSecretEnv {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("required_secret_env contains empty key")
		}
	}
	return nil
}

func (d ArtifactDescriptor) ResolveRuntimeConfigDir(descriptorPath string) (string, error) {
	return resolveDescriptorRelativePath(descriptorPath, d.RuntimeConfigDir, "runtime_config_dir")
}

func (d ArtifactDescriptor) ResolveBundleManifest(descriptorPath string) (string, error) {
	if strings.TrimSpace(d.BundleManifest) == "" {
		return "", nil
	}
	return resolveDescriptorRelativePath(descriptorPath, d.BundleManifest, "bundle_manifest")
}

func ReadArtifactDescriptor(path string) (ArtifactDescriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ArtifactDescriptor{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var descriptor ArtifactDescriptor
	if err := decoder.Decode(&descriptor); err != nil {
		return ArtifactDescriptor{}, fmt.Errorf("decode descriptor: %w", err)
	}
	if err := descriptor.Validate(); err != nil {
		return ArtifactDescriptor{}, err
	}
	return descriptor, nil
}

func WriteArtifactDescriptor(path string, descriptor ArtifactDescriptor) error {
	if err := descriptor.Validate(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create descriptor directory: %w", err)
	}

	normalized := normalizeArtifactDescriptor(descriptor)

	tmp, err := os.CreateTemp(dir, ".artifact-descriptor-*.json")
	if err != nil {
		return fmt.Errorf("create temp descriptor file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}

	encoder := json.NewEncoder(tmp)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(normalized); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("encode descriptor: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close descriptor temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		cleanup()
		return fmt.Errorf("chmod descriptor temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("commit descriptor file: %w", err)
	}
	return nil
}

func normalizeArtifactDescriptor(d ArtifactDescriptor) ArtifactDescriptor {
	normalized := d
	normalized.RequiredSecretEnv = sortedUniqueNonEmpty(normalized.RequiredSecretEnv)
	sort.SliceStable(normalized.Templates, func(i, j int) bool {
		return normalized.Templates[i].Path < normalized.Templates[j].Path
	})
	return normalized
}

func sortedUniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func resolveDescriptorRelativePath(descriptorPath, relPath, field string) (string, error) {
	if err := validateRelativePath(field, relPath); err != nil {
		return "", err
	}
	baseDir := filepath.Dir(filepath.Clean(descriptorPath))
	return filepath.Join(baseDir, filepath.Clean(relPath)), nil
}

func validateRelativePath(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s is required", field)
	}
	if filepath.IsAbs(trimmed) {
		return fmt.Errorf("%s must be a relative path", field)
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == "." {
		return fmt.Errorf("%s must not be '.'", field)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("%s must not escape artifact root", field)
	}
	return nil
}
