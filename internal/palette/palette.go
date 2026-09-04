// Package palette holds the colour gradients and the terminal capability
// degradation that keeps them legible on lesser terminals.
package palette

import (
	"fmt"
	"os"
	"strings"
)

type RGB struct{ R, G, B uint8 }

// Stop is one gradient anchor with hand-picked approximations. Automatic
// quantisation of neon gradients looks like mud, so the fallbacks are chosen
// rather than computed.
type Stop struct {
	C       RGB
	ANSI256 int
	ANSI16  int
}

type Palette struct {
	Name  string
	Desc  string
	Stops []Stop
}

type Mode int

const (
	TrueColor Mode = iota
	ANSI256
	ANSI16
	Mono
)

func (m Mode) String() string {
	switch m {
	case TrueColor:
		return "truecolor"
	case ANSI256:
		return "256"
	case ANSI16:
		return "16"
	}
	return "mono"
}

const Reset = "\x1b[0m"

func hex(s string) RGB {
	var r, g, b uint8
	fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
	return RGB{r, g, b}
}

func st(h string, c256, c16 int) Stop { return Stop{hex(h), c256, c16} }

var All = []Palette{
	{"neon", "magenta -> violet -> cyan", []Stop{
		st("2a0a3f", 53, 5), st("7b1fa2", 90, 5), st("c026d3", 164, 13),
		st("e879f9", 213, 13), st("a5b4fc", 147, 12), st("67e8f9", 87, 14),
	}},
	{"synthwave", "deep purple -> hot pink -> sunset orange", []Stop{
		st("1a0b2e", 233, 4), st("4c1d95", 55, 5), st("9d174d", 125, 1),
		st("ec4899", 205, 13), st("f97316", 208, 3), st("fbbf24", 214, 11),
	}},
	{"acid", "black-green -> lime -> white-yellow", []Stop{
		st("021a05", 232, 0), st("14532d", 22, 2), st("16a34a", 34, 2),
		st("4ade80", 84, 10), st("a3e635", 155, 10), st("f7fee7", 230, 15),
	}},
	{"bladerunner", "midnight blue -> amber -> smoke", []Stop{
		st("0a1128", 234, 4), st("1b3a6b", 24, 4), st("b45309", 130, 3),
		st("f59e0b", 214, 3), st("fcd34d", 221, 11), st("d6d3d1", 252, 7),
	}},
	{"ice", "navy -> cyan -> white", []Stop{
		st("041a2f", 233, 4), st("0e4a6e", 24, 4), st("0891b2", 31, 6),
		st("22d3ee", 45, 14), st("a5f3fc", 159, 14), st("ffffff", 15, 15),
	}},
	{"bloodmoon", "maroon -> red -> orange", []Stop{
		st("1a0505", 232, 0), st("7f1d1d", 52, 1), st("dc2626", 160, 1),
		st("f87171", 210, 9), st("fb923c", 215, 11), st("fed7aa", 223, 7),
	}},
	{"mono", "terminal foreground only", []Stop{
		st("404040", 240, 0), st("808080", 244, 8), st("c0c0c0", 250, 7),
		st("e0e0e0", 253, 7), st("f0f0f0", 255, 15), st("ffffff", 15, 15),
	}},
}

func Get(name string) (Palette, bool) {
	for _, p := range All {
		if p.Name == name {
			return p, true
		}
	}
	return Palette{}, false
}

func Names() []string {
	n := make([]string, len(All))
	for i, p := range All {
		n[i] = p.Name
	}
	return n
}

// At samples the gradient. t is clamped to [0,1].
func (p Palette) At(t float64) RGB {
	if len(p.Stops) == 0 {
		return RGB{255, 255, 255}
	}
	if t <= 0 {
		return p.Stops[0].C
	}
	if t >= 1 {
		return p.Stops[len(p.Stops)-1].C
	}
	x := t * float64(len(p.Stops)-1)
	i := int(x)
	f := x - float64(i)
	a, b := p.Stops[i].C, p.Stops[i+1].C
	lerp := func(u, v uint8) uint8 { return uint8(float64(u) + (float64(v)-float64(u))*f) }
	return RGB{lerp(a.R, b.R), lerp(a.G, b.G), lerp(a.B, b.B)}
}

// stopAt returns the nearest hand-picked stop, used by the reduced modes.
func (p Palette) stopAt(t float64) Stop {
	if len(p.Stops) == 0 {
		return Stop{RGB{255, 255, 255}, 15, 7}
	}
	i := int(t*float64(len(p.Stops)-1) + 0.5)
	if i < 0 {
		i = 0
	}
	if i >= len(p.Stops) {
		i = len(p.Stops) - 1
	}
	return p.Stops[i]
}

func scale(c RGB, dim uint8) RGB {
	if dim == 0 {
		return c
	}
	k := float64(255-dim) / 255
	return RGB{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k)}
}

