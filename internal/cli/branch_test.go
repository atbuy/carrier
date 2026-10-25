package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/gitmeta"
	carriershell "github.com/atbuy/carrier/internal/shell"
)

// fakeRepo makes a repository at dir/repo on branch, as far as gitmeta.Head
// reads one, and changes to it.
func fakeRepo(t *testing.T, dir, branch string) string {
	t.Helper()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	checkout(t, repo, branch)
	t.Chdir(repo)
	return repo
}

// checkout points HEAD at branch, or detaches it when branch is a commit.
func checkout(t *testing.T, repo, branch string) {
	t.Helper()
	head := "ref: refs/heads/" + branch
	if len(branch) == 40 {
		head = branch
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "HEAD"), []byte(head+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// captureTerminal sends what the hooks print to the terminal to a buffer.
func captureTerminal(t *testing.T) *bytes.Buffer {
	t.Helper()
	t.Setenv("CARRIER_COLOR", "never")
	var buf bytes.Buffer
	old := openTerminal
	openTerminal = func() (io.WriteCloser, error) { return nopWriteCloser{&buf}, nil }
	t.Cleanup(func() { openTerminal = old })
	return &buf
}

// beginCommand does what the hooks do before a command starts.
func beginCommand(t *testing.T, a *app, statePath string) {
	t.Helper()
	cwd, _ := os.Getwd()
	head, _ := gitmeta.Head(cwd)
	a.noteRepoBranch(statePath, head)
}

func TestNoticeBranchChangeSuggests(t *testing.T) {
	a, dir := autoSessionApp(t)
	term := captureTerminal(t)
	current := newSession(t, a, "", false)
	feat := newSession(t, a, "BUG-1234: fix login", false)
	repo := fakeRepo(t, dir, "main")
	statePath, _ := recordedShell(t, a, dir, current, "git checkout BUG-1234-fix")
	if err := a.st.LinkBranch(canonicalRepoRoot(repo), "BUG-1234-fix", feat, time.Now()); err != nil {
		t.Fatal(err)
	}

	beginCommand(t, a, statePath)
	checkout(t, repo, "BUG-1234-fix")
	a.noticeBranchChange(statePath)
	want := `carrier: branch BUG-1234-fix has session 2 · BUG-1234: fix login; switch with carrier attach 2` + "\n"
	if term.String() != want {
		t.Fatalf("terminal = %q, want %q", term.String(), want)
	}
	if state := (&carriershell.StateFile{Path: statePath}).Read(); state.SessionID != current {
		t.Fatalf("suggest moved the shell: %+v", state)
	}

	// The next command on the same branch says nothing again.
	term.Reset()
	beginCommand(t, a, statePath)
	a.noticeBranchChange(statePath)
	// A branch without a link, and a detached HEAD, say nothing either.
	checkout(t, repo, "main")
	a.noticeBranchChange(statePath)
	checkout(t, repo, strings.Repeat("a", 40))
	a.noticeBranchChange(statePath)
	if term.Len() != 0 {
		t.Fatalf("terminal = %q, want nothing", term.String())
	}
}

func TestNoticeBranchChangeFollows(t *testing.T) {
	a, dir := autoSessionApp(t)
	a.cfg.Shell.BranchSessions = config.BranchFollow
	term := captureTerminal(t)
	current := newSession(t, a, "", false)
	feat := newSession(t, a, "feat", false)
	repo := fakeRepo(t, dir, "main")
	statePath, runID := recordedShell(t, a, dir, current, "git checkout feat")
	if err := a.st.LinkBranch(canonicalRepoRoot(repo), "feat", feat, time.Now()); err != nil {
		t.Fatal(err)
	}

	beginCommand(t, a, statePath)
	checkout(t, repo, "feat")
	a.noticeBranchChange(statePath)
	if want := "carrier: branch feat → session 2 · feat\n"; term.String() != want {
		t.Fatalf("terminal = %q, want %q", term.String(), want)
	}
	if state := (&carriershell.StateFile{Path: statePath}).Read(); state.SessionID != feat {
		t.Fatalf("state = %+v, want session %d", state, feat)
	}
	// The checkout stays in the session it was typed in.
	if run, err := a.st.GetRun(runID); err != nil || run.SessionID == nil || *run.SessionID != current {
		t.Fatalf("checkout run = %+v, %v; want it in session %d", run, err, current)
	}
}

func TestNoticeBranchChangeQuiet(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, a *app, repo, statePath string)
	}{
		{"off", func(t *testing.T, a *app, repo, statePath string) {
			a.cfg.Shell.BranchSessions = config.BranchOff
			beginCommand(t, a, statePath)
		}},
		// Without a branch noted before the command, a branch is only noted.
		{"first command", func(t *testing.T, a *app, repo, statePath string) {}},
		{"another repository", func(t *testing.T, a *app, repo, statePath string) {
			if err := carriershell.WriteBranch(statePath, filepath.Join(repo, "other"), "main"); err != nil {
				t.Fatal(err)
			}
		}},
		{"current session", func(t *testing.T, a *app, repo, statePath string) {
			beginCommand(t, a, statePath)
			if err := a.st.LinkBranch(canonicalRepoRoot(repo), "feat", 1, time.Now()); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, dir := autoSessionApp(t)
			a.cfg.Shell.BranchSessions = config.BranchFollow
			term := captureTerminal(t)
			current := newSession(t, a, "", false)
			feat := newSession(t, a, "feat", false)
			repo := fakeRepo(t, dir, "main")
			statePath, _ := recordedShell(t, a, dir, current, "git checkout feat")
			if err := a.st.LinkBranch(canonicalRepoRoot(repo), "feat", feat, time.Now()); err != nil {
				t.Fatal(err)
			}
			tc.setup(t, a, repo, statePath)
			checkout(t, repo, "feat")
			a.noticeBranchChange(statePath)
			if term.Len() != 0 {
				t.Errorf("terminal = %q, want nothing", term.String())
			}
			if state := (&carriershell.StateFile{Path: statePath}).Read(); state.SessionID != current {
				t.Errorf("state = %+v, want session %d", state, current)
			}
		})
	}
}

