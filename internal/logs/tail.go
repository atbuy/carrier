package logs

import (
	"io"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TailFile(path string, out io.Writer, follow bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(out, f); err != nil {
		return err
	}
	if !follow {
		return nil
	}
	return tailFollow(f, path, out)
}

// tailFollow watches path for writes and streams new bytes to out.
// Falls back to 500ms polling if fsnotify is unavailable.
func tailFollow(f *os.File, path string, out io.Writer) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return tailPoll(f, out)
	}
	defer func() { _ = watcher.Close() }()
	if err := watcher.Add(path); err != nil {
		return tailPoll(f, out)
	}
	buf := make([]byte, 32*1024)
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}
			if err := drainFile(f, buf, out); err != nil {
				return err
			}
		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			return watchErr
		}
	}
}

// drainFile reads all available bytes from f into out until EOF.
func drainFile(f *os.File, buf []byte, out io.Writer) error {
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// tailPoll is the fallback when fsnotify is unavailable.
func tailPoll(f *os.File, out io.Writer) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
	}
}
