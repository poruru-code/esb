// Where: cli/internal/app/completion.go
// What: Shell completion command implementation.
// Why: Provide tab completion for bash, zsh, and fish.
package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"
)

// CompletionCmd defines the structure for the completion command.
type CompletionCmd struct {
	Bash CompletionBashCmd `cmd:"" help:"Generate bash completion script"`
	Zsh  CompletionZshCmd  `cmd:"" help:"Generate zsh completion script"`
	Fish CompletionFishCmd `cmd:"" help:"Generate fish completion script"`
}

type (
	CompletionBashCmd struct{}
	CompletionZshCmd  struct{}
	CompletionFishCmd struct{}
)

func runCompletionBash(cli CLI, out io.Writer) int {
	parser, _ := kong.New(&cli)

	var commands []string
	subcommands := make(map[string][]string)

	for _, node := range parser.Model.Children {
		if node.Hidden {
			continue
		}
		commands = append(commands, node.Name)
		if len(node.Children) > 0 {
			var subs []string
			for _, sub := range node.Children {
				if !sub.Hidden {
					subs = append(subs, sub.Name)
				}
			}
			subcommands[node.Name] = subs
		}
	}

	// Build case statements for subcommands
	var caseParts []string
	for cmd, subs := range subcommands {
		part := fmt.Sprintf(`        %s)
            COMPREPLY=( $(compgen -W "%s" -- ${cur}) )
            return 0
            ;;`, cmd, strings.Join(subs, " "))
		caseParts = append(caseParts, part)
	}

	script := `_esb_completion() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    opts="%s"

    case "${prev}" in
%s
    esac

    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
}
complete -F _esb_completion esb
`
	fmt.Fprintf(out, script, strings.Join(commands, " "), strings.Join(caseParts, "\n"))
	return 0
}

func runCompletionZsh(cli CLI, out io.Writer) int {
	parser, _ := kong.New(&cli)
	var commands []string
	for _, node := range parser.Model.Children {
		if !node.Hidden {
			commands = append(commands, node.Name)
		}
	}

	script := `#compdef esb
_esb_completion() {
    local -a commands
    commands=(
        %s
    )
    _describe 'commands' commands
}
compdef _esb_completion esb
`
	fmt.Fprintf(out, script, strings.Join(commands, "\n        "))
	return 0
}

func runCompletionFish(cli CLI, out io.Writer) int {
	parser, _ := kong.New(&cli)
	for _, node := range parser.Model.Children {
		if !node.Hidden {
			fmt.Fprintf(out, "complete -c esb -f -a %s -d '%s'\n", node.Name, node.Help)
		}
	}
	return 0
}
