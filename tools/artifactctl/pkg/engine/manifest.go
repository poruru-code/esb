package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const ArtifactSchemaVersionV1 = "1"

var artifactIDPattern = regexp.MustCompile(`^[a-z0-9-]+-[0-9a-f]{8}$`)

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
	ID                string                 `yaml:"id"`
	ArtifactRoot      string                 `yaml:"artifact_root"`
	RuntimeConfigDir  string                 `yaml:"runtime_config_dir"`
	BundleManifest    string                 `yaml:"bundle_manifest,omitempty"`
	ImagePrewarm      string                 `yaml:"image_prewarm,omitempty"`
	RequiredSecretEnv []string               `yaml:"required_secret_env,omitempty"`
	SourceTemplate    ArtifactSourceTemplate `yaml:"source_template"`
	RuntimeMeta       ArtifactRuntimeMeta    `yaml:"runtime_meta,omitempty"`
}

type ArtifactSourceTemplate struct {
	Path       string            `yaml:"path"`
	SHA256     string            `yaml:"sha256,omitempty"`
	Parameters map[string]string `yaml:"parameters,omitempty"`
}

type ArtifactGenerator struct {
	Name    string `yaml:"name,omitempty"`
	Version string `yaml:"version,omitempty"`
}

type ArtifactRuntimeMeta struct {
	Hooks    RuntimeHooksMeta `yaml:"runtime_hooks,omitempty"`
	Renderer RendererMeta     `yaml:"template_renderer,omitempty"`
}

type RuntimeHooksMeta struct {
	APIVersion                string `yaml:"api_version,omitempty"`
	PythonSitecustomizeDigest string `yaml:"python_sitecustomize_digest,omitempty"`
	JavaAgentDigest           string `yaml:"java_agent_digest,omitempty"`
	JavaWrapperDigest         string `yaml:"java_wrapper_digest,omitempty"`
}

type RendererMeta struct {
	Name           string `yaml:"name,omitempty"`
	APIVersion     string `yaml:"api_version,omitempty"`
	TemplateDigest string `yaml:"template_digest,omitempty"`
}

func ValidateIDs(path string) error {
	_, err := ReadArtifactManifest(path)
	return err
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
	seen := make(map[string]struct{}, len(d.Artifacts))
	for i := range d.Artifacts {
		entry := d.Artifacts[i]
		if err := entry.Validate(i); err != nil {
			return err
		}
		if _, ok := seen[entry.ID]; ok {
			return fmt.Errorf("artifacts[%d].id must be unique: %s", i, entry.ID)
		}
		seen[entry.ID] = struct{}{}
		wantID := ComputeArtifactID(entry.SourceTemplate.Path, entry.SourceTemplate.Parameters, entry.SourceTemplate.SHA256)
		if entry.ID != wantID {
			return fmt.Errorf("artifacts[%d].id mismatch: got %q want %q", i, entry.ID, wantID)
		}
	}
	return nil
}

func (e ArtifactEntry) Validate(index int) error {
	prefix := fmt.Sprintf("artifacts[%d]", index)
	if strings.TrimSpace(e.ID) == "" {
		return fmt.Errorf("%s.id is required", prefix)
	}
	if !artifactIDPattern.MatchString(e.ID) {
		return fmt.Errorf("%s.id must match %s", prefix, artifactIDPattern.String())
	}
	if err := validateArtifactRoot(fmt.Sprintf("%s.artifact_root", prefix), e.ArtifactRoot); err != nil {
		return err
	}
	if err := validateRelativePath(fmt.Sprintf("%s.runtime_config_dir", prefix), e.RuntimeConfigDir); err != nil {
		return err
	}
	if strings.TrimSpace(e.BundleManifest) != "" {
		if err := validateRelativePath(fmt.Sprintf("%s.bundle_manifest", prefix), e.BundleManifest); err != nil {
			return err
		}
	}
	for _, key := range e.RequiredSecretEnv {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("%s.required_secret_env contains empty key", prefix)
		}
	}
	if strings.TrimSpace(e.SourceTemplate.Path) == "" {
		return fmt.Errorf("%s.source_template.path is required", prefix)
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

func (d ArtifactManifest) ResolveBundleManifest(manifestPath string, index int) (string, error) {
	if index < 0 || index >= len(d.Artifacts) {
		return "", fmt.Errorf("artifact index out of range: %d", index)
	}
	entry := d.Artifacts[index]
	if strings.TrimSpace(entry.BundleManifest) == "" {
		return "", nil
	}
	artifactRoot, err := resolveArtifactRootPath(manifestPath, entry.ArtifactRoot)
	if err != nil {
		return "", err
	}
	return resolveEntryRelativePath(artifactRoot, entry.BundleManifest, "bundle_manifest")
}

func ReadArtifactManifest(path string) (ArtifactManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ArtifactManifest{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	var manifest ArtifactManifest
	if err := decoder.Decode(&manifest); err != nil {
		return ArtifactManifest{}, fmt.Errorf("decode artifact manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return ArtifactManifest{}, err
	}
	return manifest, nil
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
		entry.RequiredSecretEnv = sortedUniqueNonEmpty(entry.RequiredSecretEnv)
		if entry.SourceTemplate.Parameters != nil {
			cloned := make(map[string]string, len(entry.SourceTemplate.Parameters))
			for k, v := range entry.SourceTemplate.Parameters {
				cloned[k] = v
			}
			entry.SourceTemplate.Parameters = cloned
		}
		normalized.Artifacts[i] = entry
	}
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

func ComputeArtifactID(templatePath string, parameters map[string]string, sourceSHA256 string) string {
	templateSlug := templateSlug(templatePath)
	canonicalTemplate := canonicalTemplateRef(templatePath)
	canonicalParams := canonicalParameters(parameters)
	canonicalSHA := strings.TrimSpace(sourceSHA256)
	seed := canonicalTemplate + "\n" + canonicalParams + "\n" + canonicalSHA
	sum := sha256.Sum256([]byte(seed))
	return fmt.Sprintf("%s-%s", templateSlug, hex.EncodeToString(sum[:4]))
}

func canonicalTemplateRef(value string) string {
	trimmed := strings.TrimSpace(value)
	replaced := strings.ReplaceAll(trimmed, "\\", "/")
	cleaned := path.Clean(replaced)
	if cleaned == "" {
		return "."
	}
	return cleaned
}

func canonicalParameters(parameters map[string]string) string {
	if len(parameters) == 0 {
		return ""
	}
	keys := make([]string, 0, len(parameters))
	for key := range parameters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+parameters[key])
	}
	return strings.Join(lines, "\n")
}

func templateSlug(templatePath string) string {
	trimmed := strings.TrimSpace(templatePath)
	replaced := strings.ReplaceAll(trimmed, "\\", "/")
	base := path.Base(replaced)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	stem = strings.ToLower(stem)
	var out strings.Builder
	lastDash := false
	for _, r := range stem {
		isAlpha := r >= 'a' && r <= 'z'
		isNum := r >= '0' && r <= '9'
		if isAlpha || isNum {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			out.WriteRune('-')
			lastDash = true
		}
	}
	slug := strings.Trim(out.String(), "-")
	if slug == "" {
		return "template"
	}
	return slug
}
