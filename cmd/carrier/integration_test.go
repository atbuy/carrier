package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atbuy/carrier/internal/cli"
)

func TestCarrierCLIIntegrationRunLastAndShowJSON(t *testing.T) {
	requireShell(t)
	env := newCarrierTestEnv(t)

	script := "printf stdout-text; printf stderr-text >&2; exit 7"
	run := runCarrier(t, env, "run", "sh", "-c", script)
	if run.exitCode != 7 {
		t.Fatalf("run exit code = %d, want 7\nstderr:\n%s", run.exitCode, run.stderr)
	}
	if run.stdout != "stdout-text" {
		t.Fatalf("run stdout = %q", run.stdout)
	}
	if !strings.Contains(run.stderr, "carrier: run 1") || !strings.Contains(run.stderr, "stderr-text") {
		t.Fatalf("run stderr missing carrier status or command stderr:\n%s", run.stderr)
	}

	last := runCarrier(t, env, "last", "--json")
	last.requireExit(t, 0)
	lastView := decodeRunView(t, last.stdout)
	if lastView.ID != 1 || lastView.Status != "failed" || lastView.Mode != "run" {
		t.Fatalf("last view metadata mismatch: %#v", lastView)
	}
	if lastView.Command != "sh -c 'printf stdout-text; printf stderr-text >&2; exit 7'" {
		t.Fatalf("last command = %q", lastView.Command)
	}
	if len(lastView.Argv) != 3 || lastView.Argv[0] != "sh" || lastView.Argv[2] != script {
		t.Fatalf("last argv mismatch: %#v", lastView.Argv)
	}
	if lastView.CWD != env.work {
		t.Fatalf("last cwd = %q, want %q", lastView.CWD, env.work)
	}
	if lastView.ExitCode == nil || *lastView.ExitCode != 7 {
		t.Fatalf("last exit code = %#v, want 7", lastView.ExitCode)
	}
	if lastView.Stdout != "" || lastView.Stderr != "" {
		t.Fatalf("last JSON should not include output: %#v", lastView)
	}

	show := runCarrier(t, env, "show", "1", "--json")
	show.requireExit(t, 0)
	showView := decodeRunView(t, show.stdout)
	if showView.Stdout != "stdout-text" || showView.Stderr != "stderr-text" {
		t.Fatalf("show output mismatch: stdout=%q stderr=%q", showView.Stdout, showView.Stderr)
	}
}

func TestCarrierCLIIntegrationCleanDryRunKeepsRuns(t *testing.T) {
	requireShell(t)
	env := newCarrierTestEnv(t)

	run := runCarrier(t, env, "run", "sh", "-c", "printf keep-output")
	run.requireExit(t, 0)

	dryRun := runCarrier(t, env, "clean", "--older-than", "0s", "--dry-run")
	dryRun.requireExit(t, 0)
	for _, want := range []string{
		"1  success  sh -c 'printf keep-output'",
		"would delete 1 runs",
	} {
		if !strings.Contains(dryRun.stdout, want) {
			t.Fatalf("clean dry-run output missing %q:\n%s", want, dryRun.stdout)
		}
	}

	show := runCarrier(t, env, "show", "1", "--json")
	show.requireExit(t, 0)
	showView := decodeRunView(t, show.stdout)
	if showView.Stdout != "keep-output" {
		t.Fatalf("dry-run deleted or changed run output: %q", showView.Stdout)
	}
}

func TestCarrierCLIIntegrationStats(t *testing.T) {
	requireShell(t)
	env := newCarrierTestEnv(t)

	runCarrier(t, env, "run", "sh", "-c", "exit 0").requireExit(t, 0)
	if failed := runCarrier(t, env, "run", "sh", "-c", "exit 3"); failed.exitCode != 3 {
		t.Fatalf("failed run exit code = %d, want 3", failed.exitCode)
	}

	stats := runCarrier(t, env, "stats")
	stats.requireExit(t, 0)
	for _, want := range []string{
		"Runs:        2",
		"Completed:   2",
		"Success:     1",
		"Failed:      1",
		"Runs/day:    2.00",
		"Failure:     50.0%",
		"Slowest:",
	} {
		if !strings.Contains(stats.stdout, want) {
			t.Fatalf("stats output missing %q:\n%s", want, stats.stdout)
		}
	}
}

func TestCarrierCLIIntegrationSearchOutput(t *testing.T) {
	requireShell(t)
	env := newCarrierTestEnv(t)

	run := runCarrier(t, env, "run", "sh", "-c", "printf searchable-output-token")
	run.requireExit(t, 0)

	search := runCarrier(t, env, "search", "searchable-output-token")
	search.requireExit(t, 0)
	for _, want := range []string{
		"1  success  sh -c 'printf searchable-output-token'",
		"searchable-output-token",
	} {
		if !strings.Contains(search.stdout, want) {
			t.Fatalf("search output missing %q:\n%s", want, search.stdout)
		}
	}
}

