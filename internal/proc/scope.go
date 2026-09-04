package proc

import (
	"math"
	"time"

	"github.com/verdantran/wakeart/internal/render"
)

const scopeRamp = "·:-=+*#@"

// Scope traces a Lissajous figure the way an oscilloscope would: a phosphor
// beam runs the curve, bright at the head and fading behind it, while the
// phase offset walks so the figure slowly folds through itself.
type Scope struct {
	a, b   int // frequency ratio; coprime values give the richest figures
	sweeps int // beam passes per phase cycle
	scale  float64
	period time.Duration
	glyphs []rune
}

func NewScope(p Params) *Scope {
	a, b := 3, 2
	if p.Spin.X > 0 {
		a = int(math.Round(p.Spin.X))
	}
	if p.Spin.Y > 0 {
		b = int(math.Round(p.Spin.Y))
	}
	if a < 1 {
		a = 1
	}
	if b < 1 {
		b = 1
	}
	period := 12 * time.Second
	if p.Spin.Z > 0 {
		period = time.Duration(p.Spin.Z * float64(time.Second))
	}
	return &Scope{a: a, b: b, sweeps: 6, scale: p.Scale, period: period, glyphs: p.Glyphs}
}

func (s *Scope) Describe() string    { return "scope" }
func (s *Scope) Loop() time.Duration { return s.period }

func (s *Scope) Frame(d time.Duration, w, h int) *render.Frame {
	f := render.New(w, h)
	if w <= 0 || h <= 0 {
		return f
	}
	set := ramp(s.glyphs, scopeRamp)
	scale := s.scale
	if scale <= 0 {
		scale = 0.9
	}
	ux := float64(w) / 2 * scale
	uy := float64(h) / 2 * scale
	cx, cy := float64(w)/2, float64(h)/2

	t := wrap(d, s.Loop()).Seconds() / s.period.Seconds() // 0..1
	delta := 2 * math.Pi * t
	head := math.Mod(t*float64(s.sweeps), 1) // where the beam is on the curve

	// Sample finely enough that the curve never breaks into dashes. The beam
	// moves fastest where the curve is steepest, so this is deliberately
	// generous rather than matched to the cell count.
	steps := 24 * (w + h)
	for i := 0; i < steps; i++ {
		u := float64(i) / float64(steps)
		th := 2 * math.Pi * u
		x := math.Sin(float64(s.a)*th + delta)
		y := math.Sin(float64(s.b) * th)

		// Distance behind the beam, wrapping around the closed curve.
		lag := head - u
		if lag < 0 {
			lag++
		}
		v := math.Pow(1-lag, 7) // a short, sharp phosphor tail
		if v < 0.12 {
			v = 0.12 // the settled trace stays faintly lit
		}
		plot(f, cx+x*ux, cy-y*uy, pick(set, v), level(v))
	}
	return f
}
