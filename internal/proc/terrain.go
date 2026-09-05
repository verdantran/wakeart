package proc

import (
	"math"
	"time"

	"github.com/verdantran/wakeart/internal/render"
)

const terrainRamp = "·-=≡#"

// Terrain is a wireframe landscape sliding toward the viewer: rows of a
// heightfield on a perspective divide. The height function is periodic in the
// scroll coordinate, so a row that wraps from near to far lands on exactly the
// profile the row ahead of it had.
type Terrain struct {
	rows   int
	rate   float64 // scroll cycles per second
	scale  float64
	glyphs []rune
}

func NewTerrain(p Params) *Terrain {
	rate := math.Abs(p.Spin.Axis(2, 0.12))
	if rate == 0 {
		rate = 0.12
	}
	return &Terrain{rows: 20, rate: rate, scale: p.Scale, glyphs: p.Glyphs}
}

func (t *Terrain) Describe() string { return "terrain" }

func (t *Terrain) Loop() time.Duration {
	if t.rate <= 0 {
		return 0
	}
	return time.Duration(float64(time.Second) / t.rate)
}

// height is periodic in z with period 1, which is what lets rows wrap without
// a seam.
func height(x, z float64) float64 {
	return 0.30*math.Sin(3.1*x)*math.Cos(2*math.Pi*z) +
		0.14*math.Sin(2*math.Pi*2*z+1.7) +
		0.11*math.Sin(6.3*x+2*math.Pi*z)
}

func (t *Terrain) Frame(d time.Duration, w, h int) *render.Frame {
	f := render.New(w, h)
	if w <= 0 || h <= 0 {
		return f
	}
	set := ramp(t.glyphs, terrainRamp)
	scale := t.scale
	if scale <= 0 {
		scale = 0.95
	}
	unit := float64(h) * scale
	cx := float64(w) / 2
	horizon := float64(h) * 0.32
	phase := wrap(d, t.Loop()).Seconds() * t.rate

	// Hidden-line removal, front to back: a far ridge is drawn only where it
	// rises above everything nearer. Without this the rows cross into noise.
	skyline := make([]float64, w)
	for i := range skyline {
		skyline[i] = math.MaxFloat64
	}
	for j := 0; j < t.rows; j++ {
		zw := math.Mod(float64(j)/float64(t.rows)+phase, 1)
		if zw < 0 {
			zw++
		}
		depth := 0.40 + zw*1.35
		drawRidge(f, set, skyline, zw, depth, cx, horizon, unit, w)
	}
	return f
}

func drawRidge(f *render.Frame, set []rune, skyline []float64, zw, depth, cx, horizon, unit float64, w int) {
	bright := clamp(1.5-depth, 0.16, 1)
	steps := w * 3
	var px, py float64
	put := func(x, y float64) {
		ix := int(x + 0.5)
		if ix < 0 || ix >= w || y >= skyline[ix] {
			return
		}
		skyline[ix] = y
		plot(f, x, y, pick(set, bright), level(bright))
	}
	for i := 0; i <= steps; i++ {
		xw := -1.5 + 3.0*float64(i)/float64(steps)
		y := horizon + (0.5-height(xw, zw))*unit/(depth*2.6)
		x := cx + xw*unit*cellAspect/(depth*2.6)
		if i > 0 {
			// Join consecutive samples so a steep ridge stays a line rather
			// than a row of dots.
			dx, dy := x-px, y-py
			n := math.Max(math.Abs(dx), math.Abs(dy))
			if n > 1 && n < 400 {
				for k := 1.0; k < n; k++ {
					put(px+dx*k/n, py+dy*k/n)
				}
			}
		}
		put(x, y)
		px, py = x, y
	}
}
