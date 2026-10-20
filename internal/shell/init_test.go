package shell

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestInitScript(t *testing.T) {
	for _, name := range InitShells {
		script, err := InitScript(name, "/opt/my tools/carrier")
		if err != nil {
			t.Fatalf("InitScript(%s): %v", name, err)
		}
		for _, want := range []string{"CARRIER_SESSION_ID", "CARRIER_NO_AUTO", `'/opt/my tools/carrier' shell --auto --program ` + name} {
			if !strings.Contains(script, want) {
				t.Errorf("%s snippet missing %q:\n%s", name, want, script)
			}
		}
	}
	if _, err := InitScript("tcsh", "/bin/carrier"); err == nil || !strings.Contains(err.Error(), "zsh, bash, fish") {
		t.Fatalf("unsupported shell error = %v", err)
	}
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Fatalf("shellQuote = %s", got)
	}
	if InitLine("fish") != "carrier init fish | source" || InitLine("zsh") != `eval "$(carrier init zsh)"` {
		t.Fatal("unexpected InitLine")
	}
}

// startBashWithInit runs an interactive bash whose ~/.bashrc loads the bash
// init snippet, pointing at a stand-in carrier that logs its arguments and
// exits with fakeExit. It types lines at the prompt and returns the log, the
// terminal output, and whether bash exited on its own.
func startBashWithInit(t *testing.T, fakeExit string, extraEnv []string, lines []string) (string, string, bool) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("bash init test is Unix-only")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	fake := filepath.Join(dir, "carrier")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho \"$@\" >> '"+logPath+"'\nexit "+fakeExit+"\n"), 0o755); err != nil {
		t.Fatalf("write fake carrier: %v", err)
	}
	snippet, err := InitScript("bash", fake)
	if err != nil {
		t.Fatalf("InitScript: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".bashrc"), []byte("PS1='PROMPT> '\n"+snippet), 0o600); err != nil {
		t.Fatalf("write bashrc: %v", err)
	}
	cmd := exec.Command(bash, "-i")
	cmd.Env = append(os.Environ(), "HOME="+dir, "TERM=dumb", "HISTFILE="+filepath.Join(dir, ".hist"))
	cmd.Env = append(cmd.Env, extraEnv...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("start bash: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	var out bytes.Buffer
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			out.Write(buf[:n])
			if err != nil {
				close(done)
				return
			}
		}
	}()
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()

	time.Sleep(300 * time.Millisecond)
	for _, line := range lines {
		_, _ = ptmx.Write([]byte(line + "\n"))
		time.Sleep(200 * time.Millisecond)
	}
	closedByItself := false
	select {
	case <-exited:
		closedByItself = true
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		<-exited
	}
	_ = ptmx.Close()
	<-done
	b, _ := os.ReadFile(logPath)
	return string(b), out.String(), closedByItself
}

func TestInitSnippetClosesTerminalAfterSession(t *testing.T) {
	calls, _, closed := startBashWithInit(t, "0", nil, nil)
	if strings.TrimSpace(calls) != "shell --auto --program bash" {
		t.Fatalf("carrier calls = %q", calls)
	}
	if !closed {
		t.Fatal("bash should exit when the automatic session ends")
	}
}

func TestInitSnippetFallsBackWhenCarrierFails(t *testing.T) {
	calls, out, closed := startBashWithInit(t, "1", nil, []string{"echo still-$((40+2))", "exit"})
	if strings.TrimSpace(calls) != "shell --auto --program bash" {
		t.Fatalf("carrier calls = %q", calls)
	}
	if !strings.Contains(out, "still-42") {
		t.Fatalf("terminal should keep working as a plain shell, got:\n%s", out)
	}
	if !closed {
		t.Fatal("typed exit should close the plain shell")
	}
}

func TestInitSnippetSkipsInsideSessionsAndWhenDisabled(t *testing.T) {
	for _, env := range [][]string{{"CARRIER_SESSION_ID=7"}, {"CARRIER_NO_AUTO=1"}} {
		calls, out, _ := startBashWithInit(t, "0", env, []string{"echo plain-$((1+1))", "exit"})
		if calls != "" {
			t.Fatalf("with %v carrier must not run, calls = %q", env, calls)
		}
		if !strings.Contains(out, "plain-2") {
			t.Fatalf("with %v the shell should start normally, got:\n%s", env, out)
		}
	}
}

func TestInitSnippetSkipsNonInteractiveShells(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash init test is Unix-only")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	fake := filepath.Join(dir, "carrier")
	_ = os.WriteFile(fake, []byte("#!/bin/sh\necho called >> '"+logPath+"'\n"), 0o755)
	snippet, _ := InitScript("bash", fake)
	rc := filepath.Join(dir, "rc")
	_ = os.WriteFile(rc, []byte(snippet), 0o600)
	out, err := exec.Command("bash", "-c", "source '"+rc+"'; echo sourced").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "sourced" {
		t.Fatalf("bash -c: %v %q", err, out)
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Fatal("carrier must not run in non-interactive shells")
	}
}
