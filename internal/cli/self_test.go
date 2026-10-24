package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/selfupdate"
	"github.com/atbuy/carrier/internal/version"
)

func TestSelfCmdSubcommands(t *testing.T) {
	a := &app{}
	self := a.selfCmd()

	var names []string
	for _, sub := range self.Commands() {
		names = append(names, sub.Name())
	}
	slices.Sort(names)
	want := []string{"changelog", "update", "version"}
	if !slices.Equal(names, want) {
		t.Fatalf("carrier self subcommands = %v, want %v", names, want)
	}

	// "news" stays a documented alias of the moved changelog command.
	if cmd, _, err := self.Find([]string{"news"}); err != nil || cmd.Name() != "changelog" {
		t.Fatalf("carrier self news = %v (%v), want changelog", cmd.Name(), err)
	}
}

func TestSelfVersionCmd(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	a := &app{}
	cmd, _, err := a.selfCmd().Find([]string{"version"})
	if err != nil {
		t.Fatalf("find carrier self version: %v", err)
	}

	out, errOut, err := runEmpty(t, cmd, nil, nil)
	if err != nil {
		t.Fatalf("carrier self version: %v", err)
	}
	if !strings.HasPrefix(out, "carrier ") {
		t.Fatalf("carrier self version output = %q", out)
	}
	// The hint is how people find the new command.
	if !strings.Contains(errOut, "carrier self update") {
		t.Fatalf("carrier self version should hint at updating, got %q", errOut)
	}
}

// TestDeprecatedTopLevelCommands covers the deprecation path in
// docs/reference/compatibility.md: the old names keep working for the rest of
// 1.x, hidden from help and warning on stderr.
func TestDeprecatedTopLevelCommands(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	root := newRootCmd(&app{})

	for _, name := range []string{"version", "v", "changelog", "news"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd == root {
			t.Fatalf("carrier %s no longer resolves (%v); it is deprecated, not removed", name, err)
		}
		if !cmd.Hidden {
			t.Fatalf("carrier %s should be hidden from help", name)
		}
	}
	if cmd, _, err := root.Find([]string{"self", "update"}); err != nil || cmd.CommandPath() != "carrier self update" {
		t.Fatalf("carrier self update = %v (%v)", cmd.CommandPath(), err)
	}

	// The warning names the replacement, and the command still does its job.
	version, _, err := root.Find([]string{"version"})
	if err != nil {
		t.Fatalf("find carrier version: %v", err)
	}
	out, errOut, err := runEmpty(t, version, nil, nil)
	if err != nil {
		t.Fatalf("carrier version: %v", err)
	}
	if !strings.Contains(errOut, `"carrier version" is deprecated; use "carrier self version"`) {
		t.Fatalf("carrier version stderr = %q, want a deprecation warning", errOut)
	}
	if !strings.HasPrefix(out, "carrier ") {
		t.Fatalf("carrier version stdout = %q, want it to still print the version", out)
	}

	// With colors on, the warning is orange.
	t.Setenv("CARRIER_COLOR", "always")
	if _, errOut, err = runEmpty(t, version, nil, nil); err != nil {
		t.Fatalf("carrier version: %v", err)
	}
	if orange := "38;2;229;149;58"; !strings.Contains(errOut, orange) {
		t.Fatalf("carrier version stderr = %q, want the warning in orange (%s)", errOut, orange)
	}
}

func TestSkipsStore(t *testing.T) {
	cases := map[string]bool{
		"carrier init":           true,
		"carrier version":        true,
		"carrier changelog":      true,
		"carrier self":           true,
		"carrier self update":    true,
		"carrier self version":   true,
		"carrier self changelog": true,
		"carrier run":            false,
		"carrier history":        false,
		"carrier session list":   false,
	}
	for path, want := range cases {
		if got := skipsStore(path); got != want {
			t.Errorf("skipsStore(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestCheckInstallMethod(t *testing.T) {
	writable := t.TempDir()
	binary := filepath.Join(writable, "carrier")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	cases := []struct {
		name    string
		target  selfupdate.Target
		current string
		force   bool
		wantErr string
	}{
		{
			name:    "release in a writable directory",
			target:  selfupdate.Target{Path: binary},
			current: "v1.2.0",
		},
		{
			name:    "go install",
			target:  selfupdate.Target{Path: binary, GoBin: true},
			current: "v1.2.0",
			wantErr: "go install",
		},
		{
			name:    "source build",
			target:  selfupdate.Target{Path: binary},
			current: "dev",
			wantErr: "dev build",
		},
		{
			name:    "source build with --force",
			target:  selfupdate.Target{Path: binary},
			current: "dev",
			force:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkInstallMethod(tc.target, tc.current, tc.force)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("checkInstallMethod = %v, want no error", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("checkInstallMethod = %v, want an error mentioning %q", err, tc.wantErr)
			}
		})
	}
}

func TestCheckInstallMethodReadOnlyDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "carrier")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := checkInstallMethod(selfupdate.Target{Path: binary}, "v1.2.0", false)
	if err == nil || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("checkInstallMethod = %v, want a not-writable error naming install.sh", err)
	}
	if !strings.Contains(err.Error(), "install.sh") {
		t.Fatalf("error should name the reinstall command, got %v", err)
	}
}

