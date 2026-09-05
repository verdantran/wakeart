// Package render owns the frame buffer, ink-density colouring, viewport
// fitting and transitions.
package render

import (
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/verdantran/wakeart/internal/palette"
)

type Cell struct {
	R      rune
	Lvl    uint8 // gradient position from ink density or a colour mask
	Dim    uint8 // brightness reduction applied by effects
	Fix    palette.RGB
	HasFix bool
	// Raw is a verbatim SGR prefix, set only by ANSI passthrough scenes and
	// written to the terminal unchecked. Whatever sets it must have proved it
	// contains nothing but SGR sequences.
	Raw string
	// Cont marks the second half of a double-width glyph such as ⚡. The
	// terminal paints it as part of the previous cell, so it carries no rune
	// of its own.
	Cont bool
}

func (c Cell) Blank() bool { return !c.Cont && (c.R == 0 || c.R == ' ') }

// Wide reports whether the glyph occupies two terminal cells.
func (c Cell) Wide() bool { return RuneWidth(c.R) == 2 }

// RuneWidth is how many cells a glyph takes. Emoji-presentation symbols like
// ⚡ take two, and getting that wrong shifts everything to their right.
//
// The ASCII short-circuit matters: this runs once per drawn cell, and the
// table lookup underneath is far more expensive than the comparison.
func RuneWidth(r rune) int {
	if r < 0x80 {
		return 1
	}
	if w, ok := wideCache[r]; ok {
		return w
	}
	return runewidth.RuneWidth(r)
}

// Safe reports whether a glyph may be written to a terminal. Scene files are
// ordinary files a user may have fetched from anywhere, and a bare ESC in the
// art would let one drive the terminal: retitle the window, or write the
// clipboard with OSC 52. C1 and DEL are excluded for the same reason, and a
// tab because it would shift everything after it out of the frame's columns.
func Safe(r rune) bool {
	return r >= 0x20 && r != 0x7f && (r < 0x80 || r > 0x9f)
}

// SafeString strips what Safe rejects, for text that reaches the terminal
// outside the frame buffer: scene names in the status bar and the listings.
func SafeString(s string) string {
	if strings.IndexFunc(s, func(r rune) bool { return !Safe(r) }) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if Safe(r) {
			return r
		}
		return -1
	}, s)
}

// wideCache covers the glyphs the renderers actually emit, so the common path
// never reaches the lookup tables.
var wideCache = map[rune]int{
	'─': 1, '│': 1, '╱': 1, '╲': 1, '░': 1, '▒': 1, '▓': 1, '█': 1,
	'·': 1, '◦': 1, '⚙': 1, '⌁': 1, '↯': 1, 'ϟ': 1, '⚡': 2,
	'⊛': 1, '◉': 1, '⍟': 1, '✳': 1, '✻': 1, '❋': 1,
}

type Frame struct {
	Cells []Cell // row-major, W*H
	W, H  int
}

func New(w, h int) *Frame {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return &Frame{Cells: make([]Cell, w*h), W: w, H: h}
}

func (f *Frame) At(x, y int) Cell {
	if x < 0 || y < 0 || x >= f.W || y >= f.H {
		return Cell{}
	}
	return f.Cells[y*f.W+x]
}

func (f *Frame) Set(x, y int, c Cell) {
	if x < 0 || y < 0 || x >= f.W || y >= f.H {
		return
	}
	f.Cells[y*f.W+x] = c
}

// SetRune places a glyph, reserving the trailing cell when it is double-width.
// A wide glyph with no room at the right edge is dropped rather than allowed
// to overflow the viewport.
func (f *Frame) SetRune(x, y int, c Cell) {
	if x < 0 || y < 0 || x >= f.W || y >= f.H {
		return
	}
	// Taking over the second half of a wide glyph orphans its head, which
	// would then still claim two cells and push the row out of the viewport.
	if x > 0 && f.Cells[y*f.W+x-1].Wide() {
		f.Cells[y*f.W+x-1] = Cell{}
	}
	if RuneWidth(c.R) == 2 {
		if x+1 >= f.W {
			return
		}
		f.Cells[y*f.W+x] = c
		f.Cells[y*f.W+x+1] = Cell{Cont: true, Lvl: c.Lvl, Dim: c.Dim, Fix: c.Fix, HasFix: c.HasFix, Raw: c.Raw}
		return
	}
	f.Cells[y*f.W+x] = c
}

