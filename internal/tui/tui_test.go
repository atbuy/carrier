package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

func seed(t *testing.T) (*store.Store, config.Config) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := config.Default()
	cfg.Storage.DataDir = dir

	mk := func(cmd, argv, status string, exit int) int64 {
		id, err := st.CreateRun(store.CreateRun{
			Status: store.StatusRunning, Mode: store.ModeRun, Command: cmd,
			ArgvJSON: argv, CWD: "/tmp/project", StartedAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		if err := st.FinalizeRun(id, store.FinalizeParams{
			Status: status, ExitCode: exit, Started: time.Now(), Finished: time.Now().Add(time.Second),
		}); err != nil {
			t.Fatalf("finalize: %v", err)
		}
		return id
	}
	mk("go test ./...", `["go","test","./..."]`, store.StatusSuccess, 0)
	mk("make lint", `["make","lint"]`, store.StatusFailed, 1)
	mk("go build ./...", `["go","build","./..."]`, store.StatusSuccess, 0)
	return st, cfg
}

// newModel builds a Model with all runs loaded and a window size applied.
func newModel(t *testing.T, st *store.Store, cfg config.Config) Model {
	t.Helper()
	runs, err := st.AllRuns()
	if err != nil {
		t.Fatalf("all runs: %v", err)
	}
	m := New(st, cfg, runs)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return tm.(Model)
}

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func send(m Model, msg tea.Msg) (Model, tea.Cmd) {
	tm, cmd := m.Update(msg)
	return tm.(Model), cmd
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestNavigationMovesCursor(t *testing.T) {
	st, cfg := seed(t)
	m := newModel(t, st, cfg)

	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursor)
	}
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("after down cursor = %d, want 1", m.cursor)
	}
	m, _ = send(m, runeKey('j'))
	if m.cursor != 2 {
		t.Fatalf("after j cursor = %d, want 2", m.cursor)
	}
	// Can't move past the end.
	m, _ = send(m, runeKey('j'))
	if m.cursor != 2 {
		t.Fatalf("cursor moved past end: %d", m.cursor)
	}
	m, _ = send(m, runeKey('k'))
	if m.cursor != 1 {
		t.Fatalf("after k cursor = %d, want 1", m.cursor)
	}
	m, _ = send(m, runeKey('g'))
	if m.cursor != 0 {
		t.Fatalf("after g cursor = %d, want 0", m.cursor)
	}
}

func TestFilterNarrowsList(t *testing.T) {
	st, cfg := seed(t)
	m := newModel(t, st, cfg)

	if len(m.filtered) != 3 {
		t.Fatalf("initial filtered = %d, want 3", len(m.filtered))
	}
	m, _ = send(m, runeKey('/'))
	if m.mode != modeFilter {
		t.Fatalf("mode = %d, want modeFilter", m.mode)
	}
	for _, r := range "lint" {
		m, _ = send(m, runeKey(r))
	}
	if len(m.filtered) != 1 {
		t.Fatalf("filtered = %d, want 1", len(m.filtered))
	}
	if got := displayCommand(m.filtered[0]); got != "make lint" {
		t.Fatalf("filtered run = %q", got)
	}
	// esc clears the filter.
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeBrowse || len(m.filtered) != 3 {
		t.Fatalf("after esc: mode=%d filtered=%d", m.mode, len(m.filtered))
	}
}

func TestEnterSelectsRerunAndQuits(t *testing.T) {
	st, cfg := seed(t)
	m := newModel(t, st, cfg)

	want, _ := m.selected()
	m, cmd := send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !isQuit(cmd) {
		t.Fatal("enter did not return tea.Quit")
	}
	if m.result.Action != ActionRerun || m.result.RunID != want.ID {
		t.Fatalf("result = %+v, want rerun of %d", m.result, want.ID)
	}
}

func TestQuitKey(t *testing.T) {
	st, cfg := seed(t)
	m := newModel(t, st, cfg)
	_, cmd := send(m, runeKey('q'))
	if !isQuit(cmd) {
		t.Fatal("q did not quit")
	}
}

