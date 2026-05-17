//go:build !windows

package shell

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

func watchResize(ptmx *os.File) func() {
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	go func() {
		for range sigwinch {
			if cols, rows, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
				_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
			}
		}
	}()
	return func() {
		signal.Stop(sigwinch)
		close(sigwinch)
	}
}
