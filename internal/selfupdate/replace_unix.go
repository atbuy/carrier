//go:build !windows

package selfupdate

import (
	"fmt"
	"os"
)

// replace moves the staged binary over exe. Renaming over a running
// executable is safe here: processes already running, such as shells
// recording into a session, keep the old inode open and pick the new binary
// up the next time they exec carrier.
func replace(staged, exe string) error {
	if err := os.Rename(staged, exe); err != nil {
		return fmt.Errorf("cannot replace %s: %w", exe, err)
	}
	return nil
}