func TestLabelSetsLabel(t *testing.T) {
	st, cfg := seed(t)
	m := newModel(t, st, cfg)
	target, _ := m.selected()

	m, _ = send(m, runeKey('l'))
	if m.mode != modeLabel {
		t.Fatalf("mode = %d, want modeLabel", m.mode)
	}
	for _, r := range "deploy" {
		m, _ = send(m, runeKey(r))
	}
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeBrowse {
		t.Fatalf("mode after enter = %d", m.mode)
	}
	got, err := st.GetRun(target.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Label != "deploy" {
		t.Fatalf("label = %q, want deploy", got.Label)
	}
}

func TestDeleteRemovesRunAndLogFile(t *testing.T) {
	st, cfg := seed(t)

	// Attach a real stdout log file to the newest run so we can assert removal.
	runs, _ := st.AllRuns()
	target := runs[0]
	logPath := filepath.Join(t.TempDir(), "out.log")
	if err := os.WriteFile(logPath, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	if err := st.UpdatePaths(target.ID, logPath, "", ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}

	m := newModel(t, st, cfg)
	before := len(m.filtered)

	m, _ = send(m, runeKey('d'))
	if m.mode != modeConfirmDelete {
		t.Fatalf("mode = %d, want modeConfirmDelete", m.mode)
	}
	m, _ = send(m, runeKey('y'))
	if m.mode != modeBrowse {
		t.Fatalf("mode after delete = %d", m.mode)
	}
	if len(m.filtered) != before-1 {
		t.Fatalf("filtered = %d, want %d", len(m.filtered), before-1)
	}
	if _, err := st.GetRun(target.ID); err == nil {
		t.Fatal("run still exists after delete")
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("log file not removed: %v", err)
	}
}

func TestDeleteCancelledByOtherKey(t *testing.T) {
	st, cfg := seed(t)
	m := newModel(t, st, cfg)
	before := len(m.filtered)

	m, _ = send(m, runeKey('d'))
	m, _ = send(m, runeKey('n'))
	if m.mode != modeBrowse {
		t.Fatalf("mode = %d, want modeBrowse", m.mode)
	}
	if len(m.filtered) != before {
		t.Fatalf("filtered changed after cancel: %d", len(m.filtered))
	}
}

func TestViewRendersWithoutPanic(t *testing.T) {
	st, cfg := seed(t)
	m := newModel(t, st, cfg)
	if out := m.View(); out == "" {
		t.Fatal("View returned empty string")
	}
}

func TestStripANSI(t *testing.T) {
	in := "\x1b[31mred\x1b[0m plain \x1b]0;title\x07end"
	if got := stripANSI(in); got != "red plain end" {
		t.Fatalf("stripANSI = %q", got)
	}
}

func TestContrastFor(t *testing.T) {
	if fg, ok := contrastFor("#FFFFFF"); !ok || fg != "#000000" {
		t.Fatalf("white accent → fg=%q ok=%v, want #000000", fg, ok)
	}
	if fg, ok := contrastFor("#101820"); !ok || fg != "#FFFFFF" {
		t.Fatalf("dark accent → fg=%q ok=%v, want #FFFFFF", fg, ok)
	}
	if _, ok := contrastFor("red"); ok {
		t.Fatal("named color should not yield a contrast (fallback to reverse)")
	}
	if _, ok := contrastFor("#FFF"); ok {
		t.Fatal("#RGB shorthand is not supported by contrastFor")
	}
}

func TestPreviewShowsTerminalNoteNotRawPTY(t *testing.T) {
	st, cfg := seed(t)
	termPath := filepath.Join(t.TempDir(), "term.log")
	if err := os.WriteFile(termPath, []byte("user ~/proj % go build\r\nok\r\nuser ~/proj % \r\n"), 0o600); err != nil {
		t.Fatalf("write term log: %v", err)
	}
	if _, err := st.CreateRun(store.CreateRun{
		Status: store.StatusSuccess, Mode: store.ModeShell, Command: "shell",
		ArgvJSON: `[]`, CWD: "/tmp", StartedAt: time.Now(), TerminalOutputPath: termPath,
	}); err != nil {
		t.Fatalf("create shell run: %v", err)
	}

	m := newModel(t, st, cfg) // newest run (the shell run) is selected
	r, _ := m.selected()
	if r.Mode != store.ModeShell {
		t.Fatalf("expected shell run selected, got mode %q", r.Mode)
	}
	pv := m.renderPreview(r)
	if !strings.Contains(pv, "shell session") {
		t.Fatalf("preview missing terminal note:\n%s", pv)
	}
	if strings.Contains(pv, "go build") {
		t.Fatalf("raw PTY/prompt content leaked into preview:\n%s", pv)
	}
}

func TestCleanOutputStripsAnsiAndCarriageReturns(t *testing.T) {
	in := "line1\r\n\x1b[32mok\x1b[0m\rX"
	if got := cleanOutput(in); got != "line1\nokX" {
		t.Fatalf("cleanOutput = %q, want %q", got, "line1\nokX")
	}
	if cleanOutput("") != "" {
		t.Fatal("cleanOutput(\"\") should be empty")
	}
}

// TestListRowsAlignToWidth guards the multibyte-glyph padding fix: every
// rendered list row must occupy exactly listWidth display columns so the
// preview pane stays aligned regardless of selection.
func TestListRowsAlignToWidth(t *testing.T) {
	st, cfg := seed(t)
	m := newModel(t, st, cfg)
	lines := strings.Split(strings.TrimRight(m.renderList(), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected >=3 rows, got %d", len(lines))
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != listWidth {
			t.Fatalf("row %d width = %d, want %d", i, w, listWidth)
		}
	}
}

func TestResultInitAndEmptySelection(t *testing.T) {
	st, cfg := seed(t)
	m := New(st, cfg, nil)

	if got := m.Result(); got.Action != ActionNone || got.RunID != 0 {
		t.Fatalf("zero result = %+v, want ActionNone", got)
	}
	if _, ok := m.selected(); ok {
		t.Fatal("empty model selected a run")
	}
	if out := m.View(); out != "loading..." {
		t.Fatalf("unready view = %q, want loading...", out)
	}
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned nil command")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("Init command returned nil message")
	}
}

func TestFilterMatchesStatusCWDAndLabelAndClampsCursor(t *testing.T) {
	st, cfg := seed(t)
	runs, err := st.AllRuns()
	if err != nil {
		t.Fatalf("all runs: %v", err)
	}
	if err := st.SetLabel(runs[0].ID, "release candidate"); err != nil {
		t.Fatalf("set label: %v", err)
	}
	m := newModel(t, st, cfg)
	m.cursor = len(m.filtered) - 1

	m.filter.SetValue("FAILED")
	m.applyFilter()
	if len(m.filtered) != 1 || m.filtered[0].Status != store.StatusFailed {
		t.Fatalf("status filter got %d runs: %#v", len(m.filtered), m.filtered)
	}
	if m.cursor != 0 {
		t.Fatalf("cursor after clamp = %d, want 0", m.cursor)
	}

	m.filter.SetValue("/TMP/PROJECT")
	m.applyFilter()
	if len(m.filtered) != 3 {
		t.Fatalf("cwd filter got %d runs, want 3", len(m.filtered))
	}

	m.filter.SetValue("release")
	m.applyFilter()
	if len(m.filtered) != 1 || m.filtered[0].Label != "release candidate" {
		t.Fatalf("label filter mismatch: %#v", m.filtered)
	}

	m.filter.SetValue("missing")
	m.applyFilter()
	if len(m.filtered) != 0 || m.cursor != 0 {
		t.Fatalf("empty filter len=%d cursor=%d, want 0/0", len(m.filtered), m.cursor)
	}
}

func TestFilterEnterKeepsResultsAndLabelEscDoesNotWrite(t *testing.T) {
	st, cfg := seed(t)
	m := newModel(t, st, cfg)
	target, _ := m.selected()

	m, _ = send(m, runeKey('/'))
	for _, r := range "build" {
		m, _ = send(m, runeKey(r))
	}
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeBrowse || len(m.filtered) != 1 {
		t.Fatalf("filter enter mode=%d filtered=%d, want browse/1", m.mode, len(m.filtered))
	}

	m, _ = send(m, runeKey('l'))
	for _, r := range "temporary" {
		m, _ = send(m, runeKey(r))
	}
	m, _ = send(m, tea.KeyMsg{Type: tea.KeyEsc})
	got, err := st.GetRun(target.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Label != "" {
		t.Fatalf("label after esc = %q, want empty", got.Label)
	}
}

func TestBrowseEndAndPageKeysAreSafeWithNoRuns(t *testing.T) {
	st, cfg := seed(t)
	m := New(st, cfg, nil)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 2})
	m = tm.(Model)

	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyEnd},
		{Type: tea.KeyPgDown},
		{Type: tea.KeyPgUp},
		{Type: tea.KeyEnter},
		runeKey('l'),
		runeKey('d'),
	} {
		m, _ = send(m, msg)
	}
	if m.cursor != 0 || m.mode != modeBrowse || m.result.Action != ActionNone {
		t.Fatalf("empty browse state cursor=%d mode=%d result=%+v", m.cursor, m.mode, m.result)
	}
}

