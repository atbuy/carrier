package cli

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type setupFixture struct {
	t          *testing.T
	home       string
	configPath string
	out        bytes.Buffer
	notified   int
	notifyOK   bool
	commands   map[string]string // commands found on PATH
}

func newSetupFixture(t *testing.T) *setupFixture {
	t.Helper()
	t.Setenv("CARRIER_COLOR", "never")
	home := t.TempDir()
	setHome(t, home)
	t.Setenv("ZDOTDIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return &setupFixture{t: t, home: home, configPath: filepath.Join(home, ".config", "carrier", "config.toml"), commands: map[string]string{}}
}

func (f *setupFixture) write(rel, content string) string {
	f.t.Helper()
	path := filepath.Join(f.home, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		f.t.Fatal(err)
	}
	return path
}

func (f *setupFixture) read(rel string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.home, rel))
	if err != nil {
		f.t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// run runs the wizard with one answer per prompt.
func (f *setupFixture) run(shell string, dryRun bool, answers ...string) (*setupEnv, error) {
	f.t.Helper()
	f.out.Reset()
	input := strings.Join(answers, "\n")
	if len(answers) > 0 {
		input += "\n"
	}
	env := &setupEnv{
		in: bufio.NewReader(strings.NewReader(input)), out: &f.out, t: newTheme(&f.out),
		dryRun: dryRun, home: f.home, shell: shell, configPath: f.configPath,
		now:             func() time.Time { return time.Date(2026, 10, 20, 15, 4, 5, 0, time.UTC) },
		notifyAvailable: func() bool { return f.notifyOK },
		notifyTest:      func() error { f.notified++; return nil },
		lookPath: func(name string) (string, error) {
			if p, ok := f.commands[name]; ok {
				return p, nil
			}
			return "", errors.New("not found")
		},
	}
	return env, env.run()
}

func TestSetupFreshHome(t *testing.T) {
	f := newSetupFixture(t)
	f.write(".bashrc", "# my bashrc\nexport PATH=\"$HOME/bin:$PATH\"\n")

	// record: yes, apply; retention: keep; hints: yes, apply; alias: yes, apply
	env, err := f.run("bash", false, "y", "y", "", "y", "y", "y", "y")
	if err != nil {
		t.Fatalf("setup: %v\n%s", err, f.out.String())
	}
	wantRC := "# my bashrc\nexport PATH=\"$HOME/bin:$PATH\"\n\n" +
		"# Added by carrier setup: record every terminal in a carrier session.\neval \"$(carrier init bash)\"\n\n" +
		"# Added by carrier setup: short name for carrier.\nalias c='carrier'\n"
	if got := f.read(".bashrc"); got != wantRC {
		t.Fatalf(".bashrc:\n%s\nwant:\n%s", got, wantRC)
	}
	if got := f.read(".config/carrier/config.toml"); got != setupConfigHeader+"\n[hints]\nsimilar_failures = true\n" {
		t.Fatalf("config:\n%s", got)
	}
	if len(env.changed) != 2 {
		t.Fatalf("changed files should be listed once each: %v", env.changed)
	}
	if _, err := os.Stat(filepath.Join(f.home, ".bashrc.bak-20261020-150405")); err != nil {
		t.Fatalf("the shell config should be backed up next to it: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.home, ".config", "carrier", "backups")); !os.IsNotExist(err) {
		t.Fatal("a new config file has nothing to back up")
	}

	// A second run finds everything set up and changes nothing.
	before := f.read(".bashrc")
	env, err = f.run("bash", false, "")
	if err != nil || len(env.changed) != 0 || f.read(".bashrc") != before {
		t.Fatalf("second run should change nothing: %v %v\n%s", err, env.changed, f.out.String())
	}
	for _, want := range []string{"✓ already on in " + filepath.FromSlash("~/.bashrc"), "✓ already on", "✓ already set in " + filepath.FromSlash("~/.bashrc"), "Nothing was changed"} {
		if !strings.Contains(f.out.String(), want) {
			t.Errorf("second run output missing %q:\n%s", want, f.out.String())
		}
	}
}

func TestSetupEditsExistingConfigInPlace(t *testing.T) {
	f := newSetupFixture(t)
	f.write(".bashrc", "eval \"$(carrier init bash)\"\nalias c='carrier'\n")
	mainConfig := `# hand tuned
includes = ["conf.d/*"]

[notify]
min_duration = "5s"   # quick feedback

[[auto_label]]
label = "tests"
cmd = 'go test.*'
`
	f.write(".config/carrier/config.toml", mainConfig)
	f.write(".config/carrier/conf.d/hints.toml", "[hints]\nsimilar_failures = false # later\n")
	f.notifyOK = true

	// retention: 60d, apply; hints: yes, apply; test notification: no; min duration: 30s, apply
	if _, err := f.run("bash", false, "60d", "y", "y", "y", "n", "30s", "y"); err != nil {
		t.Fatalf("setup: %v\n%s", err, f.out.String())
	}
	want := strings.Replace(mainConfig, `min_duration = "5s"   # quick feedback`, `min_duration = "30s"   # quick feedback`, 1) +
		"\n[shell]\nauto_session_retention = \"60d\"\n"
	if got := f.read(".config/carrier/config.toml"); got != want {
		t.Fatalf("main config:\n%s\nwant:\n%s", got, want)
	}
	if got := f.read(".config/carrier/conf.d/hints.toml"); got != "[hints]\nsimilar_failures = true # later\n" {
		t.Fatalf("include should be edited where the key is set:\n%s", got)
	}
	if !strings.Contains(f.out.String(), "hints.similar_failures is set in "+filepath.FromSlash("~/.config/carrier/conf.d/hints.toml")) {
		t.Fatalf("output should explain editing the include:\n%s", f.out.String())
	}
	if f.notified != 0 {
		t.Fatal("test notification was declined")
	}

	// Backups stay out of conf.d, where "conf.d/*" would load them.
	if entries, _ := os.ReadDir(filepath.Join(f.home, ".config", "carrier", "conf.d")); len(entries) != 1 {
		t.Fatalf("conf.d should only hold the include, got %v", entries)
	}
	backups, _ := os.ReadDir(filepath.Join(f.home, ".config", "carrier", "backups"))
	if len(backups) != 3 {
		t.Fatalf("expected 3 backups (config twice, include once), got %v", backups)
	}
	// Windows has no Unix permission bits to keep.
	if info, _ := os.Stat(filepath.Join(f.home, ".config", "carrier", "config.toml")); runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Fatalf("file mode should be kept, got %v", info.Mode().Perm())
	}
}

func TestSetupDeclineAndDryRunWriteNothing(t *testing.T) {
	f := newSetupFixture(t)
	f.write(".zshrc", "# zsh\n")

	if _, err := f.run("zsh", false, "n", "n", "n"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if f.read(".zshrc") != "# zsh\n" {
		t.Fatal("declined steps must not write")
	}
	// Agreeing to a step but declining its change also writes nothing.
	if _, err := f.run("zsh", false, "y", "n", "y", "n", "y", "n"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if f.read(".zshrc") != "# zsh\n" {
		t.Fatal("declined changes must not write")
	}
	if _, err := os.Stat(f.configPath); !os.IsNotExist(err) {
		t.Fatal("declined config changes must not create the config")
	}

	if _, err := f.run("zsh", true); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	out := f.out.String()
	if f.read(".zshrc") != "# zsh\n" || !strings.Contains(out, `+ eval "$(carrier init zsh)"`) || !strings.Contains(out, "dry run: not written") {
		t.Fatalf("dry run should show changes without writing:\n%s", out)
	}
	if entries, _ := filepath.Glob(filepath.Join(f.home, ".zshrc.bak-*")); len(entries) != 0 {
		t.Fatal("dry run must not make backups")
	}
}

func TestSetupStopsOnBrokenConfig(t *testing.T) {
	f := newSetupFixture(t)
	f.write(".bashrc", "# bash\n")
	f.write(".config/carrier/config.toml", "[notify\nmin_duration = 5s\n")
	_, err := f.run("bash", false, "y", "y")
	if err == nil || !strings.Contains(err.Error(), "setup will not change it") {
		t.Fatalf("broken config should stop setup, got %v", err)
	}
	if f.read(".bashrc") != "# bash\n" {
		t.Fatal("nothing may be changed when the config is broken")
	}
}

func TestSetupAliasConflicts(t *testing.T) {
	cases := []struct {
		name, rc, command, want string
	}{
		{"other alias", "alias c='code'\n", "", `"c" is already defined in ` + filepath.FromSlash("~/.bashrc") + `: alias c='code'`},
		{"function", "c() { cd \"$1\"; }\n", "", `"c" is already defined in ` + filepath.FromSlash("~/.bashrc") + `: c() { cd "$1"; }`},
		{"command", "", "/usr/bin/c", `"c" is already a command: /usr/bin/c`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSetupFixture(t)
			f.write(".bashrc", "eval \"$(carrier init bash)\"\n"+tc.rc)
			if tc.command != "" {
				f.commands["c"] = tc.command
			}
			if _, err := f.run("bash", false, "", "n"); err != nil {
				t.Fatalf("setup: %v", err)
			}
			if !strings.Contains(f.out.String(), "skipped: "+tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, f.out.String())
			}
			if strings.Contains(f.read(".bashrc"), "alias c='carrier'") {
				t.Fatal("must not add a conflicting alias")
			}
		})
	}
}

func TestSetupUnsupportedShellAndFish(t *testing.T) {
	f := newSetupFixture(t)
	if _, err := f.run("", false, "n"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if strings.Count(f.out.String(), "skipped: your shell is not supported") != 2 {
		t.Fatalf("shell steps should be skipped:\n%s", f.out.String())
	}

	f = newSetupFixture(t)
	if _, err := f.run("fish", false, "y", "y", "", "n", "y", "y"); err != nil {
		t.Fatalf("setup fish: %v", err)
	}
	got := f.read(".config/fish/config.fish")
	if !strings.Contains(got, "carrier init fish | source\n") || !strings.Contains(got, "alias c carrier\n") {
		t.Fatalf("fish config:\n%s", got)
	}
}

func TestSetupNotifications(t *testing.T) {
	f := newSetupFixture(t)
	f.notifyOK = true
	// hints: no; test: yes; min duration: invalid, then 1m, apply
	if _, err := f.run("", false, "n", "y", "soon", "1m", "y"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if f.notified != 1 {
		t.Fatalf("test notification sent %d times", f.notified)
	}
	if !strings.Contains(f.out.String(), "invalid:") || !strings.Contains(f.read(".config/carrier/config.toml"), "[notify]\nmin_duration = \"1m\"\n") {
		t.Fatalf("min duration should be validated and saved:\n%s", f.out.String())
	}
}

func TestProposeEditRollsBackInvalidChange(t *testing.T) {
	f := newSetupFixture(t)
	env, _ := f.run("", true)
	env.dryRun = false
	env.in = bufio.NewReader(strings.NewReader("y\ny\n"))

	existing := f.write("file.toml", "a = 1\n")
	failing := func() error { return errors.New("broken") }
	if applied, err := env.proposeEdit(existing, []byte("a = 1\n"), []byte("a = 2\n"), failing); err != nil || applied {
		t.Fatalf("applied=%v err=%v", applied, err)
	}
	if f.read("file.toml") != "a = 1\n" {
		t.Fatal("failed validation should restore the old content")
	}
	newFile := filepath.Join(f.home, "new.toml")
	if applied, _ := env.proposeEdit(newFile, nil, []byte("a = 1\n"), failing); applied {
		t.Fatal("change should not count as applied")
	}
	if _, err := os.Stat(newFile); !os.IsNotExist(err) {
		t.Fatal("failed validation should remove a newly created file")
	}
}

func TestWriteFileSafely(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 20, 15, 4, 5, 0, time.UTC)
	real := filepath.Join(dir, "dotfiles", "bashrc")
	_ = os.MkdirAll(filepath.Dir(real), 0o755)
	_ = os.WriteFile(real, []byte("v1\n"), 0o600)
	link := filepath.Join(dir, ".bashrc")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	backup, err := writeFileSafely(link, []byte("v2\n"), "", now)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink must stay a symlink")
	}
	if b, _ := os.ReadFile(real); string(b) != "v2\n" {
		t.Fatalf("the link target should be updated, got %q", b)
	}
	// Windows has no Unix permission bits to keep.
	if fi, _ := os.Stat(real); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode should be kept, got %v", fi.Mode().Perm())
	}
	// The backup sits next to the link's target, which a symlinked temp
	// directory (macOS /var is /private/var) can name differently.
	backupDir, _ := filepath.EvalSymlinks(filepath.Dir(backup))
	realDir, _ := filepath.EvalSymlinks(filepath.Dir(real))
	if b, _ := os.ReadFile(backup); string(b) != "v1\n" || backupDir != realDir {
		t.Fatalf("backup %s = %q", backup, b)
	}
	// A second write in the same second keeps the first backup.
	second, err := writeFileSafely(link, []byte("v3\n"), "", now)
	if err != nil || second == backup || !strings.HasSuffix(second, "-2") {
		t.Fatalf("second backup = %q, %v", second, err)
	}
	if b, _ := os.ReadFile(backup); string(b) != "v1\n" {
		t.Fatal("the first backup must not be overwritten")
	}
	elsewhere := filepath.Join(dir, "backups")
	third, err := writeFileSafely(link, []byte("v4\n"), elsewhere, now)
	if err != nil || filepath.Dir(third) != elsewhere {
		t.Fatalf("backup dir: %q, %v", third, err)
	}
}

