package duration

import (
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	cases := map[string]time.Duration{
		"0":     0,
		"0s":    0,
		"0d":    0,
		"500ms": 500 * time.Millisecond,
		"90s":   90 * time.Second,
		"30m":   30 * time.Minute,
		"1.5h":  90 * time.Minute,
		"24h":   24 * time.Hour,
		"7d":    7 * Day,
		"1.5d":  36 * time.Hour,
		"2w":    14 * Day,
		"1d12h": 36 * time.Hour,
		"1h30m": 90 * time.Minute,
		" 30d ": 30 * Day,
		"+5m":   5 * time.Minute,
		"10us":  10 * time.Microsecond,
		"10µs":  10 * time.Microsecond,
	}
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %s, %v; want %s", in, got, err, want)
		}
	}
}

func TestParseMatchesTimeParseDuration(t *testing.T) {
	for _, in := range []string{"1ns", "2.5s", "1h2m3s", "0.25m", "72h"} {
		want, err := time.ParseDuration(in)
		if err != nil {
			t.Fatalf("time.ParseDuration(%q): %v", in, err)
		}
		if got, err := Parse(in); err != nil || got != want {
			t.Errorf("Parse(%q) = %s, %v; want %s", in, got, err, want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{"", " ", "bad", "7", "d", "1.2.3h", "5x", "1mo", "-1h", "-7d", "+", "1h-5m", "99999999999w"} {
		if got, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %s, want an error", in, got)
		}
	}
	if _, err := Parse("bad"); !strings.Contains(err.Error(), "7d") {
		t.Errorf("error should show the format: %v", err)
	}
	if _, err := Parse("-1h"); !strings.Contains(err.Error(), "negative") {
		t.Errorf("negative error = %v", err)
	}
}
