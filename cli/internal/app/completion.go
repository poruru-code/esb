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
			subcommands[node.Name] = subs
		}
	}

	// Build case statements for subcommands
	var caseParts []string
	for cmd, subs := range subcommands {
		part := fmt.Sprintf(`        %s)
            COMPREPLY=( $(compgen -W "%s" -- "${cur}") )
            return 0
            ;;`, cmd, strings.Join(subs, " "))
		caseParts = append(caseParts, part)
	}

	script := `_esb_completion() {
    local cur prev opts cmd sub
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    cmd="${COMP_WORDS[1]}"
    sub="${COMP_WORDS[2]}"
    opts="%s"

    if [[ "${prev}" == "--env" || "${prev}" == "-e" ]]; then
        COMPREPLY=( $(compgen -W "$(_esb_complete env)" -- "${cur}") )
        return 0
    fi
    if [[ "${cmd}" == "env" && ( "${sub}" == "use" || "${sub}" == "remove" ) ]]; then
        COMPREPLY=( $(compgen -W "$(_esb_complete env)" -- "${cur}") )
        return 0
    fi
    if [[ "${cmd}" == "project" && ( "${sub}" == "use" || "${sub}" == "remove" ) ]]; then
        COMPREPLY=( $(compgen -W "$(_esb_complete project)" -- "${cur}") )
        return 0
    fi
    if [[ "${cmd}" == "logs" || ( "${cmd}" == "env" && "${sub}" == "var" ) ]]; then
        COMPREPLY=( $(compgen -W "$(_esb_complete service)" -- "${cur}") )
        return 0
    fi

    case "${prev}" in
%s
    esac

    COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
}
_esb_complete() {
    command esb __complete "$1" 2>/dev/null
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
		if node.Hidden || strings.HasPrefix(node.Name, "__") {
			continue
		}
		commands = append(commands, node.Name)
	}

	script := `#compdef esb
_esb_completion() {
    local -a commands
    commands=(
        %s
    )
    local prev="${words[$CURRENT-1]}"
    local cmd="${words[2]}"
    local sub="${words[3]}"
    if [[ "${prev}" == "--env" || "${prev}" == "-e" ]]; then
        _values 'environments' ${(f)"$(command esb __complete env 2>/dev/null)"}
        return
    fi
    if [[ "${cmd}" == "env" && ( "${sub}" == "use" || "${sub}" == "remove" ) ]]; then
        _values 'environments' ${(f)"$(command esb __complete env 2>/dev/null)"}
        return
    fi
    if [[ "${cmd}" == "project" && ( "${sub}" == "use" || "${sub}" == "remove" ) ]]; then
        _values 'projects' ${(f)"$(command esb __complete project 2>/dev/null)"}
        return
    fi
    if [[ "${cmd}" == "logs" || ( "${cmd}" == "env" && "${sub}" == "var" ) ]]; then
        _values 'services' ${(f)"$(command esb __complete service 2>/dev/null)"}
        return
    fi
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
		if node.Hidden || strings.HasPrefix(node.Name, "__") {
			continue
		}
		fmt.Fprintf(out, "complete -c esb -f -a %s -d '%s'\n", node.Name, node.Help)
	}
	fmt.Fprintln(out, "complete -c esb -f -l env -s e -r -a '(esb __complete env)' -d 'Environment'")
	fmt.Fprintln(out, "complete -c esb -f -n '__fish_seen_subcommand_from env; and __fish_seen_subcommand_from use remove' -a '(esb __complete env)'")
	fmt.Fprintln(out, "complete -c esb -f -n '__fish_seen_subcommand_from project; and __fish_seen_subcommand_from use remove' -a '(esb __complete project)'")
	fmt.Fprintln(out, "complete -c esb -f -n '__fish_seen_subcommand_from logs' -a '(esb __complete service)'")
	fmt.Fprintln(out, "complete -c esb -f -n '__fish_seen_subcommand_from env; and __fish_seen_subcommand_from var' -a '(esb __complete service)'")
	return 0
}
