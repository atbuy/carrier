package version

import "testing"

func TestCurrent(t *testing.T) {
	oldVersion := Version
	oldCommit := Commit
	oldDate := Date
	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
		Date = oldDate
	})

	Version = "v1.2.3"
	Commit = "abc123"
	Date = "2026-05-10T00:00:00Z"

	info := Current()
	if info.Version != Version || info.Commit != Commit || info.Date != Date {
		t.Fatalf("Current = %#v", info)
	}
	if got := info.String(); got != "carrier v1.2.3\ncommit: abc123\ndate: 2026-05-10T00:00:00Z" {
		t.Fatalf("String = %q", got)
	}
}
