// Where: cli/internal/app/up.go
// What: Up command helpers.
// Why: Ensure up orchestration is consistent and testable.
package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/compose"
	"github.com/poruru/edge-serverless-box/cli/internal/config"
	"github.com/poruru/edge-serverless-box/cli/internal/state"
)

// UpRequest contains parameters for starting the environment.
// It includes the context information and flags for detached/wait modes.
type UpRequest struct {
	Context state.Context
	Detach  bool
	Wait    bool
}

// Upper defines the interface for starting the environment.
// Implementations use Docker Compose to bring up the services.
type Upper interface {
	Up(request UpRequest) error
}

// runUp executes the 'up' command which starts all services,
// optionally rebuilds images, provisions Lambda functions, and waits for readiness.
func runUp(cli CLI, deps Dependencies, out io.Writer) int {
	if deps.Upper == nil {
		fmt.Fprintln(out, "up: not implemented")
		return 1
	}
	if deps.Provisioner == nil {
		fmt.Fprintln(out, "up: provisioner not configured")
		return 1
	}

	opts := newResolveOptions(cli.Up.Force)
	ctxInfo, err := resolveCommandContext(cli, deps, opts)
	if err != nil {
		return exitWithError(out, err)
	}
	ctx := ctxInfo.Context
	applyModeEnv(ctx.Mode)
	applyEnvironmentDefaults(ctx.Env, ctx.Mode)
	if err := applyGeneratorConfigEnv(ctx.GeneratorPath); err != nil {
		fmt.Fprintln(out, err)
	}
	applyUpEnv(ctx)

	templatePath := resolvedTemplatePath(ctxInfo)

	if cli.Up.Build {
		if deps.Builder == nil {
			fmt.Fprintln(out, "up: builder not configured")
			return 1
		}

		request := BuildRequest{
			ProjectDir:   ctx.ProjectDir,
			TemplatePath: templatePath,
			Env:          ctxInfo.Env,
		}
		if err := deps.Builder.Build(request); err != nil {
			return exitWithError(out, err)
		}
	}

	request := UpRequest{
		Context: ctx,
		Detach:  cli.Up.Detach,
		Wait:    cli.Up.Wait,
	}
	if err := deps.Upper.Up(request); err != nil {
		return exitWithError(out, err)
	}

	discoverAndPersistPorts(ctx, deps.PortDiscoverer, out)

	if err := deps.Provisioner.Provision(ProvisionRequest{
		TemplatePath:   templatePath,
		ProjectDir:     ctx.ProjectDir,
		Env:            ctxInfo.Env,
		ComposeProject: ctx.ComposeProject,
		Mode:           ctx.Mode,
	}); err != nil {
		return exitWithError(out, err)
	}

	if cli.Up.Wait {
		if deps.Waiter == nil {
			fmt.Fprintln(out, "up: waiter not configured")
			return 1
		}
		if err := deps.Waiter.Wait(ctx); err != nil {
			return exitWithError(out, err)
		}
	}

	fmt.Fprintln(out, "up complete")
	return 0
}

// applyGeneratorConfigEnv reads the generator.yml configuration and sets
// environment variables for function/routing paths and custom parameters.
func applyGeneratorConfigEnv(generatorPath string) error {
	cfg, err := config.LoadGeneratorConfig(generatorPath)
	if err != nil {
		return err
	}

	if strings.TrimSpace(cfg.Paths.FunctionsYml) != "" {
		_ = os.Setenv("GATEWAY_FUNCTIONS_YML", cfg.Paths.FunctionsYml)
	}
	if strings.TrimSpace(cfg.Paths.RoutingYml) != "" {
		_ = os.Setenv("GATEWAY_ROUTING_YML", cfg.Paths.RoutingYml)
	}

	for key, value := range cfg.Parameters {
		if strings.TrimSpace(key) == "" || value == nil {
			continue
		}
		switch value.(type) {
		case string, bool, int, int64, int32, float64, float32, uint, uint64, uint32:
			_ = os.Setenv(key, fmt.Sprint(value))
		}
	}
	return nil
}

// applyUpEnv sets environment variables required for the 'up' command,
// including ESB_ENV, ESB_PROJECT_NAME, ESB_IMAGE_TAG, and ESB_CONFIG_DIR.
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
