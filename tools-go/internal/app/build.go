// Where: tools-go/internal/app/build.go
// What: Build command helpers.
// Why: Orchestrate build operations in a testable way.
package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type BuildRequest struct {
	ProjectDir   string
	TemplatePath string
	Env          string
}

type Builder interface {
	Build(request BuildRequest) error
}

func runBuild(cli CLI, deps Dependencies, out io.Writer) int {
	if deps.Builder == nil {
		fmt.Fprintln(out, "build: not implemented")
		return 1
	}
	if cli.Template == "" {
		fmt.Fprintln(out, "template is required")
		return 1
	}

	absTemplate, err := filepath.Abs(cli.Template)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	if _, err := os.Stat(absTemplate); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	projectDir := filepath.Dir(absTemplate)
	envDeps := deps
	envDeps.ProjectDir = projectDir
	env := resolveEnv(cli, envDeps)
	request := BuildRequest{
		ProjectDir:   projectDir,
		TemplatePath: absTemplate,
		Env:          env,
	}

	if err := deps.Builder.Build(request); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintln(out, "build complete")
	return 0
}
