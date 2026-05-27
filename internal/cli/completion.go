package cli

import (
	"github.com/spf13/cobra"
)

func (a *app) completionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "generate shell completion script",
		Long: `Generate a shell completion script for carrier.

BASH
  # Write once, then restart your shell:
  carrier completion bash > /etc/bash_completion.d/carrier

  # Or source per-session (add to ~/.bashrc for persistence):
  source <(carrier completion bash)

ZSH
  # Add to a directory in $fpath, then restart your shell:
  carrier completion zsh > "${fpath[1]}/_carrier"

  # If completions are not already enabled, add to ~/.zshrc:
  autoload -U compinit && compinit

FISH
  # Write once (fish loads completions automatically):
  carrier completion fish > ~/.config/fish/completions/carrier.fish

POWERSHELL
  # Add to your PowerShell profile ($PROFILE):
  carrier completion powershell | Out-String | Invoke-Expression`,
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			}
			return nil
		},
	}
	return cmd
}
