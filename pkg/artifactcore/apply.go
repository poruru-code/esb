package artifactcore

func applyWithWarnings(req ApplyInput) ([]string, error) {
	manifest, err := ReadArtifactManifest(req.ArtifactPath)
	if err != nil {
		return nil, err
	}
	if err := mergeWithManifest(req.ArtifactPath, req.OutputDir, manifest); err != nil {
		return nil, err
	}
	return nil, nil
}
