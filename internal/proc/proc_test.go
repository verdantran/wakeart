package proc

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/verdantran/wakeart/internal/render"
)

func TestBuildKnownShapes(t *testing.T) {
	for _, shape := range Shapes {
		r, err := Build(Params{Kind: "wireframe", Shape: shape, Spin: SpinFrom([]float64{0.31, 0.53, 0.11}), Scale: 0.85})
		if err != nil {
			t.Fatalf("%s: %v", shape, err)
		}
		f := r.Frame(time.Second, 80, 24)
		if f.W != 80 || f.H != 24 {
			t.Errorf("%s: frame is %dx%d, want the viewport", shape, f.W, f.H)
		}
		if strings.TrimSpace(render.Plain(f)) == "" {
			t.Errorf("%s: drew nothing", shape)
		}
	}
	for _, surface := range Surfaces {
		r, err := Build(Params{Kind: "shaded", Shape: surface, Spin: SpinFrom([]float64{0.31, 0.53, 0.11}), Scale: 0.9})
		if err != nil {
			t.Fatalf("%s: %v", surface, err)
		}
		if strings.TrimSpace(render.Plain(r.Frame(time.Second, 80, 24))) == "" {
			t.Errorf("%s: drew nothing", surface)
		}
	}
}

// Every kind the build switch accepts must be listed, or a user reading the
// error message is told the wrong set.
func TestEveryKindBuilds(t *testing.T) {
	for _, kind := range Kinds {
		r, err := Build(Params{Kind: kind, Spin: SpinFrom([]float64{0.31, 0.53, 0.11}), Scale: 0.9})
		if err != nil {
			t.Errorf("%s: %v", kind, err)
			continue
		}
		if r.Describe() == "" {
			t.Errorf("%s: no description", kind)
		}
		drew := false
		for step := 0; step < 24 && !drew; step++ {
			f := r.Frame(time.Duration(step)*130*time.Millisecond, 80, 24)
			drew = strings.TrimSpace(render.Plain(f)) != ""
		}
		if !drew {
			t.Errorf("%s: drew nothing across a full cycle", kind)
		}
	}
}

func TestKnotIsOneUnbrokenStrand(t *testing.T) {
	// A (p, q) knot with coprime p and q closes into a single loop, so every
	// vertex has exactly two neighbours.
	m := Knot(3, 2, 120)
	deg := make([]int, len(m.Verts))
	for _, e := range m.Edges {
		deg[e[0]]++
		deg[e[1]]++
	}
	for i, d := range deg {
		if d != 2 {
			t.Fatalf("vertex %d has degree %d, want 2", i, d)
		}
	}
}

func TestBuildRejectsUnknown(t *testing.T) {
	if _, err := Build(Params{Kind: "wireframe", Shape: "banana"}); err == nil {
		t.Error("an unknown shape should be an error, not a blank pane")
	}
	if _, err := Build(Params{Kind: "shaded", Shape: "banana"}); err == nil {
		t.Error("an unknown surface should be an error")
	}
	if _, err := Build(Params{Kind: "hologram", Shape: "cube"}); err == nil {
		t.Error("an unknown kind should be an error")
	}
}

func TestDeterministicInTime(t *testing.T) {
	r, _ := Build(Params{Kind: "wireframe", Shape: "icosahedron", Spin: SpinFrom([]float64{0.31, 0.53, 0.11}), Scale: 0.85})
	a := render.Plain(r.Frame(1234*time.Millisecond, 60, 20))
	b := render.Plain(r.Frame(1234*time.Millisecond, 60, 20))
	if a != b {
		t.Error("the same moment should draw the same frame")
	}
	if c := render.Plain(r.Frame(1900*time.Millisecond, 60, 20)); a == c {
		t.Error("the shape should have moved by a different moment")
	}
}

func TestStaysInsideTheViewport(t *testing.T) {
	sizes := []struct{ w, h int }{{20, 10}, {80, 24}, {200, 60}, {1, 1}, {0, 0}}
	for _, shape := range Shapes {
		r, _ := Build(Params{Kind: "wireframe", Shape: shape, Spin: SpinFrom([]float64{0.31, 0.53, 0.11}), Scale: 1.0})
		for _, sz := range sizes {
			for step := 0; step < 40; step++ {
				f := r.Frame(time.Duration(step)*97*time.Millisecond, sz.w, sz.h)
				if f.W != sz.w || f.H != sz.h || len(f.Cells) != sz.w*sz.h {
					t.Fatalf("%s at %dx%d: frame is %dx%d", shape, sz.w, sz.h, f.W, f.H)
				}
			}
		}
	}
}

