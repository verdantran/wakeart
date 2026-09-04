package proc

import (
	"math"
	"time"

	"github.com/verdantran/wakeart/internal/render"
)

// gearRamp runs spoke, rim, tooth: the mechanism is literally drawn out of
// gear characters.
const gearRamp = "·◦⚙"

// gearSpec describes a cog relative to the one before it. Chaining them means
// the pitch circles are tangent by construction rather than by arithmetic I
// have to keep correct by hand.
type gearSpec struct {
	r     float64
	teeth int
	angle float64 // bearing from the previous gear's centre, radians
}

type gear struct {
	cx, cy float64
	r      float64
	teeth  int
	dir    float64 // meshing gears must turn opposite ways
	phase  float64
}

// Gears draws a train of meshing cogs. Angular speed is inversely proportional
// to radius and neighbours counter-rotate, so the mechanism reads as connected
// rather than as three unrelated spinning circles.
type Gears struct {
	train  []gear
	spin   float64
	scale  float64
	glyphs []rune
	span   float64 // half-extent of the train, for fitting to the viewport
}

// Radii in a 1 : 1/2 : 1/4 ratio with teeth to match keep the module (teeth
// per unit radius) constant, which is how real gears mesh — and it means every
// cog completes a whole number of turns over the largest one's revolution, so
// the train has an exact period.
var defaultTrain = []gearSpec{
	{r: 1.00, teeth: 20},
	{r: 0.50, teeth: 10, angle: -0.34},
	{r: 0.25, teeth: 5, angle: 0.78},
}

func NewGears(p Params) *Gears {
	speed := p.Spin.Y
	if speed == 0 {
		speed = 0.5
	}
	g := &Gears{spin: speed, scale: p.Scale, glyphs: p.Glyphs}
	g.layout(defaultTrain)
	return g
}

// layout walks the chain, placing each cog exactly one pitch-radius sum from
// its neighbour, then centres the result on its own bounding box.
func (g *Gears) layout(specs []gearSpec) {
	var x, y, dir float64
	dir = 1
	for i, s := range specs {
		if i > 0 {
			d := specs[i-1].r + s.r
			x += d * math.Cos(s.angle)
			y += d * math.Sin(s.angle)
			dir = -dir
		}
		g.train = append(g.train, gear{
			cx: x, cy: y, r: s.r, teeth: s.teeth, dir: dir,
			phase: math.Pi / float64(s.teeth),
		})
	}
	minX, maxX := math.MaxFloat64, -math.MaxFloat64
	minY, maxY := math.MaxFloat64, -math.MaxFloat64
	for _, c := range g.train {
		minX, maxX = math.Min(minX, c.cx-c.r), math.Max(maxX, c.cx+c.r)
		minY, maxY = math.Min(minY, c.cy-c.r), math.Max(maxY, c.cy+c.r)
	}
	ox, oy := (minX+maxX)/2, (minY+maxY)/2
	for i := range g.train {
		g.train[i].cx -= ox
		g.train[i].cy -= oy
	}
	g.span = math.Max((maxX-minX)/2/cellAspect, (maxY-minY)/2)
}

func (g *Gears) Describe() string { return "gears" }

// Loop is one revolution of the largest cog; the smaller ones turn at exactly
// two and four times that rate, so they land back where they started too.
func (g *Gears) Loop() time.Duration {
	if len(g.train) == 0 || g.spin == 0 {
		return 0
	}
	biggest := 0.0
	for _, c := range g.train {
		if c.r > biggest {
			biggest = c.r
		}
	}
	return time.Duration(2 * math.Pi * biggest / g.spin * float64(time.Second))
}

func (g *Gears) Frame(t time.Duration, w, h int) *render.Frame {
	f := render.New(w, h)
	if w <= 0 || h <= 0 || g.span <= 0 {
		return f
	}
	set := ramp(g.glyphs, gearRamp)
	scale := g.scale
	if scale <= 0 {
		scale = 0.92
	}
	// Fit the whole train to the tighter axis, teeth included.
	unit := math.Min(float64(w)/cellAspect, float64(h)) / 2 * scale / (g.span * 1.18)
	cx, cy := float64(w)/2, float64(h)/2
	sec := wrap(t, g.Loop()).Seconds()

	for _, c := range g.train {
		theta := c.phase + c.dir*g.spin*sec/c.r // a small cog spins faster
		drawGear(f, set, cx+c.cx*unit*cellAspect, cy+c.cy*unit, c.r*unit, c.teeth, theta)
	}
	return f
}

const toothAmp = 0.3

// drawGear lays down a continuous root circle and then spikes the teeth out
// from it. Modulating the rim radius instead leaves the root arc broken
// wherever a tooth steps outward, which at terminal resolution just reads as a
// dashed circle.
func drawGear(f *render.Frame, set []rune, cx, cy, r float64, teeth int, theta float64) {
	if r < 2 {
		return
	}
	// Sample against the stretched perimeter: the circle is drawn as an
	// ellipse twice as wide as it is tall, so sampling by radius alone leaves
	// gaps along the horizontal arcs.
	steps := int(4 * math.Pi * r * cellAspect)
	if steps < 64 {
		steps = 64
	}
	ring := func(radius, intensity float64, n int) {
		for i := 0; i < n; i++ {
			a := 2 * math.Pi * float64(i) / float64(n)
			plot(f, cx+math.Cos(a)*radius*cellAspect, cy+math.Sin(a)*radius,
				pick(set, intensity), level(intensity))
		}
	}
	ring(r, 0.5, steps)

	// Each tooth is a short arc swept from the rim outward, so it has width as
	// well as height and does not vanish into a single cell.
	tip := r * (1 + toothAmp)
	half := math.Pi / float64(teeth) * 0.34
	for k := 0; k < teeth; k++ {
		centre := theta + 2*math.Pi*float64(k)/float64(teeth)
		for a := centre - half; a <= centre+half; a += 0.5 / (r * cellAspect) {
			ca, sa := math.Cos(a), math.Sin(a)
			for d := r; d <= tip; d += 0.45 {
				plot(f, cx+ca*d*cellAspect, cy+sa*d, pick(set, 1), level(1))
			}
		}
	}

	// Hub, plus four spokes that turn with the gear so the rotation stays
	// legible once the teeth blur together at small sizes.
	hub := r * 0.26
	ring(hub, 0.5, steps/3)
	for k := 0; k < 4; k++ {
		a := theta + math.Pi*float64(k)/2
		ca, sa := math.Cos(a), math.Sin(a)
		for d := hub + 0.5; d < r*0.92; d += 0.5 {
			plot(f, cx+ca*d*cellAspect, cy+sa*d, pick(set, 0), level(0.22))
		}
	}
}

// plot writes one glyph at rounded cell coordinates, ignoring anything outside
// the viewport.
func plot(f *render.Frame, x, y float64, r rune, lvl uint8) {
	ix, iy := int(x+0.5), int(y+0.5)
	if ix < 0 || iy < 0 || ix >= f.W || iy >= f.H {
		return
	}
	f.SetRune(ix, iy, render.Cell{R: r, Lvl: lvl})
}
