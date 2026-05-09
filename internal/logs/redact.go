package logs

import (
	"bytes"
	"io"
	"regexp"
)

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
	w io.Writer
	r Redactor
}

func NewRedactingWriter(w io.Writer, r Redactor) *RedactingWriter {
	return &RedactingWriter{w: w, r: r}
}

func (w *RedactingWriter) Write(p []byte) (int, error) {
	_, err := w.w.Write(w.r.Redact(bytes.Clone(p)))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