// The projection must not squash a cube: a terminal cell is about twice as
// tall as it is wide, and the drawn shape should come out roughly square.
func TestAspectIsCorrected(t *testing.T) {
	r := &Wireframe{Mesh: Cube(), Spin: Spin{}, Scale: 0.9}
	f := r.Frame(0, 100, 40)
	minX, maxX, minY, maxY := 1<<30, -1, 1<<30, -1
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			if f.At(x, y).Blank() {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	wCells, hCells := maxX-minX+1, maxY-minY+1
	ratio := float64(wCells) / float64(hCells) / cellAspect
	if math.Abs(ratio-1) > 0.2 {
		t.Errorf("drawn %dx%d cells: visual aspect %.2f, want near 1.0", wCells, hCells, ratio)
	}
}

func TestCullDropsTheFarSide(t *testing.T) {
	plain := &Wireframe{Mesh: Sphere(6, 12), Scale: 0.9}
	culled := &Wireframe{Mesh: Sphere(6, 12), Scale: 0.9, Cull: true}
	count := func(f *render.Frame) int {
		n := 0
		for _, c := range f.Cells {
			if !c.Blank() {
				n++
			}
		}
		return n
	}
	a, b := count(plain.Frame(0, 80, 24)), count(culled.Frame(0, 80, 24))
	if b >= a {
		t.Errorf("culled drew %d cells, uncelled %d; culling should draw less", b, a)
	}
	if b == 0 {
		t.Error("culling removed the whole globe")
	}
}

func TestNearGeometryIsBrighter(t *testing.T) {
	c := newCamera(80, 24, 0.85)
	near, far := c.depthLevel(c.dist-0.9), c.depthLevel(c.dist+0.9)
	if near <= far {
		t.Errorf("near level %d should exceed far level %d", near, far)
	}
}

func TestSlopeGlyphs(t *testing.T) {
	cases := []struct {
		dx, dy float64
		want   rune
	}{
		{10, 0, glyphHoriz},
		{0, 10, glyphVert},
		{2, 10, glyphVert},
		{10, 5, glyphDown},
		{10, -5, glyphUp},
		{-10, 5, glyphUp},
	}
	for _, c := range cases {
		if got := slopeGlyph(c.dx, c.dy); got != c.want {
			t.Errorf("slopeGlyph(%v, %v) = %q, want %q", c.dx, c.dy, got, c.want)
		}
	}
}

func TestMeshesAreNormalised(t *testing.T) {
	for _, shape := range Shapes {
		m := BuildMesh(shape)
		max := 0.0
		for _, v := range m.Verts {
			if d := math.Sqrt(v.dot(v)); d > max {
				max = d
			}
		}
		if math.Abs(max-1) > 1e-9 {
			t.Errorf("%s: furthest vertex at %.4f, want 1.0", shape, max)
		}
		if len(m.Edges) == 0 {
			t.Errorf("%s: no edges", shape)
		}
	}
}

func TestIcosahedronHasThirtyEdges(t *testing.T) {
	if got := len(Icosahedron().Edges); got != 30 {
		t.Errorf("icosahedron has %d edges, want 30", got)
	}
}

func TestRotationInverse(t *testing.T) {
	r := newRot(0.7, -1.3, 0.4)
	for _, v := range []Vec3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {0.3, -0.6, 0.8}} {
		got := r.applyInv(r.apply(v))
		if math.Abs(got.X-v.X) > 1e-12 || math.Abs(got.Y-v.Y) > 1e-12 || math.Abs(got.Z-v.Z) > 1e-12 {
			t.Errorf("applyInv(apply(%+v)) = %+v", v, got)
		}
	}
}

// Lighting the normal in object space must match lighting it in world space,
// or the highlight travels the wrong way around the shape.
func TestObjectSpaceLightingMatchesWorldSpace(t *testing.T) {
	r := newRot(0.5, 1.1, -0.3)
	light := Vec3{0, 0.7, -1}.norm()
	for _, n := range []Vec3{{1, 0, 0}, {0, 0, 1}, {0.4, 0.5, -0.75}} {
		world := r.apply(n).dot(light)
		object := n.dot(r.applyInv(light))
		if math.Abs(world-object) > 1e-12 {
			t.Errorf("normal %+v: world-space %.6f, object-space %.6f", n, world, object)
		}
	}
}

