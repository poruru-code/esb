package engine

import (
	"fmt"
	"os"
	"path/filepath"
)

type MergeRequest struct {
	ArtifactPath string
	OutputDir    string
}

func MergeRuntimeConfig(req MergeRequest) error {
	manifest, err := ReadArtifactManifest(req.ArtifactPath)
	if err != nil {
		return err
	}
	return mergeWithManifest(req.ArtifactPath, req.OutputDir, manifest)
}

func mergeWithManifest(manifestPath, outputDir string, manifest ArtifactManifest) error {
	if outputDir == "" {
		return fmt.Errorf("output dir is required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	for i := range manifest.Artifacts {
		runtimeDir, err := manifest.ResolveRuntimeConfigDir(manifestPath, i)
		if err != nil {
			return err
		}
		if err := mergeOneRuntimeConfig(runtimeDir, outputDir); err != nil {
			return fmt.Errorf("merge artifacts[%d]: %w", i, err)
		}
	}
	return nil
}

func mergeOneRuntimeConfig(srcDir, destDir string) error {
	if err := requireFile(filepath.Join(srcDir, "functions.yml")); err != nil {
		return err
	}
	if err := requireFile(filepath.Join(srcDir, "routing.yml")); err != nil {
		return err
	}
	if err := mergeFunctionsYML(srcDir, destDir, true); err != nil {
		return err
	}
	if err := mergeRoutingYML(srcDir, destDir, true); err != nil {
		return err
	}
	if err := mergeResourcesYML(srcDir, destDir); err != nil {
		return err
	}
	if err := mergeImageImportManifest(srcDir, destDir); err != nil {
		return err
	}
	return nil
}

func requireFile(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("required file not found: %s", path)
		}
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("required file is a directory: %s", path)
	}
	return nil
}
