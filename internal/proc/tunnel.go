package proc

import (
	"math"
	"time"

	"github.com/verdantran/wakeart/internal/render"
)

const tunnelRamp = "·-=+*#%@"

// Tunnel is a corridor rushing past: concentric rings sliding toward the
// viewer down a perspective divide, with spokes turning around them. It is the
// one scene whose motion is along Z rather than a rotation.
type Tunnel struct {
	rings  int
	rate   float64 // ring cycles per second
	turns  int     // whole spoke revolutions per ring cycle, so the loop closes
	spokes int
	scale  float64
	glyphs []rune
}

func NewTunnel(p Params) *Tunnel {
	rate := math.Abs(p.Spin.Axis(2, 0.34))
	if rate == 0 {
		rate = 0.34
	}
	return &Tunnel{rings: 22, rate: rate, turns: 1, spokes: 12, scale: p.Scale, glyphs: p.Glyphs}
}

func (t *Tunnel) Describe() string { return "tunnel" }

func (t *Tunnel) Loop() time.Duration {
	if t.rate <= 0 {
		return 0
	}
	return time.Duration(float64(time.Second) / t.rate)
}

// near is the depth at which a ring fills the viewport; rings closer than this
// have swept past the camera.
const tunnelNear = 0.085

func (t *Tunnel) Frame(d time.Duration, w, h int) *render.Frame {
	f := render.New(w, h)
	if w <= 0 || h <= 0 {
		return f
	}
	set := ramp(t.glyphs, tunnelRamp)
	scale := t.scale
	if scale <= 0 {
		scale = 0.95
	}
	unit := math.Min(float64(w)/cellAspect, float64(h)) / 2 * scale
	cx, cy := float64(w)/2, float64(h)/2
	sec := wrap(d, t.Loop()).Seconds()
	phase := sec * t.rate
	spin := 2 * math.Pi * float64(t.turns) * phase

	for k := 0; k < t.rings; k++ {
		z := math.Mod(float64(k)/float64(t.rings)+phase, 1)
		if z < 1e-6 {
			z = 1e-6
		}
		r := unit * tunnelNear / z
		if r < 1 || r > unit*7 {
			continue
		}
		// Nearer rings are brighter and, because the perspective divide
		// spreads them, need more samples to stay unbroken.
		bright := clamp(1-z, 0, 1)
		steps := int(4 * math.Pi * r * cellAspect)
		if steps < 48 {
			steps = 48
		}
		if steps > 3000 {
			steps = 3000
		}
		for i := 0; i < steps; i++ {
			a := 2*math.Pi*float64(i)/float64(steps) + spin
			plot(f, cx+math.Cos(a)*r*cellAspect, cy+math.Sin(a)*r,
				pick(set, bright), level(bright*0.92+0.08))
		}
	}

	// Spokes run the length of the corridor, tying the rings together.
	for s := 0; s < t.spokes; s++ {
		a := spin + 2*math.Pi*float64(s)/float64(t.spokes)
		ca, sa := math.Cos(a), math.Sin(a)
		for z := tunnelNear; z < 1; z += 0.006 {
			r := unit * tunnelNear / z
			if r > unit*4 {
				break
			}
			v := clamp(1-z, 0, 1) * 0.55
			plot(f, cx+ca*r*cellAspect, cy+sa*r, pick(set, v), level(v))
		}
	}
	return f
}
