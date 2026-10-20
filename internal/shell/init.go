package shell

import (
	"fmt"
	"strings"
)

// InitShells lists the shells "carrier init" supports.
var InitShells = []string{"zsh", "bash", "fish"}

// InitScript returns the shell config snippet that starts every interactive
// terminal inside an automatic carrier session. It runs carrier only when the
// shell is interactive, attached to a terminal, not already inside a carrier
// session (carrier shell loads the same config again), and CARRIER_NO_AUTO is
// unset. "carrier shell --auto" exits 0 when the recorded shell ends, so the
// outer shell exits with it; if carrier cannot start, it exits non-zero and
// the terminal carries on as a plain shell.
func InitScript(shellName, carrierPath string) (string, error) {
	quoted := shellQuote(carrierPath)
	switch shellName {
	case "zsh":
		return fmt.Sprintf(`# carrier: record this terminal in an automatic session.
if [[ -o interactive && -t 0 && -t 1 && -z "$CARRIER_SESSION_ID" && -z "$CARRIER_NO_AUTO" ]]; then
  %s shell --auto --program zsh && exit
fi
`, quoted), nil
	case "bash":
		return fmt.Sprintf(`# carrier: record this terminal in an automatic session.
if [[ $- == *i* && -t 0 && -t 1 && -z "$CARRIER_SESSION_ID" && -z "$CARRIER_NO_AUTO" ]]; then
  %s shell --auto --program bash && exit
fi
`, quoted), nil
	case "fish":
		return fmt.Sprintf(`# carrier: record this terminal in an automatic session.
if status is-interactive; and isatty stdin; and isatty stdout; and not set -q CARRIER_SESSION_ID; and not set -q CARRIER_NO_AUTO
    %s shell --auto --program fish; and exit
end
`, quoted), nil
	default:
		return "", fmt.Errorf("unsupported shell %q; use one of: %s", shellName, strings.Join(InitShells, ", "))
	}
}

// InitLine is the line users add to their shell config to load InitScript.
func InitLine(shellName string) string {
	if shellName == "fish" {
		return "carrier init fish | source"
	}
	return fmt.Sprintf(`eval "$(carrier init %s)"`, shellName)
}

// shellQuote quotes s for zsh, bash, and fish using single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
