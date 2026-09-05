package proc

import (
	"math"
	"time"

	"github.com/verdantran/wakeart/internal/render"
)

// rainGlyphs are the characters the cascade is built from. Half-width katakana
// keeps every cell one column wide.
var rainGlyphs = []rune("ｱｲｳｴｵｶｷｸｹｺｻｼｽｾｿﾀﾁﾂﾃﾄﾅﾆﾇﾈﾉ0123456789:.=*+<>|")

// Rain is the glyph cascade. Each column falls a whole number of times over
// the loop, and the glyph in a cell is a pure function of its position and
// which fall it belongs to, so the whole thing repeats exactly.
type Rain struct {
	period time.Duration
	glyphs []rune
}

func NewRain(p Params) *Rain {
	period := 8 * time.Second
	if d := time.Duration(p.Spin.Axis(1, 0) * float64(time.Second)); d > 0 {
		period = d
	}
	return &Rain{period: period, glyphs: p.Glyphs}
}

func (r *Rain) Describe() string    { return "rain" }
func (r *Rain) Loop() time.Duration { return r.period }

func (r *Rain) Frame(d time.Duration, w, h int) *render.Frame {
	f := render.New(w, h)
	if w <= 0 || h <= 0 {
		return f
	}
	custom := len(r.glyphs) > 0
	t := wrap(d, r.Loop()).Seconds() / r.period.Seconds() // 0..1

	for x := 0; x < w; x++ {
		hash := splitmix(uint64(x)*0x9E3779B97F4A7C15 + 0x243F6A88)
		if hash%5 == 0 {
			continue // gaps keep it from reading as a solid curtain
		}
		falls := 1 + int(hash>>8)%3 // whole falls per loop, so the column repeats
		tail := 5 + int(hash>>16)%14
		travel := float64(h + tail)

		pos := math.Mod(t*float64(falls), 1)
		fall := int(t * float64(falls))
		head := pos*travel - float64(tail)

		for k := 0; k < tail; k++ {
			y := int(head) - k
			if y < 0 || y >= h {
				continue
			}
			// Brightest at the head, fading down the tail.
			v := 1 - float64(k)/float64(tail)
			v = v * v
			var g rune
			if custom {
				g = pick(r.glyphs, v)
			} else {
				n := splitmix(uint64(x)*0x2545F491 + uint64(y)*0x9E3779B9 + uint64(fall)*0x85EBCA6B)
				g = rainGlyphs[n%uint64(len(rainGlyphs))]
			}
			f.SetRune(x, y, render.Cell{R: g, Lvl: level(v)})
		}
	}
	return f
}

// splitmix is a cheap integer hash; the cascade needs a stable value per cell,
// not a random stream.
func splitmix(z uint64) uint64 {
	z += 0x9E3779B97F4A7C15
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}
