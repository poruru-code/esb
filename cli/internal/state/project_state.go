// Where: cli/internal/state/project_state.go
// What: Environment selection helpers for a project.
// Why: Resolve ESB_ENV, last_env, and single-environment defaults consistently.
package state

import (
	"fmt"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

// ProjectState holds environment selection results.
type ProjectState struct {
	HasEnvironments bool
	ActiveEnv       string
	GeneratorValid  bool
}

// ProjectStateOptions configures environment selection and interaction behavior.
type ProjectStateOptions struct {
	EnvFlag     string
	EnvVar      string
	Config      config.GeneratorConfig
	Force       bool
	Interactive bool
	Prompt      PromptFunc
}

// ResolveProjectState resolves the active environment for the project.
func ResolveProjectState(opts ProjectStateOptions) (ProjectState, error) {
	envs := opts.Config.Environments
	hasEnvs := len(envs) > 0
	if !hasEnvs {
		return ProjectState{HasEnvironments: false, GeneratorValid: true}, fmt.Errorf(
			"No environments defined. Run 'esb env create <name>' first.",
		)
	}

	envFlag := strings.TrimSpace(opts.EnvFlag)
	if envFlag != "" {
		if envs.Has(envFlag) {
			return ProjectState{HasEnvironments: true, ActiveEnv: envFlag, GeneratorValid: true}, nil
		}
		return ProjectState{HasEnvironments: true, GeneratorValid: true}, fmt.Errorf(
			"Environment not registered: %s",
			envFlag,
		)
	}

	envVar := strings.TrimSpace(opts.EnvVar)
	if envVar != "" {
		if envs.Has(envVar) {
			return ProjectState{HasEnvironments: true, ActiveEnv: envVar, GeneratorValid: true}, nil
		}
		allowed, err := confirmUnset("ESB_ENV", envVar, AppStateOptions{
			Force:       opts.Force,
			Interactive: opts.Interactive,
			Prompt:      opts.Prompt,
		})
		if err != nil {
			return ProjectState{}, err
		}
		if !allowed {
			return ProjectState{}, fmt.Errorf("ESB_ENV %q not found", envVar)
		}
		_ = os.Unsetenv("ESB_ENV")
		envVar = ""
	}

	lastEnv := strings.TrimSpace(opts.Config.App.LastEnv)
	if lastEnv != "" && envs.Has(lastEnv) {
		return ProjectState{HasEnvironments: true, ActiveEnv: lastEnv, GeneratorValid: true}, nil
	}

	if len(envs) == 1 {
		name := strings.TrimSpace(envs[0].Name)
		if name != "" {
			return ProjectState{HasEnvironments: true, ActiveEnv: name, GeneratorValid: true}, nil
		}
	}

	return ProjectState{HasEnvironments: true, GeneratorValid: true}, fmt.Errorf(
		"No active environment. Run 'esb env use <name>' first.",
	)
}
