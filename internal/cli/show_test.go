package cli

import (
	"strings"
	"testing"
	"time"
)

func TestLastNLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  string
	}{
		{
			name:  "n=0 returns full string",
			input: "line1\nline2\nline3\n",
			n:     0,
			want:  "line1\nline2\nline3\n",
		},
		{
			name:  "negative n returns full string",
			input: "line1\nline2\nline3\n",
			n:     -1,
			want:  "line1\nline2\nline3\n",
		},
		{
			name:  "empty string returns empty",
			input: "",
			n:     5,
			want:  "",
		},
		{
			name:  "n=1 returns last line with trailing newline",
			input: "line1\nline2\nline3\n",
			n:     1,
			want:  "line3\n",
		},
		{
			name:  "n=2 returns last two lines",
			input: "line1\nline2\nline3\n",
			n:     2,
			want:  "line2\nline3\n",
		},
		{
			name:  "n >= total lines returns all",
			input: "line1\nline2\nline3\n",
			n:     10,
			want:  "line1\nline2\nline3\n",
		},
		{
			name:  "no trailing newline n=1",
			input: "line1\nline2\nline3",
			n:     1,
			want:  "line3",
		},
		{
			name:  "no trailing newline n=2",
			input: "line1\nline2\nline3",
			n:     2,
			want:  "line2\nline3",
		},
		{
			name:  "single line no newline n=1",
			input: "onlyone",
			n:     1,
			want:  "onlyone",
		},
		{
			name:  "single line with trailing newline n=1",
			input: "onlyone\n",
			n:     1,
			want:  "onlyone\n",
		},
		{
			name:  "n=0 on empty string returns empty",
			input: "",
			n:     0,
			want:  "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := lastNLines(tc.input, tc.n)
			if got != tc.want {
				t.Fatalf("lastNLines(%q, %d) = %q, want %q", tc.input, tc.n, got, tc.want)
			}
		})
	}
}

func TestTimeSinceMS(t *testing.T) {
	past := time.Now().Add(-5 * time.Second)
	ms := timeSinceMS(past)
	if ms <= 0 {
		t.Fatalf("timeSinceMS for a past time should be > 0, got %d", ms)
	}
	// Should be at least 5000ms and at most 10000ms (generous upper bound for slow CI)
	if ms < 4000 || ms > 10000 {
		t.Fatalf("timeSinceMS for 5s ago = %dms, expected ~5000", ms)
	}
}

func TestLastNLinesMultilineNoTrailingNewline(t *testing.T) {
	input := strings.Repeat("x\n", 100) + "last"
	got := lastNLines(input, 2)
	if !strings.HasSuffix(got, "last") {
		t.Fatalf("expected suffix 'last', got %q", got)
	}
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), got)
	}
}
