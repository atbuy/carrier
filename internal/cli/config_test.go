package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigTOML(t *testing.T) {
	out := defaultConfigTOML()
	for _, want := range []string{
		"[storage]",
		`data_dir = "~/.local/share/carrier"`,
		"[redaction]",
		"[notify]",
		"[shell]",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("default config missing %q\n%s", want, out)
		}
	}
}

func TestConfigPathCommand(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	cmd := configPathCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("config path failed: %v", err)
	}
	want := filepath.Join(xdg, "carrier", "config.toml") + "\n"
	if out.String() != want {
		t.Fatalf("config path output = %q, want %q", out.String(), want)
	}
}

func TestConfigInitCommandRefusesOverwriteUnlessForced(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	cmd := configInitCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("config init failed: %v", err)
	}
	path := filepath.Join(xdg, "carrier", "config.toml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file missing: %v", err)
	}
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatalf("config init overwrote without --force")
	}

	forced := configInitCmd()
	if err := forced.Flags().Set("force", "true"); err != nil {
		t.Fatalf("set force flag: %v", err)
	}
	if err := forced.RunE(forced, nil); err != nil {
		t.Fatalf("forced config init failed: %v", err)
	}
}

func TestConfigCheckCommandReportsOK(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	cmd := configCheckCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("config check failed: %v", err)
	}
	if !strings.Contains(out.String(), "ok\n") {
		t.Fatalf("config check output = %q", out.String())
	}
}

func TestConfigCheckCommandReportsErrors(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	configDir := filepath.Join(xdg, "carrier")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(`
[storage]
data_dir = ""
max_output_mb = 0

[redaction]
enabled = true
patterns = ["["]

[notify]
min_duration = "soon"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := configCheckCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatalf("config check accepted invalid config")
	}
	for _, want := range []string{
		"error storage.data_dir: must not be empty",
		"warn storage.max_output_mb: output cap disabled",
		"error redaction.patterns[0]: invalid regex:",
		"error notify.min_duration: invalid duration:",
		"failed: 3 errors, 1 warning",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("config check output missing %q:\n%s", want, out.String())
		}
	}
}
