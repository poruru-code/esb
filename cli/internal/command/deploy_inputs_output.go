// Where: cli/internal/command/deploy_inputs_output.go
// What: Output directory resolution helpers for deploy inputs.
// Why: Keep output path behavior isolated and easier to reason about.
package command

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/interaction"
	"github.com/poruru/edge-serverless-box/meta"
)

func resolveDeployOutput(
	value string,
	templatePath string,
	env string,
	isTTY bool,
	prompter interaction.Prompter,
	previous string,
) (string, error) {
	_ = templatePath
	_ = env

	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed, nil
	}
	if !isTTY || prompter == nil {
		return "", nil
	}
	if prev := strings.TrimSpace(previous); prev != "" {
		return prev, nil
	}
	return "", nil
}

func deriveMultiTemplateOutputDir(templatePath string, counts map[string]int) string {
	base := strings.TrimSpace(filepath.Base(templatePath))
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	stem = strings.TrimSpace(stem)
	if stem == "" {
		stem = "template"
	}
	count := counts[stem]
	counts[stem] = count + 1
	if count > 0 {
		stem = fmt.Sprintf("%s-%d", stem, count+1)
	}
	return filepath.Join(meta.OutputDir, stem)
}
