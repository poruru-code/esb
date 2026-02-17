// Where: cli/internal/usecase/deploy/artifact_manifest.go
// What: Artifact manifest aliases delegated to artifactctl engine.
// Why: Keep Go logic single-sourced in tools/artifactctl.
package deploy

import engine "github.com/poruru/edge-serverless-box/tools/artifactctl/pkg/engine"

const ArtifactSchemaVersionV1 = engine.ArtifactSchemaVersionV1

type (
	ArtifactManifest       = engine.ArtifactManifest
	ArtifactEntry          = engine.ArtifactEntry
	ArtifactSourceTemplate = engine.ArtifactSourceTemplate
	ArtifactGenerator      = engine.ArtifactGenerator
	ArtifactRuntimeMeta    = engine.ArtifactRuntimeMeta
	RuntimeHooksMeta       = engine.RuntimeHooksMeta
	RendererMeta           = engine.RendererMeta
	ArtifactApplyRequest   = engine.ApplyRequest
)

func ReadArtifactManifest(path string) (ArtifactManifest, error) {
	return engine.ReadArtifactManifest(path)
}

func WriteArtifactManifest(path string, manifest ArtifactManifest) error {
	return engine.WriteArtifactManifest(path, manifest)
}

func ComputeArtifactID(templatePath string, parameters map[string]string, sourceSHA256 string) string {
	return engine.ComputeArtifactID(templatePath, parameters, sourceSHA256)
}

func ApplyArtifact(req ArtifactApplyRequest) error {
	return engine.Apply(req)
}
