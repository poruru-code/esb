// Where: cli/internal/infra/build/go_builder_paths.go
// What: Lightweight naming/path helpers used by GoBuilder.
// Why: Keep Build() focused on orchestration sequence rather than inline derivations.
package build

import (
	"fmt"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/meta"
)

func resolveComposeProjectName(projectName, appName, env string) string {
	composeProject := strings.TrimSpace(projectName)
	if composeProject != "" {
		return composeProject
	}
	brandName := strings.ToLower(strings.TrimSpace(appName))
	if brandName == "" {
		brandName = strings.ToLower(strings.TrimSpace(os.Getenv("CLI_CMD")))
	}
	if brandName == "" {
		brandName = meta.Slug
	}
	return fmt.Sprintf("%s-%s", brandName, strings.ToLower(strings.TrimSpace(env)))
}

func resolveRuntimeRegistry(defaultRegistry string) string {
	runtimeRegistry := strings.TrimSpace(defaultRegistry)
	if value := strings.TrimSpace(os.Getenv(constants.EnvContainerRegistry)); value != "" {
		if !strings.HasSuffix(value, "/") {
			value += "/"
		}
		runtimeRegistry = value
	}
	return runtimeRegistry
}
