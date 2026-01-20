// Where: cli/internal/commands/info.go
// What: Info command for config/state output.
// Why: Give users a quick view of configuration and current status.
package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
	"github.com/poruru/edge-serverless-box/cli/internal/version"
)

// runInfo displays configuration details and current environment state.
// Used by runNoArgs when esb is invoked without arguments.
func runInfo(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(false) // No force flag for info display
	ui := legacyUI(out)
	configPath, cfg, err := loadGlobalConfigWithPath()
	if err != nil {
		ui.Warn(err.Error())
		return 1
	}

	ui.Info("ℹ️  Version")
	ui.Info(fmt.Sprintf("   %s", version.GetVersion()))

	ui.Info("")
	ui.Info("⚙️  Config")
	ui.Info(fmt.Sprintf("   path: %s", configPath))
	if cli.Template == "" && len(cfg.Projects) == 0 {
		ui.Info("")
		ui.Info("📦 No projects registered.")
		ui.Info("   Run 'esb project add . -t <template>' to get started.")
		return 1
	}

	selection, err := resolveProjectSelection(cli, deps, opts)
	if err != nil {
		ui.Warn(err.Error())
		return 1
	}

	projectDir := selection.Dir
	if strings.TrimSpace(projectDir) == "" {
		projectDir = "."
	}
	project, err := loadProjectConfig(projectDir)
	if err != nil {
		ui.Warn(err.Error())
		return 1
	}

	envState, err := state.ResolveProjectState(state.ProjectStateOptions{
		EnvFlag:     cli.EnvFlag,
		EnvVar:      envutil.GetHostEnv(constants.HostSuffixEnv),
		Config:      project.Generator,
		Force:       opts.Force,
		Interactive: opts.Interactive,
		Prompt:      opts.Prompt,
	})

	// Proceed even if environment resolution fails (e.g. no active env),
	// so we can still show project info.
	var envError error
	if err != nil {
		envError = err
		envState = state.ProjectState{
			ActiveEnv: "",
		}
	}

	var ctx state.Context
	if envError == nil {
		ctx, err = state.ResolveContext(project.Dir, envState.ActiveEnv)
		if err != nil {
			ui.Warn(err.Error())
			return 1
		}
	} else {
		ctx = state.Context{
			Env:          "(none)",
			Mode:         "unknown",
			TemplatePath: "(pending)",
			OutputDir:    project.Generator.Paths.OutputDir,
		}
	}

	ui.Info("")
	ui.Info("📦 Project")
	ui.Info(fmt.Sprintf("   name: %s", project.Name))
	ui.Info(fmt.Sprintf("   dir:  %s", project.Dir))
	ui.Info(fmt.Sprintf("   gen:  %s", project.GeneratorPath))
	ui.Info(fmt.Sprintf("   tmpl: %s", ctx.TemplatePath))
	ui.Info(fmt.Sprintf("   out:  %s", ctx.OutputDir))

	stateValue := "unknown"
	if envError == nil && deps.DetectorFactory != nil {
		detector, err := deps.DetectorFactory(project.Dir, ctx.Env)
		if err != nil {
			stateValue = fmt.Sprintf("error: %v", err)
		} else if detector != nil {
			current, err := detector.Detect()
			if err != nil {
				stateValue = fmt.Sprintf("error: %v", err)
			} else {
				stateValue = string(current)
			}
		}
	}

	ui.Info("")
	ui.Info("🌐 Environment")
	if envError != nil {
		ui.Info(fmt.Sprintf("   status: %v", envError))
	}
	ui.Info(fmt.Sprintf("   name:   %s (%s)", ctx.Env, ctx.Mode))
	ui.Info(fmt.Sprintf("   state:  %s", stateValue))
	ui.Info(fmt.Sprintf("   env:    %s", ctx.OutputEnvDir))
	ui.Info(fmt.Sprintf("   proj:   %s", ctx.ComposeProject))

	return 0
}
