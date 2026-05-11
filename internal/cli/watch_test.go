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
	if !cmd.DisableFlagParsing {
		t.Fatal("DisableFlagParsing must be true")
	}
	// Empty args → error from RunE.
	if err := cmd.RunE(cmd, []string{}); err == nil {
		t.Fatal("expected error for empty args")
	}
}

func TestParseWatchFlags_Defaults(t *testing.T) {
	pattern, debounce, rest, err := parseWatchFlags([]string{"go", "test", "./..."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pattern != "" {
		t.Errorf("pattern = %q, want empty", pattern)
	}
	if debounce != 200*time.Millisecond {
		t.Errorf("debounce = %v, want 200ms", debounce)
	}
	if len(rest) != 3 || rest[0] != "go" {
		t.Errorf("rest = %v", rest)
	}
}

func TestParseWatchFlags_PatternShort(t *testing.T) {
	_, _, rest, err := parseWatchFlags([]string{"-p", "*.go", "make", "build"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rest) != 2 || rest[0] != "make" {
		t.Errorf("rest = %v", rest)
	}
}

func TestParseWatchFlags_PatternLong(t *testing.T) {
	pattern, _, rest, err := parseWatchFlags([]string{"--pattern", "*.go", "cmd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pattern != "*.go" {
		t.Errorf("pattern = %q", pattern)
	}
	if len(rest) != 1 || rest[0] != "cmd" {
		t.Errorf("rest = %v", rest)
	}
}

func TestParseWatchFlags_PatternEquals(t *testing.T) {
	pattern, _, _, err := parseWatchFlags([]string{"--pattern=*.go", "cmd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pattern != "*.go" {
		t.Errorf("pattern = %q", pattern)
	}

	pattern2, _, _, err := parseWatchFlags([]string{"-p=*.go", "cmd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pattern2 != "*.go" {
		t.Errorf("pattern = %q", pattern2)
	}
}

func TestParseWatchFlags_DebounceShort(t *testing.T) {
	_, debounce, _, err := parseWatchFlags([]string{"-d", "50ms", "cmd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if debounce != 50*time.Millisecond {
		t.Errorf("debounce = %v", debounce)
	}
}

func TestParseWatchFlags_DebounceLong(t *testing.T) {
	_, debounce, _, err := parseWatchFlags([]string{"--debounce", "1s", "cmd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if debounce != time.Second {
		t.Errorf("debounce = %v", debounce)
	}
}

func TestParseWatchFlags_DebounceEquals(t *testing.T) {
	_, debounce, _, err := parseWatchFlags([]string{"--debounce=500ms", "cmd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if debounce != 500*time.Millisecond {
		t.Errorf("debounce = %v", debounce)
	}

	_, debounce2, _, err := parseWatchFlags([]string{"-d=2s", "cmd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if debounce2 != 2*time.Second {
		t.Errorf("debounce = %v", debounce2)
	}
}

func TestParseWatchFlags_MissingValue(t *testing.T) {
	_, _, _, err := parseWatchFlags([]string{"-p"})
	if err == nil {
		t.Fatal("expected error for -p with no value")
	}
	_, _, _, err = parseWatchFlags([]string{"--pattern"})
	if err == nil {
		t.Fatal("expected error for --pattern with no value")
	}
	_, _, _, err = parseWatchFlags([]string{"-d"})
	if err == nil {
		t.Fatal("expected error for -d with no value")
	}
	_, _, _, err = parseWatchFlags([]string{"--debounce"})
	if err == nil {
		t.Fatal("expected error for --debounce with no value")
	}
}

func TestParseWatchFlags_InvalidDebounce(t *testing.T) {
	cases := [][]string{
		{"--debounce=bogus", "cmd"},
		{"-d=notaduration", "cmd"},
		{"--debounce", "notaduration", "cmd"},
	}
	for _, args := range cases {
		_, _, _, err := parseWatchFlags(args)
		if err == nil {
			t.Fatalf("expected error for invalid debounce in args %v", args)
		}
	}
}

func TestParseWatchFlags_ChildFlags(t *testing.T) {
	// Child command flags like -v must not be consumed by parseWatchFlags.
	pattern, _, rest, err := parseWatchFlags([]string{"--pattern=*.go", "pytest", "-v", "--tb=short"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pattern != "*.go" {
		t.Errorf("pattern = %q", pattern)
	}
	if len(rest) != 3 || rest[0] != "pytest" || rest[1] != "-v" || rest[2] != "--tb=short" {
		t.Errorf("rest = %v, want [pytest -v --tb=short]", rest)
	}
}

func TestParseWatchFlags_EmptyArgs(t *testing.T) {
	_, debounce, rest, err := parseWatchFlags(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if debounce != 200*time.Millisecond {
		t.Errorf("debounce = %v", debounce)
	}
	if len(rest) != 0 {
		t.Errorf("rest = %v", rest)
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

func TestRunWatchMultiArgCommand(t *testing.T) {
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

	// Multi-word command with flags — the core use case this change enables.
	runWatch(a, []string{"sh", "-c", "exit 0"}, cwd)

	run, err := st.Latest()
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if run.Status != store.StatusSuccess {
		t.Fatalf("status = %q, want success", run.Status)
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
