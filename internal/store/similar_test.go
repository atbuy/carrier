package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestErrorSignature(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{
			"drops digits",
			"starting\ndial tcp 127.0.0.1:5432: connect: connection refused\n",
			"dial tcp connect connection refused",
		},
		{
			"newest error lines first",
			"warning: deprecated\nError: cannot open config\nexit status 1: failed\n",
			"exit status failed error cannot open config",
		},
		{
			"falls back to last line",
			"compiling\nsomething odd happened here\n\n",
			"something odd happened here",
		},
		{
			"strips ansi and stop words",
			"\x1b[31mfatal: not a git repository\x1b[0m\n",
			"fatal git repository",
		},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(ErrorSignature(tc.output), " "); got != tc.want {
				t.Fatalf("ErrorSignature = %q, want %q", got, tc.want)
			}
		})
	}
	long := strings.Repeat("alpha beta gamma delta epsilon zeta eta theta iota kappa ", 3) + "error\n"
	if got := ErrorSignature(long); len(got) != signatureMaxTerms {
		t.Fatalf("expected at most %d terms, got %d: %v", signatureMaxTerms, len(got), got)
	}
}

type similarFixture struct {
	t   *testing.T
	st  *Store
	dir string
}

func newSimilarFixture(t *testing.T) *similarFixture {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &similarFixture{t: t, st: st, dir: dir}
}

// run records a finished run whose stderr is the given text.
func (f *similarFixture) run(command, stderr, status string, started time.Time) int64 {
	f.t.Helper()
	id, err := f.st.CreateRun(CreateRun{
		Status: StatusRunning, Mode: ModeRun, Command: command, ArgvJSON: `["sh"]`,
		CWD: "/tmp", StartedAt: started,
	})
	if err != nil {
		f.t.Fatalf("create run: %v", err)
	}
	path := filepath.Join(f.dir, "runs", command+".stderr")
	if err := os.WriteFile(path, []byte(stderr), 0o600); err != nil {
		f.t.Fatalf("write stderr: %v", err)
	}
	if err := f.st.UpdatePaths(id, "", path, ""); err != nil {
		f.t.Fatalf("update paths: %v", err)
	}
	exit := 0
	if status == StatusFailed {
		exit = 1
	}
	if err := f.st.FinishRun(id, status, exit, started.Add(time.Second)); err != nil {
		f.t.Fatalf("finish run: %v", err)
	}
	return id
}

func similarIDs(runs []SimilarRun) []int64 {
	out := make([]int64, len(runs))
	for i, r := range runs {
		out[i] = r.Run.ID
	}
	return out
}

