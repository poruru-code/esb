package artifactcore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const ArtifactSchemaVersionV1 = "1"

var sourceTemplateSHA256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type ArtifactManifest struct {
	SchemaVersion string            `yaml:"schema_version"`
	Project       string            `yaml:"project"`
	Env           string            `yaml:"env"`
	Mode          string            `yaml:"mode"`
	Artifacts     []ArtifactEntry   `yaml:"artifacts"`
	GeneratedAt   string            `yaml:"generated_at,omitempty"`
	Generator     ArtifactGenerator `yaml:"generator,omitempty"`
}

type ArtifactEntry struct {
	ArtifactRoot     string                  `yaml:"artifact_root"`
	RuntimeConfigDir string                  `yaml:"runtime_config_dir"`
	SourceTemplate   *ArtifactSourceTemplate `yaml:"source_template,omitempty"`
}

type ArtifactSourceTemplate struct {
	Path    string `yaml:"path,omitempty"`
	SHA256  string `yaml:"sha256,omitempty"`
	pathSet bool
	shaSet  bool
}

type ArtifactGenerator struct {
	Name    string `yaml:"name,omitempty"`
	Version string `yaml:"version,omitempty"`
}

func (d ArtifactManifest) Validate() error {
	schemaVersion := strings.TrimSpace(d.SchemaVersion)
	if schemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if schemaVersion != ArtifactSchemaVersionV1 {
		return fmt.Errorf("unsupported schema_version: %q (supported: %q)", schemaVersion, ArtifactSchemaVersionV1)
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
	if len(d.Artifacts) == 0 {
		return fmt.Errorf("artifacts must contain at least one entry")
	}
	for i := range d.Artifacts {
		if err := d.Artifacts[i].Validate(i); err != nil {
			return err
		}
	}
	return nil
}

func (t *ArtifactSourceTemplate) UnmarshalYAML(value *yaml.Node) error {
	type rawSourceTemplate struct {
		Path   *string `yaml:"path"`
		SHA256 *string `yaml:"sha256"`
	}

	var raw rawSourceTemplate
	if err := value.Decode(&raw); err != nil {
		return err
	}

	t.pathSet = raw.Path != nil
	t.shaSet = raw.SHA256 != nil
	if raw.Path != nil {
		t.Path = *raw.Path
	} else {
		t.Path = ""
	}
	if raw.SHA256 != nil {
		t.SHA256 = *raw.SHA256
	} else {
		t.SHA256 = ""
	}
	return nil
}

func (t ArtifactSourceTemplate) Validate(prefix string) error {
	path := strings.TrimSpace(t.Path)
	if (t.pathSet || t.Path != "") && path == "" {
		return fmt.Errorf("%s.path must not be blank", prefix)
	}

	sha := strings.TrimSpace(t.SHA256)
	if (t.shaSet || t.SHA256 != "") && sha == "" {
		return fmt.Errorf("%s.sha256 must not be blank", prefix)
	}
	if sha != "" && !sourceTemplateSHA256Pattern.MatchString(sha) {
		return fmt.Errorf("%s.sha256 must be 64 lowercase hex characters", prefix)
	}
	return nil
}

func (e ArtifactEntry) Validate(index int) error {
	prefix := fmt.Sprintf("artifacts[%d]", index)
	if err := validateArtifactRoot(fmt.Sprintf("%s.artifact_root", prefix), e.ArtifactRoot); err != nil {
		return err
	}
	if err := validateRelativePath(fmt.Sprintf("%s.runtime_config_dir", prefix), e.RuntimeConfigDir); err != nil {
		return err
	}
	if e.SourceTemplate != nil {
		if err := e.SourceTemplate.Validate(fmt.Sprintf("%s.source_template", prefix)); err != nil {
			return err
		}
	}
	return nil
}

func (d ArtifactManifest) ResolveArtifactRoot(manifestPath string, index int) (string, error) {
	if index < 0 || index >= len(d.Artifacts) {
		return "", fmt.Errorf("artifact index out of range: %d", index)
	}
	return resolveArtifactRootPath(manifestPath, d.Artifacts[index].ArtifactRoot)
}

func (d ArtifactManifest) ResolveRuntimeConfigDir(manifestPath string, index int) (string, error) {
	if index < 0 || index >= len(d.Artifacts) {
		return "", fmt.Errorf("artifact index out of range: %d", index)
	}
	artifactRoot, err := resolveArtifactRootPath(manifestPath, d.Artifacts[index].ArtifactRoot)
	if err != nil {
		return "", err
	}
	return resolveEntryRelativePath(artifactRoot, d.Artifacts[index].RuntimeConfigDir, "runtime_config_dir")
}

func ReadArtifactManifest(path string) (ArtifactManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ArtifactManifest{}, fmt.Errorf("read artifact manifest: %w", MissingReferencedPathError{Path: path})
		}
		return ArtifactManifest{}, err
	}
	return decodeArtifactManifest(data, true)
}

