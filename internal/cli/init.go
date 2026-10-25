package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	carriershell "github.com/atbuy/carrier/internal/shell"
)

func (a *app) initCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "init [zsh|bash|fish]",
		Short:     "print shell config that records every terminal",
		ValidArgs: carriershell.InitShells,
		Long: `Print a snippet that starts every interactive terminal in an automatic
carrier session. Add one line to your shell config, after PATH is set:

  # ~/.zshrc
  eval "$(carrier init zsh)"

  # ~/.bashrc
  eval "$(carrier init bash)"

  # ~/.config/fish/config.fish
  carrier init fish | source

The snippet only runs for interactive terminals, never inside an existing
carrier session, and never when CARRIER_NO_AUTO is set:

  CARRIER_NO_AUTO=1 zsh    # one terminal without carrier

It also sets up tab completion for carrier and the "c" alias, so there is no
separate completion script to install.

If carrier cannot start, the terminal continues as a normal shell. Exiting the
recorded shell closes the terminal. Automatic sessions expire after
shell.auto_session_retention (default 30d) unless they have a label, notes,
or are kept with "carrier session keep".`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := carriershell.Supported(); err != nil {
				return err
			}
			name := ""
			if len(args) == 1 {
				name = args[0]
			} else {
				name = filepath.Base(os.Getenv("SHELL"))
			}
			carrierPath, err := os.Executable()
			if err != nil {
				return err
			}
			if resolved, err := filepath.EvalSymlinks(carrierPath); err == nil {
				carrierPath = resolved
			}
			script, err := carriershell.InitScript(name, carrierPath)
			if err != nil {
				if len(args) == 0 {
					return fmt.Errorf("could not tell your shell from $SHELL; run carrier init %s", strings.Join(carriershell.InitShells, "|"))
				}
				return err
			}
			completion, err := completionScript(cmd.Root(), name)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), script+completion)
			return err
		},
	}
}

// completionScript is the tab completion part of "carrier init": cobra's
// script for the shell, wrapped by carriershell.CompletionScript.
func completionScript(root *cobra.Command, shellName string) (string, error) {
	var buf bytes.Buffer
	var err error
	switch shellName {
	case "zsh":
		err = root.GenZshCompletion(&buf)
	case "bash":
		err = root.GenBashCompletionV2(&buf, true)
	case "fish":
		err = root.GenFishCompletion(&buf, true)
	}
	if err != nil {
		return "", err
	}
	return carriershell.CompletionScript(shellName, buf.String())
}
