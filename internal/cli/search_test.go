package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func TestCleanSnippet(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "hello   world",
			want:  "hello world",
		},
		{
			input: "line1\nline2\nline3",
			want:  "line1 line2 line3",
		},
		{
			input: "  leading and trailing  ",
			want:  "leading and trailing",
		},
		{
			input: "mixed\t\nnewlines  and\ttabs",
			want:  "mixed newlines and tabs",
		},
		{
			input: "",
			want:  "",
		},
		{
			input: "   ",
			want:  "",
		},
		{
			input: "single",
			want:  "single",
		},
	}
	for _, tc := range tests {
		got := cleanSnippet(tc.input)
		if got != tc.want {
			t.Errorf("cleanSnippet(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSearchCommandUsesFTSResults(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	started := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "go test ./...",
		ArgvJSON:  `["go","test","./..."]`,
		CWD:       "/tmp/project",
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	stdoutPath := filepath.Join(t.TempDir(), "stdout.log")
	if err := os.WriteFile(stdoutPath, []byte("needle output line\n"), 0o600); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := st.UpdatePaths(id, stdoutPath, "", ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}
	if err := st.FinishRun(id, store.StatusSuccess, 0, started.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	app := &app{st: st}
	cmd := app.searchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, []string{"needle"}); err != nil {
		t.Fatalf("search command failed: %v", err)
	}
	for _, want := range []string{
		"1  success  go test ./...  /tmp/project",
		"needle output line",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("search output missing %q:\n%s", want, out.String())
		}
	}
}

func TestSearchCommandJSONOutput(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	started := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "make test",
		ArgvJSON:  `["make","test"]`,
		CWD:       "/tmp/project",
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	stdoutPath := filepath.Join(t.TempDir(), "stdout.log")
	if err := os.WriteFile(stdoutPath, []byte("unique-token-xyz\n"), 0o600); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := st.UpdatePaths(id, stdoutPath, "", ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}
	if err := st.FinishRun(id, store.StatusSuccess, 0, started.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	a := &app{st: st}
	cmd := a.searchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatalf("set json flag: %v", err)
	}

	if err := cmd.RunE(cmd, []string{"unique-token-xyz"}); err != nil {
		t.Fatalf("search JSON failed: %v", err)
	}
	if !strings.Contains(out.String(), "make test") {
		t.Fatalf("JSON output missing command:\n%s", out.String())
	}
	if !strings.Contains(out.String(), `"command"`) {
		t.Fatalf("output not JSON:\n%s", out.String())
	}
}

func TestSearchCommandSinceFilter(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	old := time.Now().Add(-72 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)

	makeRun := func(cmd string, started time.Time) {
		id, err := st.CreateRun(store.CreateRun{
			Status: store.StatusSuccess, Mode: store.ModeRun,
			Command: cmd, ArgvJSON: `["` + cmd + `"]`,
			CWD: "/tmp", StartedAt: started,
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		if err := st.FinishRun(id, store.StatusSuccess, 0, started.Add(time.Second)); err != nil {
			t.Fatalf("finish run: %v", err)
		}
	}

	makeRun("sincefilter-old", old)
	makeRun("sincefilter-recent", recent)

	a := &app{st: st}
	cmd := a.searchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("since", "24h"); err != nil {
		t.Fatalf("set since flag: %v", err)
	}
	if err := cmd.RunE(cmd, []string{"sincefilter"}); err != nil {
		t.Fatalf("search with --since: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "sincefilter-old") {
		t.Errorf("old run should be excluded by --since 24h:\n%s", got)
	}
	if !strings.Contains(got, "sincefilter-recent") {
		t.Errorf("recent run should appear with --since 24h:\n%s", got)
	}
}

func TestSearchCommandStatusFilter(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now()
	for _, status := range []string{store.StatusSuccess, store.StatusFailed} {
		id, err := st.CreateRun(store.CreateRun{
			Status: status, Mode: store.ModeRun,
			Command: "statustest cmd", ArgvJSON: `["statustest","cmd"]`,
			CWD: "/tmp", StartedAt: now,
		})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		if err := st.FinishRun(id, status, 0, now.Add(time.Second)); err != nil {
			t.Fatalf("finish run: %v", err)
		}
	}

	a := &app{st: st}
	cmd := a.searchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("status", "failed"); err != nil {
		t.Fatalf("set status flag: %v", err)
	}
	if err := cmd.RunE(cmd, []string{"statustest"}); err != nil {
		t.Fatalf("search with --status: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "success") {
		t.Errorf("success run should be excluded by --status failed:\n%s", got)
	}
	if !strings.Contains(got, "failed") {
		t.Errorf("failed run should appear:\n%s", got)
	}
}

func TestSearchCommandInvalidSince(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	a := &app{st: st}
	cmd := a.searchCmd()
	if err := cmd.Flags().Set("since", "bogus"); err != nil {
		t.Fatalf("set since flag: %v", err)
	}
	if err := cmd.RunE(cmd, []string{"anything"}); err == nil {
		t.Error("expected error for invalid --since value")
	}
}

func TestSearchCommandVariadicArgs(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	started := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	id, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "go test ./...",
		ArgvJSON:  `["go","test","./..."]`,
		CWD:       "/tmp/project",
		StartedAt: started,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := st.FinishRun(id, store.StatusSuccess, 0, started.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	app := &app{st: st}
	cmd := app.searchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	// multi-word search without quoting
	if err := cmd.RunE(cmd, []string{"go", "test"}); err != nil {
		t.Fatalf("variadic search failed: %v", err)
	}
	if !strings.Contains(out.String(), "go test ./...") {
		t.Fatalf("variadic search missing result:\n%s", out.String())
	}
}
