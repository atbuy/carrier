package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func TestParseIDAndArgv(t *testing.T) {
	id, err := parseID("42")
	if err != nil {
		t.Fatalf("parseID failed: %v", err)
	}
	if id != 42 {
		t.Fatalf("id = %d", id)
	}

	argv, err := parseArgv(`["go","test","./..."]`)
	if err != nil {
		t.Fatalf("parseArgv failed: %v", err)
	}
	if len(argv) != 3 || argv[0] != "go" || argv[2] != "./..." {
		t.Fatalf("argv mismatch: %#v", argv)
	}
}

func TestFormattingHelpers(t *testing.T) {
	ms := int64(1234)
	if got := formatDuration(&ms); got != "1.23s" {
		t.Fatalf("formatDuration = %q", got)
	}
	if got := formatDuration(nil); got != "" {
		t.Fatalf("nil duration = %q", got)
	}
	if got := formatTime(time.Time{}); got != "" {
		t.Fatalf("zero time = %q", got)
	}
	if got := short("123456789012345"); got != "123456789012" {
		t.Fatalf("short = %q", got)
	}
}

func TestDirtyString(t *testing.T) {
	if got := dirtyString(nil); got != "unknown" {
		t.Fatalf("nil dirty = %q", got)
	}
	v := true
	if got := dirtyString(&v); got != "true" {
		t.Fatalf("true dirty = %q", got)
	}
	v = false
	if got := dirtyString(&v); got != "false" {
		t.Fatalf("false dirty = %q", got)
	}
}

func TestContainsTUIOutput(t *testing.T) {
	if containsTUIOutput("plain text with \x1b[1;32mcolor\x1b[0m") {
		t.Fatal("plain SGR should not be detected as TUI")
	}
	if !containsTUIOutput("before\x1b[?1049hTUI content\x1b[?1049lafter") {
		t.Fatal("?1049h should be detected as TUI")
	}
	if !containsTUIOutput("before\x1b[?47hTUI\x1b[?47l") {
		t.Fatal("?47h should be detected as TUI")
	}
	if !containsTUIOutput("\x1b[?1047hTUI\x1b[?1047l") {
		t.Fatal("?1047h should be detected as TUI")
	}
}

func TestReadTextAndContainsFold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.log")
	t.Setenv("CARRIER_COLOR", "never")
	if got := readText(""); got != "" {
		t.Fatalf("empty path read = %q", got)
	}
	if got := readText(path); got != "" {
		t.Fatalf("missing path read = %q", got)
	}
	writeTestFile(t, path, "Connection Refused")
	if got := readText(path); got != "Connection Refused" {
		t.Fatalf("readText = %q", got)
	}
	if !containsFold("Connection Refused", "connection") {
		t.Fatalf("containsFold should be case-insensitive")
	}
}

func TestDisplayCommandQuotesRunArgvButLeavesShellCommand(t *testing.T) {
	run := &store.Run{
		Mode:     store.ModeRun,
		Command:  "legacy command",
		ArgvJSON: `["bash","-c","echo hello && exit 1"]`,
	}
	if got := displayCommand(run); got != "bash -c 'echo hello && exit 1'" {
		t.Fatalf("run display command = %q", got)
	}

	shell := &store.Run{
		Mode:     store.ModeShell,
		Command:  "echo hello && exit 1",
		ArgvJSON: `["echo hello && exit 1"]`,
	}
	if got := displayCommand(shell); got != shell.Command {
		t.Fatalf("shell display command = %q", got)
	}
}

func TestRunViewFromStoreIncludesDisplayCommandAndOutput(t *testing.T) {
	dir := t.TempDir()
	stdoutPath := filepath.Join(dir, "stdout.log")
	stderrPath := filepath.Join(dir, "stderr.log")
	writeTestFile(t, stdoutPath, "out")
	writeTestFile(t, stderrPath, "err")
	duration := int64(123)
	exitCode := 7
	run := &store.Run{
		ID:         42,
		Status:     store.StatusFailed,
		Mode:       store.ModeRun,
		Command:    "legacy",
		ArgvJSON:   `["bash","-c","echo hello && exit 7"]`,
		CWD:        "/tmp/project",
		StartedAt:  time.Date(2026, 5, 10, 1, 2, 3, 0, time.UTC),
		DurationMS: &duration,
		ExitCode:   &exitCode,
		StdoutPath: stdoutPath,
		StderrPath: stderrPath,
	}

	view := runViewFromStore(run, true)
	if view.Command != "bash -c 'echo hello && exit 7'" {
		t.Fatalf("view command = %q", view.Command)
	}
	if view.Stdout != "out" || view.Stderr != "err" {
		t.Fatalf("view output mismatch: %#v", view)
	}
	if len(view.Argv) != 3 || view.Argv[0] != "bash" {
		t.Fatalf("view argv mismatch: %#v", view.Argv)
	}
}

