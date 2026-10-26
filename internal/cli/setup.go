package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/duration"
	"github.com/atbuy/carrier/internal/notify"
	carriershell "github.com/atbuy/carrier/internal/shell"
)

const setupConfigHeader = "# carrier configuration. Run \"carrier config show\" to see every setting.\n"

// setupEnv holds what the setup wizard reads and writes, so tests can run it
// against a temporary home directory with scripted answers.
type setupEnv struct {
	in              *bufio.Reader
	out             io.Writer
	t               theme
	dryRun          bool
	home            string
	shell           string // zsh, bash, fish, or "" when unsupported
	configPath      string
	now             func() time.Time
	notifyAvailable func() bool
	notifyTest      func() error
	lookPath        func(string) (string, error)
	// shellsSupported reports whether recorded shells work here; nil means
	// they do.
	shellsSupported func() error

	cfg     config.Config
	changed []string // files written
}

func (a *app) setupCmd() *cobra.Command {
	var dryRun bool
	var shellName string
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "set carrier up step by step",
		Long: `Walk through carrier's main settings one step at a time:

  1. record every terminal in an automatic session
  2. similar-failure hints
  3. desktop notifications
  4. the "c" alias

Each step checks what is already set up and skips it if nothing needs to
change. Before writing, it shows the exact change to each file and asks for
confirmation. Existing files are edited in place (comments and other settings
are kept) and backed up first.

  carrier setup
  carrier setup --dry-run   # show what would change, write nothing`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun && !term.IsTerminal(int(os.Stdin.Fd())) {
				return errors.New("setup asks questions; run it in a terminal, or use --dry-run to see what it would change")
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			configPath, err := config.Path()
			if err != nil {
				return err
			}
			if shellName == "" {
				shellName = filepath.Base(os.Getenv("SHELL"))
			}
			env := &setupEnv{
				in: bufio.NewReader(cmd.InOrStdin()), out: cmd.OutOrStdout(), t: newTheme(cmd.OutOrStdout()),
				dryRun: dryRun, home: home, shell: supportedShell(shellName), configPath: configPath,
				now: time.Now, notifyAvailable: notify.Available, notifyTest: notify.Test, lookPath: exec.LookPath,
				shellsSupported: carriershell.Supported,
			}
			return env.run()
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the changes each step would make without writing anything")
	cmd.Flags().StringVar(&shellName, "shell", "", "shell to set up: zsh, bash, or fish (default: from $SHELL)")
	return cmd
}

func supportedShell(name string) string {
	for _, s := range carriershell.InitShells {
		if name == s {
			return s
		}
	}
	return ""
}

func (e *setupEnv) run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("your carrier config has an error, so setup will not change it: %w\nfix it (see \"carrier config check\") and run setup again", err)
	}
	e.cfg = cfg

	header := []string{e.t.Bold.Render("carrier setup")}
	if _, err := os.Stat(e.configPath); err == nil {
		header = append(header, e.t.Muted.Render("config: "+collapseHome(e.configPath)))
	} else {
		header = append(header, e.t.Muted.Render("config: "+collapseHome(e.configPath)+" (created if a step changes a setting)"))
	}
	if e.dryRun {
		header = append(header, e.t.Warning.Render("dry run: nothing will be written"))
	}
	besideLogo(e.out, logo(e.out), 5, header...)

	steps := []struct {
		title string
		run   func() error
	}{
		{"Record every terminal", e.stepAutoSessions},
		{"History search", e.stepHistorySearch},
		{"Similar-failure hints", e.stepHints},
		{"Desktop notifications", e.stepNotify},
		{`The "c" alias`, e.stepAlias},
	}
	for i, step := range steps {
		e.printf("\n%s\n", e.t.Accent.Bold(true).Render(fmt.Sprintf("Step %d/%d: %s", i+1, len(steps), step.title)))
		if err := step.run(); err != nil {
			return err
		}
	}

	e.printf("\n")
	if len(e.changed) == 0 {
		e.printf("%s\n", e.t.Success.Render("Done. Nothing was changed."))
		return nil
	}
	e.printf("%s\n", e.t.Success.Render("Done. Changed:"))
	for _, path := range e.changed {
		e.printf("  %s\n", collapseHome(path))
	}
	e.printf("Open a new terminal to pick up shell changes, and run \"carrier doctor\" to check everything.\n")
	return nil
}

