package shell

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestSearchBindingKeys(t *testing.T) {
	for _, tc := range []struct{ shell, key, want string }{
		{"zsh", "ctrl-r", "bindkey '^R' _carrier_search_widget"},
		{"zsh", "alt-r", "bindkey '^[r' _carrier_search_widget"},
		{"bash", "ctrl-f", `bind -x '"\C-f": _carrier_search'`},
		{"bash", "alt-r", `bind -x '"\er": _carrier_search'`},
		{"fish", "ctrl-r", `bind \cr _carrier_search`},
		{"fish", "alt-r", `bind \er _carrier_search`},
	} {
		got, err := SearchBinding(tc.shell, "/opt/my tools/carrier", tc.key)
		if err != nil || !strings.Contains(got, tc.want) || !strings.Contains(got, `'/opt/my tools/carrier' search --pick`) {
			t.Errorf("%s %s: %v\n%s", tc.shell, tc.key, err, got)
		}
	}
	if _, err := SearchBinding("zsh", "carrier", "super-r"); err == nil {
		t.Fatal("an unbindable key should fail")
	}
	if _, err := SearchBinding("tcsh", "carrier", "ctrl-r"); err == nil {
		t.Fatal("tcsh should fail")
	}
}

// runSearchBinding types typed, then the key's bytes, then Enter, into an
// interactive shell whose config loads the binding with a stand-in carrier.
// The stand-in "picks" picked, or cancels when picked is empty, and logs the
// text it was given. It returns the terminal output and the log.
func runSearchBinding(t *testing.T, shellName, key, keyBytes, typed, picked string) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell test is Unix-only")
	}
	program, err := exec.LookPath(shellName)
	if err != nil {
		t.Skipf("%s not installed", shellName)
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	fake := filepath.Join(dir, "carrier")
	script := "#!/bin/sh\nshift 3\nprintf '%s\\n' \"$*\" >> '" + logPath + "'\n"
	if picked == "" {
		script += "exit 1\n"
	} else {
		pickPath := filepath.Join(dir, "picked")
		if err := os.WriteFile(pickPath, []byte(picked+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		script += "cat '" + pickPath + "'\n"
	}
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	binding, err := SearchBinding(shellName, fake, key)
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]struct{ path, prompt string }{
		"bash": {".bashrc", "PS1='PROMPT> '\n"},
		"zsh":  {".zshrc", "PS1='PROMPT> '\nunsetopt PROMPT_SP\n"},
		"fish": {".config/fish/config.fish", "function fish_prompt; echo -n 'PROMPT> '; end\nfunction fish_greeting; end\n"},
	}[shellName]
	rc := filepath.Join(dir, config.path)
	if err := os.MkdirAll(filepath.Dir(rc), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rc, []byte(config.prompt+binding), 0o600); err != nil {
		t.Fatal(err)
	}
	isolateZsh(t, shellName, dir)
	args := []string{"-i"}
	if shellName == "bash" {
		args = []string{"--rcfile", rc, "-i"}
	}
	cmd := exec.Command(program, args...)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "ZDOTDIR=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	// With TERM=dumb, fish 4 does not query the terminal at startup and wait
	// for answers this one never gives.
	cmd.Env = append(cmd.Env, "HOME="+dir, "TERM=dumb", "HISTFILE="+filepath.Join(dir, ".hist"),
		"XDG_CONFIG_HOME="+filepath.Join(dir, ".config"), "XDG_DATA_HOME="+filepath.Join(dir, ".local", "share"))
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("start %s: %v", shellName, err)
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
	for _, input := range []string{typed, keyBytes, "\r", "exit\r"} {
		time.Sleep(500 * time.Millisecond)
		_, _ = ptmx.Write([]byte(input))
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-exited
	}
	_ = ptmx.Close()
	<-done
	b, _ := os.ReadFile(logPath)
	return out.String(), string(b)
}

func TestSearchBindingPutsPickOnCommandLine(t *testing.T) {
	for _, sh := range InitShells {
		t.Run(sh, func(t *testing.T) {
			if sh == "bash" {
				out, _ := exec.Command("bash", "-c", "echo ${BASH_VERSINFO[0]}").Output()
				if major, _ := strconv.Atoi(strings.TrimSpace(string(out))); major < 4 {
					t.Skip("bash before 4.0 cannot edit the command line from a binding")
				}
			}
			// Ctrl+R with "echo pi" typed: the pick replaces the line, and
			// Enter runs it. The output differs from the line printf runs, so
			// it only shows when the line ran.
			out, calls := runSearchBinding(t, sh, "ctrl-r", "\x12", "echo pi", `printf 'picked-%s\n' ok`)
			if !strings.Contains(calls, "echo pi") {
				t.Errorf("carrier should get the line typed so far, got %q", calls)
			}
			if !strings.Contains(out, "picked-ok") {
				t.Fatalf("the picked command should run:\n%s", out)
			}
			// Alt+R, cancelled: the line typed stays, and Enter runs it.
			out, calls = runSearchBinding(t, sh, "alt-r", "\x1br", `printf 'kept-%s\n' line`, "")
			if calls == "" || !strings.Contains(out, "kept-line") {
				t.Fatalf("a cancelled search should keep the line, calls %q:\n%s", calls, out)
			}
		})
	}
}
