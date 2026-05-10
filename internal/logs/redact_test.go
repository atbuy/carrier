package logs

import (
	"bytes"
	"testing"
)

func TestRedactorRedactsConfiguredPatterns(t *testing.T) {
	redactor := NewRedactor(true, []string{`Bearer [A-Za-z0-9._-]+`, `TOKEN=\S+`})

	got := string(redactor.Redact([]byte("Authorization: Bearer abc.def TOKEN=secret")))
	want := "Authorization: [REDACTED] [REDACTED]"

	if got != want {
		t.Fatalf("redacted output mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRedactorDisabledLeavesInputUnchanged(t *testing.T) {
	redactor := NewRedactor(false, []string{`TOKEN=\S+`})

	got := string(redactor.Redact([]byte("TOKEN=secret")))
	want := "TOKEN=secret"

	if got != want {
		t.Fatalf("disabled redactor changed input: got %q want %q", got, want)
	}
}

func TestRedactingWriterReportsOriginalByteCount(t *testing.T) {
	var buf bytes.Buffer
	writer := NewRedactingWriter(&buf, NewRedactor(true, []string{`password=\S+`}))

	n, err := writer.Write([]byte("password=hunter2"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if n != len("password=hunter2") {
		t.Fatalf("byte count mismatch: got %d want %d", n, len("password=hunter2"))
	}
	if got := buf.String(); got != "[REDACTED]" {
		t.Fatalf("redacted write mismatch: got %q", got)
	}
}

func TestRedactingWriterRedactsSecretSplitAcrossWrites(t *testing.T) {
	var buf bytes.Buffer
	writer := NewRedactingWriter(&buf, NewRedactor(true, []string{`TOKEN=\S+`}))

	if _, err := writer.Write([]byte("prefix TOKEN=sec")); err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if _, err := writer.Write([]byte("ret suffix")); err != nil {
		t.Fatalf("second write failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if got := buf.String(); got != "prefix [REDACTED] suffix" {
		t.Fatalf("split secret redaction mismatch: %q", got)
	}
}

func TestRedactingWriterFlushesWhenBufferExceedsWindow(t *testing.T) {
	var buf bytes.Buffer
	writer := NewRedactingWriter(&buf, NewRedactor(true, []string{`SECRET=\S+`}))

	// Write more than 64KB so the flush path (flushable > 0) is triggered.
	chunk := bytes.Repeat([]byte("x"), redactionWindowBytes+1000)
	n, err := writer.Write(chunk)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != len(chunk) {
		t.Fatalf("byte count = %d, want %d", n, len(chunk))
	}
	// Close flushes remaining buffer.
	if err := writer.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected non-empty output after large write")
	}
}

func TestRedactingWriterFlushReportsWriteError(t *testing.T) {
	writer := NewRedactingWriter(&errorWriter{}, NewRedactor(true, []string{`SECRET=\S+`}))
	chunk := bytes.Repeat([]byte("x"), redactionWindowBytes+1)

	if _, err := writer.Write(chunk); err == nil {
		t.Fatal("expected flush write error")
	}
}

func TestRedactingWriterDisabledPassesThrough(t *testing.T) {
	var buf bytes.Buffer
	writer := NewRedactingWriter(&buf, NewRedactor(false, []string{`TOKEN=\S+`}))

	data := "TOKEN=secret plain text"
	n, err := writer.Write([]byte(data))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != len(data) {
		t.Fatalf("byte count = %d, want %d", n, len(data))
	}
	if buf.String() != data {
		t.Fatalf("disabled writer changed data: %q", buf.String())
	}
}

func TestRedactingWriterCloseEmptyBufferIsNoop(t *testing.T) {
	var buf bytes.Buffer
	writer := NewRedactingWriter(&buf, NewRedactor(true, []string{`TOKEN=\S+`}))
	// Close without any Write — buffer is empty, should be a no-op.
	if err := writer.Close(); err != nil {
		t.Fatalf("close empty writer: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty buf, got %d bytes", buf.Len())
	}
}

func TestRedactingWriterMultipleFlushes(t *testing.T) {
	var buf bytes.Buffer
	writer := NewRedactingWriter(&buf, NewRedactor(true, []string{`SECRET=\S+`}))

	// Two big writes — each triggers a flush.
	chunk := bytes.Repeat([]byte("a"), redactionWindowBytes+500)
	for i := 0; i < 2; i++ {
		if _, err := writer.Write(chunk); err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	wantLen := len(chunk) * 2
	if buf.Len() != wantLen {
		t.Fatalf("buf len = %d, want %d", buf.Len(), wantLen)
	}
}

func TestRedactingWriterRedactsMultilinePrivateKey(t *testing.T) {
	var buf bytes.Buffer
	writer := NewRedactingWriter(&buf, NewRedactor(true, []string{`-----BEGIN PRIVATE KEY-----[\s\S]*?-----END PRIVATE KEY-----`}))

	if _, err := writer.Write([]byte("before -----BEGIN PRIVATE KEY-----\nabc\n")); err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if _, err := writer.Write([]byte("def\n-----END PRIVATE KEY----- after")); err != nil {
		t.Fatalf("second write failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if got := buf.String(); got != "before [REDACTED] after" {
		t.Fatalf("private key redaction mismatch: %q", got)
	}
}
