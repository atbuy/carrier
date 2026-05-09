package runner

import (
	"io"
)

func copyBoth(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	return err
}

func waitCopies(errs ...<-chan error) error {
	var first error
	for _, ch := range errs {
		if err := <-ch; err != nil && first == nil {
			first = err
		}
	}
	return first
}

func asyncCopy(dst io.Writer, src io.Reader) <-chan error {
	ch := make(chan error, 1)
	go func() {
		ch <- copyBoth(dst, src)
	}()
	return ch
}
