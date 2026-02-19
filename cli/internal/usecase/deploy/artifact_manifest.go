// Where: cli/internal/usecase/deploy/artifact_manifest.go
// What: Artifact manifest aliases delegated to shared artifact core.
// Why: Keep Go logic single-sourced and decoupled from artifactctl CLI adapter.
package deploy

import (
	"io"

	engine "github.com/poruru/edge-serverless-box/pkg/artifactcore"
)

const (
	ArtifactSchemaVersionV1    = engine.ArtifactSchemaVersionV1
	RuntimeHooksAPIVersion     = engine.RuntimeHooksAPIVersion
	TemplateRendererName       = engine.TemplateRendererName
	TemplateRendererAPIVersion = engine.TemplateRendererAPIVersion
)

type (
	ArtifactManifest       = engine.ArtifactManifest
	ArtifactEntry          = engine.ArtifactEntry
	ArtifactSourceTemplate = engine.ArtifactSourceTemplate
	ArtifactGenerator      = engine.ArtifactGenerator
	ArtifactRuntimeMeta    = engine.ArtifactRuntimeMeta
	RuntimeHooksMeta       = engine.RuntimeHooksMeta
	RendererMeta           = engine.RendererMeta
)

type ArtifactApplyRequest struct {
	ArtifactPath  string
	OutputDir     string
	SecretEnvPath string
	Strict        bool
	WarningWriter io.Writer
}

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
	return engine.Apply(toEngineApplyRequest(req))
}

func toEngineApplyRequest(req ArtifactApplyRequest) engine.ApplyRequest {
	return engine.NewApplyRequest(
		req.ArtifactPath,
		req.OutputDir,
		req.SecretEnvPath,
		req.Strict,
		req.WarningWriter,
	)
}
