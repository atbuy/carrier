package shell

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// cleanMinAge leaves recent files alone, in case the process that owns
	// them is not visible from here (another PID namespace sharing /tmp).
	cleanMinAge = time.Hour
	// cleanLegacyAge is how old a hook directory from before carrier 1.2.4,
	// whose name has no process ID, must be before it is removed.
	cleanLegacyAge = 7 * 24 * time.Hour
)

// CleanTemp removes the files recorded shells leave in the temp directory
// when they are killed instead of exiting, such as by "tmux kill-server":
// state files, their session and environment files, and hook directories.
// Only this user's files, whose carrier process has ended, are removed. It
// returns the number of files and directories removed, and the last state of
// each shell whose state file it removed: a command still in flight there was
// cut short when the shell was killed.
func CleanTemp(now time.Time) (int, []State) { return cleanTemp(now, false) }

// LeftoverTemp counts the files and directories CleanTemp would remove.
func LeftoverTemp(now time.Time) int {
	n, _ := cleanTemp(now, true)
	return n
}

func cleanTemp(now time.Time, dryRun bool) (int, []State) {
	var killed []State
	tmp := os.TempDir()
	done := map[string]bool{}
	remove := func(path string, all bool) bool {
		if done[path] {
			return false
		}
		done[path] = true
		if dryRun {
			return true
		}
		if all {
			return os.RemoveAll(path) == nil
		}
		return os.Remove(path) == nil
	}
	removed := 0
	states, _ := filepath.Glob(filepath.Join(tmp, "carrier-shell-*.json"))
	for _, statePath := range states {
		pid, ok := pidFromName(strings.TrimSuffix(filepath.Base(statePath), ".json"))
		if !ok || !ownRegularFile(statePath) || !old(statePath, now, cleanMinAge) || processAlive(pid) {
			continue
		}
		if !dryRun {
			killed = append(killed, (&StateFile{Path: statePath}).Read())
		}
		for _, path := range []string{statePath, SessionInfoPath(statePath), statePath + ".env", BranchPath(statePath)} {
			if ownRegularFile(path) && remove(path, false) {
				removed++
			}
		}
	}
	// Session and environment files whose state file is already gone.
	extras, _ := filepath.Glob(filepath.Join(tmp, "carrier-shell-*.json.*"))
	for _, path := range extras {
		base, _, _ := strings.Cut(filepath.Base(path), ".")
		pid, ok := pidFromName(base)
		if ok && ownRegularFile(path) && old(path, now, cleanMinAge) && !processAlive(pid) && remove(path, false) {
			removed++
		}
	}
	dirs, _ := filepath.Glob(filepath.Join(tmp, "carrier-shell-*"))
	for _, dir := range dirs {
		if !ownDir(dir) {
			continue
		}
		name := strings.TrimPrefix(filepath.Base(dir), "carrier-shell-")
		pidText, _, hasPID := strings.Cut(name, "-")
		if hasPID {
			pid, err := strconv.Atoi(pidText)
			if err != nil || !old(dir, now, cleanMinAge) || processAlive(pid) {
				continue
			}
		} else if !old(dir, now, cleanLegacyAge) {
			continue
		}
		if remove(dir, true) {
			removed++
		}
	}
	return removed, killed
}

// pidFromName reads the process ID from a state file name, "carrier-shell-<pid>".
func pidFromName(base string) (int, bool) {
	pid, err := strconv.Atoi(strings.TrimPrefix(base, "carrier-shell-"))
	return pid, err == nil && pid > 0
}

func old(path string, now time.Time, age time.Duration) bool {
	info, err := os.Lstat(path)
	return err == nil && now.Sub(info.ModTime()) >= age
}
