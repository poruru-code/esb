// Where: runtime/templates_embed.go
// What: Embed runtime Dockerfile templates for the CLI renderer.
// Why: Keep runtime templates colocated under runtime/ while still embedding in the CLI binary.
package runtimeassets

import "embed"

//go:embed python/templates/*.tmpl java/templates/*.tmpl
var TemplatesFS embed.FS
