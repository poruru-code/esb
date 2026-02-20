package artifactcore

import (
	"sort"
	"strings"
)

// InferRuntimeModeFromServiceImages infers runtime mode from compose service image refs.
func InferRuntimeModeFromServiceImages(serviceImages map[string]string) string {
	if len(serviceImages) == 0 {
		return ""
	}
	imageRefs := make([]string, 0, len(serviceImages))
	for _, ref := range serviceImages {
		imageRefs = append(imageRefs, ref)
	}
	sort.Strings(imageRefs)
	return InferRuntimeModeFromImageRefs(imageRefs)
}

// InferRuntimeModeFromImageRefs infers runtime mode from image ref naming conventions.
func InferRuntimeModeFromImageRefs(imageRefs []string) string {
	foundDocker := false
	for _, imageRef := range imageRefs {
		trimmed := strings.TrimSpace(imageRef)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "-containerd:") {
			return "containerd"
		}
		if strings.Contains(trimmed, "-docker:") {
			foundDocker = true
		}
	}
	if foundDocker {
		return "docker"
	}
	return ""
}

// ParseRuntimeImageTag extracts the image tag from a container image reference.
func ParseRuntimeImageTag(imageRef string) string {
	withoutDigest := strings.SplitN(strings.TrimSpace(imageRef), "@", 2)[0]
	if withoutDigest == "" {
		return ""
	}
	slash := strings.LastIndex(withoutDigest, "/")
	colon := strings.LastIndex(withoutDigest, ":")
	if colon <= slash || colon+1 >= len(withoutDigest) {
		return ""
	}
	return strings.TrimSpace(withoutDigest[colon+1:])
}
