package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestLogoPixels(t *testing.T) {
	for y, row := range logoPixels {
		if len(row) != len(logoPixels[0]) {
			t.Errorf("row %d is %d pixels wide, want %d", y, len(row), len(logoPixels[0]))
		}
		for x := 0; x < len(row); x++ {
			if _, ok := logoPalette[row[x]]; !ok && row[x] != '.' {
				t.Errorf("pixel %d,%d: %q is not in logoPalette", x, y, row[x])
			}
		}
	}
}

func TestRenderLogo(t *testing.T) {
	wantLines := (len(logoPixels) + 1) / 2
	for _, tc := range []struct {
		name    string
		profile termenv.Profile
		seq     string
	}{
		{"truecolor", termenv.TrueColor, "38;2;"},
		{"256 colors", termenv.ANSI256, "38;5;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := renderLogo(tc.profile)
			if len(lines) != wantLines {
				t.Fatalf("got %d lines, want %d", len(lines), wantLines)
			}
			for i, line := range lines {
				if w := ansi.StringWidth(line); w != len(logoPixels[0]) {
					t.Errorf("line %d is %d cells wide, want %d", i, w, len(logoPixels[0]))
				}
			}
			if !strings.Contains(strings.Join(lines, ""), tc.seq) {
				t.Errorf("want %q color sequences", tc.seq)
			}
		})
	}
	for _, p := range []termenv.Profile{termenv.ANSI, termenv.Ascii} {
		if lines := renderLogo(p); lines != nil {
			t.Errorf("profile %v: want no logo, got %d lines", p, len(lines))
		}
	}
}

func TestLogoOnlyInTerminals(t *testing.T) {
	if lines := logo(&bytes.Buffer{}); lines != nil {
		t.Fatalf("want no logo for a non-terminal writer, got %d lines", len(lines))
	}
}

func TestBesideLogo(t *testing.T) {
	var out strings.Builder
	besideLogo(&out, []string{"AA", "B", "CC"}, 1, "x", "y", "z")
	want := "AA\nB    x\nCC   y\n     z\n"
	if out.String() != want {
		t.Errorf("got\n%q\nwant\n%q", out.String(), want)
	}

	out.Reset()
	besideLogo(&out, nil, 5, "x", "y")
	if out.String() != "x\ny\n" {
		t.Errorf("without a logo: got %q, want text only", out.String())
	}
}