// --- steps -----------------------------------------------------------------

func (e *setupEnv) stepAutoSessions() error {
	if e.shellsSupported != nil {
		if err := e.shellsSupported(); err != nil {
			e.skip(err.Error())
			return nil
		}
	}
	rc, ok := e.shellConfig()
	if !ok {
		return nil
	}
	content, _ := os.ReadFile(rc)
	// Recording every terminal takes the "carrier init" line, which also
	// sets up tab completion, and shell.auto_sessions turned on.
	hasLine := strings.Contains(string(content), "carrier init")
	switch {
	case hasLine && e.cfg.Shell.AutoSessions:
		e.done("already on in " + collapseHome(rc))
	case hasLine:
		e.printf("%s has the \"carrier init\" line, but shell.auto_sessions is off, so new\n", collapseHome(rc))
		e.printf("terminals are not recorded.\n")
		if !e.ask("Record every terminal?", true) {
			return nil
		}
		if err := e.setConfig("shell", "auto_sessions", "true"); err != nil {
			return err
		}
	default:
		e.printf("Start every new terminal in an automatic session, so commands and output\n")
		e.printf("are recorded without running \"carrier shell\" first. This also sets up tab\n")
		e.printf("completion for carrier and the \"c\" alias.\n")
		if !e.ask("Record every terminal?", true) {
			return nil
		}
		added := appendBlock(content, "Added by carrier setup: record every terminal in a carrier session.", carriershell.InitLine(e.shell))
		applied, err := e.proposeEdit(rc, content, added, nil)
		if err != nil {
			return err
		}
		if !applied && !e.dryRun {
			return nil
		}
		if !e.cfg.Shell.AutoSessions {
			if err := e.setConfig("shell", "auto_sessions", "true"); err != nil {
				return err
			}
		}
	}
	if !e.cfg.Shell.AutoSessions && !e.dryRun {
		return nil
	}

	current := e.cfg.Shell.AutoSessionRetention
	e.printf("Automatic sessions without a label, notes, or \"carrier session keep\" are deleted\n")
	e.printf("this long after they end (\"0\" keeps them forever).\n")
	value := e.askValue("Keep automatic sessions for", current, func(s string) error {
		_, err := config.ParseRetention(s)
		return err
	})
	if value == current {
		return nil
	}
	return e.setConfig("shell", "auto_session_retention", strconv.Quote(value))
}

// stepHistorySearch offers a key, Ctrl+R by default, that searches the
// commands of every terminal. The binding comes with the "carrier init" line.
func (e *setupEnv) stepHistorySearch() error {
	if e.shell == "" {
		e.skip(fmt.Sprintf("your shell is not supported (use --shell %s)", strings.Join(carriershell.InitShells, "|")))
		return nil
	}
	key := e.cfg.Shell.HistorySearchKey
	if e.cfg.Shell.HistorySearch {
		e.done("on, with " + key)
		return nil
	}
	rc := shellConfigFiles(e.home)[e.shell]
	if content, _ := os.ReadFile(rc); !strings.Contains(string(content), "carrier init") && !e.dryRun {
		e.skip(`needs the "carrier init" line from the step before`)
		return nil
	}
	e.printf("Press %s to search the commands of every terminal, and put the one you pick\n", key)
	e.printf("on the command line. It replaces the shell's own %s search, and fzf's or\n", key)
	e.printf("atuin's if you use them; shell.history_search_key picks another key.\n")
	if !e.ask("Search history with "+key+"?", false) {
		return nil
	}
	return e.setConfig("shell", "history_search", "true")
}

func (e *setupEnv) stepHints() error {
	if e.cfg.Hints.SimilarFailures {
		e.done("already on")
		return nil
	}
	e.printf("When a command fails with an error seen before, point at the earlier run\n")
	e.printf("and its notes, e.g. \"seen before in run 42 (3 weeks ago, 2 notes)\".\n")
	if !e.ask("Turn on similar-failure hints?", true) {
		return nil
	}
	return e.setConfig("hints", "similar_failures", "true")
}

