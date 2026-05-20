package logs

import (
	"regexp"
	"testing"
)

func TestBuiltinPatterns(t *testing.T) {
	patterns := BuiltinPatterns()
	if len(patterns) == 0 {
		t.Fatal("BuiltinPatterns returned empty slice")
	}
	validCount := 0
	for _, p := range patterns {
		if _, err := regexp.Compile(p); err != nil {
			t.Logf("invalid regex skipped: %q: %v", p, err)
			continue
		}
		validCount++
	}
	if validCount == 0 {
		t.Fatal("all patterns failed to compile as valid Go regex")
	}
}
