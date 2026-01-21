// Where: cli/internal/commands/complete.go
// What: Completion candidate provider for dynamic shell completion.
// Why: Supply env/project/service candidates without prompting.
package commands

import (
	"io"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/interaction"
)

// CompleteCmd defines hidden subcommands used by shell completion scripts.
type CompleteCmd struct {
	Env     CompleteEnvCmd     `cmd:"" help:"List environments for completion"`
	Project CompleteProjectCmd `cmd:"" help:"List projects for completion"`
}

type (
	CompleteEnvCmd     struct{}
	CompleteProjectCmd struct{}
)

func runCompleteEnv(cli CLI, deps Dependencies, out io.Writer) int {
	opts := resolveOptions{Interactive: false, Prompt: interaction.PromptYesNo}
	ctx, err := resolveEnvContext(cli, deps, opts)
	if err != nil {
		return 0
	}

	names := make([]string, 0, len(ctx.Project.Generator.Environments))
	for _, env := range ctx.Project.Generator.Environments {
		name := strings.TrimSpace(env.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	printCompletionList(out, names)
	return 0
}

func runCompleteProject(_ CLI, deps Dependencies, out io.Writer) int {
	cfg, err := loadGlobalConfigOrDefault(deps)
	if err != nil {
		return 0
	}

	names := make([]string, 0, len(cfg.Projects))
	for name := range cfg.Projects {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	printCompletionList(out, names)
	return 0
}

func printCompletionList(out io.Writer, items []string) {
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		writeLine(out, item)
	}
}
