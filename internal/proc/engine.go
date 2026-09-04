package proc

import (
	"math"
	"time"

	"github.com/verdantran/wakeart/internal/render"
)

// Renderer produces a viewport-sized frame for a moment in time.
type Renderer interface {
	Frame(t time.Duration, w, h int) *render.Frame
	Describe() string
	// Loop is the period after which the animation exactly repeats, or 0 when
	// it never does. Nothing in the running app needs it — it plays forever —
	// but a capture has to know where to cut.
	Loop() time.Duration
}

// spinLoop is the shortest span in which every spinning axis completes a whole
// number of turns: 2π over the greatest common divisor of the rates. Scene
// spin rates are chosen as integer multiples of a base so this resolves; rates
// that share no common divisor return 0 rather than a near-miss, because a
// near-miss is exactly what makes a captured loop jump.
func spinLoop(s Spin) time.Duration {
	var g float64
	for _, r := range []float64{math.Abs(s.X), math.Abs(s.Y), math.Abs(s.Z)} {
		if r < 1e-9 {
			continue
		}
		g = fgcd(g, r)
	}
	if g < 1e-6 {
		return 0
	}
	t := 2 * math.Pi / g
	if t > 60 {
		return 0
	}
	// Snap to the millisecond, then confirm every axis really does land on a
	// whole turn at that instant.
	t = math.Round(t*1000) / 1000
	for _, r := range []float64{math.Abs(s.X), math.Abs(s.Y), math.Abs(s.Z)} {
		if r < 1e-9 {
			continue
		}
		turns := r * t / (2 * math.Pi)
		if math.Abs(turns-math.Round(turns)) > 1e-4 || math.Round(turns) < 1 {
			return 0
		}
	}
	return time.Duration(t * float64(time.Second))
}

// wrap folds elapsed time into one loop period. A renderer that declares a
// period should honour it exactly rather than trust the arithmetic to land on
// it — a rate written to seven decimals drifts just enough to flip a glyph at
// a slope threshold. It also keeps sin and cos away from the large arguments a
// pane left open all day would otherwise reach.
func wrap(t, loop time.Duration) time.Duration {
	if loop <= 0 {
		return t
	}
	t %= loop
	if t < 0 {
		t += loop
	}
	return t
}

// fgcd is Euclid on floats, stopping at a tolerance rather than at zero.
func fgcd(a, b float64) float64 {
	a, b = math.Abs(a), math.Abs(b)
	for b > 1e-6 {
		a, b = b, math.Mod(a, b)
	}
	return a
}

// Terminal cells are about twice as tall as they are wide, so horizontal
// distances are doubled or every shape comes out squashed.
const cellAspect = 2.0

type Spin struct{ X, Y, Z float64 }

func (s Spin) at(t time.Duration) (float64, float64, float64) {
	sec := t.Seconds()
	return s.X * sec, s.Y * sec, s.Z * sec
}

// camera converts a rotated vertex to cell coordinates plus depth.
type camera struct {
	cx, cy, unit, dist float64
}

func newCamera(w, h int, scale float64) camera {
	if scale <= 0 {
		scale = 0.85
	}
	// The shape must fit the tighter of the two axes once the cell aspect is
	// taken into account.
	unit := math.Min(float64(w)/cellAspect, float64(h)) / 2 * scale
	return camera{cx: float64(w) / 2, cy: float64(h) / 2, unit: unit, dist: 3.2}
}

func (c camera) project(v Vec3) (x, y, depth float64) {
	z := v.Z + c.dist
	if z < 0.2 {
		z = 0.2
	}
	k := c.dist / z
	return c.cx + v.X*k*c.unit*cellAspect, c.cy - v.Y*k*c.unit, z
}

