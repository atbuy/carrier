package shell

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"

	"github.com/atbuy/carrier/internal/config"
)

func TestRunExecutesProgramAndWritesCurrentLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}

	dir := t.TempDir()
	program := filepath.Join(dir, "carrier-shell-test")
	script := `#!/bin/sh
printf '{"current_id":1,"current_log":"%s"}' "$TEST_LOG_PATH" > "$CARRIER_SHELL_STATE"
printf 'TOKEN=secret\n'
`
	if err := os.WriteFile(program, []byte(script), 0o755); err != nil {
		t.Fatalf("write shell program: %v", err)
	}

	logPath := filepath.Join(dir, "session.log")
	t.Setenv("TEST_LOG_PATH", logPath)

	cfg := config.Default()
	cfg.Storage.DataDir = filepath.Join(dir, "data")
	cfg.Storage.MaxOutputMB = 1
	cfg.Shell.Program = program
	cfg.Redaction.Enabled = true
	cfg.Redaction.Patterns = []string{`TOKEN=\S+`}

	if _, err := Run(cfg, Options{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read session log: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("log missing redacted output: %q", got)
	}
	if strings.Contains(got, "TOKEN=secret") {
		t.Fatalf("secret leaked in session log: %q", got)
	}
}

func TestStateFileReadWrite(t *testing.T) {
	sf := &StateFile{Path: filepath.Join(t.TempDir(), "state.json")}

	if got := sf.Read(); got.CurrentID != 0 || got.CurrentLog != "" {
		t.Fatalf("missing state should read zero value: %#v", got)
	}

	want := State{CurrentID: 42, CurrentLog: "/tmp/run.log"}
	if err := sf.Write(want); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if got := sf.Read(); got != want {
		t.Fatalf("state mismatch: got %#v want %#v", got, want)
	}
}

func TestStateFileCacheHitAvoidsDiskRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	sf := &StateFile{Path: path}

	first := State{CurrentID: 1, CurrentLog: "/tmp/a.log", SessionID: 7}
	if err := sf.Write(first); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Two reads without any write in between — second must return cached value.
	got1 := sf.Read()
	if got1 != first {
		t.Fatalf("first read: got %#v want %#v", got1, first)
	}
	got2 := sf.Read()
	if got2 != first {
		t.Fatalf("cache hit read: got %#v want %#v", got2, first)
	}
}

func TestStateFileCacheInvalidatesOnWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	sf := &StateFile{Path: path}

	s1 := State{CurrentID: 1, CurrentLog: "/tmp/a.log"}
	if err := sf.Write(s1); err != nil {
		t.Fatalf("write s1: %v", err)
	}
	if got := sf.Read(); got != s1 {
		t.Fatalf("read s1: got %#v", got)
	}

	// Overwrite with different state — need a distinct mtime, so backdate s1's
	// mtime by 1 second first, then write s2 so its mtime is clearly newer.
	old := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	// Prime the cache with the backdated mtime.
	_ = sf.Read()

	s2 := State{CurrentID: 2, CurrentLog: "/tmp/b.log"}
	if err := sf.Write(s2); err != nil {
		t.Fatalf("write s2: %v", err)
	}

	got := sf.Read()
	if got != s2 {
		t.Fatalf("after invalidation: got %#v want %#v", got, s2)
	}
}

func TestBoolEnv(t *testing.T) {
	if got := boolEnv("MYVAR", true); got != "MYVAR=1" {
		t.Fatalf("boolEnv true = %q", got)
	}
	if got := boolEnv("MYVAR", false); got != "MYVAR=0" {
		t.Fatalf("boolEnv false = %q", got)
	}
}

func TestStrconvPID(t *testing.T) {
	got := strconvPID()
	if got == "" {
		t.Fatal("strconvPID returned empty string")
	}
	if strings.ContainsAny(got, " \t\n") {
		t.Fatalf("strconvPID has whitespace: %q", got)
	}
}

func TestSessionLogWriterEnsureAndClose(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "log1.txt")
	path2 := filepath.Join(dir, "log2.txt")

	w := &sessionLogWriter{
		maxOutputBytes: 1024 * 1024,
	}

	// Write to first path.
	n, err := w.Write(path1, []byte("hello "))
	if err != nil {
		t.Fatalf("write path1: %v", err)
	}
	if n != 6 {
		t.Fatalf("n = %d", n)
	}

	// Write to same path again — exercises the "same path reuse" branch in ensure.
	if _, err := w.Write(path1, []byte("world")); err != nil {
		t.Fatalf("write path1 again: %v", err)
	}

	// Write to second path (triggers close of first, open of second).
	if _, err := w.Write(path2, []byte("new")); err != nil {
		t.Fatalf("write path2: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Verify files.
	b1, err := os.ReadFile(path1)
	if err != nil {
		t.Fatalf("read path1: %v", err)
	}
	if string(b1) != "hello world" {
		t.Fatalf("path1 = %q", b1)
	}

	b2, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("read path2: %v", err)
	}
	if string(b2) != "new" {
		t.Fatalf("path2 = %q", b2)
	}
}

