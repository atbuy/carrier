//go:build windows

package shell

import (
	"os"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

func watchResize(ptmx *os.File) func() {
	stop := make(chan struct{})
	go func() {
		var lastCols, lastRows int
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
				if err != nil {
					continue
				}
				if cols != lastCols || rows != lastRows {
					_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
					lastCols, lastRows = cols, rows
				}
			}
		}
	}()
	return func() { close(stop) }
}
