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
autoload -Uz add-zsh-hook
_carrier_preexec() {
  command %q internal begin --state %q --cmd "$1" --cwd "$PWD" >/dev/null 2>&1
}
_carrier_precmd() {
  local ec="$?"
  command %q internal end --state %q --exit "$ec" >/dev/null 2>&1
}
add-zsh-hook preexec _carrier_preexec
add-zsh-hook precmd _carrier_precmd
`, carrierPath, statePath, carrierPath, statePath)
}

func bashHooks(carrierPath, statePath string) string {
	return fmt.Sprintf(`
[[ -f "$HOME/.bashrc" ]] && source "$HOME/.bashrc"
_carrier_current=
trap '_carrier_current="$BASH_COMMAND"; command %q internal begin --state %q --cmd "$BASH_COMMAND" --cwd "$PWD" >/dev/null 2>&1' DEBUG
PROMPT_COMMAND='ec=$?; trap - DEBUG; command %q internal end --state %q --exit "$ec" >/dev/null 2>&1; trap '\''_carrier_current="$BASH_COMMAND"; command %q internal begin --state %q --cmd "$BASH_COMMAND" --cwd "$PWD" >/dev/null 2>&1'\'' DEBUG'
`, carrierPath, statePath, carrierPath, statePath, carrierPath, statePath)
}
