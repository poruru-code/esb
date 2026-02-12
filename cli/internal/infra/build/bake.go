// Where: cli/internal/infra/build/bake.go
// What: Shared bake types and builder name helpers.
// Why: Keep common bake definitions in one small file.
package build

import (
	"fmt"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/meta"
)

type bakeTarget struct {
	Name       string
	Context    string
	Dockerfile string
	Tags       []string
	Outputs    []string
	Labels     map[string]string
	Args       map[string]string
	Contexts   map[string]string
	Secrets    []string
	NoCache    bool
}

func buildxBuilderName() string {
	if value := strings.TrimSpace(os.Getenv("BUILDX_BUILDER")); value != "" {
		return value
	}
	return fmt.Sprintf("%s-buildx", meta.Slug)
}
