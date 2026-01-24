// Where: cli/internal/generator/build_types.go
// What: Shared build types.
// Why: Break dependency cycle between app and generator.
package generator

// BuildRequest contains parameters for a build operation.
// It specifies the project location, SAM template, environment, and cache options.
type BuildRequest struct {
	ProjectDir   string
	TemplatePath string
	Env          string
	NoCache      bool
	Verbose      bool
}
