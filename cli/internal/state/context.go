// Where: cli/internal/state/context.go
// What: Minimal context for runtime environment defaults.
// Why: Provide the builder with the fields needed to apply branding and staging env vars.
package state

// Context captures the workspace/runtime metadata needed by runtime env helpers.
type Context struct {
	ProjectDir     string
	TemplatePath   string
	OutputDir      string
	Env            string
	Mode           string
	ComposeProject string
}
