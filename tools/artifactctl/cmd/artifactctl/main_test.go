package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
	"github.com/poruru/edge-serverless-box/tools/artifactctl/pkg/deployops"
)

func newNoopDeps(out, errOut *bytes.Buffer) commandDeps {
	return commandDeps{
		executeDeploy: func(deployops.Input) (artifactcore.ApplyResult, error) {
			return artifactcore.ApplyResult{}, nil
		},
		executeProvision: func(ProvisionInput) error { return nil },
		syncManifestIDs:  func(string, bool) (int, error) { return 0, nil },
		out:              out,
		errOut:           errOut,
	}
}

func TestRunShowsHelp(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--help"}, newNoopDeps(&out, &errOut))
	if code != 0 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(out.String(), "deploy") || !strings.Contains(out.String(), "provision") {
		t.Fatalf("expected help output to mention deploy/provision, got: %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %q", errOut.String())
	}
}

func TestRunDeployHelpShowsFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"deploy", "--help"}, newNoopDeps(&out, &errOut))
	if code != 0 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(out.String(), "--artifact") || !strings.Contains(out.String(), "--out") {
		t.Fatalf("expected deploy help output, got: %q", out.String())
	}
}

func TestRunProvisionHelpShowsFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"provision", "--help"}, newNoopDeps(&out, &errOut))
	if code != 0 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(out.String(), "--project") || !strings.Contains(out.String(), "--compose-file") {
		t.Fatalf("expected provision help output, got: %q", out.String())
	}
}

func TestRunManifestSyncIDsHelpShowsFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"manifest", "sync-ids", "--help"}, newNoopDeps(&out, &errOut))
	if code != 0 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(out.String(), "--artifact") || !strings.Contains(out.String(), "--check") {
		t.Fatalf("expected manifest sync-ids help output, got: %q", out.String())
	}
}

func TestRunRequiresSubcommand(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run(nil, newNoopDeps(&out, &errOut))
	if code != 1 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(errOut.String(), "artifactctl provision --help") {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}

func TestRunRejectsLegacySubcommands(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"prepare-images", "--artifact", "artifact.yml"}, newNoopDeps(&out, &errOut))
	if code != 1 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(errOut.String(), "deploy") {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}

func TestRunDeployRequiresMandatoryFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"deploy", "--artifact", "artifact.yml"}, newNoopDeps(&out, &errOut))
	if code != 1 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(errOut.String(), "--out") {
		t.Fatalf("expected missing --out parse error, got: %q", errOut.String())
	}
}

func TestRunDeployDispatchesCanonicalDeployInput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	var warnings bytes.Buffer

	var got deployops.Input

	deps := commandDeps{
		executeDeploy: func(input deployops.Input) (artifactcore.ApplyResult, error) {
			got = input
			return artifactcore.ApplyResult{Warnings: []string{"minor mismatch"}}, nil
		},
		warningWriter: &warnings,
		out:           &out,
		errOut:        &errOut,
	}

	code := run([]string{
		"deploy",
		"--artifact", "artifact.yml",
		"--out", "out",
		"--secret-env", "secret.env",
		"--no-cache",
	}, deps)
	if code != 0 {
		t.Fatalf("run returned code=%d, stderr=%q", code, errOut.String())
	}
	if got.ArtifactPath != "artifact.yml" || !got.NoCache {
		t.Fatalf("unexpected deploy input: %#v", got)
	}
	if got.OutputDir != "out" || got.SecretEnvPath != "secret.env" {
		t.Fatalf("unexpected apply input: %#v", got)
	}
	if !strings.Contains(warnings.String(), "Warning: minor mismatch") {
		t.Fatalf("expected warning output, got %q", warnings.String())
	}
}

