// Where: cli/internal/commands/completion.go
// What: Shell completion command implementation (build-only CLI).
// Why: Provide basic subcommand completion for bash, zsh, and fish.
package commands

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
	commands, subcommands := collectCompletionCommands(cli)

	var caseParts []string
	for cmd, subs := range subcommands {
		part := fmt.Sprintf(`        %s)
            COMPREPLY=( $(compgen -W "%s" -- "${cur}") )
            return 0
            ;;`, cmd, strings.Join(subs, " "))
		caseParts = append(caseParts, part)
	}

	script := `_esb_completion() {
    local cur cmd
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    cmd="${COMP_WORDS[1]}"

    case "${cmd}" in
%s
    esac

    if [[ ${COMP_CWORD} -le 1 ]]; then
        COMPREPLY=( $(compgen -W "%s" -- "${cur}") )
        return 0
    fi
}
complete -F _esb_completion esb
`
	writeString(out, fmt.Sprintf(script, strings.Join(caseParts, "\n"), strings.Join(commands, " ")))
	return 0
}

func runCompletionZsh(cli CLI, out io.Writer) int {
	commands, subcommands := collectCompletionCommands(cli)

	script := `#compdef esb
_esb_completion() {
  local -a commands
  commands=(%s)
  local cmd="${words[2]}"

  if [[ $CURRENT -eq 2 ]]; then
    _values 'commands' ${commands[@]}
    return
  fi

%s
}
_esb_completion "$@"
`

	var subBlocks strings.Builder
	for cmd, subs := range subcommands {
		subBlocks.WriteString(fmt.Sprintf(`  if [[ "${cmd}" == "%s" && $CURRENT -eq 3 ]]; then
    _values '%s' %s
    return
  fi
`, cmd, cmd, strings.Join(subs, " ")))
	}

	writeString(out, fmt.Sprintf(script, strings.Join(commands, " "), subBlocks.String()))
	return 0
}

func runCompletionFish(cli CLI, out io.Writer) int {
	commands, subcommands := collectCompletionCommands(cli)
	writeLine(out, fmt.Sprintf("complete -c esb -f -a \"%s\"", strings.Join(commands, " ")))
	for cmd, subs := range subcommands {
		writeLine(out, fmt.Sprintf("complete -c esb -f -n \"__fish_seen_subcommand_from %s\" -a \"%s\"", cmd, strings.Join(subs, " ")))
	}
	return 0
}

func collectCompletionCommands(cli CLI) ([]string, map[string][]string) {
	parser, _ := kong.New(&cli)

	var commands []string
	subcommands := make(map[string][]string)

	for _, node := range parser.Model.Children {
		if node.Hidden || strings.HasPrefix(node.Name, "__") {
			continue
		}
		commands = append(commands, node.Name)
		if len(node.Children) > 0 {
			var subs []string
			for _, sub := range node.Children {
				if sub.Hidden || strings.HasPrefix(sub.Name, "__") {
					continue
				}
				subs = append(subs, sub.Name)
			}
			if len(subs) > 0 {
				subcommands[node.Name] = subs
			}
		}
	}

	return commands, subcommands
}
