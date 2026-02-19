package artifactcore

import (
	"bytes"
	"testing"
)

func TestNewApplyRequestNormalizesPaths(t *testing.T) {
	var warning bytes.Buffer
	req := NewApplyRequest("  artifact.yml  ", "  out/config  ", "  secret.env  ", true, &warning)
	if req.ArtifactPath != "artifact.yml" {
		t.Fatalf("ArtifactPath = %q, want artifact.yml", req.ArtifactPath)
	}
	if req.OutputDir != "out/config" {
		t.Fatalf("OutputDir = %q, want out/config", req.OutputDir)
	}
	if req.SecretEnvPath != "secret.env" {
		t.Fatalf("SecretEnvPath = %q, want secret.env", req.SecretEnvPath)
	}
	if !req.Strict {
		t.Fatal("Strict must be true")
	}
	if req.WarningWriter != &warning {
		t.Fatalf("WarningWriter mismatch: %#v", req.WarningWriter)
	}
}