// depthLevel maps distance from the camera onto the palette gradient, so
// nearer geometry reads brighter.
func (c camera) depthLevel(z float64) uint8 {
	near, far := c.dist-1, c.dist+1
	t := 1 - (z-near)/(far-near)
	return uint8(clamp(t, 0, 1)*165) + 90
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Wireframe draws a mesh's edges, choosing a glyph per segment from its slope.
type Wireframe struct {
	Mesh  *Mesh
	Spin  Spin
	Scale float64
	// Cull drops edges on the far side of the shape. It reads much better on a
	// globe, and it is wrong for anything non-convex, so it is opt-in.
	Cull bool
	// Glyphs replaces the slope characters with a depth-indexed ramp, which is
	// how a cube gets drawn out of gears or lightning bolts.
	Glyphs []rune
	name   string
}

func (r *Wireframe) Describe() string    { return "wireframe " + r.name }
func (r *Wireframe) Loop() time.Duration { return spinLoop(r.Spin) }

func (r *Wireframe) Frame(t time.Duration, w, h int) *render.Frame {
	f := render.New(w, h)
	if r.Mesh == nil || w <= 0 || h <= 0 {
		return f
	}
	ax, ay, az := r.Spin.at(wrap(t, r.Loop()))
	rt := newRot(ax, ay, az)
	cam := newCamera(w, h, r.Scale)

	type pt struct{ x, y, z float64 }
	pts := make([]pt, len(r.Mesh.Verts))
	rotated := make([]Vec3, len(r.Mesh.Verts))
	for i, v := range r.Mesh.Verts {
		rotated[i] = rt.apply(v)
		x, y, z := cam.project(rotated[i])
		pts[i] = pt{x, y, z}
	}

	// A z-buffer keeps near edges on top of far ones without sorting.
	zbuf := make([]float64, w*h)
	for i := range zbuf {
		zbuf[i] = math.MaxFloat64
	}
	for _, e := range r.Mesh.Edges {
		if r.Cull && (rotated[e[0]].Z+rotated[e[1]].Z)/2 > 0 {
			continue // midpoint on the far hemisphere
		}
		a, b := pts[e[0]], pts[e[1]]
		drawLine(f, zbuf, a.x, a.y, a.z, b.x, b.y, b.z, cam, r.Glyphs)
	}
	return f
}

var (
	glyphHoriz = '─'
	glyphVert  = '│'
	glyphDown  = '╲'
	glyphUp    = '╱'
)

// slopeGlyph picks a line character. The thresholds are in cell space, where a
// visually diagonal line has |dy/dx| near 0.5.
func slopeGlyph(dx, dy float64) rune {
	adx, ady := math.Abs(dx), math.Abs(dy)
	if adx < 1e-9 {
		return glyphVert
	}
	slope := ady / adx
	switch {
	case slope < 0.28:
		return glyphHoriz
	case slope > 1.8:
		return glyphVert
	case dx*dy > 0:
		return glyphDown
	}
	return glyphUp
}

// drawLine is Bresenham in float, interpolating depth along the segment.
func drawLine(f *render.Frame, zbuf []float64, x0, y0, z0, x1, y1, z1 float64, cam camera, glyphs []rune) {
	dx, dy := x1-x0, y1-y0
	steps := math.Max(math.Abs(dx), math.Abs(dy))
	if steps < 1 {
		steps = 1
	}
	if steps > 4096 {
		return // a vertex behind the camera; skip rather than spin
	}
	g := slopeGlyph(dx, dy)
	for i := 0.0; i <= steps; i++ {
		p := i / steps
		x, y := int(math.Round(x0+dx*p)), int(math.Round(y0+dy*p))
		if x < 0 || y < 0 || x >= f.W || y >= f.H {
			continue
		}
		z := z0 + (z1-z0)*p
		idx := y*f.W + x
		if z >= zbuf[idx] {
			continue
		}
		zbuf[idx] = z
		lvl := cam.depthLevel(z)
		r := g
		if len(glyphs) > 0 {
			// Depth chooses the glyph, so nearer edges read as the heavier
			// end of the themed set.
			r = pick(glyphs, float64(lvl-90)/165)
		}
		zbuf[idx] = z
		f.SetRune(x, y, render.Cell{R: r, Lvl: lvl})
	}
}

// shadeRamp runs dark to bright; the index is also the ink weight, so the
// palette gradient and the glyph density agree.
var shadeRamp = []rune(".,-~:;=!*%#$@")

// Shaded renders a lit parametric surface, the way donut.c does: sample the
// surface densely, keep the nearest sample per cell, and pick a glyph from the
// dot product of the surface normal with the light.
type Shaded struct {
	Surface Surface
	Spin    Spin
	Scale   float64
	Light   Vec3
	Glyphs  []rune
	name    string
}

// Surface builds a point and its outward normal from the trig of its two
// parameters. Taking trig rather than angles lets the renderer precompute one
// table per axis instead of calling sin and cos per sample.
type Surface interface {
	At(cu, su, cv, sv float64) (pos, normal Vec3)
	// Albedo scales the lit value by a pattern fixed to the surface. Without
	// one a rotating sphere is a still image: it is symmetric, so turning it
	// changes nothing you can see.
	Albedo(cu, su, cv, sv float64) float64
	// Density is samples per cell along each axis; the renderer scales the
	// count to the viewport so a small pane costs less.
	Density() (du, dv float64)
}

func (r *Shaded) Describe() string    { return "shaded " + r.name }
func (r *Shaded) Loop() time.Duration { return spinLoop(r.Spin) }

func (r *Shaded) Frame(t time.Duration, w, h int) *render.Frame {
	f := render.New(w, h)
	if r.Surface == nil || w <= 0 || h <= 0 {
		return f
	}
	ax, ay, az := r.Spin.at(wrap(t, r.Loop()))
	rt := newRot(ax, ay, az)
	cam := newCamera(w, h, r.Scale)
	light := r.Light
	if light == (Vec3{}) {
		light = Vec3{0, 0.7, -1}
	}
	light = rt.applyInv(light.norm())

	// Resolved once: building the ramp per sample allocated on every lit cell.
	set := ramp(r.Glyphs, string(shadeRamp))
	top := float64(len(set) - 1)

	du, dv := r.Surface.Density()
	nu := clampInt(int(du*float64(w)), 24, 320)
	nv := clampInt(int(dv*float64(h)), 16, 200)

	cosU, sinU := trigTable(nu)
	cosV, sinV := trigTable(nv)

	zbuf := make([]float64, w*h)
	for i := range zbuf {
		zbuf[i] = math.MaxFloat64
	}

	for i := 0; i < nu; i++ {
		cu, su := cosU[i], sinU[i]
		for j := 0; j < nv; j++ {
			pos, nrm := r.Surface.At(cu, su, cosV[j], sinV[j])

			// The light was brought into object space up front, so the normal
			// needs no rotating to be lit — only the position, to be drawn.
			lum := nrm.dot(light) * r.Surface.Albedo(cu, su, cosV[j], sinV[j])
			if lum <= 0 {
				continue // facing away from the light
			}
			x, y, z := cam.project(rt.apply(pos))
			ix, iy := int(x+0.5), int(y+0.5)
			if ix < 0 || iy < 0 || ix >= w || iy >= h {
				continue
			}
			idx := iy*w + ix
			if z >= zbuf[idx] {
				continue
			}
			zbuf[idx] = z
			k := int(clamp(lum, 0, 1) * top)
			f.SetRune(ix, iy, render.Cell{
				R:   set[k],
				Lvl: uint8(40 + k*215/int(top)),
			})
		}
	}
	return f
}

// trigTable holds cos and sin for n evenly spaced angles around a full turn.
func trigTable(n int) (cos, sin []float64) {
	cos, sin = make([]float64, n), make([]float64, n)
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		cos[i], sin[i] = math.Cos(a), math.Sin(a)
	}
	return cos, sin
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// TorusSurface is the classic donut.
type TorusSurface struct{ Inner float64 }

func (s TorusSurface) Density() (float64, float64) { return 1.9, 2.6 }

func (s TorusSurface) Albedo(cu, su, cv, sv float64) float64 { return 1 }

func (s TorusSurface) At(cu, su, cv, sv float64) (Vec3, Vec3) {
	r := s.Inner
	if r <= 0 {
		r = 0.45
	}
	const ring = 1.0
	radial := ring + r*cv
	pos := Vec3{radial * cu, radial * su, r * sv}
	return pos.scale(1 / (ring + r)), Vec3{cv * cu, cv * su, sv}
}

// SphereSurface is a lit ball.
type SphereSurface struct{}

func (s SphereSurface) Density() (float64, float64) { return 1.8, 2.4 }

// Albedo bands the sphere by latitude and varies it by longitude, so the
// rotation is actually visible. The longitude term mixes the first and third
// harmonics: a pure cos(3·lon) is three-fold symmetric, which would make the
// planet look like it turned three times per revolution. cos(3·lon) comes from
// the Chebyshev identity rather than an Atan2 call, which would run twelve
// thousand times a frame.
func (s SphereSurface) Albedo(cu, su, cv, sv float64) float64 {
	bands := 0.68 + 0.32*math.Sin(sv*8.5)
	lon1 := cu
	lon3 := 4*cu*cu*cu - 3*cu
	return clamp(bands*(0.80+0.13*lon1+0.07*lon3), 0.22, 1)
}

// At walks v over a full turn but only the half that yields a real latitude is
// distinct; the duplicate half costs nothing and keeps the trig table shared.
func (s SphereSurface) At(cu, su, cv, sv float64) (Vec3, Vec3) {
	n := Vec3{cv * cu, sv, cv * su}
	return n, n
}

// Surfaces lists the shadeable surface names a scene file may ask for.
var Surfaces = []string{"torus", "sphere"}

func buildSurface(name string) Surface {
	switch name {
	case "torus", "donut":
		return TorusSurface{Inner: 0.45}
	case "sphere", "ball":
		return SphereSurface{}
	}
	return nil
}
