package logs

import "io"

const truncationNotice = "\n[carrier: output truncated at configured max_output_mb]\n"

type CappedWriter struct {
	w         io.Writer
	limit     int64
	written   int64
	truncated bool
}

func NewCappedWriter(w io.Writer, limitBytes int64) *CappedWriter {
	return &CappedWriter{w: w, limit: limitBytes}
}

func NewCappedAppendWriter(w io.Writer, limitBytes, alreadyWritten int64) *CappedWriter {
	return &CappedWriter{w: w, limit: limitBytes, written: alreadyWritten, truncated: limitBytes > 0 && alreadyWritten > limitBytes}
}

func (w *CappedWriter) Write(p []byte) (int, error) {
	if w.limit <= 0 {
		_, err := w.w.Write(p)
		if err != nil {
			return 0, err
		}
		return len(p), nil
	}
	remaining := w.limit - w.written
	if remaining <= 0 {
		return w.markTruncated(len(p))
	}
	toWrite := p
	if int64(len(toWrite)) > remaining {
		toWrite = p[:remaining]
	}
	n, err := w.w.Write(toWrite)
	w.written += int64(n)
	if err != nil {
		return n, err
	}
	if len(toWrite) < len(p) {
		return w.markTruncated(len(p))
	}
	return len(p), nil
}

func (w *CappedWriter) markTruncated(originalLen int) (int, error) {
	if !w.truncated {
		w.truncated = true
		_, err := w.w.Write([]byte(truncationNotice))
		if err != nil {
			return 0, err
		}
	}
	return originalLen, nil
}

func MaxOutputBytes(maxOutputMB int64) int64 {
	if maxOutputMB <= 0 {
		return 0
	}
	return maxOutputMB * 1024 * 1024
}
