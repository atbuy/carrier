package runner

import (
	"errors"
	"testing"
)

func TestWaitCopiesReturnsFirstError(t *testing.T) {
	firstErr := errors.New("first")
	secondErr := errors.New("second")
	ch1 := make(chan error, 1)
	ch2 := make(chan error, 1)
	ch1 <- firstErr
	ch2 <- secondErr

	if err := waitCopies(ch1, ch2); !errors.Is(err, firstErr) {
		t.Fatalf("waitCopies error = %v, want %v", err, firstErr)
	}
}
