// Where: cli/internal/infra/build/go_builder_functions_test.go
// What: Tests for image-source digest resolution used in build skip decisions.
// Why: Ensure mutable image tags are refreshed and digest-pinned refs avoid unnecessary pulls.
package build

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/poruru-code/esb/cli/internal/domain/template"
)

func TestResolveImageSourceDigestsPullsMutableSourceAndUsesRepoDigest(t *testing.T) {
	source := "public.ecr.aws/example/repo:latest"
	runner := &recordRunner{
		outputs: map[string][]byte{
			"docker image inspect --format {{range .RepoDigests}}{{println .}}{{end}} " + source: []byte(
				"public.ecr.aws/example/repo@sha256:111\n",
			),
		},
	}

	digests, err := resolveImageSourceDigests(
		context.Background(),
		runner,
		t.TempDir(),
		[]template.FunctionSpec{{Name: "lambda-image", ImageSource: source}},
		false,
		io.Discard,
	)
	if err != nil {
		t.Fatalf("resolve image source digests: %v", err)
	}

	if got := digests[source]; got != "public.ecr.aws/example/repo@sha256:111" {
		t.Fatalf("unexpected digest seed: %q", got)
	}
	if !containsDockerPullCall(runner.calls, source) {
		t.Fatalf("expected docker pull for mutable image source %q", source)
	}
}

func TestResolveImageSourceDigestsSkipsPullForDigestPinnedSource(t *testing.T) {
	source := "public.ecr.aws/example/repo@sha256:abc123"
	runner := &recordRunner{}

	digests, err := resolveImageSourceDigests(
		context.Background(),
		runner,
		t.TempDir(),
		[]template.FunctionSpec{{Name: "lambda-image", ImageSource: source}},
		false,
		io.Discard,
	)
	if err != nil {
		t.Fatalf("resolve image source digests: %v", err)
	}

	if got := digests[source]; got != "sha256:abc123" {
		t.Fatalf("unexpected digest seed: %q", got)
	}
	if containsDockerPullCall(runner.calls, source) {
		t.Fatalf("did not expect docker pull for digest-pinned source %q", source)
	}
}

func containsDockerPullCall(calls []commandCall, source string) bool {
	for _, call := range calls {
		if call.name != "docker" {
			continue
		}
		if len(call.args) != 2 {
			continue
		}
		if call.args[0] != "pull" {
			continue
		}
		if strings.TrimSpace(call.args[1]) == source {
			return true
		}
	}
	return false
}
