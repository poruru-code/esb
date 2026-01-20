// Where: cli/internal/app/build.go
// What: Build command helpers.
// Why: Orchestrate build operations in a testable way.
package app

import (
	"fmt"
	"io"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/cli/internal/workflows"
)

// Builder defines the interface for building Lambda function images.
// Implementations generate Dockerfiles and build container images.
type Builder = ports.Builder

// runBuild executes the 'build' command which generates Dockerfiles
// and builds container images for all Lambda functions in the SAM template.
func runBuild(cli CLI, deps Dependencies, out io.Writer) int {
	opts := newResolveOptions(cli.Build.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}

	repoResolver := deps.RepoResolver
	if repoResolver == nil {
		repoResolver = config.ResolveRepoRoot
	}

	cmd, err := newBuildCommand(deps.Build, repoResolver, out)
	if err != nil {
		return exitWithError(out, err)
	}

	if err := cmd.Run(ctxInfo, cli.Build); err != nil {
		return exitWithError(out, err)
	}
	return 0
}

type buildCommand struct {
	builder    ports.Builder
	envApplier ports.RuntimeEnvApplier
	ui         ports.UserInterface
}

func newBuildCommand(deps BuildDeps, repoResolver func(string) (string, error), out io.Writer) (*buildCommand, error) {
	if deps.Builder == nil {
		return nil, fmt.Errorf("build: builder not configured")
	}
	envApplier := newRuntimeEnvApplier(repoResolver)
	ui := ports.NewLegacyUI(out)
	return &buildCommand{
		builder:    deps.Builder,
		envApplier: envApplier,
		ui:         ui,
	}, nil
}

func (c *buildCommand) Run(ctxInfo commandContext, flags BuildCmd) error {
	templatePath := resolvedTemplatePath(ctxInfo)
	request := workflows.BuildRequest{
		Context:      ctxInfo.Context,
		Env:          ctxInfo.Env,
		TemplatePath: templatePath,
		NoCache:      flags.NoCache,
		Verbose:      flags.Verbose,
	}
	return workflows.NewBuildWorkflow(c.builder, c.envApplier, c.ui).Run(request)
}
