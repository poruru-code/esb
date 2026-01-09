// Where: cli/internal/app/info.go
// What: Info command for config/state output.
// Why: Give users a quick view of configuration and current status.
package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// runInfo executes the 'info' command which displays configuration details
// and current environment state for debugging and troubleshooting.
func runInfo(cli CLI, deps Dependencies, out io.Writer) int {
	configPath, err := config.GlobalConfigPath()
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	cfg, err := loadGlobalConfig(configPath)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	projectDir := deps.ProjectDir
	if cfg.ActiveProject != "" {
		if entry, ok := cfg.Projects[cfg.ActiveProject]; ok && strings.TrimSpace(entry.Path) != "" {
			projectDir = entry.Path
		}
	}
	if strings.TrimSpace(projectDir) == "" {
		projectDir = "."
	}

	project, err := loadProjectConfig(projectDir)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	envDeps := deps
	envDeps.ProjectDir = project.Dir
	env := resolveEnv(cli, envDeps)

	ctx, err := state.ResolveContext(project.Dir, env)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	stateValue := "unknown"
	if deps.DetectorFactory != nil {
		detector, err := deps.DetectorFactory(project.Dir, env)
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

	fmt.Fprintln(out, "Config")
	fmt.Fprintf(out, "  path: %s\n", configPath)
	if cfg.ActiveProject != "" {
		fmt.Fprintf(out, "  active_project: %s\n", cfg.ActiveProject)
	}
	if activeEnv := strings.TrimSpace(cfg.ActiveEnvironments[cfg.ActiveProject]); activeEnv != "" {
		fmt.Fprintf(out, "  active_env: %s\n", activeEnv)
	}

	fmt.Fprintln(out, "Project")
	fmt.Fprintf(out, "  name: %s\n", project.Name)
	fmt.Fprintf(out, "  dir: %s\n", project.Dir)
	fmt.Fprintf(out, "  generator: %s\n", project.GeneratorPath)
	fmt.Fprintf(out, "  template: %s\n", ctx.TemplatePath)
	fmt.Fprintf(out, "  output_dir: %s\n", ctx.OutputDir)
	fmt.Fprintf(out, "  output_env_dir: %s\n", ctx.OutputEnvDir)
	fmt.Fprintf(out, "  env: %s\n", ctx.Env)
	fmt.Fprintf(out, "  mode: %s\n", ctx.Mode)
	fmt.Fprintf(out, "  compose_project: %s\n", ctx.ComposeProject)

	fmt.Fprintln(out, "State")
	fmt.Fprintf(out, "  current: %s\n", stateValue)

	return 0
}