func TestSessionLogWriterWriteToInvalidPath(t *testing.T) {
	w := &sessionLogWriter{maxOutputBytes: 1024}
	// Write to a non-existent directory → ensure fails.
	_, err := w.Write("/does/not/exist/log.txt", []byte("data"))
	if err == nil {
		t.Fatal("expected error writing to invalid path")
	}
}

func TestSessionLogWriterIdempotentClose(t *testing.T) {
	w := &sessionLogWriter{maxOutputBytes: 1024}
	// Close on zero-value writer should be safe.
	if err := w.Close(); err != nil {
		t.Fatalf("close zero writer: %v", err)
	}
}

func TestSessionLogWriterSuppressesAltScreen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")

	w := &sessionLogWriter{maxOutputBytes: 1024 * 1024}
	chunk := append([]byte("before"), altScreenEnter...)
	chunk = append(chunk, []byte("TUI content")...)
	chunk = append(chunk, altScreenLeave...)
	chunk = append(chunk, []byte("after")...)

	if _, err := w.Write(path, chunk); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != "beforeafter" {
		t.Fatalf("got %q, want %q", b, "beforeafter")
	}
}

func TestSessionLogWriterAltScreenAcrossChunks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")

	w := &sessionLogWriter{maxOutputBytes: 1024 * 1024}

	chunk1 := append([]byte("begin"), altScreenEnter...)
	if _, err := w.Write(path, chunk1); err != nil {
		t.Fatalf("Write chunk1: %v", err)
	}

	chunk2 := append([]byte("TUI stuff"), altScreenLeave...)
	chunk2 = append(chunk2, []byte("end")...)
	if _, err := w.Write(path, chunk2); err != nil {
		t.Fatalf("Write chunk2: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != "beginend" {
		t.Fatalf("got %q, want %q", b, "beginend")
	}
}

func TestSessionLogWriterMultipleTUICycles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")

	w := &sessionLogWriter{maxOutputBytes: 1024 * 1024}

	chunk := []byte("a")
	chunk = append(chunk, altScreenEnter...)
	chunk = append(chunk, []byte("TUI1")...)
	chunk = append(chunk, altScreenLeave...)
	chunk = append(chunk, []byte("b")...)
	chunk = append(chunk, altScreenEnter...)
	chunk = append(chunk, []byte("TUI2")...)
	chunk = append(chunk, altScreenLeave...)
	chunk = append(chunk, []byte("c")...)

	if _, err := w.Write(path, chunk); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != "abc" {
		t.Fatalf("got %q, want %q", b, "abc")
	}
}

func TestSessionLogWriterAltScreenNoLeave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")

	w := &sessionLogWriter{maxOutputBytes: 1024 * 1024}

	chunk1 := append([]byte("visible"), altScreenEnter...)
	chunk1 = append(chunk1, []byte("suppressed1")...)
	if _, err := w.Write(path, chunk1); err != nil {
		t.Fatalf("Write chunk1: %v", err)
	}

	if _, err := w.Write(path, []byte("suppressed2")); err != nil {
		t.Fatalf("Write chunk2: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != "visible" {
		t.Fatalf("got %q, want %q", b, "visible")
	}
	if !w.inAltScreen {
		t.Fatal("expected inAltScreen to remain true after no leave sequence")
	}
}

func TestShellArgsUsesBashRcFile(t *testing.T) {
	dir := t.TempDir()

	got := shellArgs("/bin/bash", dir)
	want := []string{"--rcfile", filepath.Join(dir, ".bashrc"), "-i"}
	if len(got) != len(want) {
		t.Fatalf("shell args length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("shell arg %d = %q, want %q", i, got[i], want[i])
		}
	}
	if got := shellArgs("/bin/zsh", dir); len(got) != 1 || got[0] != "-i" {
		t.Fatalf("zsh args = %#v", got)
	}
}

func TestRunCallsOnRunEndWithFlushedLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}

	dir := t.TempDir()
	program := filepath.Join(dir, "carrier-shell-end-test")
	// Mimic the hooks: begin run 7, print its output, end it, then print the
	// next prompt.
	script := `#!/bin/sh
printf '{"current_id":7,"current_log":"%s","session_id":1}' "$TEST_LOG_PATH" > "$CARRIER_SHELL_STATE"
printf 'connection refused\n'
sleep 0.3
printf '{"session_id":1}' > "$CARRIER_SHELL_STATE"
printf 'PROMPT$ '
sleep 0.3
`
	if err := os.WriteFile(program, []byte(script), 0o755); err != nil {
		t.Fatalf("write shell program: %v", err)
	}
	logPath := filepath.Join(dir, "run7.log")
	t.Setenv("TEST_LOG_PATH", logPath)

	cfg := config.Default()
	cfg.Storage.DataDir = filepath.Join(dir, "data")
	cfg.Shell.Program = program
	cfg.Redaction.Enabled = true

	var endedID int64
	var logAtEnd string
	onRunEnd := func(id int64) string {
		endedID = id
		b, _ := os.ReadFile(logPath)
		logAtEnd = string(b)
		return "carrier: seen before"
	}

	// Capture what Run writes to the terminal.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	stdout := os.Stdout
	os.Stdout = w
	_, runErr := Run(cfg, Options{SessionID: 1, OnRunEnd: onRunEnd})
	os.Stdout = stdout
	_ = w.Close()
	terminal, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	if endedID != 7 {
		t.Fatalf("onRunEnd called with %d, want 7", endedID)
	}
	// The redacting writer holds back short output until closed, so this only
	// holds if the log was flushed before the callback.
	if !strings.Contains(logAtEnd, "connection refused") {
		t.Fatalf("log was not flushed before onRunEnd: %q", logAtEnd)
	}
	out := string(terminal)
	hint, prompt := strings.Index(out, "carrier: seen before\r\n"), strings.Index(out, "PROMPT$ ")
	if hint < 0 || prompt < 0 || hint > prompt {
		t.Fatalf("hint should be printed before the prompt, got %q", out)
	}
	if b, _ := os.ReadFile(logPath); strings.Contains(string(b), "seen before") || strings.Contains(string(b), "PROMPT") {
		t.Fatalf("hint and prompt must not be saved in the run log: %q", b)
	}
}

func TestRunTellsTheShellItsTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY shell test is Unix-only")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "tty.txt")
	program := filepath.Join(dir, "fake-shell")
	script := "#!/bin/sh\nprintf '%s|%s|%s' \"$CARRIER_TTY\" \"$(tty)\" \"$CARRIER_ZDOTDIR\" > " + out + "\n"
	if err := os.WriteFile(program, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZDOTDIR", filepath.Join(dir, "my-zsh-config"))
	cfg := config.Default()
	cfg.Storage.DataDir = filepath.Join(dir, "data")
	cfg.Shell.Program = program
	if _, err := Run(cfg, Options{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(b), "|")
	if len(parts) != 3 || parts[0] == "" || parts[0] != parts[1] {
		t.Fatalf("CARRIER_TTY should be the shell's terminal, got %q", b)
	}
	if parts[2] != filepath.Join(dir, "my-zsh-config") {
		t.Fatalf("CARRIER_ZDOTDIR = %q, want the user's ZDOTDIR", parts[2])
	}
}

func TestUserZDOTDIRIgnoresInheritedHookDir(t *testing.T) {
	t.Setenv("ZDOTDIR", filepath.Join(os.TempDir(), "carrier-shell-123456"))
	if got := userZDOTDIR(); got != "" {
		t.Fatalf("a carrier hook dir is not the user's ZDOTDIR, got %q", got)
	}
	t.Setenv("ZDOTDIR", "/home/dev/.config/zsh")
	if got := userZDOTDIR(); got != "/home/dev/.config/zsh" {
		t.Fatalf("userZDOTDIR = %q", got)
	}
}

func TestOtherTerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY test is Unix-only")
	}
	ptmx1, tty1, err := pty.Open()
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	defer func() { _ = ptmx1.Close(); _ = tty1.Close() }()
	ptmx2, tty2, err := pty.Open()
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	defer func() { _ = ptmx2.Close(); _ = tty2.Close() }()

	oldStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = oldStdin })
	os.Stdin = tty1

	t.Setenv("CARRIER_TTY", "")
	if OtherTerminal() {
		t.Fatal("without CARRIER_TTY nothing is known, so not another terminal")
	}
	t.Setenv("CARRIER_TTY", tty1.Name())
	if OtherTerminal() {
		t.Fatal("stdin is the recorded terminal")
	}
	t.Setenv("CARRIER_TTY", tty2.Name())
	if !OtherTerminal() {
		t.Fatal("stdin is a different terminal than the recorded one")
	}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = devNull.Close() }()
	os.Stdin = devNull
	if OtherTerminal() && !term.IsTerminal(int(os.Stdout.Fd())) && !term.IsTerminal(int(os.Stderr.Fd())) {
		t.Fatal("with no terminal at all, keep trusting the variables")
	}
}
