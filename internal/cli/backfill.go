package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/gitmeta"
)

// backfillInlineEnv makes "internal begin" collect metadata itself instead of
// in a background process; the tests set it, since there the executable is
// the test binary.
const backfillInlineEnv = "CARRIER_BACKFILL_INLINE"

// startBackfill collects the metadata of a shell run that takes time, "git
// status" and the redacted environment, in a detached background process, so
// the command starts without waiting for it.
func (a *app) startBackfill(statePath string, runID int64, cwd string, inRepo bool) {
	args := []string{"internal", "backfill", "--state", statePath, "--run", strconv.FormatInt(runID, 10), "--cwd", cwd}
	if inRepo {
		args = append(args, "--in-repo")
	}
	exe, err := os.Executable()
	if err != nil || os.Getenv(backfillInlineEnv) == "1" {
		a.backfill(statePath, runID, cwd, inRepo)
		return
	}
	cmd := exec.Command(exe, args...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		a.backfill(statePath, runID, cwd, inRepo)
		return
	}
	_ = cmd.Process.Release()
}

func (a *app) internalBackfillCmd() *cobra.Command {
	var statePath, cwd string
	var runID int64
	var inRepo bool
	cmd := &cobra.Command{
		Use:    "backfill",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			a.backfill(statePath, runID, cwd, inRepo)
			return nil
		},
	}
	cmd.Flags().StringVar(&statePath, "state", "", "state file")
	cmd.Flags().Int64Var(&runID, "run", 0, "run ID")
	cmd.Flags().StringVar(&cwd, "cwd", "", "cwd")
	cmd.Flags().BoolVar(&inRepo, "in-repo", false, "the branch and commit were read from .git")
	return cmd
}

// backfill records "git status" and the environment of a shell run. When the
// branch and commit could not be read from .git, it asks git for them too.
func (a *app) backfill(statePath string, runID int64, cwd string, inRepo bool) {
	var meta gitmeta.Meta
	if inRepo {
		meta.Dirty = gitmeta.Dirty(cwd)
	} else {
		meta = gitmeta.Collect(cwd)
	}
	envID := a.cachedEnvironment(statePath)
	_ = a.st.BackfillGit(runID, meta.Root, meta.Branch, meta.Commit, meta.Dirty, envID)
}

// cachedEnvironment returns the ID of the redacted environment this process
// runs with. Redacting compiles the secret-detection rules, which is slow, and
// a shell's environment rarely changes between commands, so the shell keeps
// the last environment's ID next to its state file and reuses it while the
// environment is the same.
func (a *app) cachedEnvironment(statePath string) *int64 {
	if !a.cfg.Storage.CaptureEnv {
		return nil
	}
	cachePath := ""
	key := environmentKey(a.cfg.Storage.DataDir, a.cfg.Redaction.Patterns)
	if statePath != "" {
		cachePath = statePath + ".env"
		if b, err := os.ReadFile(cachePath); err == nil {
			if cachedKey, idText, ok := strings.Cut(strings.TrimSpace(string(b)), " "); ok && cachedKey == key {
				if id, err := strconv.ParseInt(idText, 10, 64); err == nil && a.st.EnvironmentExists(id) {
					return &id
				}
			}
		}
	}
	envJSON := captureEnvCLI(a.cfg)
	if envJSON == "" {
		return nil
	}
	id, err := a.st.InsertOrGetEnvironment(envJSON)
	if err != nil {
		return nil
	}
	if cachePath != "" {
		_ = os.WriteFile(cachePath, []byte(fmt.Sprintf("%s %d\n", key, id)), 0o600)
	}
	return &id
}

// environmentKey identifies this process's environment, and the database and
// redaction patterns its redacted form depends on, without storing any of
// its values.
func environmentKey(dataDir string, patterns []string) string {
	env := os.Environ()
	sort.Strings(env)
	h := sha256.New()
	h.Write([]byte(filepath.Clean(dataDir)))
	for _, p := range patterns {
		h.Write([]byte{0})
		h.Write([]byte(p))
	}
	h.Write([]byte{1})
	for _, kv := range env {
		h.Write([]byte(kv))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
