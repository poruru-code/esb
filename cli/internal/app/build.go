// Where: cli/internal/app/build.go
// What: Build command helpers.
// Why: Orchestrate build operations in a testable way.
package app

import (
	"fmt"
	"io"

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

	return runBuildWithDeps(deps.Build, deps.RepoResolver, ctxInfo, cli.Build.NoCache, cli.Build.Verbose, out)
}

func runBuildWithDeps(
	deps BuildDeps,
	repoResolver func(string) (string, error),
	ctxInfo commandContext,
	noCache bool,
	verbose bool,
	out io.Writer,
) int {
	if deps.Builder == nil {
		fmt.Fprintln(out, "build: not implemented")
		return 1
	}

	templatePath := resolvedTemplatePath(ctxInfo)

	envApplier := newRuntimeEnvApplier(repoResolver)
	workflow := workflows.NewBuildWorkflow(deps.Builder, envApplier, ports.NewLegacyUI(out))

	request := workflows.BuildRequest{
		Context:      ctxInfo.Context,
		Env:          ctxInfo.Env,
		TemplatePath: templatePath,
		NoCache:      noCache,
		Verbose:      verbose,
	}

	if err := workflow.Run(request); err != nil {
		return exitWithError(out, err)
	}

	return 0
}
