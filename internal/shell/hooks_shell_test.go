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

// runBashHooks runs runShellHooks with bash.
func runBashHooks(t *testing.T, userRC string, lines []string) []string {
	t.Helper()
	return runShellHooks(t, "bash", userRC, lines)
}

// runShellHooks starts an interactive shell (bash, zsh, or fish) with the
// generated hooks under a PTY, types each line at a prompt, and returns the
// begin/end calls the hooks made to a stand-in carrier binary. userRC is
// added to the user's own shell config, which the hooks load.
func runShellHooks(t *testing.T, shellName, userRC string, lines []string) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell hook test is Unix-only")
	}
	program, err := exec.LookPath(shellName)
	if err != nil {
		t.Skipf("%s not installed", shellName)
	}

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	// Each shell's own config, with a prompt the test can wait for.
	userConfig := map[string]struct{ path, prompt string }{
		"bash": {".bashrc", "PS1='PROMPT> '\n"},
		"zsh":  {".zshrc", "PS1='PROMPT> '\nunsetopt PROMPT_SP\n"},
		"fish": {".config/fish/config.fish", "function fish_prompt; echo -n 'PROMPT> '; end\nfunction fish_greeting; end\n"},
	}[shellName]
	rcPath, prompt := userConfig.path, userConfig.prompt
	if err := os.MkdirAll(filepath.Dir(filepath.Join(home, rcPath)), 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, rcPath), []byte(prompt+userRC), 0o600); err != nil {
		t.Fatalf("write user config: %v", err)
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

	hookDir, err := WriteHookDir(fakeCarrier, filepath.Join(dir, "state.json"), program, 1, "", "", true)
	if err != nil {
		t.Fatalf("WriteHookDir: %v", err)
	}
	defer func() { _ = os.RemoveAll(hookDir) }()
	isolateZsh(t, shellName, hookDir)

	cmd := exec.Command(program, shellArgs(program, hookDir)...)
	cmd.Env = append(os.Environ(), "HOME="+home, "HISTFILE="+filepath.Join(home, ".history"), "TERM=dumb",
		"ZDOTDIR="+hookDir, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "XDG_DATA_HOME="+filepath.Join(home, ".local", "share"))
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("start %s: %v", shellName, err)
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
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-exited
		_ = ptmx.Close()
		<-done
		t.Fatalf("%s did not exit after the last line; output:\n%s", shellName, out.String())
	}
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
	want = append(
		want,
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

func TestZshHooksRecordOneRunPerCommandLine(t *testing.T) {
	calls := runShellHooks(t, "zsh", "greet() { echo \"hi $1\"; }\n", []string{
		"echo one && echo two",
		"echo a | tr a b",
		"(echo sub)",
		"greet bob",
		"",
		"false",
		"exit 3",
	})
	// Nothing at startup or for the empty line: precmd only ends a run that
	// preexec began.
	want := []string{
		"begin echo one && echo two", "end 0",
		"begin echo a | tr a b", "end 0",
		"begin (echo sub)", "end 0",
		"begin greet bob", "end 0",
		"begin false", "end 1",
		"begin exit 3",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("hook calls:\n%s\n\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestFishHooksRecordOneRunPerCommandLine(t *testing.T) {
	calls := runShellHooks(t, "fish", "function greet; echo hi $argv; end\n", []string{
		"echo one && echo two",
		"echo a | tr a b",
		"begin; echo sub; end",
		"greet bob",
		"",
		"false",
		"exit 3",
	})
	want := []string{
		"begin echo one && echo two", "end 0",
		"begin echo a | tr a b", "end 0",
		"begin begin; echo sub; end", "end 0",
		"begin greet bob", "end 0",
		"begin false", "end 1",
		// fish runs its postexec hook for exit too.
		"begin exit 3", "end 3",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("hook calls:\n%s\n\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestZshRightPromptIndicatorAddedOnce(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	hookDir, err := WriteHookDir("/bin/true", filepath.Join(dir, "state.json"), "zsh", 1, "", "", false)
	if err != nil {
		t.Fatalf("WriteHookDir: %v", err)
	}
	defer func() { _ = os.RemoveAll(hookDir) }()
	cmd := exec.Command("zsh", "-i", "-c", `RPROMPT=mine; for i in 1 2 3; do _carrier_precmd; done; print -r -- "[$RPROMPT]"`)
	cmd.Env = append(os.Environ(), "ZDOTDIR="+hookDir, "HOME="+dir)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("zsh: %v", err)
	}
	if got := string(out)[strings.LastIndex(string(out), "["):]; strings.TrimSpace(got) != "[%B%F{}carrier #1%f%b mine]" {
		t.Fatalf("RPROMPT after three prompts = %q", got)
	}
}

// isolateZsh keeps zsh from reading the system-wide /etc/zsh files (zshrc,
// zprofile, zlogin), which differ between machines. On Ubuntu CI runners the
// system zshrc runs compinit, which stops to ask about insecure directories.
// dir is where zsh reads .zshenv: $ZDOTDIR, or $HOME when it is unset.
func isolateZsh(t *testing.T, shellName, dir string) {
	t.Helper()
	if shellName != "zsh" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, ".zshenv"), []byte("unsetopt GLOBAL_RCS\n"), 0o600); err != nil {
		t.Fatalf("write .zshenv: %v", err)
	}
}

// TestHooksFollowSessionSwitch checks that each shell picks up a session
// switch at the next prompt: "carrier attach" rewrites the session file, and
// the prompt indicator and CARRIER_SESSION_ID/LABEL follow. Labels are shown
// as text, never expanded.
func TestHooksFollowSessionSwitch(t *testing.T) {
	const label = `fix $(echo nope) \w 50%`
	for _, tc := range []struct {
		shell  string
		script func(hookDir, sessionFile string) []string
	}{
		{"zsh", func(hookDir, sessionFile string) []string {
			return []string{"-i", "-c", `RPROMPT=mine; _carrier_precmd
printf '9\n%s\n' 'OPS-7: deploy' > ` + sessionFile + `
_carrier_precmd; _carrier_precmd
print -r -- "PROMPT[$RPROMPT] ID[$CARRIER_SESSION_ID] LABEL[$CARRIER_SESSION_LABEL]"`}
		}},
		{"bash", func(hookDir, sessionFile string) []string {
			return []string{"--rcfile", filepath.Join(hookDir, ".bashrc"), "-i", "-c", `trap - DEBUG
printf '9\n%s\n' 'OPS-7: deploy' > ` + sessionFile + `
_carrier_end; trap - DEBUG
echo "PROMPT[${PS1@P}] ID[$CARRIER_SESSION_ID] LABEL[$CARRIER_SESSION_LABEL]"`}
		}},
		{"fish", func(hookDir, sessionFile string) []string {
			return []string{"--no-config", "-c", "source " + filepath.Join(hookDir, "carrier.fish") + `
printf '9\n%s\n' 'OPS-7: deploy' > ` + sessionFile + `
_carrier_session
echo "PROMPT["(fish_right_prompt)"] ID[$CARRIER_SESSION_ID] LABEL[$CARRIER_SESSION_LABEL]"`}
		}},
	} {
		t.Run(tc.shell, func(t *testing.T) {
			if _, err := exec.LookPath(tc.shell); err != nil {
				t.Skipf("%s not installed", tc.shell)
			}
			dir := t.TempDir()
			statePath := filepath.Join(dir, "state.json")
			if err := WriteSessionInfo(statePath, 3, label); err != nil {
				t.Fatal(err)
			}
			hookDir, err := WriteHookDir("/bin/true", statePath, tc.shell, 3, label, "", false)
			if err != nil {
				t.Fatalf("WriteHookDir: %v", err)
			}
			defer func() { _ = os.RemoveAll(hookDir) }()
			isolateZsh(t, tc.shell, hookDir)

			run := func(sessionFile string) string {
				t.Helper()
				cmd := exec.Command(tc.shell, tc.script(hookDir, sessionFile)...)
				cmd.Env = append(os.Environ(), "ZDOTDIR="+hookDir, "HOME="+dir, "XDG_CONFIG_HOME="+filepath.Join(dir, ".config"))
				out, err := cmd.Output()
				if err != nil {
					t.Fatalf("%s: %v\n%s", tc.shell, err, out)
				}
				// What the script prints, after the title escapes.
				got := string(out)
				i := strings.LastIndex(got, "PROMPT[")
				if i < 0 {
					t.Fatalf("%s printed no prompt:\n%s", tc.shell, got)
				}
				return got[i:]
			}

			// Writing to /dev/null leaves the session file as it was.
			before := run("/dev/null")
			if !strings.Contains(before, "ID[3] LABEL["+label+"]") || !strings.Contains(before, label) {
				t.Fatalf("before the switch, want session 3 with its label shown as text:\n%s", before)
			}

			after := run(SessionInfoPath(statePath))
			if !strings.Contains(after, "ID[9] LABEL[OPS-7: deploy]") {
				t.Fatalf("environment after the switch:\n%s", after)
			}
			if !strings.Contains(after, "carrier #9") || !strings.Contains(after, "OPS-7: deploy") || strings.Contains(after, "carrier #3") {
				t.Fatalf("prompt after the switch:\n%s", after)
			}
			if tc.shell == "zsh" && !strings.Contains(after, "PROMPT[%B%F{}carrier #9%f%b %F{}OPS-7: deploy%f mine]") {
				t.Fatalf("zsh should replace its indicator, not add another:\n%s", after)
			}
		})
	}
}

// TestZshHooksUsePowerlevel10kSegment checks that with powerlevel10k, which
// rebuilds RPROMPT before every prompt, the session is added as a p10k
// segment instead of to RPROMPT.
func TestZshHooksUsePowerlevel10kSegment(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	if err := WriteSessionInfo(statePath, 3, "BUG-1 50%"); err != nil {
		t.Fatal(err)
	}
	// A stand-in for powerlevel10k: "p10k segment" prints its arguments.
	userRC := `POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS=(status time)
p10k() { [[ $1 == segment ]] && print -r -- "SEGMENT[${(j: :)@[2,-1]}]"; [[ $1 == reload ]] && print -r -- RELOADED; }
`
	if err := os.WriteFile(filepath.Join(dir, ".zshrc"), []byte(userRC), 0o600); err != nil {
		t.Fatal(err)
	}
	hookDir, err := WriteHookDir("/bin/true", statePath, "zsh", 3, "BUG-1 50%", "#7aadf4", false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(hookDir) }()
	isolateZsh(t, "zsh", hookDir)
	cmd := exec.Command("zsh", "-i", "-c", `RPROMPT=theirs; _carrier_precmd
printf '9\nOPS-7\n' > `+SessionInfoPath(statePath)+`
_carrier_precmd; prompt_carrier
print -r -- "ELEMENTS[$POWERLEVEL9K_RIGHT_PROMPT_ELEMENTS] RPROMPT[$RPROMPT]"`)
	cmd.Env = append(os.Environ(), "ZDOTDIR="+hookDir, "HOME="+dir)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("zsh: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{"RELOADED", "SEGMENT[-f #7aadf4 -t carrier #9 OPS-7]", "ELEMENTS[carrier status time] RPROMPT[theirs]"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestZshHooksRestoreUserZDOTDIR checks that a recorded zsh gives ZDOTDIR back
// to the user, so shells started from it, such as tmux panes, load the user's
// config rather than carrier's hooks.
func TestZshHooksRestoreUserZDOTDIR(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	dir := t.TempDir()
	mine := filepath.Join(dir, "zdotdir")
	if err := os.MkdirAll(mine, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mine, ".zshrc"), []byte("MY_CONFIG=loaded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hookDir, err := WriteHookDir("/bin/true", filepath.Join(dir, "state.json"), "zsh", 1, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(hookDir) }()
	isolateZsh(t, "zsh", hookDir)
	for _, tc := range []struct{ userZDOTDIR, want string }{
		{mine, "ZDOTDIR[" + mine + "] CONFIG[loaded]"},
		{"", "ZDOTDIR[unset] CONFIG[]"},
	} {
		cmd := exec.Command("zsh", "-i", "-c", `print -r -- "ZDOTDIR[${ZDOTDIR-unset}] CONFIG[$MY_CONFIG]${CARRIER_ZDOTDIR+ leaked}"`)
		cmd.Env = append(os.Environ(), "ZDOTDIR="+hookDir, "HOME="+dir, "CARRIER_ZDOTDIR="+tc.userZDOTDIR)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("zsh: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), tc.want) || strings.Contains(string(out), "leaked") {
			t.Errorf("user ZDOTDIR %q: got %q, want %q", tc.userZDOTDIR, out, tc.want)
		}
	}
}
