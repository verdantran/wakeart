package proc

import (
	"fmt"
	"strings"
)

// Kinds are the procedural renderers a scene file may ask for.
var Kinds = []string{"wireframe", "shaded", "gears", "storm", "tunnel", "scope", "rain", "terrain"}

// Params is a scene's procedural frontmatter, already typed.
type Params struct {
	Kind   string
	Shape  string
	Spin   Spin
	Scale  float64
	Cull   bool
	Glyphs []rune // ramp from dim to bright; nil keeps the renderer's own
}

// Build turns scene frontmatter into a renderer. Unknown names are an error so
// wakeart doctor can report a typo rather than showing a blank pane.
func Build(p Params) (Renderer, error) {
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
		return &Wireframe{Mesh: m, Spin: p.Spin, Scale: p.Scale, Cull: p.Cull, Glyphs: p.Glyphs, name: shape}, nil

	case "shaded", "solid":
		if shape == "" {
			shape = "torus"
		}
		s := buildSurface(shape)
		if s == nil {
			return nil, fmt.Errorf("unknown shaded surface %q (have: %s)", shape, strings.Join(Surfaces, ", "))
		}
		return &Shaded{Surface: s, Spin: p.Spin, Scale: p.Scale, Glyphs: p.Glyphs, name: shape}, nil

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

// DefaultSpin gives a shape a slow tumble on all three axes when the file does
// not say otherwise. The rates are deliberately incommensurate so the motion
// never settles into an obvious loop.
func DefaultSpin() Spin { return Spin{X: 0.31, Y: 0.53, Z: 0.11} }

func SpinFrom(v []float64) Spin {
	s := DefaultSpin()
	if len(v) > 0 {
		s.X = v[0]
	}
	if len(v) > 1 {
		s.Y = v[1]
	}
	if len(v) > 2 {
		s.Z = v[2]
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
