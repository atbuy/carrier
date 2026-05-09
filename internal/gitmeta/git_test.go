package gitmeta

import "testing"

func TestCollectOutsideGitRepoReturnsZeroMeta(t *testing.T) {
	meta := Collect(t.TempDir())

	if meta.Root != "" || meta.Branch != "" || meta.Commit != "" || meta.Dirty != nil {
		t.Fatalf("expected zero metadata outside git repo, got %#v", meta)
	}
}