func ReadArtifactManifestUnchecked(path string) (ArtifactManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ArtifactManifest{}, fmt.Errorf("read artifact manifest: %w", MissingReferencedPathError{Path: path})
		}
		return ArtifactManifest{}, err
	}
	return decodeArtifactManifest(data, false)
}

func WriteArtifactManifest(path string, manifest ArtifactManifest) error {
	normalized := normalizeArtifactManifest(manifest)
	if err := normalized.Validate(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create artifact manifest directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".artifact-manifest-*.yml")
	if err != nil {
		return fmt.Errorf("create temp artifact manifest file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}

	encoder := yaml.NewEncoder(tmp)
	encoder.SetIndent(2)
	if err := encoder.Encode(normalized); err != nil {
		_ = encoder.Close()
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("encode artifact manifest: %w", err)
	}
	if err := encoder.Close(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("close artifact manifest encoder: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close artifact manifest temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		cleanup()
		return fmt.Errorf("chmod artifact manifest temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("commit artifact manifest file: %w", err)
	}
	return nil
}

func normalizeArtifactManifest(d ArtifactManifest) ArtifactManifest {
	normalized := d
	normalized.Artifacts = append([]ArtifactEntry(nil), normalized.Artifacts...)
	for i := range normalized.Artifacts {
		entry := normalized.Artifacts[i]
		if entry.SourceTemplate != nil {
			copied := *entry.SourceTemplate
			entry.SourceTemplate = &copied
		}
		normalized.Artifacts[i] = entry
	}
	return normalized
}

func resolveArtifactRootPath(manifestPath, artifactRoot string) (string, error) {
	if err := validateArtifactRoot("artifact_root", artifactRoot); err != nil {
		return "", err
	}
	value := filepath.Clean(filepath.FromSlash(strings.TrimSpace(artifactRoot)))
	if filepath.IsAbs(value) {
		return value, nil
	}
	baseDir := filepath.Dir(filepath.Clean(manifestPath))
	return filepath.Clean(filepath.Join(baseDir, value)), nil
}

func resolveEntryRelativePath(artifactRoot, relPath, field string) (string, error) {
	if err := validateRelativePath(field, relPath); err != nil {
		return "", err
	}
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relPath)))
	return filepath.Join(artifactRoot, clean), nil
}

func validateArtifactRoot(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

func validateRelativePath(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s is required", field)
	}
	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	if filepath.IsAbs(cleaned) {
		return fmt.Errorf("%s must be a relative path", field)
	}
	if cleaned == "." {
		return fmt.Errorf("%s must not be '.'", field)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("%s must not escape artifact root", field)
	}
	return nil
}

func decodeArtifactManifest(data []byte, validate bool) (ArtifactManifest, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	var manifest ArtifactManifest
	if err := decoder.Decode(&manifest); err != nil {
		return ArtifactManifest{}, fmt.Errorf("decode artifact manifest: %w", err)
	}
	if !validate {
		return manifest, nil
	}
	if err := manifest.Validate(); err != nil {
		return ArtifactManifest{}, err
	}
	return manifest, nil
}