func TestAttachBranch(t *testing.T) {
	a, dir := autoSessionApp(t)
	t.Setenv("CARRIER_COLOR", "never")
	current := newSession(t, a, "", true)
	repo := fakeRepo(t, dir, "BUG-1234-fix")
	statePath, _ := recordedShell(t, a, dir, current, "c a -b")

	// Without a session for the branch, one named after it is started.
	out, _, err := runEmpty(t, a.attachCmd(), nil, map[string]string{"branch": "true"})
	if err != nil {
		t.Fatalf("attach --branch: %v", err)
	}
	if !strings.Contains(out, `started session 2 "BUG-1234-fix" for branch BUG-1234-fix`) || !strings.Contains(out, "switched to session 2") {
		t.Fatalf("output = %q", out)
	}
	if state := (&carriershell.StateFile{Path: statePath}).Read(); state.SessionID != 2 {
		t.Fatalf("state = %+v, want session 2", state)
	}
	if sess, _ := a.st.BranchSession(canonicalRepoRoot(repo), "BUG-1234-fix"); sess == nil || sess.ID != 2 {
		t.Fatalf("branch session = %+v, want session 2", sess)
	}

	// A session labeled like the branch is used, linked, and kept when it is
	// automatic.
	checkout(t, repo, "main")
	labeled := newSession(t, a, "Main", true)
	recordedShell(t, a, dir, 2, "c a -b")
	out, errOut, err := runEmpty(t, a.attachCmd(), nil, map[string]string{"branch": "true"})
	if err != nil || !strings.Contains(out, "switched to session 3") || strings.Contains(out, "started") {
		t.Fatalf("attach --branch to a labeled session: out=%q err=%v", out, err)
	}
	if !strings.Contains(errOut, "kept") {
		t.Errorf("stderr = %q, want the kept hint", errOut)
	}
	if sess, _ := a.st.GetSession(labeled); !sess.Kept {
		t.Error("an automatic session linked to a branch should be kept")
	}

	// The linked session wins over labels from then on.
	checkout(t, repo, "BUG-1234-fix")
	if out, _, err = runEmpty(t, a.attachCmd(), nil, map[string]string{"branch": "true"}); err != nil || !strings.Contains(out, "switched to session 2") {
		t.Fatalf("attach --branch to the linked session: out=%q err=%v", out, err)
	}

	t.Chdir(dir)
	if _, _, err = runEmpty(t, a.attachCmd(), nil, map[string]string{"branch": "true"}); err != errNotOnBranch {
		t.Fatalf("attach --branch outside a repository: err=%v", err)
	}
}

