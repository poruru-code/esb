// Where: cli/internal/meta/meta.go
// What: CLI-local metadata constants.
// Why: Decouple CLI from the shared top-level meta module.
package meta

const (
	// Project Identity
	AppName     = "esb"
	Slug        = "esb"
	EnvPrefix   = "ESB"
	EnvVarEnv   = "ENV"
	ImagePrefix = "esb"
	LabelPrefix = "com.esb"

	// Directory Layout
	HomeDir    = ".esb"
	OutputDir  = ".esb"
	StagingDir = ".staging"

	// Certificate Constants
	RootCAMountID      = "esb_root_ca"
	RootCACertFilename = "rootCA.crt"
	RootCACertPath     = "/usr/local/share/ca-certificates/rootCA.crt"
)
