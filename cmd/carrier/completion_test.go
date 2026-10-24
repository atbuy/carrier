//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// completionShellEnv puts a "carrier" on PATH that runs this test binary as
// carrier, records two runs, and returns the environment for a shell.
func completionShellEnv(t *testing.T) []string {
	t.Helper()
	env := newCarrierTestEnv(t)
	bin := filepath.Join(env.home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	wrapper := "#!/bin/sh\nexec " + shellQuote(os.Args[0]) + " -test.run='^TestCarrierCLIHelperProcess$' -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "carrier"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	vars := append(os.Environ(),
		"CARRIER_TEST_CLI=1", "CARRIER_COLOR=never", "CARRIER_SESSION_ID=",
		"HOME="+env.home, "XDG_CONFIG_HOME="+env.xdgConfig,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	for _, args := range [][]string{{"run", "true"}, {"run", "-L", "BUG-1234: fix login", "true"}} {
		cmd := exec.Command(filepath.Join(bin, "carrier"), args...)
		cmd.Env = vars
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("carrier %q: %v\n%s", args, err, out)
		}
	}
	return vars
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// TestCompletionInBash sources "carrier init bash" and calls its completion
// function the way bash does on Tab, without the bash-completion package.
func TestCompletionInBash(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	env := completionShellEnv(t)
	script := `
source <(carrier init bash)
comp() {
  COMP_LINE=$1 COMP_POINT=${#1} COMP_TYPE=9
  read -ra COMP_WORDS <<< "$COMP_LINE"
  [[ $COMP_LINE == *' ' ]] && COMP_WORDS+=('')
  COMP_CWORD=$(( ${#COMP_WORDS[@]} - 1 ))
  COMPREPLY=()
  __carrier_complete carrier "${COMP_WORDS[COMP_CWORD]}" "${COMP_WORDS[COMP_CWORD-1]}" 2>/dev/null
  echo "[$1] ${COMPREPLY[*]}"
}
comp 'carrier show '
comp 'carrier show BUG'
comp 'carrier history --status fa'
comp 'carrier status '
comp 'carrier run tru'
complete -p carrier c
`
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{
		"[carrier show] 2", // newest run first, with descriptions for the menu
		`[carrier show BUG] BUG-1234:\ fix\ login`,
		"[carrier history --status fa] failed",
		"[carrier status ] \n",
		"[carrier run tru] true",
		"complete -o default -F __carrier_complete carrier",
		"complete -o default -F __carrier_complete c",
	} {
		if !strings.Contains(got, strings.Replace(want, "[carrier show] 2", "[carrier show ] 2", 1)) {
			t.Errorf("bash output missing %q:\n%s", want, got)
		}
	}
}

// TestCompletionInFish sources "carrier init fish" and asks fish what Tab
// would offer.
func TestCompletionInFish(t *testing.T) {
	if _, err := exec.LookPath("fish"); err != nil {
		t.Skip("fish not installed")
	}
	env := completionShellEnv(t)
	script := `
carrier init fish | source
for line in 'carrier show ' 'carrier show BUG' 'carrier history --status fa' 'carrier status ' 'carrier run tru'
    echo "[$line]" (complete -C "$line" | string replace -r '\t.*' '')
end
`
	cmd := exec.Command("fish", "--no-config", "-c", script)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fish: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{
		"[carrier show ] 2 1",
		"[carrier show BUG] BUG-1234: fix login",
		"[carrier history --status fa] failed",
		"[carrier status ]\n",
		"[carrier run tru] true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fish output missing %q:\n%s", want, got)
		}
	}
}
