package logs

import (
	"bytes"
	"testing"
	"time"
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

func TestSensitiveEnvKeyKnownSecretNames(t *testing.T) {
	tests := []string{
		"DB_PASS",
		"MONGO_PASSWORD",
		"STRIPE_SK",
		"SENDGRID_KEY",
		"GH_TOKEN",
		"NPM_TOKEN",
		"HEROKU_API_KEY",
		"TWILIO_AUTH_TOKEN",
		"VAULT_TOKEN",
		"PG_PASSWORD",
		"AWS_SECRET_ACCESS_KEY",
	}
	for _, tt := range tests {
		if !SensitiveEnvKey(tt) {
			t.Fatalf("SensitiveEnvKey(%q) = false, want true", tt)
		}
	}
}

func TestSensitiveEnvKeyAvoidsBenignKeySubstrings(t *testing.T) {
	for _, tt := range []string{"MONKEY", "KEYBOARD_LAYOUT", "PUBLIC_URL", "HOME"} {
		if SensitiveEnvKey(tt) {
			t.Fatalf("SensitiveEnvKey(%q) = true, want false", tt)
		}
	}
}

func TestRedactEnvValueConnectionStringsAndEntropy(t *testing.T) {
	redactor := NewRedactor(true, nil)

	tests := []struct {
		key   string
		value string
		want  string
	}{
		{key: "CACHE_URL", value: "redis://user:pass@localhost:6379/0", want: "[REDACTED]"},
		{key: "PUBLIC_URL", value: "https://example.com/app", want: "https://example.com/app"},
		{key: "PATH", value: "/usr/local/bin:/usr/bin:/bin", want: "/usr/local/bin:/usr/bin:/bin"},
		{key: "SESSION_ID", value: "mF9xQ2pL8zR4vT7nB6cD3eH1jK5w", want: "[REDACTED]"},
	}

	for _, tt := range tests {
		if got := RedactEnvValue(tt.key, tt.value, redactor); got != tt.want {
			t.Fatalf("RedactEnvValue(%q, %q) = %q, want %q", tt.key, tt.value, got, tt.want)
		}
	}
}

func TestRedactEnvValueDisabledSkipsKeyAndEntropyHeuristics(t *testing.T) {
	redactor := NewRedactor(false, []string{`secret`})
	got := RedactEnvValue("TOKEN", "secret", redactor)
	if got != "secret" {
		t.Fatalf("disabled RedactEnvValue = %q, want original value", got)
	}
}

func TestNewRedactorWithBuiltinsRedactsExtraPattern(t *testing.T) {
	r := NewRedactorWithBuiltins(true, []string{`MYTOKEN=\S+`})
	got := string(r.Redact([]byte("MYTOKEN=secret123")))
	if got != "[REDACTED]" {
		t.Fatalf("NewRedactorWithBuiltins did not redact extra pattern: %q", got)
	}
}

func TestNewRedactorWithBuiltinsDisabledLeavesInputUnchanged(t *testing.T) {
	r := NewRedactorWithBuiltins(false, []string{`MYTOKEN=\S+`})
	input := "MYTOKEN=secret123"
	if got := string(r.Redact([]byte(input))); got != input {
		t.Fatalf("disabled NewRedactorWithBuiltins changed input: %q", got)
	}
}

func TestBuiltinCompiledCachedAcrossCalls(t *testing.T) {
	a := builtinCompiled()
	b := builtinCompiled()
	if len(a) == 0 {
		t.Fatal("builtinCompiled returned empty slice")
	}
	if &a[0] != &b[0] {
		t.Fatal("builtinCompiled returned different slices on second call — not cached")
	}
	// cap must equal len so concurrent appends never share the backing array.
	if cap(a) != len(a) {
		t.Fatalf("builtinCompiled cap=%d != len=%d; concurrent appends would race", cap(a), len(a))
	}
}

func TestNewRedactorWithBuiltinsConcurrentAppendNoRace(t *testing.T) {
	// Run with -race to detect backing-array sharing between concurrent callers.
	const goroutines = 20
	done := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		go func() {
			_ = NewRedactorWithBuiltins(true, []string{`EXTRA=\S+`})
			done <- struct{}{}
		}()
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}
}

func BenchmarkNewRedactorWithBuiltinsRepeated(b *testing.B) {
	extra := []string{`MYTOKEN=\S+`}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewRedactorWithBuiltins(true, extra)
	}
}

func BenchmarkNewRedactorFromScratch(b *testing.B) {
	patterns := append(BuiltinPatterns(), `MYTOKEN=\S+`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewRedactor(true, patterns)
	}
}

func TestNewRedactorWithBuiltinsFasterThanFromScratch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing test in short mode")
	}
	patterns := append(BuiltinPatterns(), `MYTOKEN=\S+`)
	const iters = 10

	start := time.Now()
	for i := 0; i < iters; i++ {
		_ = NewRedactor(true, patterns)
	}
	scratchDur := time.Since(start)

	// warm cache
	_ = NewRedactorWithBuiltins(true, nil)
	start = time.Now()
	for i := 0; i < iters; i++ {
		_ = NewRedactorWithBuiltins(true, patterns[len(patterns)-1:])
	}
	cachedDur := time.Since(start)

	if cachedDur >= scratchDur {
		t.Fatalf("cached (%v) not faster than scratch (%v)", cachedDur, scratchDur)
	}
}
