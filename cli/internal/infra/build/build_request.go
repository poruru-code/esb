// Where: cli/internal/infra/build/build_request.go
// What: Build request parameters for generator build operations.
// Why: Keep generator inputs colocated with generator implementation.
package build

// BuildRequest contains parameters for a build operation.
type BuildRequest struct {
	ProjectDir   string
	ProjectName  string
	TemplatePath string
	Env          string
	Mode         string
	OutputDir    string
	Parameters   map[string]string
	Tag          string
	NoCache      bool
	Verbose      bool
	Bundle       bool
	Emoji        bool
}
