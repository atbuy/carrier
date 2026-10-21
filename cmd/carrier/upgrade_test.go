package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The data directories in testdata/upgrade were recorded by real old
// releases; see testdata/upgrade/README.md. docs/reference/compatibility.md
// says new releases open data from earlier ones, including 0.x.

type upgradeRun struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	Command    string `json:"command"`
	CWD        string `json:"cwd"`
	ExitCode   *int   `json:"exit_code"`
	Label      string `json:"label"`
	SessionID  *int64 `json:"session_id"`
	StderrPath string `json:"stderr_path"`
	Notes      []struct {
		Text string `json:"text"`
	} `json:"notes"`
}

func TestUpgradeFromV0_1_0(t *testing.T) {
	env := newUpgradeEnv(t, "v0.1.0")

	runs := upgradeHistory(t, env)
	if got := upgradeRunIDs(runs); !slices.Equal(got, []int64{1, 2}) {
		t.Fatalf("run IDs after upgrade = %v, want [1 2]", got)
	}
	ok, failed := runs[0], runs[1]
	if ok.Status != "success" || ok.Command != "sh -c 'echo build ok'" || ok.CWD != "/home/dev/api" {
		t.Fatalf("run 1 after upgrade = %+v", ok)
	}
	if failed.Status != "failed" || failed.ExitCode == nil || *failed.ExitCode != 3 {
		t.Fatalf("run 2 after upgrade = %+v", failed)
	}
	if failed.StderrPath != "/home/dev/.local/share/carrier/runs/000002.stderr.log" {
		t.Fatalf("run 2 stderr path = %q", failed.StderrPath)
	}
	if ids := upgradeSearch(t, env, "refused"); !slices.Contains(ids, 2) {
		t.Fatalf("search after upgrade = %v, want run 2", ids)
	}
	upgradeRecordNewRun(t, env, 3)
}

func TestUpgradeFromV0_27_0(t *testing.T) {
	env := newUpgradeEnv(t, "v0.27.0")

	runs := upgradeHistory(t, env)
	if got := upgradeRunIDs(runs); !slices.Equal(got, []int64{1, 2, 3, 4, 6}) {
		t.Fatalf("run IDs after upgrade = %v, want [1 2 3 4 6]", got)
	}
	if runs[0].Label != "deploy" {
		t.Fatalf("run 1 label = %q, want deploy", runs[0].Label)
	}
	for _, r := range runs[2:] {
		if r.SessionID == nil || *r.SessionID != 1 {
			t.Fatalf("run %d session = %v, want session 1", r.ID, r.SessionID)
		}
	}

	var sessions []struct {
		ID      int64   `json:"id"`
		Label   string  `json:"label"`
		EndedAt *string `json:"ended_at"`
		Auto    bool    `json:"auto"`
		Kept    bool    `json:"kept"`
	}
	upgradeJSON(t, env, &sessions, "session", "list", "--json")
	if len(sessions) != 1 || sessions[0].Label != "BUG-1234: fix login" || sessions[0].EndedAt == nil || sessions[0].Auto || sessions[0].Kept {
		t.Fatalf("sessions after upgrade = %+v", sessions)
	}

	var notes []struct {
		Text      string `json:"text"`
		RunID     *int64 `json:"run_id"`
		SessionID *int64 `json:"session_id"`
	}
	upgradeJSON(t, env, &notes, "notes", "--json")
	if len(notes) != 2 || notes[0].Text != "the database was restarting" || notes[0].RunID == nil || *notes[0].RunID != 2 {
		t.Fatalf("notes after upgrade = %+v", notes)
	}
	if n := notes[1]; n.SessionID == nil || *n.SessionID != 1 || n.RunID == nil || *n.RunID != 4 {
		t.Fatalf("session note after upgrade = %+v", n)
	}

	var shown upgradeRun
	upgradeJSON(t, env, &shown, "show", "2", "--json")
	if len(shown.Notes) != 1 || shown.Notes[0].Text != "the database was restarting" {
		t.Fatalf("show 2 notes after upgrade = %+v", shown.Notes)
	}

	// Session references resolve labels recorded by the old release.
	var inSession []upgradeRun
	upgradeJSON(t, env, &inSession, "history", "-s", "1234", "--json")
	if got := upgradeRunIDs(inSession); !slices.Equal(got, []int64{3, 4, 6}) {
		t.Fatalf("history -s 1234 = %v, want [3 4 6]", got)
	}
	if ids := upgradeSearch(t, env, "refused"); !slices.Contains(ids, 2) || !slices.Contains(ids, 4) {
		t.Fatalf("search after upgrade = %v, want runs 2 and 4", ids)
	}
	upgradeRecordNewRun(t, env, 7)
}

// newUpgradeEnv copies testdata/upgrade/<version> to the default data
// directory of a fresh home.
func newUpgradeEnv(t *testing.T, version string) carrierTestEnv {
	t.Helper()
	env := newCarrierTestEnv(t)
	src := filepath.Join("testdata", "upgrade", version)
	dst := filepath.Join(env.home, ".local", "share", "carrier")
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatalf("copy %s: %v", src, err)
	}
	return env
}

func upgradeJSON(t *testing.T, env carrierTestEnv, v any, args ...string) {
	t.Helper()
	res := runCarrier(t, env, args...)
	res.requireExit(t, 0)
	if err := json.Unmarshal([]byte(res.stdout), v); err != nil {
		t.Fatalf("carrier %v: invalid JSON: %v\n%s", args, err, res.stdout)
	}
}

func upgradeHistory(t *testing.T, env carrierTestEnv) []upgradeRun {
	t.Helper()
	var runs []upgradeRun
	upgradeJSON(t, env, &runs, "history", "--json")
	return runs
}

func upgradeSearch(t *testing.T, env carrierTestEnv, query string) []int64 {
	t.Helper()
	var runs []upgradeRun
	upgradeJSON(t, env, &runs, "search", "--json", query)
	return upgradeRunIDs(runs)
}

func upgradeRunIDs(runs []upgradeRun) []int64 {
	ids := make([]int64, 0, len(runs))
	for _, r := range runs {
		ids = append(ids, r.ID)
	}
	return ids
}

// upgradeRecordNewRun checks that runs recorded after the upgrade continue
// the old run IDs.
func upgradeRecordNewRun(t *testing.T, env carrierTestEnv, wantID int64) {
	t.Helper()
	requireShell(t)
	runCarrier(t, env, "run", "-q", "sh", "-c", "exit 0").requireExit(t, 0)
	var last upgradeRun
	upgradeJSON(t, env, &last, "last", "--json")
	if last.ID != wantID || last.Status != "success" {
		t.Fatalf("new run after upgrade = %+v, want ID %d", last, wantID)
	}
}
