package cli

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/store"
)

func TestParseSessionIDAndLabel(t *testing.T) {
	t.Run("first arg is integer id", func(t *testing.T) {
		t.Setenv("CARRIER_SESSION_ID", "")
		id, parts, err := parseSessionIDAndLabel([]string{"3", "my", "label"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != 3 {
			t.Fatalf("id = %d, want 3", id)
		}
		if len(parts) != 2 || parts[0] != "my" || parts[1] != "label" {
			t.Fatalf("labelParts = %v, want [my label]", parts)
		}
	})

	t.Run("non-integer args fall back to env var", func(t *testing.T) {
		t.Setenv("CARRIER_SESSION_ID", "5")
		id, parts, err := parseSessionIDAndLabel([]string{"my", "label"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != 5 {
			t.Fatalf("id = %d, want 5", id)
		}
		if len(parts) != 2 || parts[0] != "my" || parts[1] != "label" {
			t.Fatalf("labelParts = %v, want [my label]", parts)
		}
	})

	t.Run("empty args with env var", func(t *testing.T) {
		t.Setenv("CARRIER_SESSION_ID", "5")
		id, parts, err := parseSessionIDAndLabel([]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != 5 {
			t.Fatalf("id = %d, want 5", id)
		}
		if len(parts) != 0 {
			t.Fatalf("labelParts = %v, want []", parts)
		}
	})

	t.Run("non-integer args with no env var returns error", func(t *testing.T) {
		t.Setenv("CARRIER_SESSION_ID", "")
		_, _, err := parseSessionIDAndLabel([]string{"my", "label"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "CARRIER_SESSION_ID") {
			t.Fatalf("error %q does not mention CARRIER_SESSION_ID", err.Error())
		}
	})

	t.Run("empty args with no env var returns error", func(t *testing.T) {
		t.Setenv("CARRIER_SESSION_ID", "")
		_, _, err := parseSessionIDAndLabel([]string{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "CARRIER_SESSION_ID") {
			t.Fatalf("error %q does not mention CARRIER_SESSION_ID", err.Error())
		}
	})
}

func TestFormatSessionDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m30s"},
		{3723 * time.Second, "1h02m03s"},
	}
	for _, tc := range cases {
		if got := formatSessionDuration(tc.d); got != tc.want {
			t.Fatalf("formatSessionDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestSessionListCmd(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	now := time.Now()
	_, err = st.CreateSession(store.CreateSession{Label: "alpha", StartedAt: now})
	if err != nil {
		t.Fatalf("create session 1: %v", err)
	}
	id2, err := st.CreateSession(store.CreateSession{Label: "alpha", StartedAt: now})
	if err != nil {
		t.Fatalf("create session 2: %v", err)
	}
	if err := st.EndSession(id2, now.Add(time.Minute)); err != nil {
		t.Fatalf("end session: %v", err)
	}

	a := &app{st: st}
	cmd := a.sessionListCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("sessionListCmd failed: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "alpha") {
		t.Fatalf("output missing 'alpha':\n%s", got)
	}
	if !strings.Contains(got, "active") {
		t.Fatalf("output missing 'active':\n%s", got)
	}
}

func TestSessionLabelCmd(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	id, err := st.CreateSession(store.CreateSession{StartedAt: time.Now()})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	a := &app{st: st}
	cmd := a.sessionLabelCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	args := []string{strconv.FormatInt(id, 10), "my-label"}
	if err := cmd.RunE(cmd, args); err != nil {
		t.Fatalf("sessionLabelCmd failed: %v", err)
	}

	sess, err := st.GetSession(id)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Label != "my-label" {
		t.Fatalf("label = %q, want %q", sess.Label, "my-label")
	}
}

func TestHistoryCmdSessionsOnly(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	st := openHistoryStore(t)
	defer func() { _ = st.Close() }()
	seedHistoryRuns(t, st)

	now := time.Now()
	sessID, err := st.CreateSession(store.CreateSession{Label: "my-session", StartedAt: now})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	runID, err := st.CreateRun(store.CreateRun{
		Status:    store.StatusRunning,
		Mode:      store.ModeRun,
		Command:   "echo session-run",
		ArgvJSON:  `["echo","session-run"]`,
		CWD:       "/tmp",
		StartedAt: now,
		SessionID: &sessID,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := st.FinishRun(runID, store.StatusSuccess, 0, now.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	a := &app{st: st}
	cmd := a.historyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("sessions-only", "true"); err != nil {
		t.Fatalf("set sessions-only flag: %v", err)
	}

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("historyCmd --sessions-only failed: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "my-session") {
		t.Fatalf("output missing session header 'my-session':\n%s", got)
	}
	if strings.Contains(got, "echo session-run") {
		t.Fatalf("output should NOT contain individual run command:\n%s", got)
	}
	if strings.Contains(got, "go test ./...") {
		t.Fatalf("output should NOT contain standalone run commands:\n%s", got)
	}
}
