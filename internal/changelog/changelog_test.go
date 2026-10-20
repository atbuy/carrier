package changelog

import (
	"strings"
	"testing"
)

const sample = `# Changelog

Intro text is ignored.

## [Unreleased]

### Added

- Something coming.

## [1.2.0] - 2026-02-01

### Added

- First item.
- A long item that
  wraps onto the next line.

### Fixed

- A fix.

## [1.1.10] - 2026-01-15

### Changed

- Ten.

## [1.1.9] - 2026-01-10

### Changed

- Nine.
`

func TestParse(t *testing.T) {
	releases, err := Parse(sample)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(releases) != 4 || releases[0].Version != "Unreleased" || releases[1].Version != "1.2.0" {
		t.Fatalf("releases = %+v", releases)
	}
	r := releases[1]
	if r.Date != "2026-02-01" || len(r.Sections) != 2 || r.Sections[1].Name != "Fixed" {
		t.Fatalf("1.2.0 = %+v", r)
	}
	if got := r.Sections[0].Items; len(got) != 2 || got[1] != "A long item that wraps onto the next line." {
		t.Fatalf("items = %q", got)
	}
	if releases[0].Date != "" {
		t.Fatalf("Unreleased has no date, got %q", releases[0].Date)
	}
}

func TestParseErrors(t *testing.T) {
	for _, bad := range []string{
		"## 1.2.0\n",                            // missing brackets
		"## [1.0.0] - 2026-01-01\nstray text\n", // text outside a section
	} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
}

func TestFindSinceAndIsRelease(t *testing.T) {
	releases, _ := Parse(sample)
	for _, v := range []string{"1.1.9", "v1.1.9", " 1.1.9 "} {
		if r, ok := Find(releases, v); !ok || r.Version != "1.1.9" {
			t.Errorf("Find(%q) = %+v, %v", v, r, ok)
		}
	}
	if _, ok := Find(releases, "9.9.9"); ok {
		t.Fatal("Find should not find a missing version")
	}

	newer, err := Since(releases, "v1.1.9")
	if err != nil {
		t.Fatalf("Since: %v", err)
	}
	var got []string
	for _, r := range newer {
		got = append(got, r.Version)
	}
	// Compared as numbers, not strings: 1.1.10 is newer than 1.1.9.
	if strings.Join(got, ",") != "1.2.0,1.1.10" {
		t.Fatalf("Since(1.1.9) = %v", got)
	}
	if _, err := Since(releases, "latest"); err == nil {
		t.Fatal("Since should reject a non-version")
	}
	for v, want := range map[string]bool{"v0.29.0": true, "1.0.0-rc.1": true, "dev": false, "1.2": false} {
		if IsRelease(v) != want {
			t.Errorf("IsRelease(%q) = %v", v, !want)
		}
	}
}

func TestMarkdown(t *testing.T) {
	releases, _ := Parse(sample)
	r, _ := Find(releases, "1.2.0")
	want := "### Added\n\n- First item.\n- A long item that wraps onto the next line.\n\n### Fixed\n\n- A fix.\n"
	if got := Markdown(r); got != want {
		t.Fatalf("Markdown:\n%s\nwant:\n%s", got, want)
	}
}