func (e *setupEnv) stepNotify() error {
	if !e.notifyAvailable() {
		e.skip("not available on this system (needs notify-send on Linux)")
		return nil
	}
	e.printf("Notifications are sent for runs started with -n (or -N for every run).\n")
	if e.ask("Send a test notification now?", true) && !e.dryRun {
		if err := e.notifyTest(); err != nil {
			e.printf("%s\n", e.t.Warning.Render("could not send a notification: "+err.Error()))
		} else {
			e.printf("%s\n", e.t.Muted.Render("sent; check that it appeared"))
		}
	}
	if !e.cfg.Notify.Shell && (e.shellsSupported == nil || e.shellsSupported() == nil) {
		e.printf("Recorded shells can notify for every long command too, without -n; editors,\n")
		e.printf("pagers, ssh, and commands stopped with Ctrl+C never notify.\n")
		if e.ask("Notify for long commands in recorded shells?", false) {
			if err := e.setConfig("notify", "shell", "true"); err != nil {
				return err
			}
		}
	}
	current := e.cfg.Notify.MinDuration
	prompt := "With -n, notify for commands that run longer than"
	if e.cfg.Notify.Shell {
		prompt = "Notify for commands that run longer than"
	}
	value := e.askValue(prompt, current, func(s string) error {
		_, err := duration.Parse(s)
		return err
	})
	if value == current {
		return nil
	}
	return e.setConfig("notify", "min_duration", strconv.Quote(value))
}

var (
	carrierAliasLine = regexp.MustCompile(`(?m)^\s*alias\s+c(=|\s+)['"]?carrier\b`)
	otherCDefinition = regexp.MustCompile(`(?m)^\s*(alias\s+c(=|\s)|abbr\b.*\sc\s|(function\s+)?c\s*(\(\s*\))?\s*\{|function\s+c\s*$).*$`)
)

func (e *setupEnv) stepAlias() error {
	rc, ok := e.shellConfig()
	if !ok {
		return nil
	}
	content, _ := os.ReadFile(rc)
	if carrierAliasLine.Match(content) {
		e.done("already set in " + collapseHome(rc))
		return nil
	}
	if line := otherCDefinition.Find(content); line != nil {
		e.skip(fmt.Sprintf("\"c\" is already defined in %s: %s", collapseHome(rc), strings.TrimSpace(string(line))))
		return nil
	}
	if path, err := e.lookPath("c"); err == nil {
		e.skip("\"c\" is already a command: " + path)
		return nil
	}
	e.printf("Use \"c\" as a short name for carrier, e.g. \"c run make test\".\n")
	if !e.ask(`Add the "c" alias?`, true) {
		return nil
	}
	alias := "alias c='carrier'"
	if e.shell == "fish" {
		alias = "alias c carrier"
	}
	_, err := e.proposeEdit(rc, content, appendBlock(content, "Added by carrier setup: short name for carrier.", alias), nil)
	return err
}

// --- editing ---------------------------------------------------------------

// shellConfig returns the config file of the shell being set up.
func (e *setupEnv) shellConfig() (string, bool) {
	if e.shell == "" {
		e.skip(fmt.Sprintf("your shell is not supported (use --shell %s)", strings.Join(carriershell.InitShells, "|")))
		return "", false
	}
	return shellConfigFiles(e.home)[e.shell], true
}

// setConfig sets section.key in the config file whose value takes effect.
// Includes override the main config, so when an include sets the key, that
// include is edited instead.
func (e *setupEnv) setConfig(section, key, value string) error {
	target := e.configPath
	if _, err := os.Stat(e.configPath); err == nil {
		defined, err := config.DefinedIn(e.configPath, section, key)
		if err != nil {
			return err
		}
		if defined != e.configPath {
			e.printf("%s\n", e.t.Muted.Render(fmt.Sprintf("%s.%s is set in %s, which overrides the main config; changing it there.", section, key, collapseHome(defined))))
		}
		target = defined
	}
	old, err := os.ReadFile(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	base := old
	if len(base) == 0 {
		base = []byte(setupConfigHeader)
	}
	updated, err := config.SetValue(base, section, key, value)
	if err != nil {
		e.printf("%s\n", e.t.Warning.Render("skipped: "+err.Error()))
		return nil
	}
	_, err = e.proposeEdit(target, old, updated, e.validateConfig)
	return err
}

// validateConfig loads the whole config after a write; a config that no
// longer loads or fails "config check" with an error is rolled back.
func (e *setupEnv) validateConfig() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	for _, issue := range config.Check(cfg) {
		if issue.Level == config.IssueError {
			return fmt.Errorf("%s: %s", issue.Field, issue.Message)
		}
	}
	e.cfg = cfg
	return nil
}

