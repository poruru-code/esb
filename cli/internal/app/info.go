// Where: cli/internal/app/info.go
// What: Info command for config/state output.
// Why: Give users a quick view of configuration and current status.
package app

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/state"
	"github.com/poruru/edge-serverless-box/cli/internal/version"
)

// runInfo displays configuration details and current environment state.
// Used by runNoArgs when esb is invoked without arguments.
func runInfo(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(false) // No force flag for info display
	configPath, cfg, err := loadGlobalConfigWithPath()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintln(out, "ℹ️  Version")
	fmt.Fprintf(out, "   %s\n", version.GetVersion())

	fmt.Fprintln(out, "\n⚙️  Config")
	fmt.Fprintf(out, "   path: %s\n", configPath)
	if cli.Template == "" && len(cfg.Projects) == 0 {
		fmt.Fprintln(out, "\n📦 No projects registered.")
		fmt.Fprintln(out, "   Run 'esb project add . -t <template>' to get started.")
		return 1
	}

	selection, err := resolveProjectSelection(cli, deps, opts)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	projectDir := selection.Dir
	if strings.TrimSpace(projectDir) == "" {
		projectDir = "."
	}
	project, err := loadProjectConfig(projectDir)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	envState, err := state.ResolveProjectState(state.ProjectStateOptions{
		EnvFlag:     cli.EnvFlag,
		EnvVar:      os.Getenv("ESB_ENV"),
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
			fmt.Fprintln(out, err)
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

	fmt.Fprintln(out, "\n📦 Project")
	fmt.Fprintf(out, "   name: %s\n", project.Name)
	fmt.Fprintf(out, "   dir:  %s\n", project.Dir)
	fmt.Fprintf(out, "   gen:  %s\n", project.GeneratorPath)
	fmt.Fprintf(out, "   tmpl: %s\n", ctx.TemplatePath)
	fmt.Fprintf(out, "   out:  %s\n", ctx.OutputDir)

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

	fmt.Fprintln(out, "\n🌐 Environment")
	if envError != nil {
		fmt.Fprintf(out, "   status: %v\n", envError)
	}
	fmt.Fprintf(out, "   name:   %s (%s)\n", ctx.Env, ctx.Mode)
	fmt.Fprintf(out, "   state:  %s\n", stateValue)
	fmt.Fprintf(out, "   env:    %s\n", ctx.OutputEnvDir)
	fmt.Fprintf(out, "   proj:   %s\n", ctx.ComposeProject)

	return 0
}
