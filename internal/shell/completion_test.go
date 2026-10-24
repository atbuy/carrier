package shell

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCompletionScript(t *testing.T) {
	cobra := map[string]string{
		"zsh":  "#compdef carrier\ncompdef _carrier carrier\n_carrier() {\n  :\n}\n",
		"bash": "__start_carrier() {\n  :\n}\ncomplete -o default -F __start_carrier carrier\n",
		"fish": "complete -c carrier -e\n",
	}
	wants := map[string][]string{
		// zsh waits for compinit, then completes run and watch through _normal.
		"zsh": {"precmd_functions+=(__carrier_completion_setup)", "  compdef _carrier carrier", "compdef __carrier_complete carrier", "list-grouped false", "_normal"},
		// bash also registers the c alias, and works without bash-completion.
		"bash": {"complete -o default -F __carrier_complete carrier", "complete -o default -F __carrier_complete c", "_get_comp_words_by_ref() {", "_command_offset"},
		"fish": {"complete -c carrier -e", "complete -c carrier -n '__carrier_wrapped_command >/dev/null'"},
	}
	for shellName, want := range wants {
		script, err := CompletionScript(shellName, cobra[shellName])
		if err != nil {
			t.Fatalf("%s: %v", shellName, err)
		}
		for _, w := range want {
			if !strings.Contains(script, w) {
				t.Errorf("%s script missing %q:\n%s", shellName, w, script)
			}
		}
		for _, flag := range wrappedValueFlags {
			if !strings.Contains(script, flag) {
				t.Errorf("%s script does not skip %s before the wrapped command", shellName, flag)
			}
		}
		if _, err := exec.LookPath(shellName); err != nil {
			continue
		}
		check := exec.Command(shellName, "-n")
		check.Stdin = strings.NewReader(script)
		if out, err := check.CombinedOutput(); err != nil {
			t.Errorf("%s -n: %v\n%s", shellName, err, out)
		}
	}
	if _, err := CompletionScript("tcsh", ""); err == nil || !strings.Contains(err.Error(), "zsh, bash, fish") {
		t.Fatalf("unsupported shell: got %v", err)
	}
}
