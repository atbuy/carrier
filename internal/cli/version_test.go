package cli

import (
	"bytes"
	"testing"
)

func TestVersionCmd(t *testing.T) {
	a := &app{}
	cmd := a.versionCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("versionCmd: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected non-empty version output")
	}
}
