package runner

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/atbuy/carrier/internal/store"
)

func TestExitCodeSuccess(t *testing.T) {
	code, status := ExitCode(nil)
	if code != 0 || status != store.StatusSuccess {
		t.Fatalf("ExitCode(nil) = (%d, %q)", code, status)
	}
}

func TestExitCodeFromProcessFailure(t *testing.T) {
	err := exec.Command("sh", "-c", "exit 7").Run()

	code, status := ExitCode(err)
	if code != 7 || status != store.StatusFailed {
		t.Fatalf("ExitCode(exit 7) = (%d, %q)", code, status)
	}
}

func TestExitCodeUnknownError(t *testing.T) {
	code, status := ExitCode(errors.New("boom"))
	if code != 1 || status != store.StatusUnknown {
		t.Fatalf("ExitCode(unknown) = (%d, %q)", code, status)
	}
}
