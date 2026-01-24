// Where: cli/internal/generator/image_naming.go
// What: Image name sanitization helpers for function images.
// Why: Ensure function image names are Docker-safe and consistent across outputs.
package generator

import (
	"fmt"
	"strings"
)

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

func applyImageNames(functions []FunctionSpec) error {
	seen := map[string]string{}
	for i := range functions {
		imageName, err := imageSafeName(functions[i].Name)
		if err != nil {
			return err
		}
		if existing, ok := seen[imageName]; ok {
			return fmt.Errorf(
				"function image name collision: %q and %q both sanitize to %q",
				existing,
				functions[i].Name,
				imageName,
			)
		}
		seen[imageName] = functions[i].Name
		functions[i].ImageName = imageName
	}
	return nil
}
