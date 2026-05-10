package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/atbuy/carrier/internal/logs"
)

func (a *app) tailCmd() *cobra.Command {
	var stream string
	cmd := &cobra.Command{
		Use:   "tail <id>",
		Short: "stream captured output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			r, err := a.st.GetRun(id)
			if err != nil {
				return err
			}
			follow := r.Status == "running"
			if r.TerminalOutputPath != "" {
				if stream != "both" && stream != "terminal" {
					return fmt.Errorf("stream %q not available for shell run", stream)
				}
				return logs.TailFile(r.TerminalOutputPath, os.Stdout, follow)
			}
			if r.StdoutPath != "" {
				switch stream {
				case "both":
					return tailRunOutput(r.StdoutPath, r.StderrPath, follow)
				case "stdout":
					return logs.TailFile(r.StdoutPath, os.Stdout, follow)
				case "stderr":
					return logs.TailFile(r.StderrPath, os.Stderr, follow)
				default:
					return fmt.Errorf("invalid stream %q: use both, stdout, stderr, or terminal", stream)
				}
			}
			_, _ = io.WriteString(os.Stderr, outputColors(os.Stderr).paint(colorYellow, "carrier: no output logs")+"\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&stream, "stream", "both", "output stream: both, stdout, stderr, terminal")
	return cmd
}

func tailRunOutput(stdoutPath, stderrPath string, follow bool) error {
	errs := make(chan error, 2)
	out := &lockedWriter{w: os.Stdout}
	c := outputColors(os.Stdout)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs <- logs.TailFile(stdoutPath, newPrefixWriter(out, c.paint(colorGreen, "stdout")+" | "), follow)
	}()
	go func() {
		defer wg.Done()
		errs <- logs.TailFile(stderrPath, newPrefixWriter(out, c.paint(colorRed, "stderr")+" | "), follow)
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

type prefixWriter struct {
	w           io.Writer
	prefix      string
	atLineStart bool
}

func newPrefixWriter(w io.Writer, prefix string) *prefixWriter {
	return &prefixWriter{w: w, prefix: prefix, atLineStart: true}
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	originalLen := len(p)
	var out bytes.Buffer
	for len(p) > 0 {
		if w.atLineStart {
			out.WriteString(w.prefix)
			w.atLineStart = false
		}
		idx := bytes.IndexByte(p, '\n')
		if idx == -1 {
			out.Write(p)
			break
		}
		out.Write(p[:idx+1])
		p = p[idx+1:]
		w.atLineStart = true
	}
	if _, err := w.w.Write(out.Bytes()); err != nil {
		return 0, err
	}
	return originalLen, nil
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}
