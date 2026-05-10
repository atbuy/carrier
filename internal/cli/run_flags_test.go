package cli

import (
	"testing"
	"time"
)

func TestParseRunFlags_NotifyFlags(t *testing.T) {
	tests := []struct {
		args          []string
		wantNotify    bool
		wantAlways    bool
		wantRemainder []string
	}{
		{
			args:          []string{"-n", "echo", "hello"},
			wantNotify:    true,
			wantAlways:    false,
			wantRemainder: []string{"echo", "hello"},
		},
		{
			args:          []string{"--notify", "echo", "hello"},
			wantNotify:    true,
			wantAlways:    false,
			wantRemainder: []string{"echo", "hello"},
		},
		{
			args:          []string{"-N", "echo", "hello"},
			wantNotify:    false,
			wantAlways:    true,
			wantRemainder: []string{"echo", "hello"},
		},
		{
			args:          []string{"--notify-always", "echo", "hello"},
			wantNotify:    false,
			wantAlways:    true,
			wantRemainder: []string{"echo", "hello"},
		},
		{
			args:          []string{"-n", "-N", "echo"},
			wantNotify:    true,
			wantAlways:    true,
			wantRemainder: []string{"echo"},
		},
	}
	for _, tc := range tests {
		a := &app{}
		rest, err := a.parseRunFlags(tc.args)
		if err != nil {
			t.Fatalf("parseRunFlags(%v) error: %v", tc.args, err)
		}
		if a.notify != tc.wantNotify {
			t.Errorf("args=%v: notify=%v, want %v", tc.args, a.notify, tc.wantNotify)
		}
		if a.notifyAlways != tc.wantAlways {
			t.Errorf("args=%v: notifyAlways=%v, want %v", tc.args, a.notifyAlways, tc.wantAlways)
		}
		if len(rest) != len(tc.wantRemainder) {
			t.Errorf("args=%v: remainder=%v, want %v", tc.args, rest, tc.wantRemainder)
		}
	}
}

func TestParseRunFlags_QuietAndNoRedact(t *testing.T) {
	tests := []struct {
		args          []string
		wantQuiet     bool
		wantNoRedact  bool
		wantRemainder []string
	}{
		{
			args:          []string{"-q", "cmd"},
			wantQuiet:     true,
			wantNoRedact:  false,
			wantRemainder: []string{"cmd"},
		},
		{
			args:          []string{"--quiet", "cmd"},
			wantQuiet:     true,
			wantNoRedact:  false,
			wantRemainder: []string{"cmd"},
		},
		{
			args:          []string{"--no-redact", "cmd"},
			wantQuiet:     false,
			wantNoRedact:  true,
			wantRemainder: []string{"cmd"},
		},
		{
			args:          []string{"-q", "--no-redact", "cmd"},
			wantQuiet:     true,
			wantNoRedact:  true,
			wantRemainder: []string{"cmd"},
		},
	}
	for _, tc := range tests {
		a := &app{}
		rest, err := a.parseRunFlags(tc.args)
		if err != nil {
			t.Fatalf("parseRunFlags(%v) error: %v", tc.args, err)
		}
		if a.quiet != tc.wantQuiet {
			t.Errorf("args=%v: quiet=%v, want %v", tc.args, a.quiet, tc.wantQuiet)
		}
		if a.noRedact != tc.wantNoRedact {
			t.Errorf("args=%v: noRedact=%v, want %v", tc.args, a.noRedact, tc.wantNoRedact)
		}
		if len(rest) != len(tc.wantRemainder) {
			t.Errorf("args=%v: remainder=%v, want %v", tc.args, rest, tc.wantRemainder)
		}
	}
}

func TestParseRunFlags_Timeout(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantTimeout   time.Duration
		wantRemainder []string
	}{
		{
			name:          "--timeout space value",
			args:          []string{"--timeout", "1s", "echo"},
			wantTimeout:   time.Second,
			wantRemainder: []string{"echo"},
		},
		{
			name:          "--timeout=value",
			args:          []string{"--timeout=2m", "echo"},
			wantTimeout:   2 * time.Minute,
			wantRemainder: []string{"echo"},
		},
		{
			name:          "-t space value",
			args:          []string{"-t", "500ms", "echo"},
			wantTimeout:   500 * time.Millisecond,
			wantRemainder: []string{"echo"},
		},
		{
			name:          "-t=value",
			args:          []string{"-t=30s", "echo"},
			wantTimeout:   30 * time.Second,
			wantRemainder: []string{"echo"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &app{}
			rest, err := a.parseRunFlags(tc.args)
			if err != nil {
				t.Fatalf("parseRunFlags(%v) error: %v", tc.args, err)
			}
			if a.timeout != tc.wantTimeout {
				t.Errorf("timeout=%v, want %v", a.timeout, tc.wantTimeout)
			}
			if len(rest) != len(tc.wantRemainder) {
				t.Errorf("remainder=%v, want %v", rest, tc.wantRemainder)
			}
		})
	}
}

func TestParseRunFlags_TimeoutMissingValue(t *testing.T) {
	a := &app{}
	_, err := a.parseRunFlags([]string{"-t"})
	if err == nil {
		t.Fatalf("expected error for -t with no value, got nil")
	}

	a2 := &app{}
	_, err2 := a2.parseRunFlags([]string{"--timeout"})
	if err2 == nil {
		t.Fatalf("expected error for --timeout with no value, got nil")
	}
}

func TestParseRunFlags_InvalidTimeout(t *testing.T) {
	cases := [][]string{
		{"--timeout=bogus", "echo"},
		{"-t=notaduration", "echo"},
		{"--timeout", "notaduration", "echo"},
	}
	for _, args := range cases {
		a := &app{}
		_, err := a.parseRunFlags(args)
		if err == nil {
			t.Fatalf("expected error for invalid timeout in args %v, got nil", args)
		}
	}
}

func TestParseRunFlags_NoFlags(t *testing.T) {
	a := &app{}
	rest, err := a.parseRunFlags([]string{"echo", "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rest) != 2 || rest[0] != "echo" || rest[1] != "hello" {
		t.Fatalf("remainder = %v, want [echo hello]", rest)
	}
	if a.notify || a.notifyAlways || a.quiet || a.noRedact || a.timeout != 0 {
		t.Fatalf("expected all flags unset, got notify=%v notifyAlways=%v quiet=%v noRedact=%v timeout=%v",
			a.notify, a.notifyAlways, a.quiet, a.noRedact, a.timeout)
	}
}

func TestParseRunFlags_EmptyArgs(t *testing.T) {
	a := &app{}
	rest, err := a.parseRunFlags(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("expected empty remainder, got %v", rest)
	}
}

func TestParseRunFlags_AllFlags(t *testing.T) {
	a := &app{}
	rest, err := a.parseRunFlags([]string{"-n", "-N", "-q", "--no-redact", "--timeout=10s", "mycmd", "--myflag"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !a.notify {
		t.Errorf("expected notify=true")
	}
	if !a.notifyAlways {
		t.Errorf("expected notifyAlways=true")
	}
	if !a.quiet {
		t.Errorf("expected quiet=true")
	}
	if !a.noRedact {
		t.Errorf("expected noRedact=true")
	}
	if a.timeout != 10*time.Second {
		t.Errorf("expected timeout=10s, got %v", a.timeout)
	}
	if len(rest) != 2 || rest[0] != "mycmd" || rest[1] != "--myflag" {
		t.Fatalf("remainder=%v, want [mycmd --myflag]", rest)
	}
}
