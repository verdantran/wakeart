package proc

import (
	"math"
	"math/rand"
	"time"

	"github.com/verdantran/wakeart/internal/render"
)

// stormRamp fades from a spent trace to a live bolt. The brightest glyph is
// the lightning bolt itself, so a fresh strike is drawn in ⚡.
const stormRamp = "·⌁↯ϟ⚡"

const (
	strikePeriod = 900 * time.Millisecond
	strikeLife   = 1150 * time.Millisecond
	overlap      = 4 // strikes that can be on screen at once
	seedRing     = 4 // distinct bolts before the sequence repeats
)

// Storm draws branching lightning. A bolt's geometry is a pure function of its
// index, so the whole animation is reproducible from the time alone — no
// hidden state between frames.
type Storm struct {
	glyphs []rune
}

func NewStorm(p Params) *Storm { return &Storm{glyphs: p.Glyphs} }

func (s *Storm) Describe() string { return "storm" }

// Loop covers the full ring of strike seeds, so the sequence repeats exactly.
func (s *Storm) Loop() time.Duration { return seedRing * strikePeriod }

func (s *Storm) Frame(t time.Duration, w, h int) *render.Frame {
	f := render.New(w, h)
	if w <= 0 || h <= 0 {
		return f
	}
	set := ramp(s.glyphs, stormRamp)

	t = wrap(t, s.Loop())
	current := int(t / strikePeriod)
	// k runs negative near the start of a loop, so a strike still fading as
	// the loop wraps is present at the beginning too and the seam matches.
	for k := current; k > current-overlap; k-- {
		age := t - time.Duration(k)*strikePeriod
		if age < 0 || age >= strikeLife {
			continue
		}
		// A strike flares, then decays; the leading edge is the brightest.
		p := float64(age) / float64(strikeLife)
		intensity := math.Pow(1-p, 1.6)
		if p < 0.08 {
			intensity = 1
		}
		// Seeds cycle, so the strike sequence repeats on Loop rather than
		// wandering off forever.
		s.drawBolt(f, set, int64(((k%seedRing)+seedRing)%seedRing), intensity)
	}
	return f
}

func (s *Storm) drawBolt(f *render.Frame, set []rune, seed int64, intensity float64) {
	rng := rand.New(rand.NewSource(seed*2654435761 + 17))
	w, h := float64(f.W), float64(f.H)

	startX := w * (0.12 + rng.Float64()*0.76)
	endX := clamp(startX+(rng.Float64()-0.5)*w*0.45, 2, w-3)

	path := jagged(rng, startX, 0, endX, h-1, 5, w*0.16)
	s.strokePath(f, set, path, intensity, rng, true)

	// Branches peel off the main channel and die out part way down.
	branches := 2 + rng.Intn(3)
	for b := 0; b < branches; b++ {
		i := len(path) / 4 * (1 + rng.Intn(3))
		if i >= len(path) {
			continue
		}
		from := path[i]
		dropped := (h - from.y) * (0.25 + rng.Float64()*0.4)
		to := point{clamp(from.x+(rng.Float64()-0.5)*w*0.32, 1, w-2), from.y + dropped}
		s.strokePath(f, set, jagged(rng, from.x, from.y, to.x, to.y, 3, w*0.07), intensity*0.62, rng, false)
	}
}

type point struct{ x, y float64 }

// jagged subdivides a segment, displacing each midpoint sideways. Halving the
// displacement per level is what gives lightning its self-similar kink.
func jagged(rng *rand.Rand, x0, y0, x1, y1 float64, depth int, spread float64) []point {
	pts := []point{{x0, y0}, {x1, y1}}
	for d := 0; d < depth; d++ {
		next := make([]point, 0, len(pts)*2-1)
		for i := 0; i < len(pts)-1; i++ {
			a, b := pts[i], pts[i+1]
			mid := point{(a.x + b.x) / 2, (a.y + b.y) / 2}
			mid.x += (rng.Float64() - 0.5) * spread
			next = append(next, a, mid)
		}
		next = append(next, pts[len(pts)-1])
		pts = next
		spread /= 2
	}
	return pts
}

// strokePath walks a bolt. The main channel sits at the top of the ramp so a
// live strike is drawn in the brightest glyph; branches stay a step below.
func (s *Storm) strokePath(f *render.Frame, set []rune, pts []point, intensity float64, rng *rand.Rand, core bool) {
	if intensity <= 0.02 {
		return
	}
	lo, hi := 0.42, 0.70
	if core {
		lo, hi = 0.74, 1.0
	}
	for i := 0; i < len(pts)-1; i++ {
		a, b := pts[i], pts[i+1]
		dx, dy := b.x-a.x, b.y-a.y
		steps := math.Max(math.Abs(dx), math.Abs(dy))
		if steps < 1 {
			steps = 1
		}
		for k := 0.0; k <= steps; k++ {
			p := k / steps
			// Flicker along the channel so the bolt is not a uniform stripe.
			v := clamp(intensity*(lo+rng.Float64()*(hi-lo)), 0, 1)
			plot(f, a.x+dx*p, a.y+dy*p, pick(set, v), level(v))
		}
	}
}
