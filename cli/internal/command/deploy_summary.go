// Where: cli/internal/command/deploy_summary.go
// What: Deploy summary rendering and confirmation helpers.
// Why: Keep presentation logic separate from input derivation and execution.
package command

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
)

func confirmDeployInputs(inputs deployInputs, isTTY bool, prompter interaction.Prompter) (bool, error) {
	if !isTTY || prompter == nil {
		return true, nil
	}

	stackLine := ""
	if stack := strings.TrimSpace(inputs.TargetStack); stack != "" {
		stackLine = fmt.Sprintf("Target Stack: %s", stack)
	}
	projectLine := fmt.Sprintf("Project: %s", inputs.Project)
	if strings.TrimSpace(inputs.ProjectSource) != "" {
		projectLine = fmt.Sprintf("Project: %s (%s)", inputs.Project, inputs.ProjectSource)
	}
	envLine := fmt.Sprintf("Env: %s", inputs.Env)
	if strings.TrimSpace(inputs.EnvSource) != "" {
		envLine = fmt.Sprintf("Env: %s (%s)", inputs.Env, inputs.EnvSource)
	}
	summaryLines := make([]string, 0, 4)
	if stackLine != "" {
		summaryLines = append(summaryLines, stackLine)
	}
	summaryLines = append(summaryLines,
		projectLine,
		envLine,
		fmt.Sprintf("Mode: %s", inputs.Mode),
	)
	if len(inputs.Templates) == 1 {
		summaryLines = appendTemplateSummaryLines(summaryLines, inputs.Templates[0], inputs.Env, inputs.Project)
	} else if len(inputs.Templates) > 1 {
		summaryLines = append(summaryLines, fmt.Sprintf("Templates: %d", len(inputs.Templates)))
		for _, tpl := range inputs.Templates {
			summaryLines = appendTemplateSummaryLines(summaryLines, tpl, inputs.Env, inputs.Project)
		}
	}

	summary := "Review inputs:\n" + strings.Join(summaryLines, "\n")

	choice, err := prompter.SelectValue(
		summary,
		[]interaction.SelectOption{
			{Label: "Proceed", Value: "proceed"},
			{Label: "Edit", Value: "edit"},
		},
	)
	if err != nil {
		return false, fmt.Errorf("prompt confirmation: %w", err)
	}
	return choice == "proceed", nil
}

func appendTemplateSummaryLines(
	lines []string,
	tpl deployTemplateInput,
	envName string,
	project string,
) []string {
	output := domaincfg.ResolveOutputSummary(tpl.TemplatePath, tpl.OutputDir, envName)
	templateBase := filepath.Dir(tpl.TemplatePath)
	stagingDir := "<unresolved>"
	if dir, err := staging.ConfigDir(tpl.TemplatePath, project, envName); err == nil {
		stagingDir = dir
	}
	lines = append(lines,
		fmt.Sprintf("Template: %s", tpl.TemplatePath),
		fmt.Sprintf("Template base: %s", templateBase),
		fmt.Sprintf("Output: %s", output),
		fmt.Sprintf("Staging config: %s", stagingDir),
	)
	if len(tpl.Parameters) == 0 {
		return lines
	}
	keys := make([]string, 0, len(tpl.Parameters))
	for key := range tpl.Parameters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines = append(lines, "Parameters:")
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("  %s = %s", key, tpl.Parameters[key]))
	}
	return lines
}