// wideOK reports whether a wide glyph at x still owns its trailing cell.
// Cropping and the glitch effect's row shifts can break the pair apart.
func (f *Frame) wideOK(x, y int) bool {
	return x+1 < f.W && f.Cells[y*f.W+x+1].Cont
}

func (f *Frame) Ptr(x, y int) *Cell {
	if x < 0 || y < 0 || x >= f.W || y >= f.H {
		return nil
	}
	return &f.Cells[y*f.W+x]
}

func (f *Frame) Clone() *Frame {
	n := &Frame{Cells: make([]Cell, len(f.Cells)), W: f.W, H: f.H}
	copy(n.Cells, f.Cells)
	return n
}

// FromLines builds a frame from already-padded text, scoring each glyph on ink
// weight. mask, when non-nil, overrides those levels with palette indices.
func FromLines(lines []string, mask []string) *Frame {
	w := 0
	rows := make([][]rune, len(lines))
	for i, l := range lines {
		rows[i] = []rune(l)
		if n := lineCells(rows[i]); n > w {
			w = n
		}
	}
	f := New(w, len(lines))
	for y, row := range rows {
		x := 0
		for _, r := range row {
			if x >= w {
				break
			}
			if !Safe(r) {
				r = ' '
			}
			f.SetRune(x, y, Cell{R: r, Lvl: Weight(r)})
			x += RuneWidth(r)
		}
	}
	if mask != nil {
		applyMask(f, mask)
	}
	return f
}

// lineCells is the display width of a line, which is not its rune count once
// double-width glyphs are involved.
func lineCells(rs []rune) int {
	n := 0
	for _, r := range rs {
		if !Safe(r) {
			r = ' '
		}
		n += RuneWidth(r)
	}
	return n
}

func applyMask(f *Frame, mask []string) {
	for y, l := range mask {
		if y >= f.H {
			break
		}
		for x, r := range []rune(l) {
			if x >= f.W || r < '0' || r > '9' {
				continue
			}
			if c := f.Ptr(x, y); c != nil {
				c.Lvl = uint8(int(r-'0') * 255 / 9)
			}
		}
	}
}

// Weight scores a glyph's ink coverage so plain ASCII colours itself.
func Weight(r rune) uint8 {
	if w, ok := inkWeights[r]; ok {
		return w
	}
	switch {
	case r >= 0x2500 && r <= 0x257F: // box drawing
		return 140
	case r >= 0x2580 && r <= 0x259F: // block elements not listed below
		return 230
	case r >= '0' && r <= '9':
		return 170
	case r >= 'A' && r <= 'Z':
		return 180
	case r >= 'a' && r <= 'z':
		return 150
	case r < ' ':
		return 0
	}
	return 130
}

var inkWeights = map[rune]uint8{
	' ': 0, '\t': 0, 0: 0,
	'.': 30, ',': 38, '\'': 34, '`': 34, ':': 46, ';': 54, '"': 50,
	'-': 60, '_': 62, '~': 66, '^': 58, '|': 96, '!': 78, '/': 88, '\\': 88,
	'(': 84, ')': 84, '[': 92, ']': 92, '{': 96, '}': 96, '<': 82, '>': 82,
	'=': 104, '+': 112, '*': 126, 'i': 100, 'l': 100, 't': 110, 'r': 110,
	'?': 120, '$': 190, '&': 196, '#': 210, '%': 214, '@': 240,
	'░': 70, '▒': 130, '▓': 195, '█': 255,
	'▀': 235, '▄': 235, '▌': 235, '▐': 235, '▛': 240, '▜': 240, '▙': 240, '▟': 240,
	'▖': 150, '▗': 150, '▘': 150, '▝': 150, '▚': 200, '▞': 200,
	'●': 230, '○': 150, '◆': 220, '◇': 150, '■': 250, '□': 150, '▪': 190, '▫': 130,
	'╳': 150, '✦': 170, '★': 210, '☆': 150, '·': 30, '•': 120, '°': 70,
}

