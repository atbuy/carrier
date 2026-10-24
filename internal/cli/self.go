package cli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/changelog"
	"github.com/atbuy/carrier/internal/selfupdate"
	"github.com/atbuy/carrier/internal/version"
)

const selfUpdateTimeout = 5 * time.Minute

func (a *app) selfCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "self",
		Short: "update carrier, show its version and changes",
	}
	cmd.AddCommand(a.selfUpdateCmd(), a.versionCmd(), a.changelogCmd())
	return cmd
}

// deprecatedSelfAlias keeps a command that moved under "carrier self" working
// at its old top-level name. Command names are stable within 1.x (see
// docs/reference/compatibility.md), so the old name is hidden from help and
// warns on stderr here, and is removed in the next major release.
func deprecatedSelfAlias(moved *cobra.Command) *cobra.Command {
	name := moved.Name()
	moved.Hidden = true
	inner := moved.RunE
	moved.RunE = func(cmd *cobra.Command, args []string) error {
		printHint(cmd, fmt.Sprintf("%q is deprecated; use %q", "carrier "+name, "carrier self "+name))
		return inner(cmd, args)
	}
	return moved
}

func (a *app) selfUpdateCmd() *cobra.Command {
	var check, force bool
	var pinned string
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"upgrade"},
		Short:   "update carrier to the latest release",
		Long: `Replace this binary with a carrier release from GitHub.

  carrier self update                  # update to the latest release
  carrier self update --check          # report what is available, install nothing
  carrier self update --version 1.2.0  # install one release, including an older one
  carrier self update --force          # reinstall the version already installed

The download is checked against the release's checksums.txt before anything is
replaced, and the old binary stays in place if any step fails. Shells that are
already recording keep working and use the new binary from their next command.

This is the only command that reaches the network, and it only does so when you
run it. Binaries installed with "go install", built from source, or installed
somewhere this user cannot write are left alone, with the command to use
instead.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSelfUpdate(cmd, check, force, pinned)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "report whether a newer release exists, without installing it")
	cmd.Flags().StringVar(&pinned, "version", "", "install this release instead of the latest")
	cmd.Flags().BoolVar(&force, "force", false, "install even when it is not an upgrade")
	cmd.MarkFlagsMutuallyExclusive("check", "version")
	cmd.MarkFlagsMutuallyExclusive("check", "force")
	return cmd
}

func (a *app) runSelfUpdate(cmd *cobra.Command, check, force bool, pinned string) error {
	out := cmd.OutOrStdout()
	t := newTheme(out)
	current := version.Current().Version
	updater := selfupdate.New()

	// cmd.Context() is nil unless the command came through Execute.
	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, selfUpdateTimeout)
	defer cancel()

	wanted := selfupdate.NormalizeTag(pinned)
	if wanted != "" && !changelog.IsRelease(wanted) {
		return fmt.Errorf("invalid version %q; use a version like 1.3.1", pinned)
	}
	latest := ""
	if wanted == "" {
		var err error
		if latest, err = updater.LatestVersion(ctx); err != nil {
			return err
		}
		wanted = latest
	}

	if check {
		printUpdateCheck(cmd, t, current, latest)
		return nil
	}

	target, err := selfupdate.Locate()
	if err != nil {
		return err
	}
	if err := checkInstallMethod(target, current, force); err != nil {
		return err
	}
	if installed, stop := compareInstalled(cmd, current, wanted, force); stop {
		_, _ = fmt.Fprintf(out, "carrier %s is already the latest release\n", t.ID.Render(installed))
		printHint(cmd, "reinstall it with: carrier self update --force")
		return nil
	}

	_, _ = fmt.Fprintf(out, "downloading carrier %s...\n", t.ID.Render(plainVersion(wanted)))
	if err := updater.Install(ctx, wanted, target.Path); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(
		out, "%s %s %s %s\n",
		t.Success.Render("updated"),
		t.Muted.Render(collapseHome(target.Path)+":"),
		t.Muted.Render(plainVersion(current)+" →"),
		t.ID.Render(plainVersion(wanted)),
	)
	printChangesSince(cmd, target.Path, current)
	return nil
}

// printUpdateCheck reports what --check found. latest is empty when the user
// pinned a version, in which case there is nothing to compare against.
func printUpdateCheck(cmd *cobra.Command, t theme, current, latest string) {
	out := cmd.OutOrStdout()
	if latest == "" {
		_, _ = fmt.Fprintf(out, "carrier %s is installed\n", t.ID.Render(plainVersion(current)))
		return
	}
	if !changelog.IsRelease(current) {
		_, _ = fmt.Fprintf(out, "this is a %s build; %s is the latest release\n",
			t.Warning.Render(current), t.ID.Render(plainVersion(latest)))
		return
	}
	newer, err := changelog.Compare(latest, current)
	if err == nil && newer <= 0 {
		_, _ = fmt.Fprintf(out, "carrier %s is the latest release\n", t.ID.Render(plainVersion(current)))
		return
	}
	_, _ = fmt.Fprintf(out, "carrier %s is installed, %s is available\n",
		t.Muted.Render(plainVersion(current)), t.ID.Render(plainVersion(latest)))
	printHint(cmd, "update with: carrier self update")
}

// compareInstalled reports whether the wanted release is already installed, so
// the update can stop. It warns when the user pinned an older release, which
// is allowed.
func compareInstalled(cmd *cobra.Command, current, wanted string, force bool) (string, bool) {
	if !changelog.IsRelease(current) {
		return "", false
	}
	order, err := changelog.Compare(wanted, current)
	if err != nil {
		return "", false
	}
	switch {
	case order == 0 && !force:
		return plainVersion(current), true
	case order < 0:
		printHint(cmd, fmt.Sprintf("installing %s over %s, an older release", plainVersion(wanted), plainVersion(current)))
	}
	return "", false
}

// checkInstallMethod refuses to replace a binary that did not come from a
// release archive, naming the command to use instead. Replacing one would
// discard a source build or fight the tool that installed it.
func checkInstallMethod(target selfupdate.Target, current string, force bool) error {
	if target.GoBin {
		return fmt.Errorf("%s was installed with go install; update it with:\n  go install github.com/%s/cmd/carrier@latest",
			collapseHome(target.Path), selfupdate.Repo)
	}
	if !changelog.IsRelease(current) && !force {
		return fmt.Errorf("this is a %s build rather than a release; rebuild it from source with \"make install\", or pass --force to replace it with a release", current)
	}
	if !dirWritable(target.Dir()) {
		return fmt.Errorf("%s is not writable by this user; reinstall with:\n  curl -fsSL https://raw.githubusercontent.com/%s/main/install.sh | sh -s -- --system",
			collapseHome(target.Dir()), selfupdate.Repo)
	}
	return nil
}

// printChangesSince shows what the update brought by running the new binary:
// its changelog is built in, so only it knows the new entries. The output is
// buffered and dropped if the call fails, so a release too old to have this
// command does not spill an error over a successful update.
func printChangesSince(cmd *cobra.Command, exe, from string) {
	if !changelog.IsRelease(from) {
		return
	}
	var changes bytes.Buffer
	run := exec.Command(exe, "self", "changelog", "--since", plainVersion(from))
	run.Stdout = &changes
	if err := run.Run(); err != nil || changes.Len() == 0 {
		return
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	_, _ = changes.WriteTo(cmd.OutOrStdout())
}

// plainVersion drops the "v" that release tags carry, for display.
func plainVersion(version string) string {
	return strings.TrimPrefix(version, "v")
}
