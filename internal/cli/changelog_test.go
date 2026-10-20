package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/atbuy/carrier/internal/version"
)

func TestChangelogCmd(t *testing.T) {
	t.Setenv("CARRIER_COLOR", "never")
	a := &app{}
	old := version.Version
	t.Cleanup(func() { version.Version = old })

	version.Version = "v0.24.0"
	out, errOut, err := runEmpty(t, a.changelogCmd(), nil, nil)
	if err != nil || !strings.HasPrefix(out, "carrier 0.24.0 2026-10-20\n  Added\n    • carrier note <text>") {
		t.Fatalf("installed version: %v\n%s", err, out)
	}
	if !strings.Contains(errOut, "carrier changelog --all") {
		t.Fatalf("should point at older versions, got %q", errOut)
	}

	version.Version = "dev"
	out, errOut, err = runEmpty(t, a.changelogCmd(), nil, nil)
	if err != nil || !strings.Contains(errOut, "this is a dev build; showing the latest release") || !strings.HasPrefix(out, "carrier ") {
		t.Fatalf("dev build: %v %q\n%s", err, errOut, out)
	}

	out, _, err = runEmpty(t, a.changelogCmd(), []string{"v0.9.2"}, nil)
	if err != nil || !strings.Contains(out, "carrier 0.9.2") || strings.Count(out, "carrier 0.") != 1 {
		t.Fatalf("one version: %v\n%s", err, out)
	}

	out, _, err = runEmpty(t, a.changelogCmd(), nil, map[string]string{"since": "0.27.0"})
	if err != nil || !strings.Contains(out, "carrier 0.28.0") || strings.Contains(out, "carrier 0.27.0") {
		t.Fatalf("--since: %v\n%s", err, out)
	}

	out, _, err = runEmpty(t, a.changelogCmd(), nil, map[string]string{"all": "true"})
	if err != nil || !strings.Contains(out, "carrier 0.1.0") || strings.Contains(out, "`") {
		t.Fatalf("--all should list every version without Markdown backticks: %v", err)
	}

	out, _, err = runEmpty(t, a.changelogCmd(), []string{"0.1.1"}, map[string]string{"json": "true"})
	var views []struct {
		Version  string `json:"version"`
		Sections []struct {
			Name  string   `json:"name"`
			Items []string `json:"items"`
		} `json:"sections"`
	}
	if err != nil || json.Unmarshal([]byte(out), &views) != nil || len(views) != 1 || views[0].Version != "0.1.1" || len(views[0].Sections) != 2 {
		t.Fatalf("--json: %v\n%s", err, out)
	}

	out, _, err = runEmpty(t, a.changelogCmd(), []string{"0.24.1"}, map[string]string{"markdown": "true"})
	if err != nil || !strings.HasPrefix(out, "### Fixed\n\n- In bash shell mode") {
		t.Fatalf("--markdown: %v\n%s", err, out)
	}

	for _, tc := range []struct {
		args  []string
		flags map[string]string
		want  string
	}{
		{[]string{"9.9.9"}, nil, "no changelog entry for version 9.9.9"},
		{nil, map[string]string{"since": "latest"}, `invalid version "latest"`},
		{[]string{"0.1.0"}, map[string]string{"all": "true"}, "not more than one"},
		{nil, map[string]string{"all": "true", "since": "0.1.0"}, "not both"},
	} {
		if _, _, err := runEmpty(t, a.changelogCmd(), tc.args, tc.flags); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("args %v flags %v: err = %v, want %q", tc.args, tc.flags, err, tc.want)
		}
	}
	if _, errOut, _ := runEmpty(t, a.changelogCmd(), nil, map[string]string{"since": "99.0.0"}); !strings.Contains(errOut, "no versions newer than 99.0.0") {
		t.Fatalf("--since newest: %q", errOut)
	}
}

func TestWrapWords(t *testing.T) {
	got := wrapWords("one two three four five", 9)
	if strings.Join(got, "|") != "one two|three|four five" {
		t.Fatalf("wrapWords = %q", got)
	}
	if len(wrapWords("", 10)) != 0 {
		t.Fatal("empty text has no lines")
	}
}
