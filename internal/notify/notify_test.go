package notify

import (
	"testing"
)

func TestFormatDuration(t *testing.T) {
	ms := int64(1234)
	if got := formatDuration(&ms); got != "1.2s" {
		t.Fatalf("formatDuration = %q", got)
	}
}

func TestFormatDurationUnknown(t *testing.T) {
	if got := formatDuration(nil); got != "unknown" {
		t.Fatalf("formatDuration(nil) = %q", got)
	}
}
