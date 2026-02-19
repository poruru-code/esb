package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

func newNoopDeps(out, errOut *bytes.Buffer) commandDeps {
	return commandDeps{
		prepareImages: func(artifactcore.PrepareImagesRequest) error { return nil },
		apply:         func(artifactcore.ApplyRequest) error { return nil },
		out:           out,
		errOut:        errOut,
	}
}

func TestRunShowsHelp(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--help"}, newNoopDeps(&out, &errOut))
	if code != 0 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(out.String(), "deploy") {
		t.Fatalf("expected help output to mention deploy, got: %q", out.String())
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

func TestRunRequiresDeployCommand(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run(nil, newNoopDeps(&out, &errOut))
	if code != 1 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(errOut.String(), "Hint: run `artifactctl --help` or `artifactctl deploy --help`.") {
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

func TestRunDeployDispatchesPrepareThenApply(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	var warnings bytes.Buffer

	order := make([]string, 0, 2)
	var gotPrepare artifactcore.PrepareImagesRequest
	var gotApply artifactcore.ApplyRequest

	deps := commandDeps{
		prepareImages: func(req artifactcore.PrepareImagesRequest) error {
			order = append(order, "prepare")
			gotPrepare = req
			return nil
		},
		apply: func(req artifactcore.ApplyRequest) error {
			order = append(order, "apply")
			gotApply = req
			return nil
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
		"--strict",
		"--no-cache",
	}, deps)
	if code != 0 {
		t.Fatalf("run returned code=%d, stderr=%q", code, errOut.String())
	}
	if len(order) != 2 || order[0] != "prepare" || order[1] != "apply" {
		t.Fatalf("unexpected execution order: %#v", order)
	}
	if gotPrepare.ArtifactPath != "artifact.yml" || !gotPrepare.NoCache {
		t.Fatalf("unexpected prepare request: %#v", gotPrepare)
	}
	if gotApply.ArtifactPath != "artifact.yml" || gotApply.OutputDir != "out" || gotApply.SecretEnvPath != "secret.env" || !gotApply.Strict {
		t.Fatalf("unexpected apply request: %#v", gotApply)
	}
	if gotApply.WarningWriter != &warnings {
		t.Fatalf("unexpected warning writer: %#v", gotApply.WarningWriter)
	}
}

func TestRunDeployUsesErrOutWhenWarningWriterUnset(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	deps := commandDeps{
		prepareImages: func(artifactcore.PrepareImagesRequest) error { return nil },
		apply: func(req artifactcore.ApplyRequest) error {
			if req.WarningWriter != &errOut {
				t.Fatalf("unexpected warning writer: %#v", req.WarningWriter)
			}
			return nil
		},
		out:    &out,
		errOut: &errOut,
	}

	code := run([]string{"deploy", "--artifact", "artifact.yml", "--out", "out"}, deps)
	if code != 0 {
		t.Fatalf("run returned code=%d, stderr=%q", code, errOut.String())
	}
}

func TestRunDeployStopsWhenPrepareFails(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	applyCalled := false

	deps := commandDeps{
		prepareImages: func(artifactcore.PrepareImagesRequest) error { return errors.New("boom-prepare") },
		apply: func(artifactcore.ApplyRequest) error {
			applyCalled = true
			return nil
		},
		out:    &out,
		errOut: &errOut,
	}

	code := run([]string{"deploy", "--artifact", "artifact.yml", "--out", "out"}, deps)
	if code != 1 {
		t.Fatalf("run returned code=%d", code)
	}
	if applyCalled {
		t.Fatal("apply must not be called when prepare fails")
	}
	if !strings.Contains(errOut.String(), "deploy failed during image preparation: boom-prepare") {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}

func TestRunDeployReportsApplyFailure(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	deps := commandDeps{
		prepareImages: func(artifactcore.PrepareImagesRequest) error { return nil },
		apply:         func(artifactcore.ApplyRequest) error { return errors.New("boom-apply") },
		out:           &out,
		errOut:        &errOut,
	}

	code := run([]string{"deploy", "--artifact", "artifact.yml", "--out", "out"}, deps)
	if code != 1 {
		t.Fatalf("run returned code=%d", code)
	}
	if !strings.Contains(errOut.String(), "deploy failed during artifact apply: boom-apply") {
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
			name: "runtime base missing",
			err:  errors.New("runtime base dockerfile not found: /tmp/Dockerfile"),
			want: "run `esb artifact generate ...`",
		},
		{
			name: "missing secrets",
			err:  errors.New("missing required secret env keys: A, B"),
			want: "--secret-env",
		},
		{
			name: "not found",
			err:  errors.New("open /tmp/artifact.yml: no such file or directory"),
			want: "confirm `--artifact`",
		},
		{
			name: "fallback",
			err:  errors.New("other"),
			want: "artifactctl deploy --help",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hintForDeployError(tc.err)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("hint=%q want substring=%q", got, tc.want)
			}
		})
	}
}