// proposeEdit shows the change to path and, once confirmed, writes it with a
// backup. validate, when set, runs after writing; if it fails the previous
// content is restored. It reports whether the change was written.
func (e *setupEnv) proposeEdit(path string, old, updated []byte, validate func() error) (bool, error) {
	e.printf("\n%s\n", e.t.Bold.Render(collapseHome(path)))
	if target, err := filepath.EvalSymlinks(path); err == nil && target != path {
		e.printf("%s\n", e.t.Muted.Render("links to "+collapseHome(target)+"; that file is changed"))
	}
	e.printf("%s", e.renderDiff(string(old), string(updated)))
	if e.dryRun {
		e.printf("%s\n", e.t.Muted.Render("dry run: not written"))
		return false, nil
	}
	if !e.ask("Apply this change?", true) {
		e.printf("%s\n", e.t.Muted.Render("not changed"))
		return false, nil
	}
	backup, err := writeFileSafely(path, updated, e.backupDir(path), e.now())
	if err != nil {
		return false, err
	}
	if validate != nil {
		if verr := validate(); verr != nil {
			if rerr := restoreFile(path, backup); rerr != nil {
				return false, fmt.Errorf("change broke the config (%v) and restoring it failed: %w", verr, rerr)
			}
			e.printf("%s\n", e.t.Warning.Render("change undone, it made the config invalid: "+verr.Error()))
			return false, nil
		}
	}
	if backup != "" {
		e.printf("%s\n", e.t.Muted.Render("saved; backup at "+collapseHome(backup)))
	} else {
		e.printf("%s\n", e.t.Muted.Render("saved"))
	}
	if !slices.Contains(e.changed, path) {
		e.changed = append(e.changed, path)
	}
	return true, nil
}

// backupDir is where the backup of path goes. Carrier config files are backed
// up into a separate directory: a backup next to an include could itself match
// an includes pattern and override the change. Shell configs keep their
// backups next to them.
func (e *setupEnv) backupDir(path string) string {
	configDir := filepath.Dir(e.configPath)
	if rel, err := filepath.Rel(configDir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join(configDir, "backups")
	}
	return ""
}

// writeFileSafely backs up path (when it exists) and replaces its content
// atomically. The backup goes into backupDir, or next to the file when
// backupDir is "", and never overwrites an earlier backup. Symlinks are
// followed, so a link into a dotfiles repository keeps pointing at the edited
// file. It returns the backup path, or "" for a new file.
func writeFileSafely(path string, data []byte, backupDir string, now time.Time) (string, error) {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	mode := os.FileMode(0o644)
	backup := ""
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
		old, err := os.ReadFile(target)
		if err != nil {
			return "", err
		}
		backup, err = newBackupPath(target, backupDir, now)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(backup, old, mode); err != nil {
			return "", fmt.Errorf("backing up %s: %w", target, err)
		}
	} else if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".carrier-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return backup, os.Rename(tmp.Name(), target)
}

// newBackupPath returns an unused backup path for target such as
// ".bashrc.bak-20261020-150405", adding "-2", "-3", ... when needed.
func newBackupPath(target, dir string, now time.Time) (string, error) {
	if dir == "" {
		dir = filepath.Dir(target)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := filepath.Join(dir, filepath.Base(target)+".bak-"+now.Format("20060102-150405"))
	candidate := base
	for n := 2; ; n++ {
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
	}
}

// restoreFile puts back the backup taken by writeFileSafely, or removes a
// file that did not exist before.
func restoreFile(path, backup string) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	if backup == "" {
		return os.Remove(target)
	}
	return os.Rename(backup, target)
}

