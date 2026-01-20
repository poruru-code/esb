// Where: cli/internal/workflows/env.go
// What: Environment workflows for list/add/use/remove.
// Why: Move env business logic out of the CLI adapter.
package workflows

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
)

// EnvInfo represents a single environment entry with status metadata.
type EnvInfo struct {
	Name   string
	Mode   string
	Status string
	Active bool
}

// EnvListRequest captures inputs for listing environments.
type EnvListRequest struct {
	ProjectDir string
	Generator  config.GeneratorConfig
}

// EnvListResult returns the computed environment list.
type EnvListResult struct {
	Environments []EnvInfo
}

// EnvListWorkflow computes environment status for listing.
type EnvListWorkflow struct {
	DetectorFactory ports.DetectorFactory
}

// NewEnvListWorkflow constructs an EnvListWorkflow.
func NewEnvListWorkflow(detectorFactory ports.DetectorFactory) EnvListWorkflow {
	return EnvListWorkflow{DetectorFactory: detectorFactory}
}

// Run executes the env list workflow.
func (w EnvListWorkflow) Run(req EnvListRequest) (EnvListResult, error) {
	activeEnv := strings.TrimSpace(req.Generator.App.LastEnv)
	result := EnvListResult{Environments: make([]EnvInfo, 0, len(req.Generator.Environments))}

	for _, env := range req.Generator.Environments {
		name := strings.TrimSpace(env.Name)
		if name == "" {
			continue
		}

		status := "unknown"
		if w.DetectorFactory != nil {
			detector, err := w.DetectorFactory(req.ProjectDir, name)
			if err == nil && detector != nil {
				if current, err := detector.Detect(); err == nil {
					status = string(current)
				}
			}
		}

		result.Environments = append(result.Environments, EnvInfo{
			Name:   name,
			Mode:   env.Mode,
			Status: status,
			Active: name == activeEnv,
		})
	}

	return result, nil
}

// EnvAddRequest captures inputs for adding an environment.
type EnvAddRequest struct {
	GeneratorPath string
	Generator     config.GeneratorConfig
	Name          string
	Mode          string
}

// EnvAddWorkflow adds an environment to generator.yml.
type EnvAddWorkflow struct{}

// NewEnvAddWorkflow constructs an EnvAddWorkflow.
func NewEnvAddWorkflow() EnvAddWorkflow {
	return EnvAddWorkflow{}
}

// Run executes the env add workflow.
func (w EnvAddWorkflow) Run(req EnvAddRequest) error {
	name := strings.TrimSpace(req.Name)
	mode := strings.TrimSpace(req.Mode)
	if name == "" {
		return errors.New("environment name is required")
	}
	if mode == "" {
		return errors.New("environment mode is required")
	}
	if req.Generator.Environments.Has(name) {
		return fmt.Errorf("environment %q already exists", name)
	}
	req.Generator.Environments = append(req.Generator.Environments, config.EnvironmentSpec{
		Name: name,
		Mode: mode,
	})
	return config.SaveGeneratorConfig(req.GeneratorPath, req.Generator)
}

// EnvUseRequest captures inputs for switching environments.
type EnvUseRequest struct {
	EnvName          string
	ProjectName      string
	ProjectDir       string
	GeneratorPath    string
	Generator        config.GeneratorConfig
	GlobalConfig     config.GlobalConfig
	GlobalConfigPath string
	Now              time.Time
}

// EnvUseResult reports the updated selection.
type EnvUseResult struct {
	ProjectName string
	EnvName     string
}

// EnvUseWorkflow updates last-used environment and global config.
type EnvUseWorkflow struct{}

// NewEnvUseWorkflow constructs an EnvUseWorkflow.
func NewEnvUseWorkflow() EnvUseWorkflow {
	return EnvUseWorkflow{}
}

// Run executes the env use workflow.
func (w EnvUseWorkflow) Run(req EnvUseRequest) (EnvUseResult, error) {
	if strings.TrimSpace(req.GlobalConfigPath) == "" {
		return EnvUseResult{}, errors.New("global config path not available")
	}

	req.Generator.App.LastEnv = req.EnvName
	if err := config.SaveGeneratorConfig(req.GeneratorPath, req.Generator); err != nil {
		return EnvUseResult{}, err
	}

	cfg := normalizeGlobalConfig(req.GlobalConfig)
	entry := cfg.Projects[req.ProjectName]
	entry.Path = req.ProjectDir
	entry.LastUsed = req.Now.Format(time.RFC3339)
	cfg.Projects[req.ProjectName] = entry
	if err := config.SaveGlobalConfig(req.GlobalConfigPath, cfg); err != nil {
		return EnvUseResult{}, err
	}

	return EnvUseResult{ProjectName: req.ProjectName, EnvName: req.EnvName}, nil
}

// ErrEnvNotFound indicates the target environment was missing.
var ErrEnvNotFound = errors.New("environment not found")

// ErrEnvLast indicates the last environment cannot be removed.
var ErrEnvLast = errors.New("cannot remove the last environment")

// EnvRemoveRequest captures inputs for removing an environment.
type EnvRemoveRequest struct {
	Name          string
	GeneratorPath string
	Generator     config.GeneratorConfig
}

// EnvRemoveWorkflow removes an environment from generator.yml.
type EnvRemoveWorkflow struct{}

// NewEnvRemoveWorkflow constructs an EnvRemoveWorkflow.
func NewEnvRemoveWorkflow() EnvRemoveWorkflow {
	return EnvRemoveWorkflow{}
}

// Run executes the env remove workflow.
func (w EnvRemoveWorkflow) Run(req EnvRemoveRequest) error {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return errors.New("environment name is required")
	}
	if !req.Generator.Environments.Has(name) {
		return ErrEnvNotFound
	}
	if len(req.Generator.Environments) <= 1 {
		return ErrEnvLast
	}

	filtered := make(config.Environments, 0, len(req.Generator.Environments)-1)
	for _, env := range req.Generator.Environments {
		if strings.TrimSpace(env.Name) == name {
			continue
		}
		filtered = append(filtered, env)
	}
	req.Generator.Environments = filtered
	if strings.TrimSpace(req.Generator.App.LastEnv) == name {
		req.Generator.App.LastEnv = ""
	}
	return config.SaveGeneratorConfig(req.GeneratorPath, req.Generator)
}
