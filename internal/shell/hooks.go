package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteHookDir(carrierPath, statePath, shellProgram string) (string, error) {
	dir, err := os.MkdirTemp("", "carrier-shell-*")
	if err != nil {
		return "", err
	}
	name := filepath.Base(shellProgram)
	switch {
	case strings.Contains(name, "zsh"):
		return dir, os.WriteFile(filepath.Join(dir, ".zshrc"), []byte(zshHooks(carrierPath, statePath)), 0o600)
	case strings.Contains(name, "bash"):
		return dir, os.WriteFile(filepath.Join(dir, ".bashrc"), []byte(bashHooks(carrierPath, statePath)), 0o600)
	default:
		return dir, os.WriteFile(filepath.Join(dir, ".zshrc"), []byte(zshHooks(carrierPath, statePath)), 0o600)
	}
}

func zshHooks(carrierPath, statePath string) string {
	return fmt.Sprintf(`
[[ -f "$HOME/.zshrc" ]] && source "$HOME/.zshrc"
unsetopt warn_create_global 2>/dev/null
autoload -Uz add-zsh-hook
_carrier_disable_prompt_warnings() {
  unsetopt warn_create_global 2>/dev/null
}
precmd_functions=(_carrier_disable_prompt_warnings ${precmd_functions:#_carrier_disable_prompt_warnings})
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
}
add-zsh-hook preexec _carrier_preexec
add-zsh-hook precmd _carrier_precmd
`, carrierPath, statePath, carrierPath, statePath)
}

func bashHooks(carrierPath, statePath string) string {
	return fmt.Sprintf(`
[[ -f "$HOME/.bashrc" ]] && source "$HOME/.bashrc"
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
  fi
  export CARRIER_HOOK_ACTIVE=1
  command %q internal end --state %q --exit "$ec" >/dev/null 2>&1
  unset CARRIER_HOOK_ACTIVE
  trap _carrier_begin DEBUG
}
trap _carrier_begin DEBUG
PROMPT_COMMAND=_carrier_end
`, carrierPath, statePath, carrierPath, statePath)
}