func TestCarrierCLIIntegrationConfigInit(t *testing.T) {
	env := newCarrierTestEnv(t)
	configPath := filepath.Join(env.xdgConfig, "carrier", "config.toml")

	help := runCarrier(t, env, "config", "--help")
	help.requireExit(t, 0)
	for _, want := range []string{
		"config <command>",
		"Commands",
		"path       show config file path",
		"show       show active config",
		"check      validate active config",
		"init       write default config file",
	} {
		if !strings.Contains(help.stdout, want) {
			t.Fatalf("config help output missing %q:\n%s", want, help.stdout)
		}
	}

	path := runCarrier(t, env, "config", "path")
	path.requireExit(t, 0)
	if path.stdout != configPath+"\n" {
		t.Fatalf("config path output = %q, want %q", path.stdout, configPath+"\n")
	}

	init := runCarrier(t, env, "config", "init")
	init.requireExit(t, 0)
	if !strings.Contains(init.stdout, "wrote "+configPath) {
		t.Fatalf("config init output = %q", init.stdout)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file missing: %v", err)
	}
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if !strings.Contains(string(configBytes), `data_dir = "~/.local/share/carrier"`) {
		t.Fatalf("config file missing default data dir:\n%s", string(configBytes))
	}

	again := runCarrier(t, env, "config", "init")
	if again.exitCode != 1 {
		t.Fatalf("second config init exit code = %d, want 1", again.exitCode)
	}
	if !strings.Contains(again.stderr, "carrier: config already exists: "+configPath) {
		t.Fatalf("second config init stderr = %q", again.stderr)
	}

	forced := runCarrier(t, env, "config", "init", "--force")
	forced.requireExit(t, 0)

	show := runCarrier(t, env, "config", "show")
	show.requireExit(t, 0)
	dataDir := filepath.Join(env.home, ".local", "share", "carrier")
	for _, want := range []string{
		"[storage]",
		`data_dir = "` + dataDir + `"`,
		"max_output_mb = 20",
	} {
		if !strings.Contains(show.stdout, want) {
			t.Fatalf("config show missing %q:\n%s", want, show.stdout)
		}
	}

	check := runCarrier(t, env, "config", "check")
	check.requireExit(t, 0)
	if !strings.Contains(check.stdout, "Config:") || !strings.Contains(check.stdout, configPath) || !strings.Contains(check.stdout, "ok\n") {
		t.Fatalf("config check output = %q", check.stdout)
	}
}

func TestCarrierCLIHelperProcess(t *testing.T) {
	if os.Getenv("CARRIER_TEST_CLI") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"carrier"}, os.Args[i+1:]...)
			os.Exit(cli.Execute())
		}
	}
	t.Fatal("missing CLI args marker")
}

type carrierTestEnv struct {
	home      string
	xdgConfig string
	work      string
}

func newCarrierTestEnv(t *testing.T) carrierTestEnv {
	t.Helper()
	root := t.TempDir()
	env := carrierTestEnv{
		home:      filepath.Join(root, "home"),
		xdgConfig: filepath.Join(root, "config"),
		work:      filepath.Join(root, "work"),
	}
	for _, dir := range []string{env.home, env.xdgConfig, env.work} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create test dir: %v", err)
		}
	}
	return env
}

func runCarrier(t *testing.T, env carrierTestEnv, args ...string) carrierResult {
	t.Helper()
	testArgs := append([]string{"-test.run=^TestCarrierCLIHelperProcess$", "--"}, args...)
	cmd := exec.Command(os.Args[0], testArgs...)
	cmd.Dir = env.work
	cmd.Env = append(
		os.Environ(),
		"CARRIER_TEST_CLI=1",
		"CARRIER_COLOR=never",
		"HOME="+env.home,
		"XDG_CONFIG_HOME="+env.xdgConfig,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("run carrier %v: %v", args, err)
		}
	}
	return carrierResult{
		exitCode: exitCode,
		stdout:   stdout.String(),
		stderr:   stderr.String(),
	}
}

type carrierResult struct {
	exitCode int
	stdout   string
	stderr   string
}

func (r carrierResult) requireExit(t *testing.T, want int) {
	t.Helper()
	if r.exitCode != want {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", r.exitCode, want, r.stdout, r.stderr)
	}
}

type integrationRunView struct {
	ID       int64    `json:"id"`
	Status   string   `json:"status"`
	Mode     string   `json:"mode"`
	Command  string   `json:"command"`
	Argv     []string `json:"argv"`
	CWD      string   `json:"cwd"`
	ExitCode *int     `json:"exit_code"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
}

func decodeRunView(t *testing.T, input string) integrationRunView {
	t.Helper()
	var view integrationRunView
	if err := json.Unmarshal([]byte(input), &view); err != nil {
		t.Fatalf("decode run JSON: %v\n%s", err, input)
	}
	return view
}

func requireShell(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
}