func TestPrintRunColorsLabelsWhenEnabled(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "always")
	t.Setenv("NO_COLOR", "")
	duration := int64(123)
	exitCode := 1
	run := &store.Run{
		ID:         7,
		Status:     store.StatusFailed,
		Mode:       store.ModeRun,
		Command:    "legacy",
		ArgvJSON:   `["go","test"]`,
		CWD:        "/tmp/project",
		StartedAt:  time.Date(2026, 5, 10, 1, 2, 3, 0, time.UTC),
		DurationMS: &duration,
		ExitCode:   &exitCode,
	}
	var out bytes.Buffer
	printRun(&out, run)

	for _, want := range []string{"ID:", "Status:", "Command:", "failed", "go test"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("printRun output missing %q:\n%s", want, out.String())
		}
	}
}

func TestWriteJSON(t *testing.T) {
	cmd := testRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := writeJSON(cmd, map[string]string{"status": "ok"}); err != nil {
		t.Fatalf("writeJSON failed: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got["status"] != "ok" {
		t.Fatalf("JSON value mismatch: %#v", got)
	}
}

func TestStripVTResponses(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "DA1 response stripped",
			input: "foo\x1b[?6;9cbar",
			want:  "foobar",
		},
		{
			name:  "DA2 response stripped",
			input: "foo\x1b[>41;0;0cbar",
			want:  "foobar",
		},
		{
			name:  "DECRQM response stripped",
			input: "foo\x1b[?1000;2$ybar",
			want:  "foobar",
		},
		{
			name:  "DCS string stripped",
			input: "foo\x1bP>|myterm 2.0\x1b\\bar",
			want:  "foobar",
		},
		{
			name:  "OSC color response BEL stripped",
			input: "foo\x1b]10;rgb:ffff/0000/0000\x07bar",
			want:  "foobar",
		},
		{
			name:  "OSC color response ST stripped",
			input: "foo\x1b]11;rgb:abcd/abcd/abcd\x1b\\bar",
			want:  "foobar",
		},
		{
			name:  "normal SGR color preserved",
			input: "\x1b[1;32mok\x1b[0m",
			want:  "\x1b[1;32mok\x1b[0m",
		},
		{
			name:  "plain text unaffected",
			input: "hello world\n",
			want:  "hello world\n",
		},
		{
			name:  "multiple response sequences stripped",
			input: "a\x1b[?6;9cb\x1bP>|myterm\x1b\\c",
			want:  "abc",
		},
		{
			name:  "alt-screen enter stripped",
			input: "before\x1b[?1049hafter",
			want:  "beforeafter",
		},
		{
			name:  "alt-screen leave stripped",
			input: "before\x1b[?1049lafter",
			want:  "beforeafter",
		},
		{
			name:  "bracketed paste mode stripped",
			input: "x\x1b[?2004hy\x1b[?2004lz",
			want:  "xyz",
		},
		{
			name:  "mouse mode stripped",
			input: "a\x1b[?1000hb\x1b[?1000lc",
			want:  "abc",
		},
		{
			name:  "cursor show/hide stripped, SGR preserved",
			input: "\x1b[?25h\x1b[1;32mok\x1b[0m\x1b[?25l",
			want:  "\x1b[1;32mok\x1b[0m",
		},
		{
			name:  "TUI full sequence stripped",
			input: "\x1b[?1049h\x1b[2J\x1b[Hhello\x1b[?1049l",
			want:  "\x1b[2J\x1b[Hhello",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripVTResponses(tc.input); got != tc.want {
				t.Fatalf("stripVTResponses(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