func TestRenderPreviewIncludesMetadataOutputAndTerminalHint(t *testing.T) {
	st, cfg := seed(t)
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")
	termPath := filepath.Join(dir, "term.log")
	if err := os.WriteFile(stdoutPath, []byte("\x1b[32mok\x1b[0m\r\nline2\n"), 0o600); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(stderrPath, []byte("bad\rnews\n"), 0o600); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := os.WriteFile(termPath, []byte("prompt output"), 0o600); err != nil {
		t.Fatalf("write terminal: %v", err)
	}
	exitCode := 3
	duration := int64(1234)
	dirty := true
	run := store.Run{
		ID: 99, Status: store.StatusFailed, Mode: store.ModeRun,
		Command: "legacy", ArgvJSON: `["bash","-c","echo ok"]`, CWD: "/tmp/project",
		StartedAt: time.Now().Add(-time.Hour), DurationMS: &duration, ExitCode: &exitCode,
		GitRoot: "/tmp/project", GitBranch: "main", GitCommit: "1234567890abcdef", GitDirty: &dirty,
		StdoutPath: stdoutPath, StderrPath: stderrPath, TerminalOutputPath: termPath, Label: "investigate",
	}
	m := newModel(t, st, cfg)
	m.vp.Width = 120

	out := stripANSI(m.renderPreview(run))
	for _, want := range []string{
		"ID:", "99", "Status:", "failed", "Command:", "bash -c 'echo ok'", "Exit:", "3",
		"Duration:", "1.23s", "Label:", "investigate", "Git:", "main 1234567890ab",
		"terminal", "carrier show 99", "stdout", "ok\nline2", "stderr", "badnews",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("preview missing %q:\n%s", want, out)
		}
	}
}

