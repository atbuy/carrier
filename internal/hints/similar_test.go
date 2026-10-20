package hints

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

func failedRun(t *testing.T, st *store.Store, dir, name, stderr string, started time.Time, sessionID *int64) int64 {
	t.Helper()
	id, err := st.CreateRun(store.CreateRun{
		Status: store.StatusRunning, Mode: store.ModeRun, Command: name, ArgvJSON: `["sh"]`,
		CWD: "/tmp", StartedAt: started, SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	path := filepath.Join(dir, name+".stderr")
	if err := os.WriteFile(path, []byte(stderr), 0o600); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if err := st.UpdatePaths(id, "", path, ""); err != nil {
		t.Fatalf("update paths: %v", err)
	}
	if err := st.FinishRun(id, store.StatusFailed, 1, started.Add(time.Second)); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	return id
}

func TestSimilarFailure(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	sessID, _ := st.CreateSession(store.CreateSession{Label: "proxy-cert", StartedAt: time.Now()})
	weeksAgo := time.Now().Add(-21 * 24 * time.Hour)
	past := failedRun(t, st, dir, "past", "x509: certificate has expired or is not yet valid\n", weeksAgo, &sessID)
	for _, text := range []string{"cert expired on proxy", "renewed with certbot"} {
		if _, err := st.CreateNote(store.CreateNote{SessionID: &sessID, RunID: &past, Text: text, CreatedAt: weeksAgo}); err != nil {
			t.Fatalf("create note: %v", err)
		}
	}
	failedRun(t, st, dir, "retry", "x509: certificate has expired or is not yet valid\n", time.Now().Add(-5*time.Minute), nil)
	current := failedRun(t, st, dir, "current", "x509: certificate has expired or is not yet valid\n", time.Now(), nil)

	cfg := config.Default()
	if got := SimilarFailure(st, cfg, current, 1); got != "" {
		t.Fatalf("hints are opt-in, got %q", got)
	}

	cfg.Hints.SimilarFailures = true
	want := `seen before in run 1 (3 weeks ago, session "proxy-cert", 2 notes): carrier show 1`
	if got := SimilarFailure(st, cfg, current, 1); got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
	if got := SimilarFailure(st, cfg, current, exitInterrupted); got != "" {
		t.Fatalf("interrupted runs should not hint, got %q", got)
	}

	// With no age cutoff the retry matches too, so the hint points at the rest.
	cfg.Hints.SimilarMinAge = "0s"
	got := SimilarFailure(st, cfg, current, 1)
	if !strings.HasPrefix(got, "seen before in run 1 ") || !strings.HasSuffix(got, "; more: carrier similar 3") {
		t.Fatalf("hint with more matches = %q", got)
	}

	other := failedRun(t, st, dir, "other", "permission denied while opening socket\n", time.Now(), nil)
	if got := SimilarFailure(st, cfg, other, 1); got != "" {
		t.Fatalf("unrelated error should not hint, got %q", got)
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	cases := map[time.Duration]string{
		10 * time.Second:     "just now",
		time.Minute:          "1 minute ago",
		45 * time.Minute:     "45 minutes ago",
		3 * time.Hour:        "3 hours ago",
		24 * time.Hour:       "1 day ago",
		10 * 24 * time.Hour:  "10 days ago",
		21 * 24 * time.Hour:  "3 weeks ago",
		90 * 24 * time.Hour:  "3 months ago",
		800 * 24 * time.Hour: "2 years ago",
	}
	for d, want := range cases {
		if got := Ago(now.Add(-d), now); got != want {
			t.Errorf("Ago(-%s) = %q, want %q", d, got, want)
		}
	}
}
