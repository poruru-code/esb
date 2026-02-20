// Where: services/agent/internal/identity/stack_identity.go
// What: Stack identity resolution from compose/runtime context.
// Why: Allow agent runtime naming to follow running stack branding without shared meta dependency.
package identity

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	EnvBrandSlug         = "ESB_BRAND_SLUG"
	EnvProjectName       = "PROJECT_NAME"
	EnvName              = "ENV"
	EnvContainersNetwork = "CONTAINERS_NETWORK"

	defaultBrand        = "esb"
	bridgePrefix        = "esb-"
	bridgeBrandRunes    = 4
	bridgeHashRunes     = 6
	subnetPrefixLength  = 23
	subnetThirdStep     = 2
	subnetThirdSlots    = 128   // /23 blocks per second octet
	subnetPoolSize      = 32640 // 255 second-octet slots (excluding 88) x 128 third-octet slots (/23)
	subnetSecondOctetEx = 88
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
	brand := id.normalizedBrand()
	return brand + "-net"
}

func (id StackIdentity) RuntimeCNIBridge() string {
	brand := id.normalizedBrand()
	compactBrand := strings.ReplaceAll(brand, "-", "")
	if compactBrand == "" {
		compactBrand = defaultBrand
	}
	if len(compactBrand) > bridgeBrandRunes {
		compactBrand = compactBrand[:bridgeBrandRunes]
	}
	return bridgePrefix + compactBrand + shortHash(brand, bridgeHashRunes)
}

func (id StackIdentity) RuntimeCNISubnet() string {
	return id.RuntimeCNISubnetAt(0)
}

func (id StackIdentity) RuntimeCNISubnetAt(offset int) string {
	brand := id.normalizedBrand()
	base := int(hashMod(brand, subnetPoolSize))
	slot := (base + offset) % subnetPoolSize
	if slot < 0 {
		slot += subnetPoolSize
	}
	return subnetFromSlot(slot)
}

func RuntimeCNISubnetPoolSize() int {
	return subnetPoolSize
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

func (id StackIdentity) normalizedBrand() string {
	brand := normalizeSlug(id.BrandSlug)
	if brand == "" {
		return defaultBrand
	}
	return brand
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

func shortHash(value string, digits int) string {
	if digits <= 0 {
		return ""
	}
	sum := md5.Sum([]byte(value))
	hex := fmt.Sprintf("%x", sum)
	if digits >= len(hex) {
		return hex
	}
	return hex[:digits]
}

func hashMod(value string, mod int) uint32 {
	if mod <= 0 {
		return 0
	}
	sum := md5.Sum([]byte(value))
	hash := binary.BigEndian.Uint32(sum[:4])
	return hash % uint32(mod)
}

func subnetFromSlot(slot int) string {
	secondIdx := slot / subnetThirdSlots
	secondOctet := secondIdx
	if secondOctet >= subnetSecondOctetEx {
		secondOctet++
	}
	thirdOctet := (slot % subnetThirdSlots) * subnetThirdStep
	return fmt.Sprintf("10.%d.%d.0/%d", secondOctet, thirdOctet, subnetPrefixLength)
}