func TestSimilarFailures(t *testing.T) {
	f := newSimilarFixture(t)
	old := time.Now().Add(-48 * time.Hour)
	plain := f.run("plain", "dial tcp 127.0.0.1:5432: connect: connection refused\n", StatusFailed, old)
	noted := f.run("noted", "dial tcp 10.0.0.1:6543: connect: connection refused\n", StatusFailed, old.Add(time.Minute))
	if _, err := f.st.CreateNote(CreateNote{RunID: &noted, Text: "db was down", CreatedAt: old}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	f.run("succeeded", "dial tcp 1.1.1.1:1: connect: connection refused\n", StatusSuccess, old)
	f.run("other", "permission denied: /etc/shadow\n", StatusFailed, old)
	recent := f.run("recent", "dial tcp 127.0.0.1:5432: connect: connection refused\n", StatusFailed, time.Now().Add(-time.Minute))
	current := f.run("current", "dial tcp 192.168.1.9:80: connect: connection refused\n", StatusFailed, time.Now())

	got, err := f.st.SimilarFailures(current, time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("SimilarFailures: %v", err)
	}
	// Noted run first; successful, unrelated, too-recent, and self excluded.
	if want := []int64{noted, plain}; !equalIDs(similarIDs(got), want) {
		t.Fatalf("matches = %v, want %v", similarIDs(got), want)
	}
	if got[0].NoteCount != 1 || got[1].NoteCount != 0 {
		t.Fatalf("note counts = %d, %d", got[0].NoteCount, got[1].NoteCount)
	}

	all, err := f.st.SimilarFailures(current, time.Now().Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("SimilarFailures without age cutoff: %v", err)
	}
	if len(all) != 3 || !containsID(similarIDs(all), recent) {
		t.Fatalf("without cutoff the recent run should match too, got %v", similarIDs(all))
	}

	limited, err := f.st.SimilarFailures(current, time.Now(), 1)
	if err != nil || len(limited) != 1 || limited[0].Run.ID != noted {
		t.Fatalf("limit 1 should return the noted run, got %v (err %v)", similarIDs(limited), err)
	}
}

func TestSimilarFailuresLabelRanksFirst(t *testing.T) {
	f := newSimilarFixture(t)
	old := time.Now().Add(-48 * time.Hour)
	f.run("a", "Error: cannot open config file\n", StatusFailed, old)
	labeled := f.run("b", "Error: cannot open config file\n", StatusFailed, old)
	if err := f.st.SetLabel(labeled, "config"); err != nil {
		t.Fatalf("set label: %v", err)
	}
	current := f.run("c", "Error: cannot open config file\n", StatusFailed, time.Now())
	got, err := f.st.SimilarFailures(current, time.Now(), 10)
	if err != nil {
		t.Fatalf("SimilarFailures: %v", err)
	}
	if len(got) != 2 || got[0].Run.ID != labeled {
		t.Fatalf("labeled run should rank first, got %v", similarIDs(got))
	}
}

func TestSimilarFailuresIgnoresCommandText(t *testing.T) {
	f := newSimilarFixture(t)
	old := time.Now().Add(-48 * time.Hour)
	// The error words appear only in this run's command, not its output.
	if _, err := f.st.CreateRun(CreateRun{
		Status: StatusRunning, Mode: ModeRun, Command: "echo connection refused upstream", ArgvJSON: `["echo"]`,
		CWD: "/tmp", StartedAt: old,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	current := f.run("current", "connection refused upstream\n", StatusFailed, time.Now())
	got, err := f.st.SimilarFailures(current, time.Now(), 10)
	if err != nil || len(got) != 0 {
		t.Fatalf("command-text match should not count, got %v (err %v)", similarIDs(got), err)
	}
}

func TestSimilarFailuresNeedsSignature(t *testing.T) {
	f := newSimilarFixture(t)
	f.run("old", "boom\n", StatusFailed, time.Now().Add(-48*time.Hour))
	current := f.run("current", "boom\n", StatusFailed, time.Now())
	got, err := f.st.SimilarFailures(current, time.Now(), 10)
	if err != nil || got != nil {
		t.Fatalf("one-word output should not search, got %v (err %v)", similarIDs(got), err)
	}
	if _, err := f.st.SimilarFailures(9999, time.Now(), 10); err == nil {
		t.Fatal("expected error for missing run")
	}
}

func TestReindexRunPicksUpLaterOutput(t *testing.T) {
	f := newSimilarFixture(t)
	id := f.run("late", "", StatusFailed, time.Now())
	r, err := f.st.GetRun(id)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	// Output lands after the run was indexed, as with buffered shell logs.
	if err := os.WriteFile(r.StderrPath, []byte("segmentation fault in worker\n"), 0o600); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	if res, _ := f.st.SearchRuns("segmentation", 10, SearchFilter{}); len(res) != 0 {
		t.Fatalf("output should not be indexed before ReindexRun, got %d results", len(res))
	}
	if err := f.st.ReindexRun(id); err != nil {
		t.Fatalf("ReindexRun: %v", err)
	}
	if res, _ := f.st.SearchRuns("segmentation", 10, SearchFilter{}); len(res) != 1 {
		t.Fatalf("output should be searchable after ReindexRun, got %d results", len(res))
	}
}

func TestReadTailKeepsEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.log")
	content := strings.Repeat("x", 100) + "tail-marker"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := readTail(path, 20); got != content[len(content)-20:] {
		t.Fatalf("readTail = %q", got)
	}
	if got := readTail(path, 1000); got != content {
		t.Fatal("readTail should return whole small file")
	}
	if readTail("", 10) != "" || readTail(filepath.Join(t.TempDir(), "missing"), 10) != "" {
		t.Fatal("readTail should return empty for missing paths")
	}
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsID(list []int64, id int64) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}
