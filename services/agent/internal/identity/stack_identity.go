// Where: services/agent/internal/identity/stack_identity.go
// What: Stack identity resolution from compose/runtime context.
// Why: Allow agent runtime naming to follow running stack branding without shared meta dependency.
package identity

import (
	"fmt"
	"strings"
)

const (
	EnvBrandSlug         = "ESB_BRAND_SLUG"
	EnvProjectName       = "PROJECT_NAME"
	EnvName              = "ENV"
	EnvContainersNetwork = "CONTAINERS_NETWORK"

	fixedCNIBridge = "esb0"
)

type StackIdentity struct {
	BrandSlug string
	Source    string
}

func ResolveStackIdentityFrom(brandSlug, projectName, envName, containersNetwork string) (StackIdentity, error) {
	if slug := normalizeSlug(brandSlug); slug != "" {
		return StackIdentity{BrandSlug: slug, Source: EnvBrandSlug}, nil
	}
	if slug := deriveBrandFromProject(projectName, envName); slug != "" {
		return StackIdentity{BrandSlug: slug, Source: EnvProjectName}, nil
	}
	if slug := deriveBrandFromNetwork(containersNetwork, envName); slug != "" {
		return StackIdentity{BrandSlug: slug, Source: EnvContainersNetwork}, nil
	}
	return StackIdentity{}, fmt.Errorf(
		"stack identity is not resolvable: set %s, or provide %s/%s, or provide %s",
		EnvBrandSlug,
		EnvProjectName,
		EnvName,
		EnvContainersNetwork,
	)
}

func (id StackIdentity) RuntimeNamespace() string {
	return id.BrandSlug
}

func (id StackIdentity) RuntimeCNIName() string {
	return id.BrandSlug + "-net"
}

func (id StackIdentity) RuntimeCNIBridge() string {
	// Keep bridge name fixed for compatibility with runtime-node iptables forwarding rules.
	return fixedCNIBridge
}

func (id StackIdentity) RuntimeResolvConfPath() string {
	return "/run/containerd/" + id.RuntimeNamespace() + "/resolv.conf"
}

func (id StackIdentity) RuntimeContainerPrefix() string {
	return id.BrandSlug
}

func (id StackIdentity) ImagePrefix() string {
	return id.BrandSlug
}

func (id StackIdentity) EnvPrefix() string {
	return strings.ToUpper(strings.ReplaceAll(id.BrandSlug, "-", "_"))
}

func (id StackIdentity) LabelPrefix() string {
	return "com." + id.BrandSlug
}

func (id StackIdentity) RuntimeLabelEnv() string {
	return id.BrandSlug + "_env"
}

func (id StackIdentity) RuntimeLabelFunction() string {
	return id.BrandSlug + "_function"
}

func (id StackIdentity) RuntimeLabelCreatedBy() string {
	return "created_by"
}

func (id StackIdentity) RuntimeLabelCreatedByValue() string {
	return id.BrandSlug + "-agent"
}

func deriveBrandFromProject(projectName, envName string) string {
	project := strings.TrimSpace(projectName)
	if project == "" {
		return ""
	}
	lowerProject := strings.ToLower(project)
	lowerEnv := strings.ToLower(strings.TrimSpace(envName))
	if lowerEnv != "" {
		hyphenSuffix := "-" + lowerEnv
		underscoreSuffix := "_" + lowerEnv
		switch {
		case strings.HasSuffix(lowerProject, hyphenSuffix):
			project = project[:len(project)-len(hyphenSuffix)]
		case strings.HasSuffix(lowerProject, underscoreSuffix):
			project = project[:len(project)-len(underscoreSuffix)]
		}
	}
	slug := normalizeSlug(project)
	if slug != "" {
		return slug
	}
	return normalizeSlug(projectName)
}

func deriveBrandFromNetwork(containersNetwork, envName string) string {
	network := strings.TrimSpace(containersNetwork)
	if network == "" {
		return ""
	}
	lowerNetwork := strings.ToLower(network)
	switch lowerNetwork {
	case "bridge", "host", "none":
		return ""
	}
	network = trimKnownSuffix(network, "-external")
	network = trimKnownSuffix(network, "_external")

	lowerEnv := strings.ToLower(strings.TrimSpace(envName))
	if lowerEnv != "" {
		network = trimKnownSuffix(network, "-"+lowerEnv)
		network = trimKnownSuffix(network, "_"+lowerEnv)
	}

	slug := normalizeSlug(network)
	if slug != "" {
		return slug
	}
	return normalizeSlug(containersNetwork)
}

func trimKnownSuffix(value, suffix string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(trimmed), strings.ToLower(suffix)) {
		return trimmed[:len(trimmed)-len(suffix)]
	}
	return trimmed
}

func normalizeSlug(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return ""
	}
	return slug
}
