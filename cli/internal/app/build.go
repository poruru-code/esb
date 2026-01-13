// Where: cli/internal/app/build.go
// What: Build command helpers.
// Why: Orchestrate build operations in a testable way.
package app

import (
	"fmt"
	"io"
)

// BuildRequest contains parameters for a build operation.
// It specifies the project location, SAM template, environment, and cache options.
type BuildRequest struct {
	ProjectDir   string
	TemplatePath string
	Env          string
	NoCache      bool
	Verbose      bool
}

// Builder defines the interface for building Lambda function images.
// Implementations generate Dockerfiles and build container images.
type Builder interface {
	Build(request BuildRequest) error
}

// runBuild executes the 'build' command which generates Dockerfiles
// and builds container images for all Lambda functions in the SAM template.
func runBuild(cli CLI, deps Dependencies, out io.Writer) int {
	if deps.Builder == nil {
		fmt.Fprintln(out, "build: not implemented")
		return 1
	}

	opts := newResolveOptions(cli.Build.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}
	ctx := ctxInfo.Context
	applyRuntimeEnv(ctx, deps.RepoResolver)

	templatePath := resolvedTemplatePath(ctxInfo)

	request := BuildRequest{
		ProjectDir:   ctx.ProjectDir,
		TemplatePath: templatePath,
		Env:          ctxInfo.Env,
		NoCache:      cli.Build.NoCache,
		Verbose:      cli.Build.Verbose,
	}

	if err := deps.Builder.Build(request); err != nil {
		return exitWithError(out, err)
	}

	fmt.Fprintln(out, "✓ Build complete")
	fmt.Fprintln(out, "Next: esb up")
	return 0
}
