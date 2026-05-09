package logs

import (
	"io"
	"os"
	"time"
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
