//go:build !windows

package shell

import (
	"os"
	"os/exec"
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

// startPTY starts cmd on a new pseudo-terminal, like pty.Start, and tells the
// shell which terminal it is recording as CARRIER_TTY. Variables a shell
// exports are copied into tmux panes started from it, which run in other
// terminals; CARRIER_TTY lets carrier tell those apart from the recorded shell.
func startPTY(cmd *exec.Cmd) (*os.File, error) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tty.Close() }()
	cmd.Env = append(cmd.Env, "CARRIER_TTY="+tty.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		_ = ptmx.Close()
		return nil, err
	}
	return ptmx, nil
}
