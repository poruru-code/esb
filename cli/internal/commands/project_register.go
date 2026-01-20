// Where: cli/internal/commands/project_register.go
// What: Project registration after init.
// Why: Persist project metadata into global config for later selection.
package commands

import "github.com/poruru/edge-serverless-box/cli/internal/workflows"

// registerProject adds a project to the global configuration after init.
// It persists its path and last-used timestamp.
func registerProject(generatorPath string, deps Dependencies) error {
	workflow := workflows.NewProjectRegisterWorkflow()
	return workflow.Run(workflows.ProjectRegisterRequest{
		GeneratorPath: generatorPath,
		Now:           now(deps),
	})
}
