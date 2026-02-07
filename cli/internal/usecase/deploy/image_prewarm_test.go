// Where: cli/internal/usecase/deploy/image_prewarm_test.go
// What: Tests for deploy-time image prewarm.
// Why: Ensure prewarm executes pull/tag/push and fails with classified errors.
package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunImagePrewarmSuccess(t *testing.T) {
	manifest := `{
  "version": "1",
  "push_target": "127.0.0.1:5010",
  "images": [
    {
      "function_name": "lambda-image",
      "image_source": "public.ecr.aws/example/repo:latest",
      "image_ref": "registry:5010/public.ecr.aws/example/repo:latest"
    }
  ]
}`
	path := filepath.Join(t.TempDir(), "image-import.json")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &fakeComposeRunner{}
	if err := runImagePrewarm(context.Background(), runner, nil, path, false); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(runner.commands) != 3 {
		t.Fatalf("expected 3 docker commands, got %d", len(runner.commands))
	}
	if got := strings.Join(runner.commands[0], " "); !strings.Contains(got, "docker pull public.ecr.aws/example/repo:latest") {
		t.Fatalf("unexpected pull command: %s", got)
	}
	if got := strings.Join(runner.commands[1], " "); !strings.Contains(got, "docker tag public.ecr.aws/example/repo:latest 127.0.0.1:5010/public.ecr.aws/example/repo:latest") {
		t.Fatalf("unexpected tag command: %s", got)
	}
	if got := strings.Join(runner.commands[2], " "); !strings.Contains(got, "docker push 127.0.0.1:5010/public.ecr.aws/example/repo:latest") {
		t.Fatalf("unexpected push command: %s", got)
	}
}

func TestRunImagePrewarmAuthFailureClassification(t *testing.T) {
	manifest := `{
  "version": "1",
  "images": [
    {
      "function_name": "lambda-image",
      "image_source": "private.example.com/repo:latest",
      "image_ref": "registry:5010/private.example.com/repo:latest"
    }
  ]
}`
	path := filepath.Join(t.TempDir(), "image-import.json")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &fakeComposeRunner{
		err:    os.ErrPermission,
		output: []byte("unauthorized: authentication required"),
	}
	err := runImagePrewarm(context.Background(), runner, nil, path, false)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), imageAuthFailedCode) {
		t.Fatalf("expected auth classification, got %v", err)
	}
}

func TestRunImagePrewarmWithoutPushTargetUsesImageRef(t *testing.T) {
	manifest := `{
  "version": "1",
  "images": [
    {
      "function_name": "lambda-image",
      "image_source": "public.ecr.aws/example/repo:latest",
      "image_ref": "registry:5010/public.ecr.aws/example/repo:latest"
    }
  ]
}`
	path := filepath.Join(t.TempDir(), "image-import.json")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &fakeComposeRunner{}
	if err := runImagePrewarm(context.Background(), runner, nil, path, false); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := strings.Join(runner.commands[2], " "); !strings.Contains(got, "docker push registry:5010/public.ecr.aws/example/repo:latest") {
		t.Fatalf("unexpected push command: %s", got)
	}
}

func TestResolveHostPushTarget(t *testing.T) {
	got := resolveHostPushTarget(
		"registry:5010/public.ecr.aws/example/repo:latest",
		"http://127.0.0.1:5010/",
	)
	want := "127.0.0.1:5010/public.ecr.aws/example/repo:latest"
	if got != want {
		t.Fatalf("unexpected push target: got %q, want %q", got, want)
	}

	unchanged := resolveHostPushTarget("repo:latest", "127.0.0.1:5010")
	if unchanged != "repo:latest" {
		t.Fatalf("expected unchanged reference, got %q", unchanged)
	}
}
