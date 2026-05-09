package command

import "testing"

func TestQuote(t *testing.T) {
	tests := map[string]string{
		"":                     "''",
		"go":                   "go",
		"./...":                "./...",
		"hello world":          "'hello world'",
		"echo hello && exit 1": "'echo hello && exit 1'",
		"it's":                 "'it'\\''s'",
	}

	for input, want := range tests {
		if got := Quote(input); got != want {
			t.Fatalf("Quote(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDisplay(t *testing.T) {
	got := Display([]string{"bash", "-c", "echo hello && exit 1"})
	want := "bash -c 'echo hello && exit 1'"

	if got != want {
		t.Fatalf("Display = %q, want %q", got, want)
	}
}