func TestSessionBranch(t *testing.T) {
	a, dir := autoSessionApp(t)
	current := newSession(t, a, "", false)
	other := newSession(t, a, "BUG-1234: fix login", false)
	repo := fakeRepo(t, dir, "feat")
	root := canonicalRepoRoot(repo)
	recordedShell(t, a, dir, current, "carrier session branch")

	out, _, err := runEmpty(t, a.sessionBranchCmd(), nil, nil)
	if err != nil || out != "carrier: linked branch feat to session 1\n" {
		t.Fatalf("session branch: out=%q err=%v", out, err)
	}
	if out, _, err = runEmpty(t, a.sessionBranchCmd(), []string{"BUG-1234"}, nil); err != nil || !strings.Contains(out, "to session 2") {
		t.Fatalf("session branch BUG-1234: out=%q err=%v", out, err)
	}
	if sess, _ := a.st.BranchSession(root, "feat"); sess == nil || sess.ID != other {
		t.Fatalf("linking again should move the branch: %+v", sess)
	}

	if out, _, err = runEmpty(t, a.sessionBranchCmd(), nil, map[string]string{"unlink": "true"}); err != nil || out != "carrier: unlinked branch feat\n" {
		t.Fatalf("session branch --unlink: out=%q err=%v", out, err)
	}
	if out, _, _ = runEmpty(t, a.sessionBranchCmd(), nil, map[string]string{"unlink": "true"}); !strings.Contains(out, "is not linked") {
		t.Fatalf("unlink twice: out=%q", out)
	}
	if _, _, err = runEmpty(t, a.sessionBranchCmd(), []string{"1"}, map[string]string{"unlink": "true"}); err == nil {
		t.Fatal("--unlink with a session should fail")
	}

	t.Setenv("CARRIER_SESSION_ID", "")
	t.Setenv("CARRIER_SHELL_STATE", "")
	if _, _, err = runEmpty(t, a.sessionBranchCmd(), nil, nil); err == nil || !strings.Contains(err.Error(), "recorded shell") {
		t.Fatalf("outside a recorded shell: err=%v", err)
	}
	t.Chdir(dir)
	if _, _, err = runEmpty(t, a.sessionBranchCmd(), nil, nil); err != errNotOnBranch {
		t.Fatalf("outside a repository: err=%v", err)
	}
}

func TestStatusAndSessionShowListBranches(t *testing.T) {
	a, dir := autoSessionApp(t)
	t.Setenv("CARRIER_COLOR", "never")
	t.Setenv("TMUX", "")
	current := newSession(t, a, "", true)
	feat := newSession(t, a, "BUG-1234: fix login", false)
	repo := fakeRepo(t, dir, "main")
	recordedShell(t, a, dir, current, "carrier status")

	if out := statusOutput(t, a, nil); !strings.Contains(out, `Branch     main not linked to a session; "carrier session branch" links it`) {
		t.Fatalf("status:\n%s", out)
	}
	if err := a.st.LinkBranch(canonicalRepoRoot(repo), "main", feat, time.Now()); err != nil {
		t.Fatal(err)
	}
	out := statusOutput(t, a, nil)
	for _, want := range []string{"Branch     main → session 2 · BUG-1234: fix login", `"carrier attach 2" switches to it`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	out, _, err := runEmpty(t, a.sessionShowCmd(), []string{"2"}, nil)
	if err != nil || !strings.Contains(out, "Branches: main (repo)") {
		t.Fatalf("session show: out=%q err=%v", out, err)
	}
}
