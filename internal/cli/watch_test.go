package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/atbuy/carrier/internal/config"
	"github.com/atbuy/carrier/internal/store"
)

func TestWatchCmdMetadataAndFlags(t *testing.T) {
	cmd := (&app{}).watchCmd()
	if cmd.Use != "watch <command...>" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	if cmd.Flags().Lookup("pattern") == nil {
		t.Fatal("pattern flag missing")
	}
	if cmd.Flags().Lookup("debounce") == nil {
		t.Fatal("debounce flag missing")
	}
	if err := cmd.Flags().Set("debounce", "75ms"); err != nil {
		t.Fatalf("set debounce: %v", err)
	}
	d, err := cmd.Flags().GetDuration("debounce")
	if err != nil {
		t.Fatalf("get debounce: %v", err)
	}
	if d != 75*time.Millisecond {
		t.Fatalf("debounce = %s, want 75ms", d)
	}
	if err := cmd.Args(cmd, nil); err == nil {
		t.Fatal("expected arg validation error")
	}
}

func TestRunWatchRecordsRun(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	cfg := config.Default()
	cfg.Storage.DataDir = dir
	a := &app{st: st, cfg: cfg, quiet: true}
	cwd := t.TempDir()

	runWatch(a, []string{"sh", "-c", "echo watched"}, cwd)

	run, err := st.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if run.Status != store.StatusSuccess {
		t.Fatalf("status = %q, want success", run.Status)
	}
	if run.CWD != cwd {
		t.Fatalf("cwd = %q, want %q", run.CWD, cwd)
	}
}

func TestAddDirRecursive(t *testing.T) {
	dir := t.TempDir()

	// Create some subdirectories including ones that should be skipped.
	for _, sub := range []string{"subdir", ".git", "node_modules", "vendor", "subdir/nested"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify not available: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	if err := addDirRecursive(watcher, dir); err != nil {
		t.Fatalf("addDirRecursive: %v", err)
	}

	// Verify that the root and subdir are watched but .git/node_modules/vendor are not.
	watched := make(map[string]bool)
	for _, p := range watcher.WatchList() {
		watched[p] = true
	}

	if !watched[dir] {
		t.Errorf("root dir %q should be watched", dir)
	}
	if !watched[filepath.Join(dir, "subdir")] {
		t.Errorf("subdir should be watched")
	}
	if !watched[filepath.Join(dir, "subdir", "nested")] {
		t.Errorf("nested subdir should be watched")
	}
	if watched[filepath.Join(dir, ".git")] {
		t.Errorf(".git should NOT be watched")
	}
	if watched[filepath.Join(dir, "node_modules")] {
		t.Errorf("node_modules should NOT be watched")
	}
	if watched[filepath.Join(dir, "vendor")] {
		t.Errorf("vendor should NOT be watched")
	}
}
