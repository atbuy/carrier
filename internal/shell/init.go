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
// unset. A terminal that only inherited a recorded shell's variables, such as
// a tmux pane started from one, has a different terminal than CARRIER_TTY,
// so it is not inside that session. "carrier shell --auto" exits 0 when the recorded shell ends, so the
// outer shell exits with it; if carrier cannot start, it exits non-zero and
// the terminal carries on as a plain shell.
func InitScript(shellName, carrierPath string) (string, error) {
	quoted := shellQuote(carrierPath)
	switch shellName {
	case "zsh":
		return fmt.Sprintf(`# carrier: record this terminal in an automatic session.
if [[ -o interactive && -t 0 && -t 1 && -z "$CARRIER_NO_AUTO" &&
      ( -z "$CARRIER_SESSION_ID" || ( -n "$CARRIER_TTY" && "$CARRIER_TTY" != "$TTY" ) ) ]]; then
  %s shell --auto --program zsh && exit
fi
`, quoted), nil
	case "bash":
		return fmt.Sprintf(`# carrier: record this terminal in an automatic session.
if [[ $- == *i* && -t 0 && -t 1 && -z "$CARRIER_NO_AUTO" &&
      ( -z "$CARRIER_SESSION_ID" || ( -n "$CARRIER_TTY" && "$CARRIER_TTY" != "$(tty)" ) ) ]]; then
  %s shell --auto --program bash && exit
fi
`, quoted), nil
	case "fish":
		// "carrier init fish | source" runs this snippet with the pipe as its
		// standard input, so the recorded shell reads the terminal from
		// /dev/tty, and tty names it from standard error. Unlike "tty
		// </dev/tty", which names /dev/tty itself, that gives the name to
		// compare with CARRIER_TTY; when tty fails, nothing starts, so a
		// recorded shell never starts another inside itself.
		//
		// In fish, exit while reading config only stops reading it; the shell
		// keeps running. Leave at the first prompt instead. From fish 4, exit
		// in an event handler only ends the handler, so also queue the exit
		// input function, which older fish may not have.
		return fmt.Sprintf(`# carrier: record this terminal in an automatic session.
if status is-interactive; and isatty stdout; and isatty stderr; and test -r /dev/tty; and not set -q CARRIER_NO_AUTO; and begin; not set -q CARRIER_SESSION_ID; or begin; set -q CARRIER_TTY; and set -l __carrier_tty (tty <&2); and test "$CARRIER_TTY" != "$__carrier_tty"; end; end
    if %s shell --auto --program fish </dev/tty
        set -g fish_greeting
        function __carrier_auto_exit --on-event fish_prompt
            commandline -f exit 2>/dev/null
            exit
        end
    end
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
