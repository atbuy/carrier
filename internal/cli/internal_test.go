package cli

import "testing"

func TestShouldIgnoreCarrierInternalCommands(t *testing.T) {
	tests := []string{
		"",
		"_carrier_begin",
		"_carrier_end",
		"carrier internal begin --state /tmp/state --cmd ls",
		"/usr/local/bin/carrier internal end --state /tmp/state --exit 0",
		"vim file.go",
		"/usr/bin/vim file.go",
	}

	for _, input := range tests {
		if !shouldIgnore(input, []string{"vim"}) {
			t.Fatalf("shouldIgnore(%q) = false, want true", input)
		}
	}
}

func TestShouldIgnoreAllowsNormalCommands(t *testing.T) {
	for _, input := range []string{"go test ./...", "carrier status", "echo carrier internal"} {
		if shouldIgnore(input, []string{"vim"}) {
			t.Fatalf("shouldIgnore(%q) = true, want false", input)
		}
	}
}
