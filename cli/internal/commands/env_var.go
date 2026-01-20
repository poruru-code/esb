// Where: cli/internal/commands/env_var.go
// What: Show container environment variables command.
// Why: Allow users to inspect running container configurations.
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
)

// runEnvVar executes the 'env var' command which shows environment variables
// of a running container. If no service is specified, prompts interactively.
func runEnvVar(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Env.Var.Force)
	opts.Interactive = interaction.IsTerminal(os.Stdin) && deps.Prompter != nil
	ui := legacyUI(out)

	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}
	ctx := ctxInfo.Context

	// Get list of services
	rootDir, err := config.ResolveRepoRoot(ctx.ProjectDir)
	if err != nil {
		return exitWithError(out, err)
	}

	logsOpts := compose.LogsOptions{
		RootDir: rootDir,
		Project: ctx.ComposeProject,
		Mode:    ctx.Mode,
		Target:  "control",
	}

	services, err := compose.ListServices(context.Background(), compose.ExecRunner{}, logsOpts)
	if err != nil {
		return exitWithError(out, err)
	}

	if len(services) == 0 {
		ui.Info("No services found in the environment.")
		return 1
	}

	service := strings.TrimSpace(cli.Env.Var.Service)

	// Interactive selection if no service specified
	if service == "" {
		if !opts.Interactive {
			return exitWithSuggestionAndAvailable(out,
				"Service name required (non-interactive mode).",
				[]string{"esb env var <service>"},
				services,
			)
		}

		if deps.Prompter == nil {
			return exitWithError(out, fmt.Errorf("prompter not configured"))
		}

		selected, err := deps.Prompter.Select("Select service", services)
		if err != nil {
			return exitWithError(out, err)
		}
		service = selected
	}

	// Validate service exists
	found := false
	for _, s := range services {
		if s == service {
			found = true
			break
		}
	}
	if !found {
		return exitWithSuggestionAndAvailable(out,
			fmt.Sprintf("Service '%s' not found.", service),
			[]string{"esb env var <service>"},
			services,
		)
	}

	if deps.Logs.Logger == nil {
		return exitWithError(out, fmt.Errorf("logger not configured"))
	}

	// Get containers for the project
	containers, err := deps.Logs.Logger.ListContainers(ctx.ComposeProject)
	if err != nil {
		return exitWithError(out, fmt.Errorf("failed to list containers: %w", err))
	}

	// Find the container for the selected service
	var containerName string
	for _, ctr := range containers {
		if ctr.Service == service {
			containerName = ctr.Name
			break
		}
	}

	if containerName == "" {
		return exitWithError(out, fmt.Errorf("no running container found for service '%s'", service))
	}

	// Get environment variables
	envVars, err := compose.GetContainerEnv(context.Background(), compose.ExecRunner{}, containerName)
	if err != nil {
		return exitWithError(out, fmt.Errorf("failed to get env for %s (%s): %w", service, containerName, err))
	}

	// Sort for consistent output
	sort.Strings(envVars)

	// Output based on format
	format := strings.ToLower(cli.Env.Var.Format)
	switch format {
	case "json":
		envMap := make(map[string]string)
		for _, env := range envVars {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}
		data, _ := json.MarshalIndent(envMap, "", "  ")
		ui.Info(string(data))
	case "export":
		for _, env := range envVars {
			ui.Info(fmt.Sprintf("export %s", env))
		}
	default: // plain
		ui.Info(fmt.Sprintf("--- Environment variables for service '%s' (container: %s) ---", service, containerName))
		for _, env := range envVars {
			ui.Info(env)
		}
	}

	return 0
}
