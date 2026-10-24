package shell

import (
	"fmt"
	"strings"
)

// CompletionScript returns shell code, printed by "carrier init" after the
// automatic session snippet, that sets up tab completion for carrier and the
// "c" alias. script is cobra's completion script for the shell. Arguments
// after "carrier run" or "carrier watch" are completed as the wrapped command
// would be on its own, which cobra cannot do.
func CompletionScript(shellName, script string) (string, error) {
	switch shellName {
	case "zsh":
		// compdef only exists once compinit has run, which zshrc files often
		// do after this line, so setup waits for the first prompt if needed.
		// zsh expands aliases before completing, so "c" needs nothing extra.
		return fmt.Sprintf(`
# carrier: tab completion, also for the c alias.
__carrier_completion_setup() {
  (( $+functions[compdef] )) || return 0
  precmd_functions=(${precmd_functions:#__carrier_completion_setup})
%s
  compdef __carrier_complete carrier
  # Grouping runs with the same description sorts the menu by ID; keep
  # carrier's newest-first order instead.
  zstyle ':completion:*:*:carrier:*' list-grouped false
}
__carrier_complete() {
  local i=2
  while (( i < CURRENT )) && [[ ${words[i]} == -* ]]; do (( i++ )); done
  if (( i < CURRENT )) && [[ ${words[i]} == (run|r|watch|w) ]]; then
    (( i++ ))
    while (( i < CURRENT )); do
      case ${words[i]} in
        %s) (( i += 2 )) ;;
        -*) (( i++ )) ;;
        *) break ;;
      esac
    done
    (( i > CURRENT )) && return 1
    if (( i < CURRENT )) || [[ ${words[CURRENT]} != -* ]]; then
      words=("${(@)words[i,-1]}")
      (( CURRENT -= i - 1 ))
      _normal
      return
    fi
  fi
  _carrier "$@"
}
typeset -ga precmd_functions
precmd_functions+=(__carrier_completion_setup)
__carrier_completion_setup
`, indent(script, "  "), strings.Join(wrappedValueFlags, "|")), nil
	case "bash":
		// bash does not complete through aliases, so "c" is registered too,
		// unless it is already something other than carrier.
		return fmt.Sprintf(`
# carrier: tab completion, also for the c alias.
if ! declare -F _get_comp_words_by_ref >/dev/null; then
  # cobra's script needs this function from the bash-completion package.
  # Without the package, this stand-in sets cur, prev, words, and cword,
  # joining words that bash split at ":" or "=" back together.
  _get_comp_words_by_ref() {
    local -a _w=()
    local _i _c=0
    for (( _i = 0; _i < ${#COMP_WORDS[@]}; _i++ )); do
      if (( _i > 0 )) && [[ ${COMP_WORDS[_i]} == [:=]* || ${COMP_WORDS[_i-1]} == *[:=] ]] &&
         [[ ${COMP_LINE} == *"${COMP_WORDS[_i-1]}${COMP_WORDS[_i]}"* ]]; then
        _w[${#_w[@]}-1]+=${COMP_WORDS[_i]}
      else
        _w+=("${COMP_WORDS[_i]}")
      fi
      (( _i == COMP_CWORD )) && _c=$(( ${#_w[@]} - 1 ))
    done
    words=("${_w[@]}")
    cword=$_c
    cur=${_w[_c]}
    prev=${_w[_c-1]}
  }
fi
%s
__carrier_complete() {
  local i=1
  while (( i < COMP_CWORD )) && [[ ${COMP_WORDS[i]} == -* ]]; do (( i++ )); done
  if (( i < COMP_CWORD )); then
    case ${COMP_WORDS[i]} in
      run|r|watch|w)
        (( i++ ))
        while (( i < COMP_CWORD )); do
          case ${COMP_WORDS[i]} in
            %s) (( i += 2 )) ;;
            -*) (( i++ )) ;;
            *) break ;;
          esac
        done
        (( i > COMP_CWORD )) && return 0
        if (( i < COMP_CWORD )) || [[ ${COMP_WORDS[COMP_CWORD]} != -* ]]; then
          if declare -F _comp_command_offset >/dev/null; then
            _comp_command_offset "$i"
          elif declare -F _command_offset >/dev/null; then
            _command_offset "$i"
          elif (( i == COMP_CWORD )); then
            COMPREPLY=($(compgen -c -- "${COMP_WORDS[COMP_CWORD]}"))
          fi
          return 0
        fi
        ;;
    esac
  fi
  __start_carrier "$@"
}
complete -o default -F __carrier_complete carrier
if ! type -t c >/dev/null 2>&1 || [[ $(alias c 2>/dev/null) == *carrier* ]]; then
  complete -o default -F __carrier_complete c
fi
`, script, strings.Join(wrappedValueFlags, "|")), nil
	case "fish":
		// fish completes an alias like the command it wraps, so "c" needs
		// nothing extra.
		return fmt.Sprintf(`
# carrier: tab completion, also for the c alias.
%s
function __carrier_wrapped_command --description 'Print the command after carrier run or watch, if the cursor is in it'
    set -l tokens (commandline -opc)
    set -e tokens[1]
    while set -q tokens[1]; and string match -q -- '-*' $tokens[1]
        set -e tokens[1]
    end
    set -q tokens[1]; and contains -- $tokens[1] run r watch w; or return 1
    set -e tokens[1]
    while set -q tokens[1]
        switch $tokens[1]
            case %s
                set -e tokens[1]
                set -q tokens[1]; or return 1
                set -e tokens[1]
            case '-*'
                set -e tokens[1]
            case '*'
                break
        end
    end
    if not set -q tokens[1]; and string match -q -- '-*' (commandline -ct)
        return 1
    end
    # printf with no words would still print an empty line.
    set -q tokens[1]; and printf '%%s\n' $tokens
    return 0
end
complete -c carrier -n '__carrier_wrapped_command >/dev/null' -f -a '(complete -C (string join " " -- (__carrier_wrapped_command | string escape) (commandline -ct)))'
`, script, strings.Join(wrappedValueFlags, " ")), nil
	default:
		return "", fmt.Errorf("unsupported shell %q; use one of: %s", shellName, strings.Join(InitShells, ", "))
	}
}

// wrappedValueFlags are the flags of "carrier run" and "carrier watch" that
// take a value, so completion can find where the wrapped command starts.
var wrappedValueFlags = []string{"-L", "--label", "-t", "--timeout", "-p", "--pattern", "-d", "--debounce"}

func indent(text, prefix string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}
