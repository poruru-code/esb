// Where: cli/internal/generator/build_request.go
// What: Build request parameters for generator build operations.
// Why: Keep generator inputs colocated with generator implementation.
package generator

// BuildRequest contains parameters for a build operation.
type BuildRequest struct {
	ProjectDir   string
	ProjectName  string
	TemplatePath string
	Env          string
	NoCache      bool
	Verbose      bool
}
