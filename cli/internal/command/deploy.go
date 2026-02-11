// Where: cli/internal/command/deploy.go
// What: Shared deploy command models and validation errors.
// Why: Keep cross-file deploy internals centralized after file split.
package command

import "errors"

type deployInputs struct {
	ProjectDir    string
	TargetStack   string
	Env           string
	EnvSource     string
	Mode          string
	Templates     []deployTemplateInput
	Project       string
	ProjectSource string
	ComposeFiles  []string
}

type deployTemplateInput struct {
	TemplatePath string
	OutputDir    string
	Parameters   map[string]string
}

type deployTargetStack struct {
	Name    string
	Project string
	Env     string
}

type envChoice struct {
	Value    string
	Source   string
	Explicit bool
}

// storedDeployDefaults mirrors storedBuildDefaults for deploy.
type storedDeployDefaults struct {
	Env       string
	Mode      string
	OutputDir string
	Params    map[string]string
}

// samParameter represents a SAM template parameter definition.
type samParameter struct {
	Type        string
	Description string
	Default     any
}

const (
	templateHistoryLimit = 10
	templateManualOption = "Enter path..."
)

var (
	errDeployBuilderNotConfigured = errors.New("deploy: builder not configured")
	errTemplatePathRequired       = errors.New("template path is required")
	errEnvironmentRequired        = errors.New("environment is required")
	errModeRequired               = errors.New("mode is required")
	errMultipleRunningProjects    = errors.New("multiple running projects found (use --project)")
	errComposeProjectRequired     = errors.New("compose project is required")
	errTemplatePathEmpty          = errors.New("template path is empty")
	errTemplateNotFound           = errors.New("no template.yaml or template.yml found in directory")
	errParameterRequiresValue     = errors.New("parameter requires a value")
	errMultipleTemplateOutput     = errors.New("output directory cannot be used with multiple templates")
)
