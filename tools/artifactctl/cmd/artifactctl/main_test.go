package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

func TestRunRequiresCommand(t *testing.T) {
	err := run(nil, commandDeps{})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != usageMessage {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run([]string{"unknown"}, commandDeps{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown command: unknown") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunValidateIDRequiresArtifact(t *testing.T) {
	err := run([]string{"validate-id"}, commandDeps{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--artifact is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunMergeRequiresFlags(t *testing.T) {
	err := run([]string{"merge"}, commandDeps{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--artifact is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	err = run([]string{"merge", "--artifact", "artifact.yml"}, commandDeps{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--out is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunApplyRequiresFlags(t *testing.T) {
	err := run([]string{"apply"}, commandDeps{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--artifact is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	err = run([]string{"apply", "--artifact", "artifact.yml"}, commandDeps{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--out is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunPrepareImagesRequiresArtifact(t *testing.T) {
	err := run([]string{"prepare-images"}, commandDeps{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--artifact is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunReturnsFlagParseErrors(t *testing.T) {
	err := run([]string{"validate-id", "--unknown-flag"}, commandDeps{})
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunSubcommandHelpPrintsUsageAndReturnsNil(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "validate-id", args: []string{"validate-id", "--help"}},
		{name: "merge", args: []string{"merge", "--help"}},
		{name: "prepare-images", args: []string{"prepare-images", "--help"}},
		{name: "apply", args: []string{"apply", "--help"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			deps := commandDeps{
				helpWriter: &out,
				validateIDs: func(string) error {
					t.Fatal("validateIDs must not be called on --help")
					return nil
				},
				mergeConfig: func(artifactcore.MergeRequest) error {
					t.Fatal("mergeConfig must not be called on --help")
					return nil
				},
				prepareImages: func(artifactcore.PrepareImagesRequest) error {
					t.Fatal("prepareImages must not be called on --help")
					return nil
				},
				apply: func(artifactcore.ApplyRequest) error {
					t.Fatal("apply must not be called on --help")
					return nil
				},
			}

			err := run(tc.args, deps)
			if err != nil {
				t.Fatalf("expected nil error for --help, got: %v", err)
			}
			if !strings.Contains(out.String(), "Usage of "+tc.name+":") {
				t.Fatalf("expected usage output for %s, got: %q", tc.name, out.String())
			}
		})
	}
}

func TestRunDispatchesValidateID(t *testing.T) {
	called := false
	deps := commandDeps{
		validateIDs: func(path string) error {
			called = true
			if path != "artifact.yml" {
				t.Fatalf("validate path = %q", path)
			}
			return nil
		},
	}
	if err := run([]string{"validate-id", "--artifact", "artifact.yml"}, deps); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !called {
		t.Fatal("expected validateIDs call")
	}
}

func TestRunDispatchesMerge(t *testing.T) {
	called := false
	deps := commandDeps{
		mergeConfig: func(req artifactcore.MergeRequest) error {
			called = true
			if req.ArtifactPath != "artifact.yml" || req.OutputDir != "out" {
				t.Fatalf("unexpected merge request: %#v", req)
			}
			return nil
		},
	}
	if err := run([]string{"merge", "--artifact", "artifact.yml", "--out", "out"}, deps); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !called {
		t.Fatal("expected mergeConfig call")
	}
}

func TestRunDispatchesPrepareImages(t *testing.T) {
	called := false
	deps := commandDeps{
		prepareImages: func(req artifactcore.PrepareImagesRequest) error {
			called = true
			if req.ArtifactPath != "artifact.yml" || !req.NoCache {
				t.Fatalf("unexpected prepare request: %#v", req)
			}
			return nil
		},
	}
	if err := run([]string{"prepare-images", "--artifact", "artifact.yml", "--no-cache"}, deps); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !called {
		t.Fatal("expected prepareImages call")
	}
}

func TestRunDispatchesApply(t *testing.T) {
	var warnings bytes.Buffer
	called := false
	deps := commandDeps{
		warningWriter: &warnings,
		apply: func(req artifactcore.ApplyRequest) error {
			called = true
			if req.ArtifactPath != "artifact.yml" || req.OutputDir != "out" || req.SecretEnvPath != "secret.env" || !req.Strict {
				t.Fatalf("unexpected apply request: %#v", req)
			}
			if req.WarningWriter != &warnings {
				t.Fatalf("unexpected warning writer: %#v", req.WarningWriter)
			}
			return nil
		},
	}
	if err := run([]string{"apply", "--artifact", "artifact.yml", "--out", "out", "--secret-env", "secret.env", "--strict"}, deps); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !called {
		t.Fatal("expected apply call")
	}
}

func TestRunApplyUsesStderrWhenWarningWriterIsUnset(t *testing.T) {
	called := false
	deps := commandDeps{
		apply: func(req artifactcore.ApplyRequest) error {
			called = true
			if req.WarningWriter != os.Stderr {
				t.Fatalf("unexpected default warning writer: %#v", req.WarningWriter)
			}
			return nil
		},
	}
	if err := run([]string{"apply", "--artifact", "artifact.yml", "--out", "out"}, deps); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !called {
		t.Fatal("expected apply call")
	}
}

func TestRunPropagatesBackendErrors(t *testing.T) {
	validateErr := errors.New("boom-validate")
	err := run([]string{"validate-id", "--artifact", "artifact.yml"}, commandDeps{
		validateIDs: func(string) error { return validateErr },
	})
	if err == nil || !strings.Contains(err.Error(), "validate-id failed: boom-validate") {
		t.Fatalf("unexpected error: %v", err)
	}

	mergeErr := errors.New("boom-merge")
	err = run([]string{"merge", "--artifact", "artifact.yml", "--out", "out"}, commandDeps{
		mergeConfig: func(artifactcore.MergeRequest) error { return mergeErr },
	})
	if err == nil || !strings.Contains(err.Error(), "merge failed: boom-merge") {
		t.Fatalf("unexpected error: %v", err)
	}

	prepareErr := errors.New("boom-prepare")
	err = run([]string{"prepare-images", "--artifact", "artifact.yml"}, commandDeps{
		prepareImages: func(artifactcore.PrepareImagesRequest) error { return prepareErr },
	})
	if err == nil || !strings.Contains(err.Error(), "prepare-images failed: boom-prepare") {
		t.Fatalf("unexpected error: %v", err)
	}

	applyErr := errors.New("boom-apply")
	err = run([]string{"apply", "--artifact", "artifact.yml", "--out", "out"}, commandDeps{
		apply: func(artifactcore.ApplyRequest) error { return applyErr },
	})
	if err == nil || !strings.Contains(err.Error(), "apply failed: boom-apply") {
		t.Fatalf("unexpected error: %v", err)
	}
}
