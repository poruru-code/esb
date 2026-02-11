// Where: cli/internal/command/deploy_inputs_resolve_test.go
// What: Integration-style unit tests for deploy input resolution orchestration.
// Why: Ensure runtime discovery failures are surfaced as warnings instead of being silently ignored.
package command

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
)

func TestResolveDeployInputsWarnsWhenRuntimeDiscoveryFails(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.docker.yml"), []byte("version: '3'\n"), 0o600); err != nil {
		t.Fatalf("write compose marker: %v", err)
	}
	templatePath := filepath.Join(repoRoot, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	var errOut bytes.Buffer
	cli := CLI{
		Template: []string{templatePath},
		EnvFlag:  "dev",
		Deploy: DeployCmd{
			Mode:   "docker",
			NoSave: true,
		},
	}
	deps := Dependencies{
		ErrOut: &errOut,
		RepoResolver: func(string) (string, error) {
			return repoRoot, nil
		},
		Deploy: DeployDeps{
			Runtime: DeployRuntimeDeps{
				DockerClient: func() (compose.DockerClient, error) {
					return nil, errors.New("docker unavailable")
				},
			},
		},
	}

	inputs, err := resolveDeployInputs(cli, deps)
	if err != nil {
		t.Fatalf("resolve deploy inputs: %v", err)
	}
	if strings.TrimSpace(inputs.Project) == "" {
		t.Fatalf("expected compose project to be resolved")
	}
	out := errOut.String()
	if !strings.Contains(out, "failed to discover running stacks") {
		t.Fatalf("expected stack discovery warning, got %q", out)
	}
	if !strings.Contains(out, "failed to infer runtime mode") {
		t.Fatalf("expected mode inference warning, got %q", out)
	}
}

func TestResolveDeployInputsRejectsTemplateRepoRootMismatch(t *testing.T) {
	repoA := filepath.Join(t.TempDir(), "repo-a")
	repoB := filepath.Join(t.TempDir(), "repo-b")
	if err := os.MkdirAll(repoA, 0o755); err != nil {
		t.Fatalf("mkdir repo a: %v", err)
	}
	if err := os.MkdirAll(repoB, 0o755); err != nil {
		t.Fatalf("mkdir repo b: %v", err)
	}
	templateA := filepath.Join(repoA, "template-a.yaml")
	templateB := filepath.Join(repoB, "template-b.yaml")
	if err := os.WriteFile(templateA, []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template a: %v", err)
	}
	if err := os.WriteFile(templateB, []byte("Resources: {}\n"), 0o600); err != nil {
		t.Fatalf("write template b: %v", err)
	}

	cli := CLI{
		Template: []string{templateA, templateB},
		EnvFlag:  "dev",
		Deploy: DeployCmd{
			Mode:    "docker",
			Project: "esb-dev",
			NoSave:  true,
		},
	}
	deps := Dependencies{
		RepoResolver: func(path string) (string, error) {
			cleaned := filepath.Clean(path)
			switch {
			case cleaned == "" || cleaned == ".":
				return repoA, nil
			case strings.HasPrefix(cleaned, repoA):
				return repoA, nil
			case strings.HasPrefix(cleaned, repoB):
				return repoB, nil
			default:
				return "", errors.New("unexpected path")
			}
		},
	}

	_, err := resolveDeployInputs(cli, deps)
	if err == nil {
		t.Fatal("expected template repo root mismatch error")
	}
	if !strings.Contains(err.Error(), "template repo root mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}
