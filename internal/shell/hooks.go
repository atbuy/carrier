package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteHookDir(carrierPath, statePath, shellProgram string, sessionID int64, label string) (string, error) {
	dir, err := os.MkdirTemp("", "carrier-shell-*")
	if err != nil {
		return "", err
	}
	name := filepath.Base(shellProgram)
	switch {
	case strings.Contains(name, "zsh"):
		return dir, os.WriteFile(filepath.Join(dir, ".zshrc"), []byte(zshHooks(carrierPath, statePath, sessionID, label)), 0o600)
	case strings.Contains(name, "bash"):
		return dir, os.WriteFile(filepath.Join(dir, ".bashrc"), []byte(bashHooks(carrierPath, statePath, sessionID, label)), 0o600)
	case strings.Contains(name, "fish"):
		return dir, os.WriteFile(filepath.Join(dir, "carrier.fish"), []byte(fishHooks(carrierPath, statePath, sessionID, label)), 0o600)
	default:
		return dir, os.WriteFile(filepath.Join(dir, ".zshrc"), []byte(zshHooks(carrierPath, statePath, sessionID, label)), 0o600)
	}
}

// termTitle returns the terminal window/tab title string for a session.
func termTitle(sessionID int64, label string) string {
	if label != "" {
		return fmt.Sprintf("carrier #%d · %s", sessionID, label)
	}
	return fmt.Sprintf("carrier #%d", sessionID)
}

// shellBanner returns a printf command that prints a one-line colored startup
// banner. Works in bash, zsh, and fish (all support printf with ANSI escapes).
func shellBanner(sessionID int64, label string) string {
	// accent blue bold + dim session ID + optional label
	line := fmt.Sprintf(`\033[38;2;91;141;239;1mcarrier\033[0m  \033[2msession #%d\033[0m`, sessionID)
	if label != "" {
		line += fmt.Sprintf(`  \033[38;2;122;173;244m%s\033[0m`, label)
	}
	return `printf '` + line + `\n'`
}

func zshHooks(carrierPath, statePath string, sessionID int64, label string) string {
	title := termTitle(sessionID, label)
	// _carrier_rprompt is embedded so no runtime env var lookup is needed.
	rprompt := fmt.Sprintf("%%F{75}carrier #%d%%f", sessionID)
	if label != "" {
		rprompt += fmt.Sprintf(" %%F{110}%s%%f", label)
	}
	return fmt.Sprintf(`
[[ -f "$HOME/.zshrc" ]] && source "$HOME/.zshrc"
unsetopt warn_create_global 2>/dev/null
autoload -Uz add-zsh-hook
_carrier_zle_init() {
  unsetopt warn_create_global 2>/dev/null
}
zle -N zle-line-init _carrier_zle_init
_carrier_preexec() {
  [[ -n "$CARRIER_HOOK_ACTIVE" ]] && return
  export CARRIER_HOOK_ACTIVE=1
  command %q internal begin --state %q --cmd "$1" --cwd "$PWD" >/dev/null 2>&1
  unset CARRIER_HOOK_ACTIVE
}
_carrier_precmd() {
  local ec="$?"
  [[ -n "$CARRIER_HOOK_ACTIVE" ]] && return
  export CARRIER_HOOK_ACTIVE=1
  command %q internal end --state %q --exit "$ec" >/dev/null 2>&1
  unset CARRIER_HOOK_ACTIVE
  printf '\033]0;%s\007'
  RPROMPT="%s${RPROMPT:+ $RPROMPT}"
}
add-zsh-hook preexec _carrier_preexec
add-zsh-hook precmd _carrier_precmd
%s
printf '\033]0;%s\007'
`, carrierPath, statePath, carrierPath, statePath, title, rprompt, shellBanner(sessionID, label), title)
}

func bashHooks(carrierPath, statePath string, sessionID int64, label string) string {
	title := termTitle(sessionID, label)
	// Static indicator embedded at hook-write time; no env var lookups at runtime.
	indicator := fmt.Sprintf(`\[\033[38;2;91;141;239m\]carrier #%d\[\033[0m\] `, sessionID)
	if label != "" {
		indicator += fmt.Sprintf(`\[\033[38;2;122;173;244m\]%s\[\033[0m\] `, label)
	}
	return fmt.Sprintf(`
[[ -f "$HOME/.bashrc" ]] && source "$HOME/.bashrc"
_carrier_base_ps1="${PS1}"
_carrier_original_prompt_command="${PROMPT_COMMAND:-}"
_carrier_begin() {
  [[ -n "$CARRIER_HOOK_ACTIVE" ]] && return
  [[ "$BASH_COMMAND" == _carrier_* ]] && return
  export CARRIER_HOOK_ACTIVE=1
  command %q internal begin --state %q --cmd "$BASH_COMMAND" --cwd "$PWD" >/dev/null 2>&1
  unset CARRIER_HOOK_ACTIVE
}
_carrier_end() {
  local ec="$?"
  trap - DEBUG
  if [[ -n "$_carrier_original_prompt_command" ]]; then
    eval "$_carrier_original_prompt_command"
    PS1="%s${PS1}"
  else
    PS1="%s${_carrier_base_ps1}"
  fi
  export CARRIER_HOOK_ACTIVE=1
  command %q internal end --state %q --exit "$ec" >/dev/null 2>&1
  unset CARRIER_HOOK_ACTIVE
  printf '\033]0;%s\007'
  trap _carrier_begin DEBUG
}
trap _carrier_begin DEBUG
PROMPT_COMMAND=_carrier_end
%s
printf '\033]0;%s\007'
`, carrierPath, statePath, indicator, indicator, carrierPath, statePath, title, shellBanner(sessionID, label), title)
}

func fishHooks(carrierPath, statePath string, sessionID int64, label string) string {
	title := termTitle(sessionID, label)
	// Build the carrier indicator string for fish_right_prompt.
	indicator := fmt.Sprintf("carrier #%d", sessionID)
	if label != "" {
		indicator += " · " + label
	}
	return fmt.Sprintf(`
function _carrier_preexec --on-event fish_preexec
    set -q CARRIER_HOOK_ACTIVE; and return
    set -xg CARRIER_HOOK_ACTIVE 1
    command %q internal begin --state %q --cmd "$argv" --cwd "$PWD" >/dev/null 2>&1
    set -eg CARRIER_HOOK_ACTIVE
end
function _carrier_postcmd --on-event fish_postexec
    set -l ec $status
    set -q CARRIER_HOOK_ACTIVE; and return
    set -xg CARRIER_HOOK_ACTIVE 1
    command %q internal end --state %q --exit $ec >/dev/null 2>&1
    set -eg CARRIER_HOOK_ACTIVE
    printf '\033]0;%s\007'
end
if functions -q fish_right_prompt
    functions --copy fish_right_prompt _carrier_orig_right_prompt
else
    function _carrier_orig_right_prompt; end
end
function fish_right_prompt
    set_color 5B8DEF
    echo -n '%s'
    set_color normal
    set -l _orig (_carrier_orig_right_prompt)
    if test -n "$_orig"
        echo -n " $_orig"
    end
end
%s
printf '\033]0;%s\007'
`, carrierPath, statePath, carrierPath, statePath, title, indicator, shellBanner(sessionID, label), title)
}
