package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	supportedRuntimeHooksAPIVersion = "1.0"
	supportedTemplateRendererName   = "esb-cli-embedded-templates"
	supportedTemplateRendererAPI    = "1.0"
	artifactPythonSitecustomizeRel  = "runtime-base/runtime-hooks/python/sitecustomize/site-packages/sitecustomize.py"
)

type runtimeAssetDigests struct {
	pythonSitecustomize string
}

func validateRuntimeMetadata(manifest ArtifactManifest, manifestPath string, strict bool) ([]string, error) {
	warnings := make([]string, 0)

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

		if !hasRuntimeDigest(entry.RuntimeMeta) {
			continue
		}

		artifactRoot, err := manifest.ResolveArtifactRoot(manifestPath, i)
		if err != nil {
			message := fmt.Sprintf("%s resolve artifact_root failed: %v", prefix, err)
			if strict {
				return nil, errors.New(message)
			}
			warnings = append(warnings, message)
			continue
		}

		digests := runtimeAssetDigests{}
		verifyPython := false
		digests.pythonSitecustomize, verifyPython, err = resolveArtifactFileDigest(
			prefix+".runtime_hooks.python_sitecustomize_digest",
			entry.RuntimeMeta.Hooks.PythonSitecustomizeDigest,
			artifactRoot,
			artifactPythonSitecustomizeRel,
			strict,
			&warnings,
		)
		if err != nil {
			return nil, err
		}

		if verifyPython {
			if err := validateDigest(
				prefix+".runtime_hooks.python_sitecustomize_digest",
				entry.RuntimeMeta.Hooks.PythonSitecustomizeDigest,
				digests.pythonSitecustomize,
				strict,
				&warnings,
			); err != nil {
				return nil, err
			}
		}
	}

	return warnings, nil
}

func hasRuntimeDigest(meta ArtifactRuntimeMeta) bool {
	return strings.TrimSpace(meta.Hooks.PythonSitecustomizeDigest) != ""
}

func resolveArtifactFileDigest(
	field string,
	actual string,
	artifactRoot string,
	relPath string,
	strict bool,
	warnings *[]string,
) (string, bool, error) {
	if strings.TrimSpace(actual) == "" {
		return "", false, nil
	}
	sourcePath := filepath.Join(artifactRoot, relPath)
	digest, err := fileSHA256(sourcePath)
	if err != nil {
		message := fmt.Sprintf("%s source unreadable at %s: %v", field, sourcePath, err)
		if strict {
			return "", false, errors.New(message)
		}
		*warnings = append(*warnings, message)
		return "", false, nil
	}
	return digest, true, nil
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

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
