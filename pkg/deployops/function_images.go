package deployops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func resolveRuntimeFunctionRegistry() string {
	for _, key := range []string{"CONTAINER_REGISTRY", "HOST_REGISTRY_ADDR", "REGISTRY"} {
		value := strings.TrimSuffix(strings.TrimSpace(os.Getenv(key)), "/")
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveHostFunctionRegistry() string {
	for _, key := range []string{"HOST_REGISTRY_ADDR", "CONTAINER_REGISTRY", "REGISTRY"} {
		value := strings.TrimSuffix(strings.TrimSpace(os.Getenv(key)), "/")
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveRegistryAliases() []string {
	return sortedUniqueNonEmpty([]string{
		strings.TrimSuffix(strings.TrimSpace(os.Getenv("CONTAINER_REGISTRY")), "/"),
		strings.TrimSuffix(strings.TrimSpace(os.Getenv("HOST_REGISTRY_ADDR")), "/"),
		strings.TrimSuffix(strings.TrimSpace(os.Getenv("REGISTRY")), "/"),
		"127.0.0.1:5010",
		"localhost:5010",
		"registry:5010",
	})
}

func normalizeFunctionImageRefForRuntime(imageRef string) (string, bool) {
	trimmed := strings.TrimSpace(imageRef)
	if !isLambdaFunctionRef(trimmed) {
		return trimmed, false
	}
	return rewriteRegistryAlias(trimmed, resolveRuntimeFunctionRegistry(), resolveRegistryAliases())
}

func rewriteLambdaBaseRefForBuild(imageRef string) (string, bool) {
	trimmed := strings.TrimSpace(imageRef)
	if !isLambdaBaseRef(trimmed) {
		return trimmed, false
	}
	return rewriteRegistryAlias(trimmed, resolveHostFunctionRegistry(), resolveRegistryAliases())
}

func rewriteRegistryAlias(imageRef, targetRegistry string, aliases []string) (string, bool) {
	trimmed := strings.TrimSpace(imageRef)
	target := strings.TrimSuffix(strings.TrimSpace(targetRegistry), "/")
	if trimmed == "" || target == "" {
		return trimmed, false
	}
	targetPrefix := target + "/"
	if strings.HasPrefix(trimmed, targetPrefix) {
		return trimmed, false
	}
	for _, alias := range aliases {
		current := strings.TrimSuffix(strings.TrimSpace(alias), "/")
		if current == "" || current == target {
			continue
		}
		prefix := current + "/"
		if strings.HasPrefix(trimmed, prefix) {
			return targetPrefix + strings.TrimPrefix(trimmed, prefix), true
		}
	}
	return trimmed, false
}

func normalizeOutputFunctionImages(outputDir string, publishedFunctionImages map[string]struct{}) error {
	functionsPath := filepath.Join(outputDir, "functions.yml")
	payload, ok, err := loadYAML(functionsPath)
	if err != nil {
		return fmt.Errorf("load output functions config: %w", err)
	}
	if !ok {
		return nil
	}
	functionsRaw, ok := payload["functions"].(map[string]any)
	if !ok {
		return fmt.Errorf("functions must be map in %s", functionsPath)
	}

	changed := false
	for functionName, raw := range functionsRaw {
		functionPayload, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		imageRaw, ok := functionPayload["image"]
		if !ok {
			continue
		}
		imageRef := strings.TrimSpace(fmt.Sprintf("%v", imageRaw))
		if imageRef == "" {
			continue
		}
		normalized, rewritten := normalizeFunctionImageRefForRuntime(imageRef)
		if !rewritten || normalized == imageRef {
			continue
		}
		if _, ok := publishedFunctionImages[normalized]; !ok {
			continue
		}
		functionPayload["image"] = normalized
		functionsRaw[functionName] = functionPayload
		changed = true
	}

	if !changed {
		return nil
	}
	encoded, err := yaml.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal output functions config: %w", err)
	}
	if err := os.WriteFile(functionsPath, encoded, 0o644); err != nil {
		return fmt.Errorf("write output functions config: %w", err)
	}
	return nil
}

func isLambdaFunctionRef(imageRef string) bool {
	lastSegment := imageRepoLastSegment(imageRef)
	return strings.HasPrefix(lastSegment, "esb-lambda-") && lastSegment != "esb-lambda-base"
}

func imageRepoLastSegment(imageRef string) string {
	ref := strings.TrimSpace(imageRef)
	if ref == "" {
		return ""
	}
	withoutDigest := strings.SplitN(ref, "@", 2)[0]
	if withoutDigest == "" {
		return ""
	}
	slash := strings.LastIndex(withoutDigest, "/")
	colon := strings.LastIndex(withoutDigest, ":")
	repo := withoutDigest
	if colon > slash {
		repo = withoutDigest[:colon]
	}
	if slash >= 0 && slash+1 < len(repo) {
		return repo[slash+1:]
	}
	return repo
}