// Writer is the subset of strings.Builder that WriteSGR needs.
type Writer interface {
	WriteString(string) (int, error)
	WriteByte(byte) error
}

func writeUint(w Writer, v int) {
	if v >= 100 {
		w.WriteByte(byte('0' + v/100))
	}
	if v >= 10 {
		w.WriteByte(byte('0' + v/10%10))
	}
	w.WriteByte(byte('0' + v%10))
}

// WriteSGR appends the escape sequence for a gradient position directly to w.
// Paint calls this once per colour run, so it does no allocation.
func (p Palette) WriteSGR(w Writer, m Mode, t float64, dim uint8) {
	switch m {
	case TrueColor:
		c := scale(p.At(t), dim)
		w.WriteString("\x1b[38;2;")
		writeUint(w, int(c.R))
		w.WriteByte(';')
		writeUint(w, int(c.G))
		w.WriteByte(';')
		writeUint(w, int(c.B))
		w.WriteByte('m')
	case ANSI256:
		s := p.stopAt(t)
		if dim > 128 {
			w.WriteString("\x1b[2;38;5;")
		} else {
			w.WriteString("\x1b[38;5;")
		}
		writeUint(w, s.ANSI256)
		w.WriteByte('m')
	case ANSI16:
		s := p.stopAt(t)
		base := 30 + s.ANSI16
		if s.ANSI16 >= 8 {
			base = 90 + s.ANSI16 - 8
		}
		if dim > 128 {
			w.WriteString("\x1b[2;")
		} else {
			w.WriteString("\x1b[")
		}
		writeUint(w, base)
		w.WriteByte('m')
	default:
		switch {
		case dim > 128 || t < 0.34:
			w.WriteString("\x1b[2m")
		case t > 0.72:
			w.WriteString("\x1b[1m")
		default:
			w.WriteString("\x1b[22m")
		}
	}
}

// WriteSGRFixed emits an explicit colour, used by the chroma-split effect.
func WriteSGRFixed(w Writer, m Mode, c RGB) {
	switch m {
	case TrueColor:
		w.WriteString("\x1b[38;2;")
		writeUint(w, int(c.R))
		w.WriteByte(';')
		writeUint(w, int(c.G))
		w.WriteByte(';')
		writeUint(w, int(c.B))
		w.WriteByte('m')
	case ANSI256:
		w.WriteString("\x1b[38;5;")
		writeUint(w, cube256(c))
		w.WriteByte('m')
	case ANSI16:
		w.WriteString("\x1b[")
		writeUint(w, 90+nearest16(c))
		w.WriteByte('m')
	default:
		w.WriteString("\x1b[2m")
	}
}

// SGR is the allocating form, kept for callers outside the render loop.
func (p Palette) SGR(m Mode, t float64, dim uint8) string {
	var b strings.Builder
	p.WriteSGR(&b, m, t, dim)
	return b.String()
}

func SGRFixed(m Mode, c RGB) string {
	var b strings.Builder
	WriteSGRFixed(&b, m, c)
	return b.String()
}

func cube256(c RGB) int {
	q := func(v uint8) int { return int(v) * 5 / 255 }
	return 16 + 36*q(c.R) + 6*q(c.G) + q(c.B)
}

func nearest16(c RGB) int {
	i := 0
	if c.R > 100 {
		i |= 1
	}
	if c.G > 100 {
		i |= 2
	}
	if c.B > 100 {
		i |= 4
	}
	return i
}

// Detect reads the terminal's colour capability. NO_COLOR wins over everything.
func Detect() Mode {
	if os.Getenv("NO_COLOR") != "" {
		return Mono
	}
	term := os.Getenv("TERM")
	if term == "" || term == "dumb" {
		return Mono
	}
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return TrueColor
	}
	if strings.Contains(term, "256color") {
		return ANSI256
	}
	if strings.Contains(term, "kitty") || strings.Contains(term, "alacritty") || strings.Contains(term, "ghostty") {
		return TrueColor
	}
	return ANSI16
}

// DetectReason explains the choice; wakeart doctor prints it because this is
// what generates bug reports.
func DetectReason() string {
	if v := os.Getenv("NO_COLOR"); v != "" {
		return "NO_COLOR is set"
	}
	term := os.Getenv("TERM")
	if term == "" || term == "dumb" {
		return fmt.Sprintf("TERM=%q", term)
	}
	if ct := os.Getenv("COLORTERM"); ct == "truecolor" || ct == "24bit" {
		return fmt.Sprintf("COLORTERM=%s", ct)
	}
	if strings.Contains(term, "256color") {
		return fmt.Sprintf("TERM=%s contains 256color", term)
	}
	return fmt.Sprintf("TERM=%s, no COLORTERM", term)
}
