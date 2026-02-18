// Where: cli/internal/domain/template/types.go
// What: Template-derived domain types.
// Why: Keep parsed template data independent from infra implementations.
package template

import "github.com/poruru/edge-serverless-box/cli/internal/domain/manifest"

// ParseResult contains the parsed template outputs.
type ParseResult struct {
	Functions []FunctionSpec
	Resources manifest.ResourcesSpec
	Warnings  []string
}

// FunctionSpec captures resolved function metadata.
type FunctionSpec struct {
	LogicalID               string
	Name                    string
	ImageRef                string
	ImageSource             string
	ImageName               string
	CodeURI                 string
	AppCodeJarPath          string
	Handler                 string
	Runtime                 string
	Timeout                 int
	MemorySize              int
	HasRequirements         bool
	Environment             map[string]string
	Events                  []EventSpec
	Scaling                 ScalingSpec
	Layers                  []manifest.LayerSpec
	Architectures           []string
	RuntimeManagementConfig RuntimeManagementConfig
}

// EventSpec captures supported event configurations.
type EventSpec struct {
	Type               string
	Path               string
	Method             string
	ScheduleExpression string
	Input              string
}

// ScalingSpec captures scaling configuration.
type ScalingSpec struct {
	MaxCapacity *int
	MinCapacity *int
}

// RuntimeManagementConfig captures runtime update policies.
type RuntimeManagementConfig struct {
	UpdateRuntimeOn string
}

// DockerConfig captures Dockerfile rendering settings.
type DockerConfig struct {
	SitecustomizeSource string
}
