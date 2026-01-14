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

	script := `_esb_find_index() {
    local target="$1"
    local i
    for ((i=1; i<${#COMP_WORDS[@]}; i++)); do
        if [[ "${COMP_WORDS[i]}" == "${target}" ]]; then
            echo "${i}"
            return 0
        fi
    done
    return 1
}
_esb_has_positional_after() {
    local start="$1"
    local i word
    for ((i=start; i<COMP_CWORD; i++)); do
        word="${COMP_WORDS[i]}"
        if [[ -n "${word}" && "${word}" != -* ]]; then
            return 0
        fi
    done
    return 1
}
_esb_completion() {
    local cur prev opts cmd sub cmd_index sub_index arg_index
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
        cmd_index=$(_esb_find_index "env") || cmd_index=1
        sub_index=$((cmd_index+1))
        arg_index=$((sub_index+1))
        if _esb_has_positional_after "${arg_index}"; then
            return 0
        fi
        COMPREPLY=( $(compgen -W "$(_esb_complete env)" -- "${cur}") )
        return 0
    fi
    if [[ "${cmd}" == "project" && ( "${sub}" == "use" || "${sub}" == "remove" ) ]]; then
        cmd_index=$(_esb_find_index "project") || cmd_index=1
        sub_index=$((cmd_index+1))
        arg_index=$((sub_index+1))
        if _esb_has_positional_after "${arg_index}"; then
            return 0
        fi
        COMPREPLY=( $(compgen -W "$(_esb_complete project)" -- "${cur}") )
        return 0
    fi
    if [[ "${cmd}" == "logs" ]]; then
        cmd_index=$(_esb_find_index "logs") || cmd_index=1
        arg_index=$((cmd_index+1))
        if _esb_has_positional_after "${arg_index}"; then
            return 0
        fi
        COMPREPLY=( $(compgen -W "$(_esb_complete service)" -- "${cur}") )
        return 0
    fi
    if [[ "${cmd}" == "env" && "${sub}" == "var" ]]; then
        cmd_index=$(_esb_find_index "env") || cmd_index=1
        sub_index=$((cmd_index+1))
        arg_index=$((sub_index+1))
        if _esb_has_positional_after "${arg_index}"; then
            return 0
        fi
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
_esb_find_index() {
    local target="$1"
    local i
    for ((i=1; i<${#words[@]}; i++)); do
        if [[ "${words[i]}" == "${target}" ]]; then
            echo "${i}"
            return 0
        fi
    done
    return 1
}
_esb_has_positional_after() {
    local start="$1"
    local i word
    for ((i=start; i<CURRENT; i++)); do
        word="${words[i]}"
        if [[ -n "${word}" && "${word}" != -* ]]; then
            return 0
        fi
    done
    return 1
}
_esb_completion() {
    local -a commands
    commands=(
        %s
    )
    local prev="${words[$CURRENT-1]}"
    local cmd="${words[2]}"
    local sub="${words[3]}"
    local cmd_index sub_index arg_index
    if [[ "${prev}" == "--env" || "${prev}" == "-e" ]]; then
        _values 'environments' ${(f)"$(command esb __complete env 2>/dev/null)"}
        return
    fi
    if [[ "${cmd}" == "env" && ( "${sub}" == "use" || "${sub}" == "remove" ) ]]; then
        cmd_index=$(_esb_find_index "env") || cmd_index=2
        sub_index=$((cmd_index+1))
        arg_index=$((sub_index+1))
        if _esb_has_positional_after "${arg_index}"; then
            return
        fi
        _values 'environments' ${(f)"$(command esb __complete env 2>/dev/null)"}
        return
    fi
    if [[ "${cmd}" == "project" && ( "${sub}" == "use" || "${sub}" == "remove" ) ]]; then
        cmd_index=$(_esb_find_index "project") || cmd_index=2
        sub_index=$((cmd_index+1))
        arg_index=$((sub_index+1))
        if _esb_has_positional_after "${arg_index}"; then
            return
        fi
        _values 'projects' ${(f)"$(command esb __complete project 2>/dev/null)"}
        return
    fi
    if [[ "${cmd}" == "logs" ]]; then
        cmd_index=$(_esb_find_index "logs") || cmd_index=2
        arg_index=$((cmd_index+1))
        if _esb_has_positional_after "${arg_index}"; then
            return
        fi
        _values 'services' ${(f)"$(command esb __complete service 2>/dev/null)"}
        return
    fi
    if [[ "${cmd}" == "env" && "${sub}" == "var" ]]; then
        cmd_index=$(_esb_find_index "env") || cmd_index=2
        sub_index=$((cmd_index+1))
        arg_index=$((sub_index+1))
        if _esb_has_positional_after "${arg_index}"; then
            return
        fi
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
	fmt.Fprintln(out, "function __esb_has_positional_after")
	fmt.Fprintln(out, "    set -l cmd $argv[1]")
	fmt.Fprintln(out, "    set -l sub $argv[2]")
	fmt.Fprintln(out, "    set -l tokens (commandline -opc)")
	fmt.Fprintln(out, "    set -l current (commandline -ct)")
	fmt.Fprintln(out, "    if test (count $tokens) -gt 0")
	fmt.Fprintln(out, "        if test \"$tokens[-1]\" = \"$current\"")
	fmt.Fprintln(out, "            set -e tokens[-1]")
	fmt.Fprintln(out, "        end")
	fmt.Fprintln(out, "    end")
	fmt.Fprintln(out, "    set -l found 0")
	fmt.Fprintln(out, "    for tok in $tokens")
	fmt.Fprintln(out, "        if test $found -eq 0")
	fmt.Fprintln(out, "            if test \"$tok\" = \"$cmd\"")
	fmt.Fprintln(out, "                set found 1")
	fmt.Fprintln(out, "                if test -z \"$sub\"")
	fmt.Fprintln(out, "                    set found 2")
	fmt.Fprintln(out, "                end")
	fmt.Fprintln(out, "            end")
	fmt.Fprintln(out, "        else if test $found -eq 1")
	fmt.Fprintln(out, "            if test \"$tok\" = \"$sub\"")
	fmt.Fprintln(out, "                set found 2")
	fmt.Fprintln(out, "            end")
	fmt.Fprintln(out, "        else")
	fmt.Fprintln(out, "            if not string match -r '^-' -- \"$tok\"")
	fmt.Fprintln(out, "                return 0")
	fmt.Fprintln(out, "            end")
	fmt.Fprintln(out, "        end")
	fmt.Fprintln(out, "    end")
	fmt.Fprintln(out, "    return 1")
	fmt.Fprintln(out, "end")
	for _, node := range parser.Model.Children {
		if node.Hidden || strings.HasPrefix(node.Name, "__") {
			continue
		}
		fmt.Fprintf(out, "complete -c esb -f -a %s -d '%s'\n", node.Name, node.Help)
	}
	fmt.Fprintln(out, "complete -c esb -f -l env -s e -r -a '(esb __complete env)' -d 'Environment'")
	fmt.Fprintln(out, "complete -c esb -f -n '__fish_seen_subcommand_from env; and __fish_seen_subcommand_from use remove; and not __esb_has_positional_after env use; and not __esb_has_positional_after env remove' -a '(esb __complete env)'")
	fmt.Fprintln(out, "complete -c esb -f -n '__fish_seen_subcommand_from project; and __fish_seen_subcommand_from use remove; and not __esb_has_positional_after project use; and not __esb_has_positional_after project remove' -a '(esb __complete project)'")
	fmt.Fprintln(out, "complete -c esb -f -n '__fish_seen_subcommand_from logs; and not __esb_has_positional_after logs \"\"' -a '(esb __complete service)'")
	fmt.Fprintln(out, "complete -c esb -f -n '__fish_seen_subcommand_from env; and __fish_seen_subcommand_from var; and not __esb_has_positional_after env var' -a '(esb __complete service)'")
	return 0
}
