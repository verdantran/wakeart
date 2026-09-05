package proc

import (
	"fmt"
	"math"
	"strings"
)

// Kinds are the procedural renderers a scene file may ask for.
var Kinds = []string{"wireframe", "shaded", "gears", "storm", "tunnel", "scope", "rain", "terrain"}

// Params is a scene's procedural frontmatter, already typed.
type Params struct {
	Kind   string
	Shape  string
	Spin   *SpinSpec // nil when the file gave none; each kind has its own default
	Scale  float64
	Cull   bool
	Glyphs []rune // ramp from dim to bright; nil keeps the renderer's own
}

// maxScale bounds the frontmatter's scale. It is documented as a fraction of
// the viewport, so anything past a few multiples of it is a typo — and the
// generators size their loops from it, so a large one costs seconds per frame.
const maxScale = 4

// Build turns scene frontmatter into a renderer. Unknown names are an error so
// wakeart doctor can report a typo rather than showing a blank pane.
func Build(p Params) (Renderer, error) {
	if err := p.check(); err != nil {
		return nil, err
	}
	shape := strings.ToLower(strings.TrimSpace(p.Shape))
	switch strings.ToLower(strings.TrimSpace(p.Kind)) {
	case "wireframe", "wire":
		if shape == "" {
			shape = "cube"
		}
		m := BuildMesh(shape)
		if m == nil {
			return nil, fmt.Errorf("unknown shape %q (have: %s)", shape, strings.Join(Shapes, ", "))
		}
		return &Wireframe{Mesh: m, Spin: p.Spin.Rates(DefaultSpin()), Scale: p.Scale, Cull: p.Cull, Glyphs: p.Glyphs, name: shape}, nil

	case "shaded", "solid":
		if shape == "" {
			shape = "torus"
		}
		s := buildSurface(shape)
		if s == nil {
			return nil, fmt.Errorf("unknown shaded surface %q (have: %s)", shape, strings.Join(Surfaces, ", "))
		}
		return &Shaded{Surface: s, Spin: p.Spin.Rates(DefaultSpin()), Scale: p.Scale, Glyphs: p.Glyphs, name: shape}, nil

	case "gears":
		return NewGears(p), nil

	case "storm", "lightning":
		return NewStorm(p), nil

	case "tunnel":
		return NewTunnel(p), nil

	case "scope", "lissajous":
		return NewScope(p), nil

	case "rain", "cascade":
		return NewRain(p), nil

	case "terrain", "ridge":
		return NewTerrain(p), nil
	}
	return nil, fmt.Errorf("unknown kind %q (have: %s)", p.Kind, strings.Join(Kinds, ", "))
}

// check rejects frontmatter the generators cannot survive. They do arithmetic
// on these numbers and index glyph ramps with the result, so a non-finite value
// or a degenerate ramp becomes a panic several call layers down.
func (p *Params) check() error {
	if !isFinite(p.Scale) {
		return fmt.Errorf("scale must be a finite number, got %v", p.Scale)
	}
	if p.Scale < 0 || p.Scale > maxScale {
		return fmt.Errorf("scale %v is out of range (0 to %d)", p.Scale, maxScale)
	}
	for i := 0; i < 3; i++ {
		if !p.Spin.Set(i) {
			continue
		}
		v := p.Spin.Axis(i, 0)
		if !isFinite(v) {
			return fmt.Errorf("spin[%d] must be a finite number, got %v", i, v)
		}
		if math.Abs(v) > maxSpin {
			return fmt.Errorf("spin[%d] %v is out of range (magnitude at most %d)", i, v, maxSpin)
		}
	}
	// A ramp is interpolated across len-1 steps, so one glyph divides by zero.
	if len(p.Glyphs) == 1 {
		return fmt.Errorf("glyphs needs at least two characters to form a ramp, got %q", string(p.Glyphs))
	}
	return nil
}

// maxSpin keeps a rate from truncating a derived period to zero nanoseconds,
// which the generators then divide by.
const maxSpin = 1000

func isFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// DefaultSpin gives a shape a slow tumble on all three axes when the file does
// not say otherwise. The rates are deliberately incommensurate so the motion
// never settles into an obvious loop.
func DefaultSpin() Spin { return Spin{X: 0.31, Y: 0.53, Z: 0.11} }

// SpinSpec records which axes the file actually named. SPEC.md gives spin a
// different meaning per kind — rotation rates for the mesh kinds, a frequency
// ratio and phase period for scope, a fall period for rain — so an axis the
// file left out has to fall back to the kind's own default, not a shared one.
type SpinSpec struct {
	v   [3]float64
	set [3]bool
}

// Axis reads one named component, or fallback when the file omitted it.
func (s *SpinSpec) Axis(i int, fallback float64) float64 {
	if s == nil || !s.set[i] {
		return fallback
	}
	return s.v[i]
}

// Rates is the mesh reading: three rotation rates, each defaulting separately
// so `spin = [0.5]` means "faster about x, leave the rest alone".
func (s *SpinSpec) Rates(fallback Spin) Spin {
	return Spin{
		X: s.Axis(0, fallback.X),
		Y: s.Axis(1, fallback.Y),
		Z: s.Axis(2, fallback.Z),
	}
}

// Set reports whether the file named this axis at all.
func (s *SpinSpec) Set(i int) bool { return s != nil && s.set[i] }

func SpinFrom(v []float64) *SpinSpec {
	if len(v) == 0 {
		return nil
	}
	s := &SpinSpec{}
	for i := 0; i < 3 && i < len(v); i++ {
		s.v[i], s.set[i] = v[i], true
	}
	return s
}

// ramp picks a glyph set: the scene's own if it gave one, else the default.
func ramp(custom []rune, fallback string) []rune {
	if len(custom) > 0 {
		return custom
	}
	return []rune(fallback)
}

// pick indexes a ramp by a 0..1 intensity. It rounds rather than truncates:
// truncating makes the brightest glyph reachable only at exactly 1.0, so the
// top of every themed set would go unused.
func pick(r []rune, v float64) rune {
	if len(r) == 0 {
		return '*'
	}
	i := int(clamp(v, 0, 1)*float64(len(r)-1) + 0.5)
	if i >= len(r) {
		i = len(r) - 1
	}
	return r[i]
}

// level maps a 0..1 intensity onto the palette gradient, keeping the dimmest
// glyphs off the very bottom of the ramp where they would vanish.
func level(v float64) uint8 { return uint8(60 + clamp(v, 0, 1)*195) }
