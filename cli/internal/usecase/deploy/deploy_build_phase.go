// Where: cli/internal/usecase/deploy/deploy_build_phase.go
// What: Build-phase preparation and invocation helpers for deploy workflow.
// Why: Isolate staging snapshot/build request logic from Run orchestration.
package deploy

import (
	"fmt"

	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

func (w Workflow) prepareBuildPhase(req Request) (string, domaincfg.Snapshot, error) {
	stagingDir, err := staging.ConfigDir(req.TemplatePath, req.Context.ComposeProject, req.Env)
	if err != nil {
		return "", domaincfg.Snapshot{}, err
	}

	var preSnapshot domaincfg.Snapshot
	if w.UserInterface != nil {
		snapshot, err := loadConfigSnapshot(stagingDir)
		if err != nil {
			w.UserInterface.Warn(fmt.Sprintf("Warning: failed to read existing config: %v", err))
		} else {
			preSnapshot = snapshot
		}
	}
	return stagingDir, preSnapshot, nil
}

func (w Workflow) runBuildPhase(req Request) error {
	return w.Build(w.buildRequest(req))
}
