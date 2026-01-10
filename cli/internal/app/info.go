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
)

// runInfo executes the 'info' command which displays configuration details
// and current environment state for debugging and troubleshooting.
func runInfo(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Info.Force)
	configPath, cfg, err := loadGlobalConfigWithPath()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintln(out, "Config")
	fmt.Fprintf(out, "  path: %s\n", configPath)
	if cli.Template == "" && len(cfg.Projects) == 0 {
		fmt.Fprintln(out, "No projects registered.")
		fmt.Fprintln(out, "Run 'esb init -t <template>' to get started.")
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

	fmt.Fprintln(out, "Project")
	fmt.Fprintf(out, "  name: %s\n", project.Name)
	fmt.Fprintf(out, "  dir: %s\n", project.Dir)
	fmt.Fprintf(out, "  generator: %s\n", project.GeneratorPath)

	envState, err := state.ResolveProjectState(state.ProjectStateOptions{
		EnvFlag:     cli.EnvFlag,
		EnvVar:      os.Getenv("ESB_ENV"),
		Config:      project.Generator,
		Force:       opts.Force,
		Interactive: opts.Interactive,
		Prompt:      opts.Prompt,
	})
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	ctx, err := state.ResolveContext(project.Dir, envState.ActiveEnv)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintf(out, "  template: %s\n", ctx.TemplatePath)
	fmt.Fprintf(out, "  output_dir: %s\n", ctx.OutputDir)
	fmt.Fprintf(out, "  output_env_dir: %s\n", ctx.OutputEnvDir)
	fmt.Fprintf(out, "  env: %s\n", ctx.Env)
	fmt.Fprintf(out, "  mode: %s\n", ctx.Mode)
	fmt.Fprintf(out, "  compose_project: %s\n", ctx.ComposeProject)

	stateValue := "unknown"
	if deps.DetectorFactory != nil {
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

	fmt.Fprintln(out, "State")
	fmt.Fprintf(out, "  current: %s\n", stateValue)

	return 0
}