// styleKey is what a run of cells shares. Comparing keys rather than built
// strings keeps Paint allocation-free.
type styleKey struct {
	lvl, dim uint8
	fix      palette.RGB
	hasFix   bool
	raw      string
	set      bool
}

func keyOf(c Cell) styleKey {
	return styleKey{lvl: c.Lvl, dim: c.Dim, fix: c.Fix, hasFix: c.HasFix, raw: c.Raw, set: true}
}

// glyph weights for the themed symbol sets, so the density ramp still colours
// them sensibly.
func init() {
	for r, w := range map[rune]uint8{
		'⚡': 240, 'ϟ': 200, '↯': 190, '⌁': 170, '☇': 200,
		'⚙': 235, '✳': 195, '✻': 200, '❋': 215, '✱': 175, '✲': 185,
		'⎔': 160, '◉': 230, '⌬': 190, '⚛': 200, '⊛': 205, '⍟': 205,
	} {
		inkWeights[r] = w
	}
}

// Paint writes the frame as ANSI, coalescing colour runs so a screenful is a
// few hundred escape sequences rather than a few thousand.
func Paint(f *Frame, p palette.Palette, m palette.Mode, b *strings.Builder) {
	var cur styleKey
	for y := 0; y < f.H; y++ {
		if y > 0 {
			b.WriteString("\r\n")
		}
		for x := 0; x < f.W; x++ {
			c := f.Cells[y*f.W+x]
			// A continuation cell was already painted by its wide head.
			// One orphaned by a crop or a glitch shift becomes a space.
			if c.Cont {
				if x > 0 && f.Cells[y*f.W+x-1].Wide() {
					continue
				}
				if cur.set {
					b.WriteString(palette.Reset)
					cur = styleKey{}
				}
				b.WriteByte(' ')
				continue
			}
			if c.Blank() || !Safe(c.R) || (c.Wide() && !f.wideOK(x, y)) {
				if cur.set {
					b.WriteString(palette.Reset)
					cur = styleKey{}
				}
				b.WriteByte(' ')
				continue
			}
			if want := keyOf(c); want != cur {
				cur = want
				switch {
				case c.Raw != "":
					b.WriteString(c.Raw)
				case c.HasFix:
					palette.WriteSGRFixed(b, m, scaleFix(c.Fix, c.Dim))
				default:
					p.WriteSGR(b, m, float64(c.Lvl)/255, c.Dim)
				}
			}
			b.WriteRune(c.R)
		}
		if cur.set {
			b.WriteString(palette.Reset)
			cur = styleKey{}
		}
	}
}

func scaleFix(c palette.RGB, dim uint8) palette.RGB {
	if dim == 0 {
		return c
	}
	k := float64(255-dim) / 255
	return palette.RGB{R: uint8(float64(c.R) * k), G: uint8(float64(c.G) * k), B: uint8(float64(c.B) * k)}
}

// Plain renders without colour, for golden tests and non-TTY output.
func Plain(f *Frame) string {
	var b strings.Builder
	for y := 0; y < f.H; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		var row []rune
		for x := 0; x < f.W; x++ {
			c := f.Cells[y*f.W+x]
			if c.Cont {
				if x == 0 || !f.Cells[y*f.W+x-1].Wide() {
					row = append(row, ' ')
				}
				continue
			}
			r := c.R
			if r == 0 || !Safe(r) || (c.Wide() && !f.wideOK(x, y)) {
				r = ' '
			}
			row = append(row, r)
		}
		b.WriteString(strings.TrimRight(string(row), " "))
	}
	return b.String()
}
