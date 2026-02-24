package artifactcore

import "testing"

func TestNormalizeApplyInputNormalizesPaths(t *testing.T) {
	req, err := normalizeApplyInput(ApplyInput{
		ArtifactPath: "  artifact.yml  ",
		OutputDir:    "  out/config  ",
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
}
