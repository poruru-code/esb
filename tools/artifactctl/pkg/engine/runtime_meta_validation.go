package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	supportedRuntimeHooksAPIVersion = "1.0"
	supportedTemplateRendererName   = "esb-cli-embedded-templates"
	supportedTemplateRendererAPI    = "1.0"
)

type runtimeAssetDigests struct {
	pythonSitecustomize string
	javaAgent           string
	javaWrapper         string
	templateRenderer    string
}

func validateRuntimeMetadata(manifest ArtifactManifest, manifestPath string, strict bool) ([]string, error) {
	warnings := make([]string, 0)

	needsDigestCheck := false
	for _, entry := range manifest.Artifacts {
		if hasRuntimeDigest(entry.RuntimeMeta) {
			needsDigestCheck = true
			break
		}
	}

	var digests runtimeAssetDigests
	if needsDigestCheck {
		repoRoot, ok := resolveRepositoryRoot(manifestPath)
		if !ok {
			message := "runtime digest verification requires repository root (runtime-hooks and cli/assets/runtime-templates)"
			if strict {
				return nil, errors.New(message)
			}
			warnings = append(warnings, message)
		} else {
			loaded, err := loadRuntimeAssetDigests(repoRoot)
			if err != nil {
				if strict {
					return nil, err
				}
				warnings = append(warnings, err.Error())
			} else {
				digests = loaded
			}
		}
	}

	for i, entry := range manifest.Artifacts {
		prefix := fmt.Sprintf("artifacts[%d].runtime_meta", i)

		if err := validateAPIVersion(
			prefix+".runtime_hooks.api_version",
			entry.RuntimeMeta.Hooks.APIVersion,
			supportedRuntimeHooksAPIVersion,
			strict,
			&warnings,
		); err != nil {
			return nil, err
		}
		if err := validateAPIVersion(
			prefix+".template_renderer.api_version",
			entry.RuntimeMeta.Renderer.APIVersion,
			supportedTemplateRendererAPI,
			strict,
			&warnings,
		); err != nil {
			return nil, err
		}
		if name := strings.TrimSpace(entry.RuntimeMeta.Renderer.Name); name != "" && name != supportedTemplateRendererName {
			warnings = append(
				warnings,
				fmt.Sprintf("%s.name is %q (expected %q)", prefix+".template_renderer", name, supportedTemplateRendererName),
			)
		}

		if err := validateDigest(
			prefix+".runtime_hooks.python_sitecustomize_digest",
			entry.RuntimeMeta.Hooks.PythonSitecustomizeDigest,
			digests.pythonSitecustomize,
			strict,
			&warnings,
		); err != nil {
			return nil, err
		}
		if err := validateDigest(
			prefix+".runtime_hooks.java_agent_digest",
			entry.RuntimeMeta.Hooks.JavaAgentDigest,
			digests.javaAgent,
			strict,
			&warnings,
		); err != nil {
			return nil, err
		}
		if err := validateDigest(
			prefix+".runtime_hooks.java_wrapper_digest",
			entry.RuntimeMeta.Hooks.JavaWrapperDigest,
			digests.javaWrapper,
			strict,
			&warnings,
		); err != nil {
			return nil, err
		}
		if err := validateDigest(
			prefix+".template_renderer.template_digest",
			entry.RuntimeMeta.Renderer.TemplateDigest,
			digests.templateRenderer,
			strict,
			&warnings,
		); err != nil {
			return nil, err
		}
	}

	return warnings, nil
}

func hasRuntimeDigest(meta ArtifactRuntimeMeta) bool {
	return strings.TrimSpace(meta.Hooks.PythonSitecustomizeDigest) != "" ||
		strings.TrimSpace(meta.Hooks.JavaAgentDigest) != "" ||
		strings.TrimSpace(meta.Hooks.JavaWrapperDigest) != "" ||
		strings.TrimSpace(meta.Renderer.TemplateDigest) != ""
}

