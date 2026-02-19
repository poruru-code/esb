// Where: cli/internal/usecase/deploy/deploy_postbuild_summary.go
// What: Post-build delta summary helpers for deploy workflow.
// Why: Keep diff reporting separate from build and runtime provisioning logic.
package deploy

import (
	"fmt"

	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
)

func (w Workflow) emitPostBuildSummary(req Request) {
	if w.UserInterface == nil {
		return
	}

	templateConfigDir, err := resolveTemplateConfigDir(req.Context.TemplatePath, req.OutputDir, req.Context.Env)
	if err != nil {
		w.UserInterface.Warn(fmt.Sprintf("Warning: failed to resolve template config dir: %v", err))
	} else {
		templateSnapshot, err := loadConfigSnapshot(templateConfigDir)
		if err != nil {
			w.UserInterface.Warn(fmt.Sprintf("Warning: failed to read template config: %v", err))
		} else {
			diff := diffConfigSnapshots(domaincfg.Snapshot{}, templateSnapshot)
			emitTemplateDeltaSummary(w.UserInterface, templateConfigDir, diff)
		}
	}

	w.UserInterface.Info("Runtime-config apply summary is handled in apply phase")
}
