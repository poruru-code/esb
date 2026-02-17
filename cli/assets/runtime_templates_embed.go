// Where: cli/assets/runtime_templates_embed.go
// What: Embed runtime Dockerfile templates for the CLI renderer.
// Why: Keep template assets owned by CLI after artifact-first responsibility split.
package assets

import "embed"

//go:embed runtime-templates/python/templates/*.tmpl runtime-templates/java/templates/*.tmpl
var RuntimeTemplatesFS embed.FS
