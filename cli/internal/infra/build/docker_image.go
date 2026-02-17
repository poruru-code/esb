// Where: cli/internal/infra/build/docker_image.go
// What: Docker image discovery env.
// Why: Avoid rebuilding shared base images when they already exist.
package build

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
)

func dockerImageHasLabelValue(
	ctx context.Context,
	runner compose.CommandRunner,
	contextDir string,
	imageTag string,
	label string,
	expected string,
) bool {
	if runner == nil || imageTag == "" || label == "" || expected == "" {
		return false
	}
	if !dockerImageExists(ctx, runner, contextDir, imageTag) {
		return false
	}
	format := fmt.Sprintf("{{ index .Config.Labels %q }}", label)
	out, err := runner.RunOutput(ctx, contextDir, "docker", "image", "inspect", "--format", format, imageTag)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == expected
}

func dockerImageID(
	ctx context.Context,
	runner compose.CommandRunner,
	contextDir string,
	imageTag string,
) string {
	if runner == nil || imageTag == "" {
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

func dockerImageRepoDigests(
	ctx context.Context,
	runner compose.CommandRunner,
	contextDir string,
	imageTag string,
) []string {
	if runner == nil || imageTag == "" {
		return nil
	}
	out, err := runner.RunOutput(
		ctx,
		contextDir,
		"docker",
		"image",
		"inspect",
		"--format",
		"{{range .RepoDigests}}{{println .}}{{end}}",
		imageTag,
	)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(out), "\n")
	digests := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		digest := strings.TrimSpace(line)
		if digest == "" {
			continue
		}
		if _, ok := seen[digest]; ok {
			continue
		}
		seen[digest] = struct{}{}
		digests = append(digests, digest)
	}
	sort.Strings(digests)
	return digests
}

func dockerImageExists(
	ctx context.Context,
	runner compose.CommandRunner,
	contextDir string,
	imageTag string,
) bool {
	if runner == nil || imageTag == "" {
		return false
	}
	out, err := runner.RunOutput(ctx, contextDir, "docker", "image", "ls", "-q", imageTag)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
