// Where: cli/internal/infra/templategen/image_helpers.go
// What: Shared image tag and inspect helpers for templategen pipeline.
// Why: Keep bundle/image manifest logic self-contained outside infra/build.
package templategen

import (
	"context"
	"fmt"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/meta"
)

func lambdaBaseImageTag(registry, tag string) string {
	imageTag := fmt.Sprintf("%s-lambda-base:%s", meta.ImagePrefix, tag)
	return joinRegistry(registry, imageTag)
}

func joinRegistry(registry, image string) string {
	if strings.TrimSpace(registry) == "" {
		return image
	}
	if strings.HasSuffix(registry, "/") {
		return registry + image
	}
	return registry + "/" + image
}

func dockerImageID(
	ctx context.Context,
	runner compose.CommandRunner,
	contextDir string,
	imageTag string,
) string {
	if runner == nil || strings.TrimSpace(imageTag) == "" {
		return ""
	}
	if !dockerImageExists(ctx, runner, contextDir, imageTag) {
		return ""
	}
	out, err := runner.RunOutput(ctx, contextDir, "docker", "image", "inspect", "--format", "{{.Id}}", imageTag)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func dockerImageExists(
	ctx context.Context,
	runner compose.CommandRunner,
	contextDir string,
	imageTag string,
) bool {
	if runner == nil || strings.TrimSpace(imageTag) == "" {
		return false
	}
	out, err := runner.RunOutput(ctx, contextDir, "docker", "image", "ls", "-q", imageTag)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