func validateAPIVersion(field, actual, expected string, strict bool, warnings *[]string) error {
	got := strings.TrimSpace(actual)
	if got == "" {
		return nil
	}
	want := strings.TrimSpace(expected)
	if want == "" {
		return fmt.Errorf("%s expected version is not configured", field)
	}

	gotMajor, gotMinor, err := parseAPIVersion(got)
	if err != nil {
		if strict {
			return fmt.Errorf("%s is invalid (%q): %w", field, got, err)
		}
		*warnings = append(*warnings, fmt.Sprintf("%s is invalid (%q): %v", field, got, err))
		return nil
	}
	wantMajor, wantMinor, err := parseAPIVersion(want)
	if err != nil {
		return fmt.Errorf("supported api_version for %s is invalid (%q): %w", field, want, err)
	}

	if gotMajor != wantMajor {
		return fmt.Errorf("%s major mismatch: got %q, supported %q", field, got, want)
	}
	if gotMinor != wantMinor {
		message := fmt.Sprintf("%s minor mismatch: got %q, supported %q", field, got, want)
		if strict {
			return errors.New(message)
		}
		*warnings = append(*warnings, message)
	}
	return nil
}

func parseAPIVersion(value string) (int, int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, 0, fmt.Errorf("empty version")
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) < 1 || len(parts) > 2 {
		return 0, 0, fmt.Errorf("expected major.minor format")
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return 0, 0, fmt.Errorf("invalid major")
	}
	minor := 0
	if len(parts) == 2 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil || minor < 0 {
			return 0, 0, fmt.Errorf("invalid minor")
		}
	}
	return major, minor, nil
}

func validateDigest(field, actual, expected string, strict bool, warnings *[]string) error {
	got := strings.TrimSpace(actual)
	if got == "" {
		return nil
	}
	want := strings.TrimSpace(expected)
	if want == "" {
		message := fmt.Sprintf("%s cannot be verified in current environment", field)
		if strict {
			return errors.New(message)
		}
		*warnings = append(*warnings, message)
		return nil
	}
	if !strings.EqualFold(got, want) {
		message := fmt.Sprintf("%s mismatch: got %q, expected %q", field, got, want)
		if strict {
			return errors.New(message)
		}
		*warnings = append(*warnings, message)
	}
	return nil
}

func resolveRepositoryRoot(manifestPath string) (string, bool) {
	candidates := make([]string, 0, 2)
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if manifestPath != "" {
		if abs, err := filepath.Abs(manifestPath); err == nil {
			candidates = append(candidates, filepath.Dir(abs))
		}
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, start := range candidates {
		clean := filepath.Clean(start)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		if root, ok := findRepositoryRootFrom(clean); ok {
			return root, true
		}
	}
	return "", false
}

func findRepositoryRootFrom(start string) (string, bool) {
	dir := filepath.Clean(start)
	for {
		if isRepositoryRoot(dir) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func isRepositoryRoot(dir string) bool {
	return pathExists(filepath.Join(dir, "runtime-hooks")) &&
		pathExists(filepath.Join(dir, "cli", "assets", "runtime-templates"))
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func loadRuntimeAssetDigests(repoRoot string) (runtimeAssetDigests, error) {
	pythonSitecustomize, err := fileSHA256(filepath.Join(repoRoot, "runtime-hooks", "python", "sitecustomize", "site-packages", "sitecustomize.py"))
	if err != nil {
		return runtimeAssetDigests{}, fmt.Errorf("calculate runtime-hooks python digest: %w", err)
	}
	javaAgent, err := fileSHA256(filepath.Join(repoRoot, "runtime-hooks", "java", "agent", "lambda-java-agent.jar"))
	if err != nil {
		return runtimeAssetDigests{}, fmt.Errorf("calculate runtime-hooks java agent digest: %w", err)
	}
	javaWrapper, err := fileSHA256(filepath.Join(repoRoot, "runtime-hooks", "java", "wrapper", "lambda-java-wrapper.jar"))
	if err != nil {
		return runtimeAssetDigests{}, fmt.Errorf("calculate runtime-hooks java wrapper digest: %w", err)
	}
	templateRenderer, err := directoryDigest(filepath.Join(repoRoot, "cli", "assets", "runtime-templates"))
	if err != nil {
		return runtimeAssetDigests{}, fmt.Errorf("calculate runtime templates digest: %w", err)
	}

	return runtimeAssetDigests{
		pythonSitecustomize: pythonSitecustomize,
		javaAgent:           javaAgent,
		javaWrapper:         javaWrapper,
		templateRenderer:    templateRenderer,
	}, nil
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
