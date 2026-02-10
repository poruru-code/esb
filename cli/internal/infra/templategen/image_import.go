// Where: cli/internal/infra/templategen/image_import.go
// What: Image source/reference normalization and import manifest generation.
// Why: Support SAM image functions by mapping external image sources to the internal registry.
package templategen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
)

type imageImportManifest struct {
	Version    string             `json:"version"`
	PushTarget string             `json:"push_target"`
	Images     []imageImportEntry `json:"images"`
}

type imageImportEntry struct {
	FunctionName string `json:"function_name"`
	ImageSource  string `json:"image_source"`
	ImageRef     string `json:"image_ref"`
}

func resolveImageImports(functions []template.FunctionSpec, runtimeRegistry string) ([]imageImportEntry, error) {
	internalRegistry := resolveInternalRegistry(runtimeRegistry)
	entries := make([]imageImportEntry, 0)

	for i := range functions {
		source := strings.TrimSpace(functions[i].ImageSource)
		if source == "" {
			continue
		}
		entry, needsImport, err := buildImageImportEntry(functions[i].Name, source, internalRegistry)
		if err != nil {
			return nil, err
		}
		functions[i].ImageRef = entry.ImageRef
		if needsImport {
			entries = append(entries, entry)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].FunctionName == entries[j].FunctionName {
			return entries[i].ImageRef < entries[j].ImageRef
		}
		return entries[i].FunctionName < entries[j].FunctionName
	})

	if len(entries) == 0 {
		return nil, nil
	}
	return entries, nil
}

func writeImageImportManifest(path string, entries []imageImportEntry) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("image import manifest path is required")
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	pushTarget, _ := resolveHostRegistryAddress()
	if entries == nil {
		entries = []imageImportEntry{}
	}
	manifest := imageImportManifest{
		Version:    "1",
		PushTarget: pushTarget,
		Images:     entries,
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal image import manifest: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write image import manifest: %w", err)
	}
	return nil
}

func resolveInternalRegistry(runtimeRegistry string) string {
	trimmed := strings.TrimSpace(runtimeRegistry)
	if trimmed == "" {
		return constants.DefaultContainerRegistry
	}
	trimmed = strings.TrimPrefix(trimmed, "http://")
	trimmed = strings.TrimPrefix(trimmed, "https://")
	trimmed = strings.TrimSuffix(trimmed, "/")
	if slash := strings.Index(trimmed, "/"); slash != -1 {
		trimmed = trimmed[:slash]
	}
	if trimmed == "" {
		return constants.DefaultContainerRegistry
	}
	return trimmed
}

func buildImageImportEntry(
	functionName string,
	imageSource string,
	internalRegistry string,
) (imageImportEntry, bool, error) {
	host, repo, tag, _, err := splitImageSource(imageSource)
	if err != nil {
		return imageImportEntry{}, false, err
	}
	sourceHost := strings.TrimSpace(host)
	if sourceHost == "" {
		sourceHost = "docker.io"
	}
	if strings.EqualFold(sourceHost, internalRegistry) {
		return imageImportEntry{
			FunctionName: functionName,
			ImageSource:  imageSource,
			ImageRef:     imageSource,
		}, false, nil
	}

	normalizedHost := strings.ReplaceAll(sourceHost, ":", "_")
	ref := fmt.Sprintf("%s/%s/%s:%s", internalRegistry, normalizedHost, repo, tag)
	return imageImportEntry{
		FunctionName: functionName,
		ImageSource:  imageSource,
		ImageRef:     ref,
	}, true, nil
}

func splitImageSource(ref string) (host string, repo string, tag string, digest string, err error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", "", "", "", fmt.Errorf("image source is required")
	}

	namePart := trimmed
	if at := strings.Index(trimmed, "@"); at != -1 {
		namePart = trimmed[:at]
		digest = strings.TrimSpace(trimmed[at+1:])
	}

	tag = ""
	lastSlash := strings.LastIndex(namePart, "/")
	lastColon := strings.LastIndex(namePart, ":")
	if lastColon > lastSlash {
		tag = namePart[lastColon+1:]
		namePart = namePart[:lastColon]
	}

	if tag == "" {
		if digest != "" {
			tag = digestToTag(digest)
		} else {
			tag = "latest"
		}
	}

	firstSlash := strings.Index(namePart, "/")
	if firstSlash == -1 {
		host = "docker.io"
		repo = ensureLibraryRepo(namePart)
		return host, repo, tag, digest, nil
	}
	firstPart := namePart[:firstSlash]
	rest := namePart[firstSlash+1:]
	if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") || firstPart == "localhost" {
		host = firstPart
		repo = rest
	} else {
		host = "docker.io"
		repo = namePart
	}
	if strings.TrimSpace(repo) == "" {
		return "", "", "", "", fmt.Errorf("invalid image source: %s", ref)
	}
	return host, repo, tag, digest, nil
}

func digestToTag(digest string) string {
	trimmed := strings.TrimSpace(digest)
	if trimmed == "" {
		return "latest"
	}
	if strings.HasPrefix(trimmed, "sha256:") {
		hex := strings.TrimPrefix(trimmed, "sha256:")
		if len(hex) > 12 {
			hex = hex[:12]
		}
		return "sha256-" + hex
	}
	replacer := strings.NewReplacer(":", "-", "/", "-", "@", "-")
	return replacer.Replace(trimmed)
}

func ensureLibraryRepo(repo string) string {
	trimmed := strings.TrimSpace(repo)
	if strings.Count(trimmed, "/") == 0 {
		return "library/" + trimmed
	}
	return trimmed
}
