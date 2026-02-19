package artifactcore

import "testing"

func TestNormalizeApplyInputNormalizesPaths(t *testing.T) {
	req, err := normalizeApplyInput(ApplyInput{
		ArtifactPath:  "  artifact.yml  ",
		OutputDir:     "  out/config  ",
		SecretEnvPath: "  secret.env  ",
		Strict:        true,
	})
	if err != nil {
		t.Fatalf("normalizeApplyInput() error = %v", err)
	}
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
}