func TestRunDeployUsesErrOutWhenWarningWriterUnset(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	deps := commandDeps{
		executeDeploy: func(input deployops.Input) (artifactcore.ApplyResult, error) {
			return artifactcore.ApplyResult{Warnings: []string{"runtime mismatch"}}, nil
		},
		out:    &out,
		errOut: &errOut,
	}

	code := run([]string{"deploy", "--artifact", "artifact.yml", "--out", "out"}, deps)
	if code != 0 {
		t.Fatalf("run returned code=%d, stderr=%q", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "Warning: runtime mismatch") {
		t.Fatalf("expected warning in stderr, got %q", errOut.String())
	}
}

func TestRunDeployReportsExecuteFailure(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	deps := commandDeps{
		executeDeploy: func(deployops.Input) (artifactcore.ApplyResult, error) {
			return artifactcore.ApplyResult{}, errors.New("boom-deploy")
		},
		out:    &out,
		errOut: &errOut,
	}

	code := run([]string{"deploy", "--artifact", "artifact.yml", "--out", "out"}, deps)
	if code != 1 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(errOut.String(), "deploy failed: boom-deploy") {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}

func TestRunManifestSyncIDsDispatchesWriteMode(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	gotPath := ""
	gotWrite := false

	deps := commandDeps{
		syncManifestIDs: func(path string, write bool) (int, error) {
			gotPath = path
			gotWrite = write
			return 2, nil
		},
		out:    &out,
		errOut: &errOut,
	}

	code := run([]string{"manifest", "sync-ids", "--artifact", "artifact.yml"}, deps)
	if code != 0 {
		t.Fatalf("run returned code=%d, stderr=%q", code, errOut.String())
	}
	if gotPath != "artifact.yml" || !gotWrite {
		t.Fatalf("unexpected sync args: path=%q write=%v", gotPath, gotWrite)
	}
	if !strings.Contains(out.String(), "updated=2") {
		t.Fatalf("expected sync summary, got %q", out.String())
	}
}

func TestRunManifestSyncIDsCheckFailsWhenDifferenceExists(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	deps := commandDeps{
		syncManifestIDs: func(path string, write bool) (int, error) {
			if write {
				t.Fatal("check mode must not write")
			}
			if path != "artifact.yml" {
				t.Fatalf("unexpected path: %q", path)
			}
			return 1, nil
		},
		out:    &out,
		errOut: &errOut,
	}

	code := run([]string{"manifest", "sync-ids", "--artifact", "artifact.yml", "--check"}, deps)
	if code != 1 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(errOut.String(), "requires id sync") {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}

func TestRunProvisionDispatchesCanonicalInput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	var got ProvisionInput

	deps := commandDeps{
		executeDeploy: func(deployops.Input) (artifactcore.ApplyResult, error) {
			return artifactcore.ApplyResult{}, nil
		},
		executeProvision: func(input ProvisionInput) error {
			got = input
			return nil
		},
		out:    &out,
		errOut: &errOut,
	}

	code := run([]string{
		"provision",
		"--project", "esb-dev",
		"--compose-file", "compose.yml",
		"--env-file", ".env",
		"--project-dir", "/tmp/esb",
		"--with-deps",
		"-v",
	}, deps)
	if code != 0 {
		t.Fatalf("run returned code=%d, stderr=%q", code, errOut.String())
	}
	if got.ComposeProject != "esb-dev" {
		t.Fatalf("unexpected compose project: %#v", got)
	}
	if len(got.ComposeFiles) != 1 || got.ComposeFiles[0] != "compose.yml" {
		t.Fatalf("unexpected compose files: %#v", got)
	}
	if got.EnvFile != ".env" || got.ProjectDir != "/tmp/esb" {
		t.Fatalf("unexpected provision input: %#v", got)
	}
	if got.NoDeps {
		t.Fatalf("expected NoDeps=false when --with-deps is set: %#v", got)
	}
	if !got.Verbose {
		t.Fatalf("expected verbose=true: %#v", got)
	}
}

func TestRunProvisionReportsExecuteFailure(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	deps := commandDeps{
		executeDeploy: func(deployops.Input) (artifactcore.ApplyResult, error) {
			return artifactcore.ApplyResult{}, nil
		},
		executeProvision: func(ProvisionInput) error { return errors.New("boom-provision") },
		out:              &out,
		errOut:           &errOut,
	}

	code := run([]string{
		"provision",
		"--project", "esb-dev",
		"--compose-file", "compose.yml",
	}, deps)
	if code != 1 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(errOut.String(), "boom-provision") {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "artifactctl provision --help") {
		t.Fatalf("expected provision hint, got: %q", errOut.String())
	}
}

func TestRunProvisionPreservesSharedRunErrorClass(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	deps := commandDeps{
		executeDeploy: func(deployops.Input) (artifactcore.ApplyResult, error) {
			return artifactcore.ApplyResult{}, nil
		},
		executeProvision: func(ProvisionInput) error {
			return errors.New("run provisioner: compose failed")
		},
		out:    &out,
		errOut: &errOut,
	}

	code := run([]string{
		"provision",
		"--project", "esb-dev",
		"--compose-file", "compose.yml",
	}, deps)
	if code != 1 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(errOut.String(), "run provisioner: compose failed") {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}

func TestHintForDeployError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "secret env required",
			err:  artifactcore.ErrSecretEnvFileRequired,
			want: "set `--secret-env <path>` with all required secret keys listed in artifact.yml.",
		},
		{
			name: "missing secrets",
			err:  artifactcore.MissingSecretKeysError{Keys: []string{"B", "A"}},
			want: "set `--secret-env <path>` with all required secret keys listed in artifact.yml.",
		},
		{
			name: "not found",
			err:  artifactcore.MissingReferencedPathError{Path: "/tmp/artifact.yml"},
			want: "confirm `--artifact` and referenced files exist and are readable.",
		},
		{
			name: "wrapped not found",
			err:  fmt.Errorf("deploy failed during artifact apply: %w", artifactcore.MissingReferencedPathError{Path: "/tmp/a.yml"}),
			want: "confirm `--artifact` and referenced files exist and are readable.",
		},
		{
			name: "fallback",
			err:  errors.New("other"),
			want: "run `artifactctl deploy --help` for required arguments.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hintForDeployError(tc.err)
			if got != tc.want {
				t.Fatalf("hint=%q want=%q", got, tc.want)
			}
		})
	}
}
