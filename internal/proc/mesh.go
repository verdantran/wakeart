// Package proc renders procedural scenes: geometry generated per frame rather
// than read from a file. Output is an ordinary render.Frame, so palettes,
// effects and transitions apply to it unchanged.
package proc

import "math"

type Vec3 struct{ X, Y, Z float64 }

func (v Vec3) add(o Vec3) Vec3      { return Vec3{v.X + o.X, v.Y + o.Y, v.Z + o.Z} }
func (v Vec3) scale(k float64) Vec3 { return Vec3{v.X * k, v.Y * k, v.Z * k} }
func (v Vec3) dot(o Vec3) float64   { return v.X*o.X + v.Y*o.Y + v.Z*o.Z }

func (v Vec3) norm() Vec3 {
	l := math.Sqrt(v.dot(v))
	if l == 0 {
		return v
	}
	return v.scale(1 / l)
}

// rot is a rotation with its trig taken once. Rotating tens of thousands of
// surface samples per frame makes recomputing sin/cos per point the single
// most expensive thing in the renderer.
type rot struct{ sx, cx, sy, cy, sz, cz float64 }

func newRot(ax, ay, az float64) rot {
	return rot{
		sx: math.Sin(ax), cx: math.Cos(ax),
		sy: math.Sin(ay), cy: math.Cos(ay),
		sz: math.Sin(az), cz: math.Cos(az),
	}
}

// apply rotates around X, then Y, then Z.
func (r rot) apply(v Vec3) Vec3 {
	y, z := v.Y*r.cx-v.Z*r.sx, v.Y*r.sx+v.Z*r.cx
	x, z2 := v.X*r.cy+z*r.sy, -v.X*r.sy+z*r.cy
	return Vec3{x*r.cz - y*r.sz, x*r.sz + y*r.cz, z2}
}

// applyInv undoes apply. The shaded renderer uses it to bring the light into
// object space, so surface normals never need rotating.
func (r rot) applyInv(v Vec3) Vec3 {
	x2 := v.X*r.cz + v.Y*r.sz
	y1 := -v.X*r.sz + v.Y*r.cz
	x := x2*r.cy - v.Z*r.sy
	z1 := x2*r.sy + v.Z*r.cy
	return Vec3{x, y1*r.cx + z1*r.sx, -y1*r.sx + z1*r.cx}
}

// rotate is the convenience form, for callers outside a hot loop.
func (v Vec3) rotate(ax, ay, az float64) Vec3 { return newRot(ax, ay, az).apply(v) }

// Mesh is a unit-radius wireframe. Surfaces that can be shaded also carry a
// parametric sampler; see surface.
type Mesh struct {
	Verts []Vec3
	Edges [][2]int
}

func (m *Mesh) edge(a, b int) { m.Edges = append(m.Edges, [2]int{a, b}) }

// normalise scales the mesh so its furthest vertex sits at radius 1, which
// lets every shape share one projection scale.
func (m *Mesh) normalise() *Mesh {
	max := 0.0
	for _, v := range m.Verts {
		if d := math.Sqrt(v.dot(v)); d > max {
			max = d
		}
	}
	if max > 0 {
		for i := range m.Verts {
			m.Verts[i] = m.Verts[i].scale(1 / max)
		}
	}
	return m
}

func Cube() *Mesh {
	m := &Mesh{Verts: []Vec3{
		{-1, -1, -1}, {1, -1, -1}, {1, 1, -1}, {-1, 1, -1},
		{-1, -1, 1}, {1, -1, 1}, {1, 1, 1}, {-1, 1, 1},
	}}
	for i := 0; i < 4; i++ {
		m.edge(i, (i+1)%4)
		m.edge(i+4, (i+1)%4+4)
		m.edge(i, i+4)
	}
	return m.normalise()
}

func Tetrahedron() *Mesh {
	m := &Mesh{Verts: []Vec3{{1, 1, 1}, {1, -1, -1}, {-1, 1, -1}, {-1, -1, 1}}}
	for i := 0; i < 4; i++ {
		for j := i + 1; j < 4; j++ {
			m.edge(i, j)
		}
	}
	return m.normalise()
}

func Octahedron() *Mesh {
	m := &Mesh{Verts: []Vec3{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}}
	for _, a := range []int{0, 1} {
		for _, b := range []int{2, 3} {
			for _, c := range []int{4, 5} {
				m.edge(a, b)
				m.edge(b, c)
				m.edge(a, c)
			}
		}
	}
	return m.normalise()
}