// appendBlock adds a commented block at the end of a shell config, leaving
// the existing content untouched.
func appendBlock(content []byte, comment, line string) []byte {
	out := string(content)
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if out != "" {
		out += "\n"
	}
	return []byte(out + "# " + comment + "\n" + line + "\n")
}

// --- prompts and output ----------------------------------------------------

func (e *setupEnv) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(e.out, format, args...)
}

func (e *setupEnv) done(msg string) {
	e.printf("%s %s\n", e.t.Success.Render("✓"), msg)
}

func (e *setupEnv) skip(msg string) {
	e.printf("%s %s\n", e.t.Muted.Render("-"), e.t.Muted.Render("skipped: "+msg))
}

// ask asks a yes/no question. Enter picks def; end of input means no. In a
// dry run it answers def without asking, so every step shows its change.
func (e *setupEnv) ask(question string, def bool) bool {
	hint := "[Y/n]"
	if !def {
		hint = "[y/N]"
	}
	if e.dryRun {
		e.printf("%s %s %s\n", question, hint, e.t.Muted.Render("(dry run: assuming the default)"))
		return def
	}
	for {
		e.printf("%s %s ", question, hint)
		line, err := e.in.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		switch {
		case answer == "" && err == nil:
			return def
		case answer == "y" || answer == "yes":
			return true
		case answer == "n" || answer == "no":
			return false
		case err != nil:
			e.printf("\n")
			return false
		}
		e.printf("please answer y or n\n")
	}
}

// askValue asks for a value, keeping current when the answer is empty.
func (e *setupEnv) askValue(question, current string, validate func(string) error) string {
	if e.dryRun {
		e.printf("%s [%s]: %s\n", question, current, e.t.Muted.Render("(dry run: keeping it)"))
		return current
	}
	for {
		e.printf("%s [%s]: ", question, current)
		line, err := e.in.ReadString('\n')
		value := strings.TrimSpace(line)
		if value == "" {
			if err != nil {
				e.printf("\n")
			}
			return current
		}
		if verr := validate(value); verr != nil {
			e.printf("%s\n", e.t.Warning.Render("invalid: "+verr.Error()))
			if err != nil {
				return current
			}
			continue
		}
		return value
	}
}

func (e *setupEnv) renderDiff(old, updated string) string {
	var b strings.Builder
	for _, l := range lineDiff(old, updated, 2) {
		switch l.op {
		case '+':
			b.WriteString(e.t.Success.Render("+ "+l.text) + "\n")
		case '-':
			b.WriteString(e.t.Danger.Render("- "+l.text) + "\n")
		case '~':
			b.WriteString(e.t.Muted.Render("  ...") + "\n")
		default:
			b.WriteString(e.t.Muted.Render("  "+l.text) + "\n")
		}
	}
	return b.String()
}

type diffLine struct {
	op   byte // ' ', '+', '-', or '~' for skipped unchanged lines
	text string
}

// lineDiff returns the changed lines between old and updated with context
// lines around each change. Config files are small, so a plain LCS is fine.
func lineDiff(old, updated string, context int) []diffLine {
	a := splitLines(old)
	b := splitLines(updated)
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var all []diffLine
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			all = append(all, diffLine{' ', a[i]})
			i++
			j++
		case i < len(a) && (j == len(b) || lcs[i+1][j] >= lcs[i][j+1]):
			all = append(all, diffLine{'-', a[i]})
			i++
		default:
			all = append(all, diffLine{'+', b[j]})
			j++
		}
	}
	keep := make([]bool, len(all))
	for k, l := range all {
		if l.op == ' ' {
			continue
		}
		for c := max(0, k-context); c <= min(len(all)-1, k+context); c++ {
			keep[c] = true
		}
	}
	var out []diffLine
	skipped := false
	for k, l := range all {
		if !keep[k] {
			skipped = true
			continue
		}
		if skipped && len(out) > 0 {
			out = append(out, diffLine{'~', ""})
		}
		skipped = false
		out = append(out, l)
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