func TestLineDiff(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\ng\n"
	updated := "a\nb\nc\nD\ne\nf\ng\nh\n"
	var got []string
	for _, l := range lineDiff(old, updated, 1) {
		got = append(got, string(l.op)+l.text)
	}
	want := []string{" c", "-d", "+D", " e", "~", " g", "+h"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lineDiff = %q, want %q", got, want)
	}
	if len(lineDiff("", "x\n", 2)) != 1 {
		t.Fatal("new file diff should be all additions")
	}
}

func TestSetupAskHandlesBadInputAndEOF(t *testing.T) {
	f := newSetupFixture(t)
	env, _ := f.run("", true)
	env.dryRun = false
	env.in = bufio.NewReader(strings.NewReader("maybe\nYES\n"))
	if !env.ask("Continue?", false) || !strings.Contains(f.out.String(), "please answer y or n") {
		t.Fatal("ask should re-ask on bad input")
	}
	env.in = bufio.NewReader(strings.NewReader(""))
	if env.ask("Continue?", true) {
		t.Fatal("end of input must mean no, even when the default is yes")
	}
	env.in = bufio.NewReader(strings.NewReader("bad"))
	if got := env.askValue("Keep for", "30d", func(string) error { return errors.New("nope") }); got != "30d" {
		t.Fatalf("askValue at end of input should keep the current value, got %q", got)
	}
}

func TestSetupCmdNeedsTerminal(t *testing.T) {
	cmd := (&app{}).setupCmd()
	if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "--dry-run") {
		t.Fatalf("setup without a terminal should suggest --dry-run, got %v", err)
	}
}