func TestSpinFrom(t *testing.T) {
	if got := SpinFrom([]float64{1, 2, 3}).Rates(DefaultSpin()); got != (Spin{1, 2, 3}) {
		t.Errorf("SpinFrom = %+v", got)
	}
	// A mesh kind still gets the default tumble when the file says nothing,
	// and a partial spin still keeps the defaults for the axes it omits.
	if got := SpinFrom(nil).Rates(DefaultSpin()); got != DefaultSpin() {
		t.Errorf("an empty spin should fall back to the default tumble, got %+v", got)
	}
	if got := SpinFrom([]float64{9}).Rates(DefaultSpin()); got.X != 9 || got.Y != DefaultSpin().Y {
		t.Errorf("a partial spin should keep the remaining defaults, got %+v", got)
	}
	// An axis the file did not name must not be mistaken for a zero it did.
	if SpinFrom([]float64{9}).Set(1) {
		t.Error("an omitted axis reports as set")
	}
	if got := SpinFrom([]float64{0, 0, 0}); !got.Set(1) || got.Axis(1, 7) != 0 {
		t.Error("an explicit zero should override the fallback")
	}
}

func BenchmarkWireframeCube(b *testing.B) {
	r, _ := Build(Params{Kind: "wireframe", Shape: "cube", Spin: SpinFrom([]float64{0.31, 0.53, 0.11}), Scale: 0.85})
	for i := 0; i < b.N; i++ {
		r.Frame(time.Duration(i)*time.Millisecond, 120, 40)
	}
}

func BenchmarkWireframeSphere(b *testing.B) {
	r, _ := Build(Params{Kind: "wireframe", Shape: "sphere", Spin: SpinFrom([]float64{0.31, 0.53, 0.11}), Scale: 0.9, Cull: true})
	for i := 0; i < b.N; i++ {
		r.Frame(time.Duration(i)*time.Millisecond, 120, 40)
	}
}

func BenchmarkShadedTorus(b *testing.B) {
	r, _ := Build(Params{Kind: "shaded", Shape: "torus", Spin: SpinFrom([]float64{0.31, 0.53, 0.11}), Scale: 0.95})
	for i := 0; i < b.N; i++ {
		r.Frame(time.Duration(i)*time.Millisecond, 120, 40)
	}
}

func TestThemedKinds(t *testing.T) {
	for _, kind := range []string{"gears", "storm", "tunnel", "scope", "rain", "terrain"} {
		r, err := Build(Params{Kind: kind, Scale: 0.9})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		drew := false
		for step := 0; step < 30; step++ {
			f := r.Frame(time.Duration(step)*80*time.Millisecond, 88, 26)
			if f.W != 88 || f.H != 26 {
				t.Fatalf("%s: frame is %dx%d", kind, f.W, f.H)
			}
			if strings.TrimSpace(render.Plain(f)) != "" {
				drew = true
			}
		}
		if !drew {
			t.Errorf("%s drew nothing across 30 frames", kind)
		}
	}
}

func TestThemedKindsUseTheirGlyphs(t *testing.T) {
	g, _ := Build(Params{Kind: "gears", Scale: 0.95})
	if !strings.ContainsRune(render.Plain(g.Frame(600*time.Millisecond, 88, 26)), '⚙') {
		t.Error("the gear train should be drawn out of gear characters")
	}
	s, _ := Build(Params{Kind: "storm"})
	found := false
	for step := 0; step < 40 && !found; step++ {
		found = strings.ContainsRune(render.Plain(s.Frame(time.Duration(step)*40*time.Millisecond, 88, 26)), '⚡')
	}
	if !found {
		t.Error("a live strike should reach the lightning-bolt glyph")
	}
}

func TestCustomGlyphsOverrideTheDefaults(t *testing.T) {
	r, err := Build(Params{Kind: "wireframe", Shape: "cube", Spin: SpinFrom([]float64{0.31, 0.53, 0.11}), Scale: 0.85,
		Glyphs: []rune("·⌁↯ϟ⚡")})
	if err != nil {
		t.Fatal(err)
	}
	out := render.Plain(r.Frame(900*time.Millisecond, 80, 24))
	for _, def := range []rune{'─', '│', '╱', '╲'} {
		if strings.ContainsRune(out, def) {
			t.Errorf("a custom glyph set should replace the slope character %q", def)
		}
	}
	if !strings.ContainsRune(out, 'ϟ') && !strings.ContainsRune(out, '⚡') {
		t.Errorf("the custom set was not used:\n%s", out)
	}
}

