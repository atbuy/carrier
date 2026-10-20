package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/store"
)

// runEmpty runs a command and returns its stdout and stderr separately.
func runEmpty(t *testing.T, cmd *cobra.Command, args []string, flags map[string]string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set flag %s: %v", k, err)
		}
	}
	err := cmd.RunE(cmd, args)
	return out.String(), errOut.String(), err
}

func TestEmptyListsPrintFirstRunHintToStderr(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	t.Setenv("CARRIER_SESSION_ID", "")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	a := &app{st: st}

	cases := []struct {
		name string
		cmd  *cobra.Command
		args []string
		want string
	}{
		{"history", a.historyCmd(), nil, firstRunHint},
		{"failed", a.failedCmd(), nil, firstRunHint},
		{"running", a.runningCmd(), nil, firstRunHint},
		{"search", a.searchCmd(), []string{"timeout"}, firstRunHint},
		{"stats", a.statsCmd(), nil, firstRunHint},
		{"session list", a.sessionListCmd(), nil, noSessionsHint},
		{"notes", a.notesCmd(), nil, "no notes yet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, err := runEmpty(t, tc.cmd, tc.args, nil)
			if err != nil {
				t.Fatalf("empty list should not be an error: %v", err)
			}
			if !strings.Contains(errOut, tc.want) {
				t.Fatalf("stderr = %q, want hint %q", errOut, tc.want)
			}
			if strings.Contains(out, "carrier:") {
				t.Fatalf("hint must not be written to stdout, got %q", out)
			}
		})
	}
}

func TestEmptyListsWithRunsExplainFilters(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	createFinishedRun(t, st, store.StatusSuccess, "echo hi", `["echo","hi"]`, "/tmp")
	a := &app{st: st}

	cases := []struct {
		name  string
		cmd   *cobra.Command
		args  []string
		flags map[string]string
		want  string
	}{
		{"history filter", a.historyCmd(), nil, map[string]string{"status": "failed"}, "no runs match these filters"},
		{"failed", a.failedCmd(), nil, nil, "no failed runs"},
		{"running", a.runningCmd(), nil, nil, "no runs are running"},
		{"search", a.searchCmd(), []string{"zzz"}, nil, `no runs match "zzz"`},
		{"stats command", a.statsCmd(), nil, map[string]string{"command": "zzz"}, `no runs match command "zzz"`},
		{"notes for run", a.notesCmd(), nil, map[string]string{"run": "1"}, "no notes for run 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errOut, err := runEmpty(t, tc.cmd, tc.args, tc.flags)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(errOut, tc.want) {
				t.Fatalf("stderr = %q, want %q", errOut, tc.want)
			}
			if strings.Contains(errOut, firstRunHint) {
				t.Fatalf("first-run hint should not show when runs exist, got %q", errOut)
			}
		})
	}
}

func TestEmptyJSONOutputHasNoHint(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	a := &app{st: st}
	for name, cmd := range map[string]*cobra.Command{
		"history": a.historyCmd(), "running": a.runningCmd(), "notes": a.notesCmd(),
	} {
		out, errOut, err := runEmpty(t, cmd, nil, map[string]string{"json": "true"})
		if err != nil {
			t.Fatalf("%s --json: %v", name, err)
		}
		if strings.TrimSpace(out) != "[]" || errOut != "" {
			t.Fatalf("%s --json should print [] with no hint, got stdout=%q stderr=%q", name, out, errOut)
		}
	}
	out, errOut, err := runEmpty(t, a.searchCmd(), []string{"x"}, map[string]string{"json": "true"})
	if err != nil || strings.TrimSpace(out) != "[]" || errOut != "" {
		t.Fatalf("search --json should print [] with no hint, got stdout=%q stderr=%q err=%v", out, errOut, err)
	}
}

func TestMissingRunErrors(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	a := &app{st: st}

	_, _, err := runEmpty(t, a.lastCmd(), nil, nil)
	if err == nil || err.Error() != firstRunHint {
		t.Fatalf("last with no runs: got %v", err)
	}
	_, _, err = runEmpty(t, a.lastCmd(), []string{"3"}, nil)
	if err == nil || err.Error() != firstRunHint {
		t.Fatalf("last 3 with no runs: got %v", err)
	}
	for name, run := range map[string]func() error{
		"show":   func() error { _, _, err := runEmpty(t, a.showCmd(), []string{"7"}, nil); return err },
		"tail":   func() error { _, _, err := runEmpty(t, a.tailCmd(), []string{"7"}, nil); return err },
		"export": func() error { _, _, err := runEmpty(t, a.exportCmd(), []string{"7"}, nil); return err },
		"rerun":  func() error { _, _, err := runEmpty(t, a.rerunCmd(), []string{"7"}, nil); return err },
		"diff":   func() error { _, _, err := runEmpty(t, a.diffCmd(), []string{"7", "8"}, nil); return err },
		"note": func() error {
			_, _, err := runEmpty(t, a.noteCmd(), []string{"x"}, map[string]string{"run": "7"})
			return err
		},
	} {
		if err := run(); err == nil || err.Error() != "run 7 not found" {
			t.Fatalf("%s with missing run: got %v, want run 7 not found", name, err)
		}
	}
}

func TestResolveSessionMissing(t *testing.T) {
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()

	if _, err := resolveSession(st, "5"); err == nil || !strings.HasPrefix(err.Error(), "session 5 not found;") {
		t.Fatalf("missing session ID: got %v", err)
	}
	if _, err := resolveSession(st, "dbg"); err == nil || !strings.HasPrefix(err.Error(), `no session with label "dbg";`) {
		t.Fatalf("missing session label: got %v", err)
	}
	if _, err := resolveSession(st, "dbg"); strings.Contains(err.Error(), "sql:") {
		t.Fatalf("error should not leak database text: %v", err)
	}
}

func TestSessionsOnlyLabelFilterHint(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openCmdStore(t)
	defer func() { _ = st.Close() }()
	if _, err := st.CreateSession(store.CreateSession{Label: "api", StartedAt: time.Now()}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	a := &app{st: st}
	_, errOut, err := runEmpty(t, a.historyCmd(), nil, map[string]string{"sessions-only": "true", "label": "web"})
	if err != nil {
		t.Fatalf("history --sessions-only: %v", err)
	}
	if !strings.Contains(errOut, `no sessions match label "web"`) {
		t.Fatalf("stderr = %q", errOut)
	}
}
