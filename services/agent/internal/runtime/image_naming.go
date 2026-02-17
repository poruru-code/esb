// Where: services/agent/internal/runtime/image_naming.go
// What: Image name sanitization helpers for agent runtime.
// Why: Ensure agent resolves function image names consistently and safely.
package runtime

import (
	"fmt"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/services/agent/internal/identity"
)

func resolveImageIdentity() (identity.StackIdentity, error) {
	return identity.ResolveStackIdentityFrom(
		strings.TrimSpace(os.Getenv(identity.EnvBrandSlug)),
		strings.TrimSpace(os.Getenv(identity.EnvProjectName)),
		strings.TrimSpace(os.Getenv(identity.EnvName)),
		strings.TrimSpace(os.Getenv(identity.EnvContainersNetwork)),
	)
}

func resolveImagePrefix() (string, error) {
	id, err := resolveImageIdentity()
	if err != nil {
		return "", err
	}
	return id.ImagePrefix(), nil
}

func imageSafeName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("function name is required")
	}
	lower := strings.ToLower(trimmed)

	var b strings.Builder
	prevSeparator := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevSeparator = false
			continue
		}
		if r == '.' || r == '_' || r == '-' {
			if !prevSeparator {
				b.WriteRune(r)
				prevSeparator = true
			}
			continue
		}
		if !prevSeparator {
			b.WriteByte('-')
			prevSeparator = true
		}
	}

	result := strings.Trim(b.String(), "._-")
	if result == "" {
		return "", fmt.Errorf("function name %q yields empty image name", name)
	}
	return result, nil
}

func ResolveFunctionImageName(functionName string) (string, error) {
	safeName, err := imageSafeName(functionName)
	if err != nil {
		return "", err
	}
	prefix, err := resolveImagePrefix()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", prefix, safeName), nil
}

func ResolveFunctionImageTag() (string, error) {
	id, err := resolveImageIdentity()
	if err != nil {
		return "", err
	}
	key := id.EnvPrefix() + "_TAG"
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value, nil
	}
	if key != "ESB_TAG" {
		if value := strings.TrimSpace(os.Getenv("ESB_TAG")); value != "" {
			return value, nil
		}
	}
	return "latest", nil
}
