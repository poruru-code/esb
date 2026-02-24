package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/poruru-code/esb/pkg/artifactcore"
	"github.com/poruru-code/esb/pkg/deployops"
)

func newNoopDeps(out, errOut *bytes.Buffer) commandDeps {
	return commandDeps{
		executeDeploy: func(deployops.Input) (artifactcore.ApplyResult, error) {
			return artifactcore.ApplyResult{}, nil
		},
		executeProvision: func(ProvisionInput) error { return nil },
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

func TestRunInternalMavenShimEnsureOutputsJSON(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	var got MavenShimEnsureInput
	deps := commandDeps{
		executeDeploy: func(deployops.Input) (artifactcore.ApplyResult, error) {
			return artifactcore.ApplyResult{}, nil
		},
		executeProvision: func(ProvisionInput) error { return nil },
		ensureMavenShim: func(input MavenShimEnsureInput) (MavenShimEnsureResult, error) {
			got = input
			return MavenShimEnsureResult{
				SchemaVersion: 1,
				ShimImage:     "127.0.0.1:5010/esb-maven-shim:deadbeefdeadbeef",
			}, nil
		},
		out:    &out,
		errOut: &errOut,
	}

	code := run([]string{
		"internal",
		"maven-shim",
		"ensure",
		"--base-image", "public.ecr.aws/sam/build-java21@sha256:example",
		"--host-registry", "127.0.0.1:5010",
		"--output", "json",
	}, deps)
	if code != 0 {
		t.Fatalf("run returned code=%d, stderr=%q", code, errOut.String())
	}
	if got.BaseImage != "public.ecr.aws/sam/build-java21@sha256:example" {
		t.Fatalf("unexpected base image: %#v", got)
	}
	if got.HostRegistry != "127.0.0.1:5010" {
		t.Fatalf("unexpected host registry: %#v", got)
	}

	var payload MavenShimEnsureResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not JSON: %v, raw=%q", err, out.String())
	}
	if payload.SchemaVersion != 1 || payload.ShimImage == "" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestRunInternalMavenShimEnsureReportsFailure(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	deps := commandDeps{
		executeDeploy: func(deployops.Input) (artifactcore.ApplyResult, error) {
			return artifactcore.ApplyResult{}, nil
		},
		executeProvision: func(ProvisionInput) error { return nil },
		ensureMavenShim: func(MavenShimEnsureInput) (MavenShimEnsureResult, error) {
			return MavenShimEnsureResult{}, errors.New("boom-shim")
		},
		out:    &out,
		errOut: &errOut,
	}

	code := run([]string{
		"internal",
		"maven-shim",
		"ensure",
		"--base-image", "maven:3.9.11-eclipse-temurin-21",
	}, deps)
	if code != 1 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(errOut.String(), "maven shim ensure failed: boom-shim") {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "artifactctl internal maven-shim ensure --help") {
		t.Fatalf("expected internal command hint, got: %q", errOut.String())
	}
}

func TestRunInternalMavenShimEnsureKeepsStdoutMachineReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script setup for fake docker is not portable to windows")
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	deps := newNoopDeps(&out, &errOut)

	fakeBinDir := t.TempDir()
	fakeDockerPath := filepath.Join(fakeBinDir, "docker")
	script := strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"echo \"fake-docker-stdout:$*\"",
		"echo \"fake-docker-stderr:$*\" >&2",
		"exit 0",
		"",
	}, "\n")
	if err := os.WriteFile(fakeDockerPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+originalPath)

	code := run([]string{
		"internal",
		"maven-shim",
		"ensure",
		"--base-image", "maven:3.9.11-eclipse-temurin-21",
		"--no-cache",
		"--output", "json",
	}, deps)
	if code != 0 {
		t.Fatalf("run returned code=%d, stderr=%q", code, errOut.String())
	}

	if strings.Contains(out.String(), "fake-docker-stdout:") || strings.Contains(out.String(), "fake-docker-stderr:") {
		t.Fatalf("stdout contains docker noise: %q", out.String())
	}
	var payload MavenShimEnsureResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not pure JSON payload: %v, raw=%q", err, out.String())
	}
	if payload.SchemaVersion != 1 || payload.ShimImage == "" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if !strings.Contains(errOut.String(), "fake-docker-stdout:") {
		t.Fatalf("expected docker stdout to be redirected to stderr, got: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "fake-docker-stderr:") {
		t.Fatalf("expected docker stderr to be redirected to stderr, got: %q", errOut.String())
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