func Icosahedron() *Mesh {
	p := (1 + math.Sqrt(5)) / 2
	m := &Mesh{Verts: []Vec3{
		{0, 1, p}, {0, -1, p}, {0, 1, -p}, {0, -1, -p},
		{1, p, 0}, {-1, p, 0}, {1, -p, 0}, {-1, -p, 0},
		{p, 0, 1}, {-p, 0, 1}, {p, 0, -1}, {-p, 0, -1},
	}}
	m.normalise()
	// Every pair at the minimum separation is an edge; the icosahedron is
	// regular, so this recovers exactly the 30 of them.
	min := math.MaxFloat64
	for i := range m.Verts {
		for j := i + 1; j < len(m.Verts); j++ {
			if d := dist(m.Verts[i], m.Verts[j]); d < min {
				min = d
			}
		}
	}
	for i := range m.Verts {
		for j := i + 1; j < len(m.Verts); j++ {
			if dist(m.Verts[i], m.Verts[j]) < min*1.05 {
				m.edge(i, j)
			}
		}
	}
	return m
}

func dist(a, b Vec3) float64 {
	d := Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z}
	return math.Sqrt(d.dot(d))
}

// Torus builds a wireframe ring; rings and segments control how dense it is.
func Torus(rings, segments int, inner float64) *Mesh {
	m := &Mesh{}
	idx := func(i, j int) int { return (i%rings)*segments + j%segments }
	for i := 0; i < rings; i++ {
		u := 2 * math.Pi * float64(i) / float64(rings)
		for j := 0; j < segments; j++ {
			v := 2 * math.Pi * float64(j) / float64(segments)
			r := 1 + inner*math.Cos(v)
			m.Verts = append(m.Verts, Vec3{r * math.Cos(u), r * math.Sin(u), inner * math.Sin(v)})
		}
	}
	for i := 0; i < rings; i++ {
		for j := 0; j < segments; j++ {
			m.edge(idx(i, j), idx(i, j+1))
			m.edge(idx(i, j), idx(i+1, j))
		}
	}
	return m.normalise()
}

// Sphere is a lat/long globe: parallels and meridians, as a wire globe reads
// better in ASCII than a triangulated ball.
func Sphere(parallels, meridians int) *Mesh {
	m := &Mesh{}
	idx := func(i, j int) int { return i*meridians + j%meridians }
	for i := 0; i <= parallels; i++ {
		lat := math.Pi*float64(i)/float64(parallels) - math.Pi/2
		for j := 0; j < meridians; j++ {
			lon := 2 * math.Pi * float64(j) / float64(meridians)
			m.Verts = append(m.Verts, Vec3{
				math.Cos(lat) * math.Cos(lon),
				math.Sin(lat),
				math.Cos(lat) * math.Sin(lon),
			})
		}
	}
	for i := 0; i <= parallels; i++ {
		for j := 0; j < meridians; j++ {
			m.edge(idx(i, j), idx(i, j+1))
			if i < parallels {
				m.edge(idx(i, j), idx(i+1, j))
			}
		}
	}
	return m.normalise()
}

// Shapes lists the mesh names a scene file may ask for.
var Shapes = []string{"cube", "tetrahedron", "octahedron", "icosahedron", "torus", "sphere", "diamond", "knot"}

func BuildMesh(name string) *Mesh {
	switch name {
	case "cube":
		return Cube()
	case "tetrahedron":
		return Tetrahedron()
	case "octahedron":
		return Octahedron()
	case "icosahedron":
		return Icosahedron()
	case "torus":
		return Torus(20, 12, 0.42)
	case "sphere":
		return Sphere(6, 12)
	case "diamond":
		return diamond()
	case "knot":
		return Knot(3, 2, 220)
	}
	return nil
}

// Knot traces a (p, q) torus knot: a closed curve winding p times around the
// ring while looping q times through its hole. Coprime p and q give a single
// unbroken strand.
func Knot(p, q, samples int) *Mesh {
	m := &Mesh{}
	for i := 0; i < samples; i++ {
		th := 2 * math.Pi * float64(i) / float64(samples)
		r := 2 + math.Cos(float64(q)*th)
		m.Verts = append(m.Verts, Vec3{
			r * math.Cos(float64(p)*th),
			r * math.Sin(float64(p)*th),
			math.Sin(float64(q) * th),
		})
		m.edge(i, (i+1)%samples)
	}
	return m.normalise()
}

// diamond is two cones base to base: a brilliant-cut silhouette.
func diamond() *Mesh {
	const n = 10
	m := &Mesh{Verts: []Vec3{{0, 1.4, 0}, {0, -1.6, 0}}}
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / n
		m.Verts = append(m.Verts, Vec3{math.Cos(a), 0.35, math.Sin(a)})
	}
	for i := 0; i < n; i++ {
		cur, next := 2+i, 2+(i+1)%n
		m.edge(cur, next)
		m.edge(0, cur)
		m.edge(1, cur)
	}
	return m.normalise()
}
