//go:build windows

package selfupdate

import (
	"fmt"
	"os"
)

// replace moves the staged binary over exe. Windows refuses to overwrite a
// running executable, so the current one is renamed aside first. It stays open
// until this process exits, so removing it is best effort; a leftover is swept
// by the next update.
func replace(staged, exe string) error {
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("cannot move the current binary aside: %w", err)
	}
	if err := os.Rename(staged, exe); err != nil {
		// Put the working binary back rather than leaving nothing on PATH.
		if restore := os.Rename(old, exe); restore != nil {
			return fmt.Errorf("cannot replace %s: %w; the previous binary is at %s", exe, err, old)
		}
		return fmt.Errorf("cannot replace %s: %w", exe, err)
	}
	_ = os.Remove(old)
	return nil
}
