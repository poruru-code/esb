// Where: cli/internal/infra/templategen/bundle_manifest_types.go
// What: Data contracts for bundle manifest generation.
// Why: Keep manifest schema definitions separate from write and image collection flow.
package templategen

import (
	"github.com/poruru-code/esb/cli/internal/domain/template"
	"github.com/poruru-code/esb/cli/internal/infra/compose"
)

const bundleManifestSchemaVersion = "1.1"

type bundleManifest struct {
	SchemaVersion string                `json:"schema_version"`
	GeneratedAt   string                `json:"generated_at"`
	Template      bundleTemplate        `json:"template,omitempty"`
	Templates     []bundleTemplate      `json:"templates"`
	Build         bundleBuild           `json:"build"`
	Images        []bundleManifestImage `json:"images"`
}

type bundleTemplate struct {
	Path       string            `json:"path"`
	Sha256     string            `json:"sha256"`
	Parameters map[string]string `json:"parameters"`
}

type bundleBuild struct {
	Project     string         `json:"project"`
	Env         string         `json:"env"`
	Mode        string         `json:"mode"`
	ImagePrefix string         `json:"image_prefix"`
	ImageTag    string         `json:"image_tag"`
	Git         bundleBuildGit `json:"git"`
}

type bundleBuildGit struct {
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}

type bundleManifestImage struct {
	Name     string            `json:"name"`
	Digest   string            `json:"digest"`
	Kind     string            `json:"kind"`
	Source   string            `json:"source"`
	Labels   map[string]string `json:"labels,omitempty"`
	Platform string            `json:"platform"`
}

// BundleManifestInput captures bundle manifest generation inputs.
type BundleManifestInput struct {
	RepoRoot        string
	OutputDir       string
	TemplatePath    string
	Parameters      map[string]any
	Project         string
	Env             string
	Mode            string
	ImageTag        string
	Registry        string
	ServiceRegistry string
	Functions       []template.FunctionSpec
	Runner          compose.CommandRunner
}
