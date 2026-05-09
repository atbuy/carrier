package logs

import (
	"bytes"
	"strings"
	"testing"
)

func TestCappedWriterWritesWithinLimit(t *testing.T) {
	var buf bytes.Buffer
	writer := NewCappedWriter(&buf, 10)

	n, err := writer.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 5 {
		t.Fatalf("byte count = %d", n)
	}
	if got := buf.String(); got != "hello" {
		t.Fatalf("buffer = %q", got)
	}
}

func TestCappedWriterTruncatesAndReportsOriginalByteCount(t *testing.T) {
	var buf bytes.Buffer
	writer := NewCappedWriter(&buf, 5)

	n, err := writer.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != len("hello world") {
		t.Fatalf("byte count = %d", n)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "hello") {
		t.Fatalf("buffer missing capped content: %q", got)
	}
	if !strings.Contains(got, "output truncated") {
		t.Fatalf("buffer missing truncation notice: %q", got)
	}
}

func TestCappedAppendWriterHonorsAlreadyWrittenBytes(t *testing.T) {
	var buf bytes.Buffer
	writer := NewCappedAppendWriter(&buf, 5, 5)

	n, err := writer.Write([]byte("more"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 4 {
		t.Fatalf("byte count = %d", n)
	}
	if got := buf.String(); !strings.Contains(got, "output truncated") || strings.Contains(got, "more") {
		t.Fatalf("append cap mismatch: %q", got)
	}
}

func TestCappedAppendWriterDoesNotRepeatNoticeAfterLimitExceeded(t *testing.T) {
	var buf bytes.Buffer
	writer := NewCappedAppendWriter(&buf, 5, 64)

	n, err := writer.Write([]byte("more"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 4 {
		t.Fatalf("byte count = %d", n)
	}
	if got := buf.String(); got != "" {
		t.Fatalf("expected repeated overflow to be discarded, got %q", got)
	}
}

func TestMaxOutputBytes(t *testing.T) {
	if got := MaxOutputBytes(2); got != 2*1024*1024 {
		t.Fatalf("MaxOutputBytes = %d", got)
	}
	if got := MaxOutputBytes(0); got != 0 {
		t.Fatalf("zero should disable limit, got %d", got)
	}
}
