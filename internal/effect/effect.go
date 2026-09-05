// Package effect applies the CRT and glitch post-processing that gives the
// carousel its look. Effects run on a clone of the composed frame, and with a
// fixed seed a run replays identically, which is what makes them testable.
package effect

import (
	"math/rand"
	"strings"

	"github.com/verdantran/wakeart/internal/palette"
	"github.com/verdantran/wakeart/internal/render"
)

type Intensity int

const (
	Off Intensity = iota
	Subtle
	Heavy
)

func ParseIntensity(s string) Intensity {
	switch strings.ToLower(s) {
	case "off", "none":
		return Off
	case "heavy":
		return Heavy
	}
	return Subtle
}

func (i Intensity) String() string {
	switch i {
	case Off:
		return "off"
	case Heavy:
		return "heavy"
	}
	return "subtle"
}

func (i Intensity) Next() Intensity { return (i + 1) % 3 }

var Names = []string{"scanlines", "flicker", "glitch", "chroma"}

func Valid(name string) bool {
	for _, n := range Names {
		if n == name {
			return true
		}
	}
	return false
}

type Set struct {
	Names     []string
	Intensity Intensity
}

func (s Set) Has(name string) bool {
	if s.Intensity == Off {
		return false
	}
	for _, n := range s.Names {
		if n == name {
			return true
		}
	}
	return false
}

// State carries the animation the effects need between ticks.
type State struct {
	rng        *rand.Rand
	flicker    float64
	glitchLeft int
	glitchSeed int64
}

func NewState(seed int64) *State {
	return &State{rng: rand.New(rand.NewSource(seed))}
}

// Tick advances the schedules. Glitch bursts arrive on a Poisson-ish schedule,
// roughly one every twelve seconds at subtle.
func (st *State) Tick(s Set, fps float64) {
	if s.Intensity == Off {
		return
	}
	if fps <= 0 {
		fps = 10
	}
	st.flicker += (st.rng.Float64() - 0.5) * 0.35
	st.flicker = clamp(st.flicker, -1, 1)

	if st.glitchLeft > 0 {
		st.glitchLeft--
		return
	}
	if !s.Has("glitch") {
		return
	}
	mean := 12.0
	if s.Intensity == Heavy {
		mean = 3.5
	}
	if st.rng.Float64() < 1/(mean*fps) {
		st.glitchLeft = 2 + st.rng.Intn(3)
		st.glitchSeed = st.rng.Int63()
	}
}

func (st *State) Glitching() bool { return st.glitchLeft > 0 }

// Apply mutates f in place. Callers pass a clone; the cached composition must
// survive untouched.
func (s Set) Apply(f *render.Frame, st *State) {
	if s.Intensity == Off || f == nil || f.W == 0 || f.H == 0 {
		return
	}
	if s.Has("scanlines") {
		scanlines(f, s.Intensity)
	}
	if s.Has("flicker") {
		flicker(f, s.Intensity, st.flicker)
	}
	if s.Has("glitch") && st.glitchLeft > 0 {
		glitch(f, rand.New(rand.NewSource(st.glitchSeed+int64(st.glitchLeft))), s.Intensity)
	}
	if s.Has("chroma") {
		chroma(f, s.Intensity)
	}
}

func scanlines(f *render.Frame, in Intensity) {
	d := uint8(38)
	if in == Heavy {
		d = 76
	}
	for y := 1; y < f.H; y += 2 {
		for x := 0; x < f.W; x++ {
			c := &f.Cells[y*f.W+x]
			c.Dim = addDim(c.Dim, d)
		}
	}
}

func flicker(f *render.Frame, in Intensity, walk float64) {
	amp := 8.0
	if in == Heavy {
		amp = 26.0
	}
	d := walk * amp
	if d <= 0 {
		return
	}
	v := uint8(d)
	for i := range f.Cells {
		f.Cells[i].Dim = addDim(f.Cells[i].Dim, v)
	}
}

var corrupt = []rune("▓▒░█▄▀╳")

func glitch(f *render.Frame, rng *rand.Rand, in Intensity) {
	rows, maxShift := 2+rng.Intn(3), 3
	if in == Heavy {
		rows, maxShift = 4+rng.Intn(4), 7
	}
	for i := 0; i < rows; i++ {
		y := rng.Intn(f.H)
		shift := rng.Intn(2*maxShift+1) - maxShift
		shiftRow(f, y, shift)
		n := f.W / 10
		for k := 0; k < n; k++ {
			x := rng.Intn(f.W)
			c := f.Ptr(x, y)
			if c != nil && !c.Blank() {
				c.R = corrupt[rng.Intn(len(corrupt))]
				c.Lvl = render.Weight(c.R)
			}
		}
	}
}

func shiftRow(f *render.Frame, y, by int) {
	if by == 0 || y < 0 || y >= f.H {
		return
	}
	row := make([]render.Cell, f.W)
	copy(row, f.Cells[y*f.W:(y+1)*f.W])
	for x := 0; x < f.W; x++ {
		sx := x - by
		if sx < 0 || sx >= f.W {
			f.Cells[y*f.W+x] = render.Cell{}
			continue
		}
		f.Cells[y*f.W+x] = row[sx]
	}
}

var (
	ghostL = palette.RGB{R: 255, G: 40, B: 200}
	ghostR = palette.RGB{R: 40, G: 230, B: 255}
)

// chroma fakes an RGB split by ghosting bright glyphs into the blank cell on
// either side. Cheap to compute, and the most cyberpunk thing in the package.
func chroma(f *render.Frame, in Intensity) {
	threshold, dim := uint8(190), uint8(140)
	if in == Heavy {
		threshold, dim = 150, 90
	}
	type write struct {
		i int
		c render.Cell
	}
	var writes []write
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			c := f.Cells[y*f.W+x]
			if c.Blank() || c.Lvl < threshold || c.Raw != "" {
				continue
			}
			if x > 0 && f.Cells[y*f.W+x-1].Blank() {
				writes = append(writes, write{y*f.W + x - 1,
					render.Cell{R: c.R, Fix: ghostL, HasFix: true, Dim: dim}})
			}
			if x+1 < f.W && f.Cells[y*f.W+x+1].Blank() {
				writes = append(writes, write{y*f.W + x + 1,
					render.Cell{R: c.R, Fix: ghostR, HasFix: true, Dim: dim}})
			}
		}
	}
	for _, w := range writes {
		f.Cells[w.i] = w.c
	}
}

func addDim(a, b uint8) uint8 {
	v := int(a) + int(b)
	if v > 235 {
		return 235
	}
	return uint8(v)
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
