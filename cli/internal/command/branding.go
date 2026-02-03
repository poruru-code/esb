// Where: cli/internal/command/branding.go
// What: Brand-aware CLI naming env.
// Why: Keep user-facing command names consistent with the current brand.
package command

import (
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/meta"
)

func cliName() string {
	name := strings.TrimSpace(os.Getenv("CLI_CMD"))
	if name == "" {
		name = strings.TrimSpace(meta.Slug)
	}
	if name == "" {
		name = "esb"
	}
	return name
}
