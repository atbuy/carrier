// Package changelog parses CHANGELOG.md, written in the Keep a Changelog
// format: "## [1.2.3] - 2026-01-02" per release, "### Added" per section,
// and "- " bullet items.
package changelog

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Release is one version's entry.
type Release struct {
	Version  string    `json:"version"`
	Date     string    `json:"date,omitempty"`
	Sections []Section `json:"sections"`
}

// Section is a group of changes such as "Added" or "Fixed".
type Section struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

var releaseHeading = regexp.MustCompile(`^## \[([^\]]+)\](?:\s+-\s+(\S+))?\s*$`)

// Parse returns the releases in text, in the order they appear (newest
// first). Text before the first release heading is ignored. An item that
// wraps onto indented lines is joined into one.
func Parse(text string) ([]Release, error) {
	var releases []Release
	var rel *Release
	var sec *Section
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "## "):
			m := releaseHeading.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("line %d: release heading must look like \"## [1.2.3] - 2026-01-02\": %q", i+1, line)
			}
			releases = append(releases, Release{Version: m[1], Date: m[2]})
			rel, sec = &releases[len(releases)-1], nil
		case rel == nil:
			continue
		case strings.HasPrefix(line, "### "):
			rel.Sections = append(rel.Sections, Section{Name: strings.TrimSpace(line[4:])})
			sec = &rel.Sections[len(rel.Sections)-1]
		case strings.HasPrefix(trimmed, "- ") && sec != nil && !strings.HasPrefix(line, "  "):
			sec.Items = append(sec.Items, strings.TrimSpace(trimmed[2:]))
		case trimmed != "" && sec != nil && len(sec.Items) > 0:
			sec.Items[len(sec.Items)-1] += " " + trimmed
		case trimmed != "":
			return nil, fmt.Errorf("line %d: expected a \"### Section\" heading or a \"- \" item: %q", i+1, line)
		}
	}
	return releases, nil
}

// Find returns the release for version ("1.2.3" or "v1.2.3").
func Find(releases []Release, version string) (Release, bool) {
	want := strings.TrimPrefix(strings.TrimSpace(version), "v")
	for _, r := range releases {
		if r.Version == want {
			return r, true
		}
	}
	return Release{}, false
}

// Since returns the releases newer than version, newest first.
func Since(releases []Release, version string) ([]Release, error) {
	base, err := parseSemver(version)
	if err != nil {
		return nil, err
	}
	var newer []Release
	for _, r := range releases {
		v, err := parseSemver(r.Version)
		if err != nil {
			continue // e.g. "Unreleased"
		}
		if compareSemver(v, base) > 0 {
			newer = append(newer, r)
		}
	}
	return newer, nil
}

// Markdown renders a release's sections as Markdown, without the version
// heading, for use in release notes.
func Markdown(r Release) string {
	var b strings.Builder
	for i, sec := range r.Sections {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("### " + sec.Name + "\n\n")
		for _, item := range sec.Items {
			b.WriteString("- " + item + "\n")
		}
	}
	return b.String()
}

// IsRelease reports whether version looks like a released version number,
// as opposed to a development build such as "dev".
func IsRelease(version string) bool {
	_, err := parseSemver(version)
	return err == nil
}

func parseSemver(s string) ([3]int, error) {
	var v [3]int
	core := strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("invalid version %q; use a version like 0.24.0", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, fmt.Errorf("invalid version %q; use a version like 0.24.0", s)
		}
		v[i] = n
	}
	return v, nil
}

func compareSemver(a, b [3]int) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] > b[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}
