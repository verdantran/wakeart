package render

import "math/rand"

type Transition string

const (
	Cut       Transition = "cut"
	Dissolve  Transition = "dissolve"
	ScanWipe  Transition = "scanwipe"
	GlitchCut Transition = "glitchcut"
)

var AllTransitions = []Transition{Cut, Dissolve, ScanWipe, GlitchCut}

func ValidTransition(s string) bool {
	for _, t := range AllTransitions {
		if string(t) == s {
			return true
		}
	}
	return false
}

// Duration is how long the transition runs. Cut is instant.
func (t Transition) Frames(fps float64) int {
	switch t {
	case Dissolve:
		return int(0.25 * fps)
	case ScanWipe:
		return int(0.35 * fps)
	case GlitchCut:
		return int(0.12 * fps)
	}
	return 0
}

var glitchRunes = []rune("▓▒░█▄▀╳#%@*")

// Blend composes two fitted frames at progress p in [0,1]. The two are read
// against a shared geometry, so a mismatch — the status bar appearing mid
// transition, say — ends the blend rather than indexing off the end of one.
func Blend(from, to *Frame, t Transition, p float64, rng *rand.Rand) *Frame {
	if from == nil || p >= 1 || t == Cut {
		return to
	}
	if to == nil || from.W != to.W || from.H != to.H {
		return to
	}
	if p <= 0 {
		return from
	}
	out := to.Clone()
	switch t {
	case Dissolve:
		// Deterministic per-cell threshold so cells settle instead of flickering.
		for i := range out.Cells {
			if hash(i)%1000 >= uint32(p*1000) {
				out.Cells[i] = from.Cells[i]
			}
		}
	case ScanWipe:
		edge := int(p * float64(out.H))
		for y := 0; y < out.H; y++ {
			switch {
			case y > edge:
				copy(out.Cells[y*out.W:(y+1)*out.W], from.Cells[y*from.W:(y+1)*from.W])
			case y == edge:
				for x := 0; x < out.W; x++ {
					out.Set(x, y, Cell{R: '─', Lvl: 255})
				}
			}
		}
	case GlitchCut:
		src := from
		if p > 0.5 {
			src = to
		}
		copy(out.Cells, src.Cells)
		for y := 0; y < out.H; y++ {
			if rng.Intn(3) > 0 {
				continue
			}
			shift := rng.Intn(9) - 4
			shiftRow(out, y, shift)
			for k := 0; k < out.W/8; k++ {
				x := rng.Intn(out.W)
				if c := out.Ptr(x, y); c != nil && !c.Blank() {
					c.R = glitchRunes[rng.Intn(len(glitchRunes))]
				}
			}
		}
	}
	return out
}

func shiftRow(f *Frame, y, by int) {
	if by == 0 {
		return
	}
	row := make([]Cell, f.W)
	copy(row, f.Cells[y*f.W:(y+1)*f.W])
	for x := 0; x < f.W; x++ {
		sx := x - by
		if sx < 0 || sx >= f.W {
			f.Cells[y*f.W+x] = Cell{}
			continue
		}
		f.Cells[y*f.W+x] = row[sx]
	}
}

func hash(i int) uint32 {
	x := uint32(i)*2654435761 + 0x9e3779b9
	x ^= x >> 15
	x *= 0x85ebca6b
	x ^= x >> 13
	return x
}