// Wide glyphs must not push a row past the viewport, whatever the renderer.
func TestThemedOutputRespectsCellWidth(t *testing.T) {
	for _, p := range []Params{
		{Kind: "storm"},
		{Kind: "gears", Scale: 0.9},
		{Kind: "tunnel", Spin: SpinFrom([]float64{0, 0, 0.25}), Scale: 0.98},
		{Kind: "scope", Spin: SpinFrom([]float64{3, 2, 10}), Scale: 0.88},
		{Kind: "rain", Spin: SpinFrom([]float64{0, 8})},
		{Kind: "terrain", Spin: SpinFrom([]float64{0, 0, 0.125}), Scale: 0.95},
		{Kind: "wireframe", Shape: "knot", Spin: SpinFrom([]float64{0.31, 0.53, 0.11}), Scale: 0.82},
		{Kind: "wireframe", Shape: "cube", Spin: SpinFrom([]float64{0.31, 0.53, 0.11}), Glyphs: []rune("·⚡")},
		{Kind: "shaded", Shape: "torus", Spin: SpinFrom([]float64{0.31, 0.53, 0.11}), Glyphs: []rune("·◦⚙")},
	} {
		r, err := Build(p)
		if err != nil {
			t.Fatal(err)
		}
		for step := 0; step < 25; step++ {
			f := r.Frame(time.Duration(step)*70*time.Millisecond, 61, 19)
			for _, line := range strings.Split(render.Plain(f), "\n") {
				if w := runewidth.StringWidth(line); w > f.W {
					t.Fatalf("%s/%s: line is %d cells, frame is %d", p.Kind, p.Shape, w, f.W)
				}
			}
		}
	}
}

func TestGearTrainMeshes(t *testing.T) {
	g := NewGears(Params{})
	for i := 1; i < len(g.train); i++ {
		a, b := g.train[i-1], g.train[i]
		d := math.Hypot(a.cx-b.cx, a.cy-b.cy)
		if math.Abs(d-(a.r+b.r)) > 1e-9 {
			t.Errorf("gears %d and %d are %.4f apart, want %.4f", i-1, i, d, a.r+b.r)
		}
		if a.dir == b.dir {
			t.Errorf("gears %d and %d turn the same way; meshing cogs counter-rotate", i-1, i)
		}
	}
}

func TestStormIsPureInTime(t *testing.T) {
	r, _ := Build(Params{Kind: "storm"})
	for _, ms := range []int{40, 300, 1500} {
		a := render.Plain(r.Frame(time.Duration(ms)*time.Millisecond, 70, 20))
		b := render.Plain(r.Frame(time.Duration(ms)*time.Millisecond, 70, 20))
		if a != b {
			t.Fatalf("t=%dms drew differently on a second call", ms)
		}
	}
}

func TestPickReachesBothEnds(t *testing.T) {
	set := []rune("abcde")
	if got := pick(set, 0); got != 'a' {
		t.Errorf("pick(0) = %q, want 'a'", got)
	}
	if got := pick(set, 1); got != 'e' {
		t.Errorf("pick(1) = %q, want 'e'", got)
	}
	if got := pick(set, 0.9); got != 'e' {
		t.Errorf("pick(0.9) = %q, want 'e'; the top glyph must be reachable below 1.0", got)
	}
}

func BenchmarkGears(b *testing.B) {
	r, _ := Build(Params{Kind: "gears", Scale: 0.9})
	for i := 0; i < b.N; i++ {
		r.Frame(time.Duration(i)*time.Millisecond, 120, 40)
	}
}

func BenchmarkStorm(b *testing.B) {
	r, _ := Build(Params{Kind: "storm"})
	for i := 0; i < b.N; i++ {
		r.Frame(time.Duration(i)*time.Millisecond, 120, 40)
	}
}

