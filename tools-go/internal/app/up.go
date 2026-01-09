// Where: tools-go/internal/app/up.go
// What: Up command helpers.
// Why: Ensure up orchestration is consistent and testable.
package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/tools-go/internal/compose"
	"github.com/poruru/edge-serverless-box/tools-go/internal/state"
)

type UpRequest struct {
	Context state.Context
	Detach  bool
	Wait    bool
}

type Upper interface {
	Up(request UpRequest) error
}

func runUp(cli CLI, deps Dependencies, out io.Writer) int {
	if deps.Upper == nil {
		fmt.Fprintln(out, "up: not implemented")
		return 1
	}
	if deps.Provisioner == nil {
		fmt.Fprintln(out, "up: provisioner not configured")
		return 1
	}

	projectDir := deps.ProjectDir
	if projectDir == "" {
		projectDir = "."
	}

	env := resolveEnv(cli, deps)

	ctx, err := state.ResolveContext(projectDir, env)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	applyModeEnv(ctx.Mode)
	applyUpEnv(ctx)

	templatePath := ctx.TemplatePath
	if cli.Template != "" {
		absTemplate, err := filepath.Abs(cli.Template)
		if err != nil {
			fmt.Fprintln(out, err)
			return 1
		}
		if _, err := os.Stat(absTemplate); err != nil {
			fmt.Fprintln(out, err)
			return 1
		}
		templatePath = absTemplate
	}

	if cli.Up.Build {
		if deps.Builder == nil {
			fmt.Fprintln(out, "up: builder not configured")
			return 1
		}

		request := BuildRequest{
			ProjectDir:   ctx.ProjectDir,
			TemplatePath: templatePath,
			Env:          env,
		}
		if err := deps.Builder.Build(request); err != nil {
			fmt.Fprintln(out, err)
			return 1
		}
	}

	request := UpRequest{
		Context: ctx,
		Detach:  cli.Up.Detach,
		Wait:    cli.Up.Wait,
	}
	if err := deps.Upper.Up(request); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	if err := deps.Provisioner.Provision(ProvisionRequest{
		TemplatePath:   templatePath,
		ProjectDir:     ctx.ProjectDir,
		Env:            env,
		ComposeProject: ctx.ComposeProject,
		Mode:           ctx.Mode,
	}); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}

	fmt.Fprintln(out, "up complete")
	return 0
}

func applyUpEnv(ctx state.Context) {
	env := strings.TrimSpace(ctx.Env)
	if env == "" {
		return
	}
	_ = os.Setenv("ESB_ENV", env)

	if strings.TrimSpace(os.Getenv("ESB_PROJECT_NAME")) == "" {
		_ = os.Setenv("ESB_PROJECT_NAME", fmt.Sprintf("esb-%s", strings.ToLower(env)))
	}
	if strings.TrimSpace(os.Getenv("ESB_IMAGE_TAG")) == "" {
		_ = os.Setenv("ESB_IMAGE_TAG", env)
	}
	if strings.TrimSpace(os.Getenv("ESB_CONFIG_DIR")) != "" {
		return
	}

	root, err := compose.FindRepoRoot(ctx.ProjectDir)
	if err != nil {
		return
	}
	stagingRel := filepath.Join("services", "gateway", ".esb-staging", env, "config")
	stagingAbs := filepath.Join(root, stagingRel)
	if _, err := os.Stat(stagingAbs); err != nil {
		return
	}
	_ = os.Setenv("ESB_CONFIG_DIR", filepath.ToSlash(stagingRel))
}
