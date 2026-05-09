package logs

import (
	"bytes"
	"io"
	"regexp"
)

const redactionWindowBytes = 64 * 1024

type Redactor struct {
	enabled  bool
	patterns []*regexp.Regexp
}

func NewRedactor(enabled bool, patterns []string) Redactor {
	r := Redactor{enabled: enabled}
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err == nil {
			r.patterns = append(r.patterns, re)
		}
	}
	return r
}

func (r Redactor) Redact(b []byte) []byte {
	if !r.enabled || len(r.patterns) == 0 {
		return b
	}
	out := b
	for _, re := range r.patterns {
		out = re.ReplaceAll(out, []byte("[REDACTED]"))
	}
	return out
}

type RedactingWriter struct {
	w      io.Writer
	r      Redactor
	buffer []byte
}

func NewRedactingWriter(w io.Writer, r Redactor) *RedactingWriter {
	return &RedactingWriter{w: w, r: r}
}

func (w *RedactingWriter) Write(p []byte) (int, error) {
	if !w.r.enabled || len(w.r.patterns) == 0 {
		_, err := w.w.Write(p)
		if err != nil {
			return 0, err
		}
		return len(p), nil
	}
	w.buffer = append(w.buffer, p...)
	flushable := len(w.buffer) - redactionWindowBytes
	if flushable <= 0 {
		return len(p), nil
	}
	_, err := w.w.Write(w.r.Redact(bytes.Clone(w.buffer[:flushable])))
	if err != nil {
		return 0, err
	}
	w.buffer = append(w.buffer[:0], w.buffer[flushable:]...)
	return len(p), nil
}

func (w *RedactingWriter) Close() error {
	if len(w.buffer) == 0 {
		return nil
	}
	_, err := w.w.Write(w.r.Redact(bytes.Clone(w.buffer)))
	w.buffer = nil
	return err
}