// Every shipped renderer must return to its exact starting frame after one
// Loop, or a captured replay visibly jumps at the seam.
func TestLoopClosesTheCircle(t *testing.T) {
	cases := []Params{
		{Kind: "wireframe", Shape: "cube", Spin: SpinFrom([]float64{0.6283185, 1.2566371, 0.6283185}), Scale: 0.8},
		{Kind: "wireframe", Shape: "icosahedron", Spin: SpinFrom([]float64{1.2566371, 0.6283185, 0.6283185}), Scale: 0.85},
		{Kind: "wireframe", Shape: "sphere", Spin: SpinFrom([]float64{0, 0.6283185, 0}), Scale: 0.9, Cull: true},
		{Kind: "shaded", Shape: "torus", Spin: SpinFrom([]float64{1.2566371, 0, 0.6283185}), Scale: 0.95},
		{Kind: "wireframe", Shape: "knot", Spin: SpinFrom([]float64{0.6283185, 1.2566371, 0}), Scale: 0.82},
		{Kind: "shaded", Shape: "sphere", Spin: SpinFrom([]float64{0, 0.6283185, 0}), Scale: 0.92},
		{Kind: "gears", Spin: SpinFrom([]float64{0, 0.7853982}), Scale: 0.92},
		{Kind: "storm"},
		{Kind: "tunnel", Spin: SpinFrom([]float64{0, 0, 0.25}), Scale: 0.98},
		{Kind: "scope", Spin: SpinFrom([]float64{3, 2, 10}), Scale: 0.88},
		{Kind: "rain", Spin: SpinFrom([]float64{0, 8})},
		{Kind: "terrain", Spin: SpinFrom([]float64{0, 0, 0.125}), Scale: 0.95},
	}
	for _, p := range cases {
		r, err := Build(p)
		if err != nil {
			t.Fatal(err)
		}
		loop := r.Loop()
		if loop <= 0 {
			t.Errorf("%s/%s: no loop period", p.Kind, p.Shape)
			continue
		}
		if loop > 20*time.Second {
			t.Errorf("%s/%s: loop is %v, too long to capture", p.Kind, p.Shape, loop)
		}
		start := render.Plain(r.Frame(0, 80, 24))
		end := render.Plain(r.Frame(loop, 80, 24))
		if start != end {
			t.Errorf("%s/%s: frame at %v differs from frame at 0", p.Kind, p.Shape, loop)
		}
		// A partial loop must actually differ, or the check above is vacuous.
		// Several probes, because a symmetric pattern can repeat at a neat
		// fraction of the period without the scene being static.
		moved := false
		for _, frac := range []float64{0.17, 0.33, 0.5, 0.71} {
			if render.Plain(r.Frame(time.Duration(float64(loop)*frac), 80, 24)) != start {
				moved = true
				break
			}
		}
		if !moved {
			t.Errorf("%s/%s: nothing moved anywhere within the loop", p.Kind, p.Shape)
		}
	}
}

func TestShippedSpinsAreCommensurate(t *testing.T) {
	// The rates in scenes/*.scene are integer multiples of a base, which is
	// what makes spinLoop resolve at all.
	for _, s := range []Spin{
		{0.6283185, 1.2566371, 0.6283185},
		{1.2566371, 0.6283185, 0.6283185},
		{0, 0.6283185, 0},
		{1.2566371, 0, 0.6283185},
	} {
		if got := spinLoop(s); got <= 0 || got > 20*time.Second {
			t.Errorf("spinLoop(%+v) = %v", s, got)
		}
	}
	if got := spinLoop(Spin{0.31, 0.53, 0.11}); got != 0 {
		t.Errorf("incommensurate rates should report no loop, got %v", got)
	}
}

func BenchmarkTunnel(b *testing.B) {
	r, _ := Build(Params{Kind: "tunnel", Spin: SpinFrom([]float64{0, 0, 0.25}), Scale: 0.98})
	for i := 0; i < b.N; i++ {
		r.Frame(time.Duration(i)*time.Millisecond, 120, 40)
	}
}

func BenchmarkScope(b *testing.B) {
	r, _ := Build(Params{Kind: "scope", Spin: SpinFrom([]float64{3, 2, 10}), Scale: 0.88})
	for i := 0; i < b.N; i++ {
		r.Frame(time.Duration(i)*time.Millisecond, 120, 40)
	}
}

func BenchmarkTerrain(b *testing.B) {
	r, _ := Build(Params{Kind: "terrain", Spin: SpinFrom([]float64{0, 0, 0.125}), Scale: 0.95})
	for i := 0; i < b.N; i++ {
		r.Frame(time.Duration(i)*time.Millisecond, 120, 40)
	}
}

func BenchmarkRain(b *testing.B) {
	r, _ := Build(Params{Kind: "rain", Spin: SpinFrom([]float64{0, 8})})
	for i := 0; i < b.N; i++ {
		r.Frame(time.Duration(i)*time.Millisecond, 120, 40)
	}
}
