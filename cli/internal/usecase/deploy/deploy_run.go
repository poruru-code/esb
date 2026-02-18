// Where: cli/internal/usecase/deploy/deploy_run.go
// What: Workflow.Run orchestration skeleton.
// Why: Keep deploy phase order visible while details live in dedicated files.
package deploy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/constants"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/state"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/envutil"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

var (
	errApplyTemplatePathRequired  = errors.New("template path is required for apply phase")
	errApplyComposeProjectMissing = errors.New("compose project is required for apply phase")
	errApplyEnvRequired           = errors.New("env is required for apply phase")
	errApplyModeRequired          = errors.New("mode is required for apply phase")
	errApplyTemplatePathConflict  = errors.New("template path mismatch between request and context for apply phase")
	errApplyEnvConflict           = errors.New("env mismatch between request and context for apply phase")
	errApplyModeConflict          = errors.New("mode mismatch between request and context for apply phase")
)

// Run executes the deploy workflow.
func (w Workflow) Run(req Request) error {
	if w.Build == nil {
		return errBuilderNotConfigured
	}
	if w.ComposeRunner == nil {
		return errComposeRunnerNotConfigured
	}

	imagePrewarm, err := NormalizeImagePrewarmMode(req.ImagePrewarm)
	if err != nil {
		return err
	}

	req = w.alignGatewayRuntime(req)
	if !req.BuildOnly {
		req, err = normalizeApplyRequest(req)
		if err != nil {
			return err
		}
	}
	if w.ApplyRuntimeEnv != nil {
		if err := w.ApplyRuntimeEnv(req.Context); err != nil {
			return err
		}
	}

	if err := w.runGeneratePhase(req); err != nil {
		return err
	}
	if !req.BuildOnly {
		if err := w.runApplyPhase(req, imagePrewarm); err != nil {
			return err
		}
	}

	// For containerd mode, function images are pulled by agent/runtime-node.
	// This ensures proper image store management in containerd environments.
	// See: agent/runtime-node IMAGE_PULL_POLICY configuration.

	if w.UserInterface != nil {
		w.UserInterface.Success(w.successMessage(req))
	}
	return nil
}

// Apply executes apply-only deploy flow (no build/generation).
func (w Workflow) Apply(req Request) error {
	if w.ComposeRunner == nil {
		return errComposeRunnerNotConfigured
	}

	imagePrewarm, err := NormalizeImagePrewarmMode(req.ImagePrewarm)
	if err != nil {
		return err
	}

	req = w.alignGatewayRuntime(req)
	req, err = normalizeApplyRequest(req)
	if err != nil {
		return err
	}
	if w.ApplyRuntimeEnv != nil {
		if err := w.ApplyRuntimeEnv(req.Context); err != nil {
			return err
		}
	}

	if err := w.runApplyPhase(req, imagePrewarm); err != nil {
		return err
	}

	if w.UserInterface != nil {
		w.UserInterface.Success(w.successMessage(req))
	}
	return nil
}

// runGeneratePhase executes build/generation-side deploy steps.
func (w Workflow) runGeneratePhase(req Request) error {
	if err := w.runBuildPhase(req); err != nil {
		return err
	}
	w.emitPostBuildSummary(req)
	return nil
}

// runApplyPhase executes runtime/provision-side deploy steps.
func (w Workflow) runApplyPhase(req Request, imagePrewarm string) error {
	stagingDir, err := resolveApplyConfigDir(req.Context)
	if err != nil {
		return err
	}
	if err := setConfigDirEnv(stagingDir); err != nil {
		return err
	}
	if err := w.waitRegistryAndServices(req); err != nil {
		return err
	}
	return w.runRuntimeProvisionPhase(req, stagingDir, imagePrewarm)
}

func normalizeApplyRequest(req Request) (Request, error) {
	ctx := req.Context

	reqTemplate := strings.TrimSpace(req.TemplatePath)
	ctxTemplate := strings.TrimSpace(ctx.TemplatePath)
	if reqTemplate != "" && ctxTemplate != "" && !sameCleanPath(reqTemplate, ctxTemplate) {
		return Request{}, errApplyTemplatePathConflict
	}
	if ctxTemplate == "" {
		ctxTemplate = reqTemplate
	}
	if ctxTemplate == "" {
		return Request{}, errApplyTemplatePathRequired
	}

	reqEnv := strings.TrimSpace(req.Env)
	ctxEnv := strings.TrimSpace(ctx.Env)
	if reqEnv != "" && ctxEnv != "" && reqEnv != ctxEnv {
		return Request{}, errApplyEnvConflict
	}
	if ctxEnv == "" {
		ctxEnv = reqEnv
	}
	if ctxEnv == "" {
		return Request{}, errApplyEnvRequired
	}

	if strings.TrimSpace(ctx.ComposeProject) == "" {
		return Request{}, errApplyComposeProjectMissing
	}

	reqMode := strings.TrimSpace(req.Mode)
	ctxMode := strings.TrimSpace(ctx.Mode)
	if reqMode != "" && ctxMode != "" && !strings.EqualFold(reqMode, ctxMode) {
		return Request{}, errApplyModeConflict
	}
	if ctxMode == "" {
		ctxMode = reqMode
	}
	if ctxMode == "" {
		return Request{}, errApplyModeRequired
	}

	ctx.TemplatePath = ctxTemplate
	ctx.Env = ctxEnv
	ctx.Mode = ctxMode
	req.Context = ctx
	req.TemplatePath = ctxTemplate
	req.Env = ctxEnv
	req.Mode = ctxMode
	return req, nil
}

func resolveApplyConfigDir(ctx state.Context) (string, error) {
	templatePath := strings.TrimSpace(ctx.TemplatePath)
	if templatePath == "" {
		return "", errApplyTemplatePathRequired
	}
	composeProject := strings.TrimSpace(ctx.ComposeProject)
	if composeProject == "" {
		return "", errApplyComposeProjectMissing
	}
	env := strings.TrimSpace(ctx.Env)
	if env == "" {
		return "", errApplyEnvRequired
	}
	return staging.ConfigDir(templatePath, composeProject, env)
}

func sameCleanPath(left, right string) bool {
	absLeft, errLeft := filepath.Abs(left)
	absRight, errRight := filepath.Abs(right)
	if errLeft == nil && errRight == nil {
		return filepath.Clean(absLeft) == filepath.Clean(absRight)
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func setConfigDirEnv(configDir string) error {
	normalized := filepath.ToSlash(configDir)
	if strings.TrimSpace(os.Getenv("ENV_PREFIX")) != "" {
		if err := envutil.SetHostEnv(constants.HostSuffixConfigDir, normalized); err != nil {
			return fmt.Errorf("set host env %s: %w", constants.HostSuffixConfigDir, err)
		}
	}
	if err := os.Setenv(constants.EnvConfigDir, normalized); err != nil {
		return fmt.Errorf("set %s: %w", constants.EnvConfigDir, err)
	}
	return nil
}
