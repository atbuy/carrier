package cli

import (
	"strings"
	"testing"
)

func TestDefaultConfigTOML(t *testing.T) {
	out := defaultConfigTOML()
	for _, want := range []string{
		"[storage]",
		`data_dir = "~/.local/share/carrier"`,
		"[redaction]",
		"[notify]",
		"[shell]",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("default config missing %q\n%s", want, out)
		}
	}
}
