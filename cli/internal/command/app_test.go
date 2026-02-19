// Where: cli/internal/command/app_test.go
// What: Tests for CLI run behavior.
// Why: Ensure command routing remains stable.
package command

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/poruru/edge-serverless-box/cli/internal/version"
)

func TestRunNodeDisabled(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run([]string{"node", "add"}, Dependencies{Out: &out})
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for disabled node command")
	}
	if !strings.Contains(out.String(), "node command is disabled") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRunNodeDisabledWithGlobalFlags(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run([]string{"--env", "staging", "--template", "template.yaml", "node", "up"}, Dependencies{Out: &out})
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for disabled node command")
	}
	if !strings.Contains(out.String(), "node command is disabled") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRunNodeDisabledWithImageOverrideFlags(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run([]string{
		"--image-uri", "lambda-image=public.ecr.aws/example/repo:latest",
		"--image-runtime", "lambda-image=python",
		"node", "up",
	}, Dependencies{Out: &out})
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for disabled node command")
	}
	if !strings.Contains(out.String(), "node command is disabled") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestCompletionCommandRemoved(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run([]string{"completion", "bash"}, Dependencies{Out: &out})
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for removed completion command")
	}
	output := strings.ToLower(out.String())
	if !strings.Contains(output, "unknown") &&
		!strings.Contains(output, "expected one of") &&
		!strings.Contains(output, "unexpected argument") {
		t.Fatalf("unexpected output for removed completion command: %q", out.String())
	}
}

func TestDispatchCommandVersion(t *testing.T) {
	var out bytes.Buffer
	exitCode, handled := dispatchCommand("version", CLI{}, Dependencies{}, &out)
	if !handled {
		t.Fatal("expected version command to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(out.String(), version.GetVersion()) {
		t.Fatalf("expected version output, got %q", out.String())
	}
}

func TestDispatchCommandArtifactGenerate(t *testing.T) {
	var out bytes.Buffer
	exitCode, handled := dispatchCommand("artifact generate", CLI{}, Dependencies{}, &out)
	if !handled {
		t.Fatal("expected artifact generate command to be handled")
	}
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code when dependencies are missing, got %d", exitCode)
	}
}

func TestDeployParseApplyOptions(t *testing.T) {
	cli := CLI{}
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	if _, err := parser.Parse([]string{"deploy", "--secret-env", "secret.env", "--strict"}); err != nil {
		t.Fatalf("parser.Parse() error = %v", err)
	}
	if cli.Deploy.SecretEnv != "secret.env" {
		t.Fatalf("SecretEnv=%q, want secret.env", cli.Deploy.SecretEnv)
	}
	if !cli.Deploy.Strict {
		t.Fatal("Strict=false, want true")
	}
}

func TestDeployParseRejectsNoDepsFlag(t *testing.T) {
	cli := CLI{}
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	if _, err := parser.Parse([]string{"deploy", "--no-deps"}); err == nil {
		t.Fatal("parser.Parse() succeeded for removed --no-deps flag")
	}
}

func TestDispatchCommandUnknown(t *testing.T) {
	exitCode, handled := dispatchCommand("unknown", CLI{}, Dependencies{}, &bytes.Buffer{})
	if handled {
		t.Fatal("expected unknown command to be unhandled")
	}
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
}

func TestHandleParseErrorTemplateFlag(t *testing.T) {
	var out bytes.Buffer
	exitCode := handleParseError(nil, errors.New("expected string value for --template"), Dependencies{}, &out)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	output := out.String()
	if !strings.Contains(output, "`-t/--template` expects a value") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestHandleParseErrorFallback(t *testing.T) {
	var out bytes.Buffer
	exitCode := handleParseError(nil, errors.New("boom"), Dependencies{}, &out)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(out.String(), "boom") {
		t.Fatalf("expected error message in output, got %q", out.String())
	}
}

func TestHandleParseErrorKnownFlags(t *testing.T) {
	tests := []string{"--env", "--mode", "--env-file", "--manifest", "--image-uri", "--image-runtime"}
	for _, flag := range tests {
		t.Run(flag, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := handleParseError(nil, errors.New("expected string value for "+flag), Dependencies{}, &out)
			if exitCode != 1 {
				t.Fatalf("expected exit code 1, got %d", exitCode)
			}
			if !strings.Contains(out.String(), "expects a value") {
				t.Fatalf("unexpected output: %q", out.String())
			}
		})
	}
}

func TestRunNoArgsShowsUsage(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run(nil, Dependencies{Out: &out, ErrOut: &out})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("expected usage output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "esb deploy --template") {
		t.Fatalf("expected fixed cli name in usage output, got %q", out.String())
	}
}

func TestRunVersionCommand(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run([]string{"version"}, Dependencies{Out: &out, ErrOut: &out})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(out.String(), version.GetVersion()) {
		t.Fatalf("expected version output, got %q", out.String())
	}
}

func TestRunRepoScopedCommandFailsOutsideRepo(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run([]string{"deploy"}, Dependencies{
		Out:    &out,
		ErrOut: &out,
		RepoResolver: func(string) (string, error) {
			return "", errors.New("repo not found")
		},
	})
	if exitCode != repoRequiredExitCode {
		t.Fatalf("expected exit code %d, got %d", repoRequiredExitCode, exitCode)
	}
	if !strings.Contains(out.String(), repoRequiredErrorMessage) {
		t.Fatalf("expected repo-scope warning, got %q", out.String())
	}
}

func TestRunVersionAllowedOutsideRepo(t *testing.T) {
	var out bytes.Buffer
	exitCode := Run([]string{"version"}, Dependencies{
		Out:    &out,
		ErrOut: &out,
		RepoResolver: func(string) (string, error) {
			return "", errors.New("repo not found")
		},
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(out.String(), version.GetVersion()) {
		t.Fatalf("expected version output, got %q", out.String())
	}
}

func TestRequiresRepoScope(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "no args", args: nil, want: true},
		{name: "version", args: []string{"version"}, want: false},
		{name: "root help flag", args: []string{"--help"}, want: false},
		{name: "root short help flag", args: []string{"-h"}, want: false},
		{name: "help command", args: []string{"help"}, want: false},
		{name: "help command with global value flag", args: []string{"--env", "dev", "help"}, want: false},
		{name: "subcommand help", args: []string{"deploy", "--help"}, want: false},
		{name: "subcommand short help", args: []string{"deploy", "-h"}, want: false},
		{name: "deploy", args: []string{"deploy"}, want: true},
		{name: "deploy with global flags", args: []string{"--env", "dev", "deploy"}, want: true},
		{name: "help token in flag value is ignored", args: []string{"--env", "help", "deploy"}, want: true},
		{name: "help token in compose-file value is ignored", args: []string{"--compose-file", "help", "deploy"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requiresRepoScope(tt.args)
			if got != tt.want {
				t.Fatalf("requiresRepoScope(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestCommandFlagExpectsValueUsesCLIReflection(t *testing.T) {
	tests := []struct {
		flag string
		want bool
	}{
		{flag: "--compose-file", want: true},
		{flag: "--env", want: true},
		{flag: "--no-cache", want: false},
		{flag: "-v", want: false},
		{flag: "-e", want: true},
		{flag: "--manifest=artifact.yml", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			got := commandFlagExpectsValue(tt.flag)
			if got != tt.want {
				t.Fatalf("commandFlagExpectsValue(%q) = %v, want %v", tt.flag, got, tt.want)
			}
		})
	}
}
