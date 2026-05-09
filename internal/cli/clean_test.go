package cli

import (
	"testing"
	"time"
)

func TestParseAge(t *testing.T) {
	tests := map[string]time.Duration{
		"30d": 30 * 24 * time.Hour,
		"2h":  2 * time.Hour,
		"15m": 15 * time.Minute,
	}

	for input, want := range tests {
		got, err := parseAge(input)
		if err != nil {
			t.Fatalf("parseAge(%q) failed: %v", input, err)
		}
		if got != want {
			t.Fatalf("parseAge(%q) = %s, want %s", input, got, want)
		}
	}
}

func TestParseAgeRejectsInvalidInput(t *testing.T) {
	if _, err := parseAge("bad"); err == nil {
		t.Fatalf("parseAge accepted invalid input")
	}
}
