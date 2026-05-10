package cli

import (
	"bytes"
	"testing"
)

func TestPrefixWriterPrefixesEachLine(t *testing.T) {
	var buf bytes.Buffer
	writer := newPrefixWriter(&buf, "stdout | ")

	n, err := writer.Write([]byte("one\ntwo"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != len("one\ntwo") {
		t.Fatalf("byte count = %d", n)
	}
	if _, err := writer.Write([]byte("\nthree\n")); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	want := "stdout | one\nstdout | two\nstdout | three\n"
	if got := buf.String(); got != want {
		t.Fatalf("prefixed output = %q, want %q", got, want)
	}
}
