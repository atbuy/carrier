package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/atbuy/carrier/internal/selfupdate"
	"github.com/atbuy/carrier/internal/version"
)

func TestVersionCmd(t *testing.T) {
	a := &app{}
	cmd := a.versionCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("versionCmd: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected non-empty version output")
	}
}

// TestVersionUpdateHint checks that "carrier version" only suggests updating
// when a newer release exists, against a local stand-in for GitHub.
func TestVersionUpdateHint(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	latest := "v1.9.0"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if latest == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Location", "/"+selfupdate.Repo+"/releases/tag/"+latest)
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	t.Setenv(selfupdate.BaseURLEnv, srv.URL)
	old := version.Version
	t.Cleanup(func() { version.Version = old })

	for _, tc := range []struct {
		installed, latest, want string
	}{
		{"v1.8.1", "v1.9.0", "carrier 1.9.0 is available; update with: carrier self update"},
		{"v1.9.0", "v1.9.0", ""},
		{"v1.9.0", "", ""},    // the check failed
		{"dev", "v1.9.0", ""}, // not a release build
	} {
		version.Version, latest = tc.installed, tc.latest
		_, errOut, err := runEmpty(t, (&app{}).versionCmd(), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(errOut), "carrier:")); got != tc.want {
			t.Errorf("%s with %q released: hint %q, want %q", tc.installed, tc.latest, errOut, tc.want)
		}
	}
}
