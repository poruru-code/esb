// Where: services/agent/internal/runtime/image_naming.go
// What: Image name sanitization helpers for agent runtime.
// Why: Ensure agent resolves function image names consistently and safely.
package runtime

import (
	"fmt"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/meta"
)

func resolveImagePrefix() string {
	if prefix := strings.TrimSpace(os.Getenv("IMAGE_PREFIX")); prefix != "" {
		return prefix
	}
	return meta.ImagePrefix
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
	return fmt.Sprintf("%s-%s", resolveImagePrefix(), safeName), nil
}
