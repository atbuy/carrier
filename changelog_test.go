package carrier

import (
	"testing"

	"github.com/atbuy/carrier/internal/changelog"
)

// TestChangelogIsWellFormed checks the CHANGELOG.md built into the binary, so
// a malformed entry fails here instead of in the release workflow.
func TestChangelogIsWellFormed(t *testing.T) {
	releases, err := changelog.Parse(Changelog)
	if err != nil {
		t.Fatalf("CHANGELOG.md: %v", err)
	}
	if len(releases) == 0 {
		t.Fatal("CHANGELOG.md has no releases")
	}
	seen := map[string]bool{}
	var previous string
	for _, r := range releases {
		if r.Version == "Unreleased" {
			continue
		}
		if !changelog.IsRelease(r.Version) {
			t.Errorf("%s: not a version number", r.Version)
		}
		if seen[r.Version] {
			t.Errorf("%s: listed twice", r.Version)
		}
		seen[r.Version] = true
		if r.Date == "" {
			t.Errorf("%s: missing date", r.Version)
		}
		if len(r.Sections) == 0 {
			t.Errorf("%s: no sections", r.Version)
		}
		for _, sec := range r.Sections {
			switch sec.Name {
			case "Added", "Changed", "Deprecated", "Removed", "Fixed", "Security":
			default:
				t.Errorf("%s: unknown section %q", r.Version, sec.Name)
			}
			if len(sec.Items) == 0 {
				t.Errorf("%s: empty %s section", r.Version, sec.Name)
			}
		}
		if previous != "" {
			if newer, _ := changelog.Since([]changelog.Release{r}, previous); len(newer) != 0 {
				t.Errorf("%s is listed after %s; list newest first", r.Version, previous)
			}
		}
		previous = r.Version
	}
}
