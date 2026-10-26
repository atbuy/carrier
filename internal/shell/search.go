package shell

import (
	"fmt"
	"strings"

	"github.com/atbuy/carrier/internal/config"
)

// SearchBinding returns the shell config that binds key to carrier's history
// search: it opens "carrier search --pick" with the command line typed so
// far, and puts the command picked on the command line, without running it.
// The picker draws on the terminal through standard error, and prints the
// command picked on standard output; it exits non-zero when cancelled, which
// leaves the command line as it was. Bash needs 4.0 or newer to edit the
// command line from a binding, so older bash keeps its own binding.
func SearchBinding(shellName, carrierPath, key string) (string, error) {
	k, err := config.ParseKey(key)
	if err != nil {
		return "", err
	}
	quoted := shellQuote(carrierPath)
	letter := string(k.Letter)
	switch shellName {
	case "zsh":
		seq := "^" + strings.ToUpper(letter)
		if k.Alt {
			seq = "^[" + letter
		}
		return fmt.Sprintf(`# carrier: %[2]s searches the commands of every terminal.
_carrier_search_widget() {
  local picked
  picked=$(%[1]s search --pick -- "$BUFFER" </dev/tty 2>/dev/tty) && BUFFER=$picked && CURSOR=${#BUFFER}
  zle reset-prompt
}
zle -N _carrier_search_widget
bindkey '%[3]s' _carrier_search_widget
bindkey -M viins '%[3]s' _carrier_search_widget
`, quoted, key, seq), nil
	case "bash":
		seq := `\C-` + letter
		if k.Alt {
			seq = `\e` + letter
		}
		return fmt.Sprintf(`# carrier: %[2]s searches the commands of every terminal.
if (( BASH_VERSINFO[0] >= 4 )); then
  _carrier_search() {
    local picked
    picked=$(%[1]s search --pick -- "$READLINE_LINE" </dev/tty 2>/dev/tty) && READLINE_LINE=$picked && READLINE_POINT=${#READLINE_LINE}
  }
  bind -x '"%[3]s": _carrier_search'
fi
`, quoted, key, seq), nil
	case "fish":
		seq := `\c` + letter
		if k.Alt {
			seq = `\e` + letter
		}
		return fmt.Sprintf(`# carrier: %[2]s searches the commands of every terminal.
function _carrier_search
    set -l picked (%[1]s search --pick -- (commandline) </dev/tty 2>/dev/tty | string collect)
    and commandline -r -- $picked
    commandline -f repaint
end
bind %[3]s _carrier_search
bind -M insert %[3]s _carrier_search 2>/dev/null
`, quoted, key, seq), nil
	}
	return "", fmt.Errorf("unsupported shell %q; use one of: %s", shellName, strings.Join(InitShells, ", "))
}
