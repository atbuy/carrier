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
