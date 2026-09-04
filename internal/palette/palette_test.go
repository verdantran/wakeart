package palette

import (
	"math"
	"os"
	"strings"
	"testing"
)

func TestAllPalettesHaveStops(t *testing.T) {
	for _, p := range All {
		if len(p.Stops) < 2 {
			t.Errorf("%s has %d stops, need at least 2", p.Name, len(p.Stops))
		}
		for i, s := range p.Stops {
			if s.ANSI256 < 0 || s.ANSI256 > 255 {
				t.Errorf("%s stop %d: ANSI256 %d out of range", p.Name, i, s.ANSI256)
			}
			if s.ANSI16 < 0 || s.ANSI16 > 15 {
				t.Errorf("%s stop %d: ANSI16 %d out of range", p.Name, i, s.ANSI16)
			}
		}
	}
}

// luminance is the relative-luminance term of the WCAG contrast ratio.
func luminance(c RGB) float64 {
	f := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
}

// The brightest end of each gradient must stand off a dark terminal, or the
// art disappears on the ground it is most often shown against.
func TestBrightEndIsVisible(t *testing.T) {
	bg := RGB{0, 0, 0}
	for _, p := range All {
		top := p.At(1)
		ratio := (luminance(top) + 0.05) / (luminance(bg) + 0.05)
		if ratio < 4.5 {
			t.Errorf("%s: brightest stop contrast %.2f against black, want >= 4.5", p.Name, ratio)
		}
	}
}

func TestGradientRisesInBrightness(t *testing.T) {
	for _, p := range All {
		lo, hi := luminance(p.At(0)), luminance(p.At(1))
		if hi <= lo {
			t.Errorf("%s: gradient does not brighten (%.4f -> %.4f)", p.Name, lo, hi)
		}
	}
}

func TestAtClamps(t *testing.T) {
	p := All[0]
	if p.At(-5) != p.Stops[0].C {
		t.Error("At below 0 should clamp to the first stop")
	}
	if p.At(5) != p.Stops[len(p.Stops)-1].C {
		t.Error("At above 1 should clamp to the last stop")
	}
}

func TestSGRPerMode(t *testing.T) {
	p := All[0]
	cases := map[Mode]string{
		TrueColor: "38;2;",
		ANSI256:   "38;5;",
	}
	for m, want := range cases {
		if got := p.SGR(m, 0.5, 0); !strings.Contains(got, want) {
			t.Errorf("%v SGR = %q, want it to contain %q", m, got, want)
		}
	}
	if got := p.SGR(Mono, 0.5, 0); strings.Contains(got, "38;") {
		t.Errorf("mono SGR = %q, should carry no colour", got)
	}
}

func TestDetectHonoursNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("COLORTERM", "truecolor")
	if Detect() != Mono {
		t.Error("NO_COLOR must win over COLORTERM")
	}
}

func TestDetectModes(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	cases := []struct {
		term, colorterm string
		want            Mode
	}{
		{"xterm-256color", "truecolor", TrueColor},
		{"xterm-256color", "", ANSI256},
		{"xterm", "", ANSI16},
		{"dumb", "", Mono},
		{"", "", Mono},
	}
	for _, c := range cases {
		t.Setenv("NO_COLOR", "")
		os.Unsetenv("NO_COLOR")
		t.Setenv("TERM", c.term)
		t.Setenv("COLORTERM", c.colorterm)
		if got := Detect(); got != c.want {
			t.Errorf("TERM=%q COLORTERM=%q: got %v, want %v", c.term, c.colorterm, got, c.want)
		}
	}
}