func TestRenderHelpModesAndStatusStyles(t *testing.T) {
	st, cfg := seed(t)
	m := newModel(t, st, cfg)
	m.statusMsg = "updated"
	if got := stripANSI(m.renderHelp()); !strings.Contains(got, "updated") || !strings.Contains(got, "enter rerun") {
		t.Fatalf("browse help = %q", got)
	}
	m.mode = modeFilter
	if got := stripANSI(m.renderHelp()); !strings.Contains(got, "filter:") {
		t.Fatalf("filter help = %q", got)
	}
	m.mode = modeLabel
	if got := stripANSI(m.renderHelp()); !strings.Contains(got, "label:") {
		t.Fatalf("label help = %q", got)
	}
	m.mode = modeConfirmDelete
	if got := stripANSI(m.renderHelp()); !strings.Contains(got, "delete run") || !strings.Contains(got, "(y/N)") {
		t.Fatalf("confirm help = %q", got)
	}

	for _, status := range []string{store.StatusSuccess, store.StatusFailed, store.StatusKilled, store.StatusRunning, "custom"} {
		_ = m.statusStyle(status).Render(status)
	}
}

func TestDisplayHelpersEdgeCases(t *testing.T) {
	for status, want := range map[string]string{
		store.StatusSuccess: "✓",
		store.StatusFailed:  "✗",
		store.StatusRunning: "●",
		store.StatusKilled:  "⊘",
		"other":             "•",
	} {
		if got := statusGlyph(status); got != want {
			t.Fatalf("statusGlyph(%q) = %q, want %q", status, got, want)
		}
	}
	if got := short("123456789012345"); got != "123456789012" {
		t.Fatalf("short = %q", got)
	}
	if got := padRight("abcdef", 3); got != "abcdef" {
		t.Fatalf("padRight long = %q", got)
	}
	if got := padRight("go", 4); got != "go  " {
		t.Fatalf("padRight short = %q", got)
	}
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("truncate short = %q", got)
	}
	if got := lipgloss.Width(truncate("long command with spaces", 8)); got > 8 {
		t.Fatalf("truncate width = %d, want <= 8", got)
	}

	now := time.Now()
	cases := []struct {
		name string
		ts   time.Time
		want string
	}{
		{"zero", time.Time{}, ""},
		{"future clamps", now.Add(time.Hour), "just now"},
		{"seconds", now.Add(-30 * time.Second), "30s ago"},
		{"minutes", now.Add(-5 * time.Minute), "5m ago"},
		{"hours", now.Add(-2 * time.Hour), "2h ago"},
		{"days", now.Add(-3 * 24 * time.Hour), "3d ago"},
		{"weeks", now.Add(-14 * 24 * time.Hour), "2w ago"},
		{"months", now.Add(-60 * 24 * time.Hour), "2mo ago"},
		{"years", now.Add(-400 * 24 * time.Hour), "1y ago"},
	}
	for _, tc := range cases {
		if got := formatRelativeTime(tc.ts); got != tc.want {
			t.Fatalf("%s: formatRelativeTime = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := timeWithRelative(time.Time{}); got != "" {
		t.Fatalf("zero timeWithRelative = %q", got)
	}
	if got := timeWithRelative(now.Add(-time.Minute)); !strings.Contains(got, "(1m ago)") {
		t.Fatalf("timeWithRelative = %q, want relative suffix", got)
	}
}

func TestReadTailLimitsAndHandlesMissingFiles(t *testing.T) {
	if got := readTail(""); got != "" {
		t.Fatalf("empty readTail = %q", got)
	}
	if got := readTail(filepath.Join(t.TempDir(), "missing.log")); got != "" {
		t.Fatalf("missing readTail = %q", got)
	}
	path := filepath.Join(t.TempDir(), "big.log")
	var b strings.Builder
	for i := 0; i < previewTailLines+5; i++ {
		fmt.Fprintf(&b, "line-%03d\n", i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
	got := readTail(path)
	if strings.Contains(got, "line-000") || !strings.Contains(got, "line-005") || !strings.Contains(got, "line-404") {
		t.Fatalf("tail did not keep last %d lines:\n%s", previewTailLines, got[:min(len(got), 200)])
	}
}

func TestCollapseHomeAndShortEdgeCases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{home, "~"},
		{filepath.Join(home, "project"), "~/project"},
		{home + "suffix", home + "suffix"},
		{"/var/tmp", "/var/tmp"},
	}
	for _, tc := range cases {
		if got := collapseHome(tc.in); got != tc.want {
			t.Fatalf("collapseHome(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := short("short"); got != "short" {
		t.Fatalf("short short = %q", got)
	}
}