func TestPrintUpdateCheck(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	cases := []struct {
		name            string
		current, latest string
		wantOut         string
		wantHint        bool
	}{
		{
			name:    "behind",
			current: "v1.2.0", latest: "v1.3.1",
			wantOut:  "carrier 1.2.0 is installed, 1.3.1 is available",
			wantHint: true,
		},
		{
			name:    "up to date",
			current: "v1.3.1", latest: "v1.3.1",
			wantOut: "carrier 1.3.1 is the latest release",
		},
		{
			name:    "ahead of the latest release",
			current: "v2.0.0", latest: "v1.3.1",
			wantOut: "carrier 2.0.0 is the latest release",
		},
		{
			name:    "dev build",
			current: "dev", latest: "v1.3.1",
			wantOut: "this is a dev build; 1.3.1 is the latest release",
		},
		{
			name:    "pinned version, nothing to compare",
			current: "v1.2.0", latest: "",
			wantOut: "carrier 1.2.0 is installed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)

			printUpdateCheck(cmd, newTheme(&out), tc.current, tc.latest)
			if !strings.Contains(out.String(), tc.wantOut) {
				t.Fatalf("output = %q, want %q", out.String(), tc.wantOut)
			}
			if hinted := strings.Contains(errOut.String(), "carrier self update"); hinted != tc.wantHint {
				t.Fatalf("hint = %v, want %v (stderr %q)", hinted, tc.wantHint, errOut.String())
			}
		})
	}
}

func TestCompareInstalled(t *testing.T) {
	cases := []struct {
		name             string
		current, wanted  string
		force            bool
		wantStop         bool
		wantDowngradeMsg bool
	}{
		{name: "upgrade", current: "v1.2.0", wanted: "v1.3.1"},
		{name: "same version stops", current: "v1.3.1", wanted: "v1.3.1", wantStop: true},
		{name: "same version with --force", current: "v1.3.1", wanted: "v1.3.1", force: true},
		{name: "downgrade warns", current: "v1.3.1", wanted: "v1.2.0", wantDowngradeMsg: true},
		{name: "dev build never stops", current: "dev", wanted: "v1.3.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CARRIER_COLOR", "never")
			var out, errOut bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)

			_, stop := compareInstalled(cmd, tc.current, tc.wanted, tc.force)
			if stop != tc.wantStop {
				t.Fatalf("stop = %v, want %v", stop, tc.wantStop)
			}
			if warned := strings.Contains(errOut.String(), "an older release"); warned != tc.wantDowngradeMsg {
				t.Fatalf("downgrade warning = %v, want %v", warned, tc.wantDowngradeMsg)
			}
		})
	}
}

// TestSelfUpdateCheckResolvesLatest drives the command against a local stand-in
// for GitHub, so the test never reaches the network.
func TestSelfUpdateCheckResolvesLatest(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			w.Header().Set("Location", "/"+selfupdate.Repo+"/releases/tag/v9.9.9")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv(selfupdate.BaseURLEnv, srv.URL)

	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = "v1.2.0"

	a := &app{}
	cmd, _, err := a.selfCmd().Find([]string{"update"})
	if err != nil {
		t.Fatalf("find carrier self update: %v", err)
	}
	out, _, err := runEmpty(t, cmd, nil, map[string]string{"check": "true"})
	if err != nil {
		t.Fatalf("carrier self update --check: %v", err)
	}
	if !strings.Contains(out, "9.9.9 is available") {
		t.Fatalf("--check output = %q, want the server's version", out)
	}
}

func TestSelfUpdateRejectsInvalidVersion(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	a := &app{}
	cmd, _, err := a.selfCmd().Find([]string{"update"})
	if err != nil {
		t.Fatalf("find carrier self update: %v", err)
	}
	_, _, err = runEmpty(t, cmd, nil, map[string]string{"version": "nightly"})
	if err == nil || !strings.Contains(err.Error(), "invalid version") {
		t.Fatalf("error = %v, want an invalid version complaint", err)
	}
}
