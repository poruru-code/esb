// Where: cli/internal/app/build.go
// What: Build command helpers.
// Why: Orchestrate build operations in a testable way.
package app

import (
	"fmt"
	"io"
)

type BuildRequest struct {
	ProjectDir   string
	TemplatePath string
	Env          string
	NoCache      bool
}

type Builder interface {
	Build(request BuildRequest) error
}

func runBuild(cli CLI, deps Dependencies, out io.Writer) int {
	if deps.Builder == nil {
		fmt.Fprintln(out, "build: not implemented")
		return 1
	}

	ctxInfo, err := resolveCommandContext(cli, deps)
	if err != nil {
		return exitWithError(out, err)
	}
	ctx := ctxInfo.Context
	applyModeEnv(ctx.Mode)
	applyEnvironmentDefaults(ctx.Env, ctx.Mode)

	templatePath := resolvedTemplatePath(ctxInfo)

	request := BuildRequest{
		ProjectDir:   ctx.ProjectDir,
		TemplatePath: templatePath,
		Env:          ctxInfo.Env,
		NoCache:      cli.Build.NoCache,
	}

	if err := deps.Builder.Build(request); err != nil {
		return exitWithError(out, err)
	}

	fmt.Fprintln(out, "build complete")
	return 0
}
