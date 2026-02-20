package runtimeimage

import (
	"sort"
	"strings"
)

// InferModeFromServiceImages infers runtime mode from compose service image refs.
func InferModeFromServiceImages(serviceImages map[string]string) string {
	if len(serviceImages) == 0 {
		return ""
	}
	imageRefs := make([]string, 0, len(serviceImages))
	for _, ref := range serviceImages {
		imageRefs = append(imageRefs, ref)
	}
	sort.Strings(imageRefs)
	return InferModeFromImageRefs(imageRefs)
}

// InferModeFromImageRefs infers runtime mode from image ref naming conventions.
func InferModeFromImageRefs(imageRefs []string) string {
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

// ParseTag extracts the image tag from a container image reference.
func ParseTag(imageRef string) string {
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

// PreferredServiceImage selects the image ref from preferred runtime services, then lexicographic fallback.
func PreferredServiceImage(serviceImages map[string]string) (string, string) {
	preferred := []string{"gateway", "agent", "provisioner", "runtime-node"}
	for _, service := range preferred {
		if imageRef := strings.TrimSpace(serviceImages[service]); imageRef != "" {
			return imageRef, service
		}
	}
	keys := make([]string, 0, len(serviceImages))
	for key := range serviceImages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return "", ""
	}
	service := keys[0]
	return strings.TrimSpace(serviceImages[service]), service
}
