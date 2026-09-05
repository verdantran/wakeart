package proc

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// Frontmatter is untrusted: it comes from any .scene file on disk. The
// generators do arithmetic on these numbers and index glyph ramps with the
// result, so Build has to be the place bad values stop.
func TestBuildRejectsHostileParams(t *testing.T) {
	cases := map[string]Params{
		"single glyph ramp": {Kind: "shaded", Shape: "torus", Glyphs: []rune("@")},
		"nan scale":         {Kind: "shaded", Shape: "torus", Scale: math.NaN()},
		"inf scale":         {Kind: "wireframe", Shape: "cube", Scale: math.Inf(1)},
		"negative scale":    {Kind: "gears", Scale: -1},
		"huge scale":        {Kind: "gears", Scale: 1e9},
		"nan spin":          {Kind: "tunnel", Spin: SpinFrom([]float64{0, 0, math.NaN()})},
		"inf spin":          {Kind: "scope", Spin: SpinFrom([]float64{math.Inf(-1)})},
		"huge spin":         {Kind: "rain", Spin: SpinFrom([]float64{0, 1e12})},
	}
	for name, p := range cases {
		if _, err := Build(p); err == nil {
			t.Errorf("%s: Build accepted %+v", name, p)
		}
	}
}

func TestBuildAcceptsOrdinaryParams(t *testing.T) {
	for _, kind := range Kinds {
		p := Params{Kind: kind, Scale: 0.9, Spin: SpinFrom([]float64{0.6, 1.2, 0.3})}
		r, err := Build(p)
		if errors.Is(err, ErrKindUnavailable) {
			continue // a kind this machine cannot run is not a failure
		}
		if err != nil {
			t.Errorf("%s: %v", kind, err)
			continue
		}
		r.Frame(1700*time.Millisecond, 80, 24)
	}
}

// A rate small enough to truncate its derived period to zero nanoseconds used
// to leave Frame dividing by it.
func TestDerivedPeriodsAreNeverZero(t *testing.T) {
	for _, kind := range []string{"rain", "scope", "tunnel", "terrain"} {
		r, err := Build(Params{Kind: kind, Spin: SpinFrom([]float64{1e-12, 1e-12, 1e-12})})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if d := r.Loop(); d <= 0 {
			t.Errorf("%s: loop period is %v", kind, d)
		}
		r.Frame(time.Second, 80, 24)
	}
}

// Every kind must survive a viewport the TUI could hand it.
func TestFrameSurvivesOddViewports(t *testing.T) {
	for _, kind := range Kinds {
		r, err := Build(Params{Kind: kind, Scale: 0.9})
		if errors.Is(err, ErrKindUnavailable) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, wh := range [][2]int{{0, 0}, {1, 1}, {0, 24}, {80, 0}, {1, 200}, {200, 1}} {
			f := r.Frame(1700*time.Millisecond, wh[0], wh[1])
			if f == nil {
				t.Errorf("%s at %dx%d: nil frame", kind, wh[0], wh[1])
			}
		}
	}
}

func TestGlyphRampErrorNamesTheField(t *testing.T) {
	_, err := Build(Params{Kind: "shaded", Shape: "torus", Glyphs: []rune("@")})
	if err == nil || !strings.Contains(err.Error(), "glyphs") {
		t.Errorf("error should name the field, got %v", err)
	}
}

// SPEC.md gives spin a different meaning per kind, so a file that omits it
// must get that kind's own default rather than the mesh tumble.
func loopFor(rate float64) time.Duration {
	return time.Duration(float64(time.Second) / rate)
}

func TestOmittedSpinUsesEachKindsOwnDefault(t *testing.T) {
	want := map[string]time.Duration{
		"rain":    8 * time.Second,
		"scope":   12 * time.Second,
		"tunnel":  loopFor(0.34),
		"terrain": loopFor(0.12),
	}
	for kind, d := range want {
		r, err := Build(Params{Kind: kind})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if got := r.Loop(); got != d {
			t.Errorf("%s with no spin loops in %v, want %v", kind, got, d)
		}
	}
}

// An explicit spin still wins, on every kind.
func TestExplicitSpinOverridesTheDefault(t *testing.T) {
	r, err := Build(Params{Kind: "rain", Spin: SpinFrom([]float64{0, 3})})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Loop(); got != 3*time.Second {
		t.Errorf("rain loop = %v, want 3s", got)
	}
}
