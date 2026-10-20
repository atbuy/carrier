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

// runBashHooks starts an interactive bash with the generated hooks under a
// PTY, types each line at a prompt, and returns the begin/end calls the hooks
// made to a stand-in carrier binary.
func runBashHooks(t *testing.T, userRC string, lines []string) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("bash hook test is Unix-only")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte("PS1='PROMPT> '\n"+userRC), 0o600); err != nil {
		t.Fatalf("write user bashrc: %v", err)
	}
	logPath := filepath.Join(dir, "calls.log")
	fakeCarrier := filepath.Join(dir, "carrier")
	// Called as: internal begin --state S --cmd CMD --cwd D
	//        or: internal end --state S --exit EC
	script := `#!/bin/sh
printf '%s %s\n' "$2" "$6" >> "` + logPath + `"
`
	if err := os.WriteFile(fakeCarrier, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake carrier: %v", err)
	}

	hookDir, err := WriteHookDir(fakeCarrier, filepath.Join(dir, "state.json"), bash, 1, "", "")
	if err != nil {
		t.Fatalf("WriteHookDir: %v", err)
	}
	defer func() { _ = os.RemoveAll(hookDir) }()

	cmd := exec.Command(bash, shellArgs(bash, hookDir)...)
	cmd.Env = append(os.Environ(), "HOME="+home, "HISTFILE="+filepath.Join(home, ".bash_history"), "TERM=dumb")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("start bash: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	var out bytes.Buffer
	chunks := make(chan []byte, 64)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunks <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				close(chunks)
				return
			}
		}
	}()
	waitPrompt := func(n int) {
		deadline := time.After(10 * time.Second)
		for strings.Count(out.String(), "PROMPT> ") < n {
			select {
			case b, ok := <-chunks:
				if !ok {
					return
				}
				out.Write(b)
			case <-deadline:
				t.Fatalf("timed out waiting for prompt %d; output:\n%s", n, out.String())
			}
		}
	}

	for i, line := range lines {
		waitPrompt(i + 1)
		if _, err := ptmx.Write([]byte(line + "\n")); err != nil {
			t.Fatalf("write line %q: %v", line, err)
		}
	}
	done := make(chan struct{})
	go func() {
		for b := range chunks {
			out.Write(b)
		}
		close(done)
	}()
	_ = cmd.Wait()
	<-done

	b, err := os.ReadFile(logPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read calls log: %v", err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// bashHasPS0 reports whether the installed bash supports PS0 (bash >= 4.4).
// Without it, lines made only of a subshell are not recorded.
func bashHasPS0(t *testing.T) bool {
	t.Helper()
	out, err := exec.Command("bash", "-c", `(( BASH_VERSINFO[0] > 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] >= 4) )) && echo yes`).Output()
	return err == nil && strings.TrimSpace(string(out)) == "yes"
}

func TestBashHooksRecordOneRunPerCommandLine(t *testing.T) {
	calls := runBashHooks(t, "greet() { echo \"hi $1\"; }\n", []string{
		"echo one && echo two",
		"echo a | tr a b",
		"(echo sub)",
		"greet bob",
		"",
		"false",
		"exit 3",
	})
	subshell := []string{"begin (echo sub)", "end 0"}
	if !bashHasPS0(t) {
		subshell = nil
	}
	want := []string{
		"begin echo one && echo two", "end 0",
		"begin echo a | tr a b", "end 0",
	}
	want = append(want, subshell...)
	want = append(want,
		"begin greet bob", "end 0",
		"begin false", "end 1",
		// The shell exits before its prompt hook runs; carrier shell finishes
		// this run from the final state.
		"begin exit 3",
	)
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("hook calls:\n%s\n\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestBashHooksHistoryControl(t *testing.T) {
	calls := runBashHooks(t, "HISTCONTROL=ignoreboth\n", []string{
		"echo dup",
		"echo dup",
		" echo hidden && echo more",
		"exit",
	})
	want := []string{
		"begin echo dup", "end 0",
		// ignoredups: no new history entry, but the newest one is identical.
		"begin echo dup", "end 0",
		// ignorespace: not in history, so only the first simple command is known.
		"begin echo hidden", "end 0",
		"begin exit",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("hook calls:\n%s\n\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestBashHooksKeepUserPromptCommand(t *testing.T) {
	calls := runBashHooks(t, "PROMPT_COMMAND='USER_PC=ran'\n", []string{
		`[ "$USER_PC" = ran ]`,
		"exit",
	})
	if len(calls) < 2 || calls[1] != "end 0" {
		t.Fatalf("user PROMPT_COMMAND should still run before each prompt, calls: %q", calls)
	}
}
