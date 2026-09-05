package scene

import (
	"fmt"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/verdantran/wakeart/internal/proc"
	"github.com/verdantran/wakeart/internal/render"
)

type rawMeta struct {
	Name    string   `toml:"name"`
	Author  string   `toml:"author"`
	Palette string   `toml:"palette"`
	FPS     float64  `toml:"fps"`
	Loop    string   `toml:"loop"`
	Hold    string   `toml:"hold"`
	Align   string   `toml:"align"`
	Effects []string `toml:"effects"`
	Tags    []string `toml:"tags"`

	Kind   string    `toml:"kind"`
	Shape  string    `toml:"shape"`
	Spin   []float64 `toml:"spin"`
	Scale  float64   `toml:"scale"`
	Cull   bool      `toml:"cull"`
	Glyphs string    `toml:"glyphs"`
	Device string    `toml:"device"`
}

func safeAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = render.SafeString(s)
	}
	return out
}

const (
	frontDelim = "---"
	frameDelim = "==="
	maskDelim  = "~~~"
)

// Parse reads a .scene file. Frontmatter is optional; a bare text file is a
// valid single-frame scene.
func Parse(source string, data []byte) (*Scene, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	front, body := splitFrontmatter(text)

	var rm rawMeta
	if front != "" {
		if err := toml.Unmarshal([]byte(front), &rm); err != nil {
			return nil, fmt.Errorf("frontmatter: %w", err)
		}
	}

	// Frontmatter is displayed — in the status bar, in `list`, in `doctor` —
	// so it goes through the same control-character filter as the art. A name
	// carrying an OSC sequence would otherwise drive the user's terminal.
	m := Meta{
		Name:    render.SafeString(rm.Name),
		Author:  render.SafeString(rm.Author),
		Palette: render.SafeString(rm.Palette),
		FPS:     rm.FPS,
		Loop:    LoopMode(rm.Loop),
		Align:   render.ParseAlign(rm.Align),
		Effects: rm.Effects,
		Tags:    safeAll(rm.Tags),
		Kind:    render.SafeString(rm.Kind),
		Shape:   render.SafeString(rm.Shape),
		Scale:   rm.Scale,
		Source:  render.SafeString(source),
	}
	if m.Name == "" {
		m.Name = titleFromFilename(source)
	}
	if m.FPS <= 0 && rm.Kind == "" {
		m.FPS = 10
	}
	switch m.Loop {
	case Forward, PingPong, Once:
	default:
		m.Loop = Forward
	}
	if rm.Hold != "" {
		d, err := time.ParseDuration(rm.Hold)
		if err != nil {
			return nil, fmt.Errorf("hold: %w", err)
		}
		m.Hold = d
	}

	s := &Scene{Meta: m, body: body}

	// A procedural scene generates its own geometry, so it carries no art.
	if rm.Kind != "" {
		r, err := proc.Build(proc.Params{
			Kind:   rm.Kind,
			Shape:  rm.Shape,
			Spin:   proc.SpinFrom(rm.Spin),
			Scale:  rm.Scale,
			Cull:   rm.Cull,
			Glyphs: []rune(rm.Glyphs),
			Device: rm.Device,
		})
		if err != nil {
			return nil, err
		}
		s.proc = r
		if m.FPS <= 0 {
			m.FPS = 20
		}
		s.Meta = m
		return s, nil
	}

	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("no frames")
	}
	return s, nil
}

// splitFrontmatter is line-based so an empty block (--- immediately followed
// by ---) is recognised rather than leaking its delimiters into the art.
func splitFrontmatter(text string) (front, body string) {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != frontDelim {
		return "", text
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == frontDelim {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n")
		}
	}
	return "", text // unterminated: treat the whole file as art
}

func parseFrames(body string) ([]*render.Frame, error) {
	var frames []*render.Frame
	for _, chunk := range splitOn(body, frameDelim) {
		art, mask := splitMask(chunk)
		lines := trimBlankEdges(strings.Split(art, "\n"))
		if len(lines) == 0 {
			continue
		}
		var f *render.Frame
		if strings.Contains(art, "\x1b[") {
			f = framePassthrough(lines)
		} else {
			var maskLines []string
			if mask != "" {
				maskLines = trimBlankEdges(strings.Split(mask, "\n"))
			}
			f = render.FromLines(lines, maskLines)
		}
		frames = append(frames, f)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames")
	}
	return frames, nil
}

// splitOn breaks the body on lines consisting only of the delimiter.
func splitOn(body, delim string) []string {
	var out []string
	var cur []string
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimRight(line, " \t") == delim {
			out = append(out, strings.Join(cur, "\n"))
			cur = nil
			continue
		}
		cur = append(cur, line)
	}
	return append(out, strings.Join(cur, "\n"))
}

func splitMask(chunk string) (art, mask string) {
	parts := splitOn(chunk, maskDelim)
	if len(parts) < 2 {
		return chunk, ""
	}
	return parts[0], parts[1]
}

// trimBlankEdges drops leading and trailing empty lines; interior blanks are
// part of the art.
func trimBlankEdges(lines []string) []string {
	i, j := 0, len(lines)
	for i < j && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	for j > i && strings.TrimSpace(lines[j-1]) == "" {
		j--
	}
	out := make([]string, 0, j-i)
	for _, l := range lines[i:j] {
		out = append(out, strings.TrimRight(l, " \t"))
	}
	return out
}

// maxSGRPrefix caps the style prefix carried per cell. A line can legally
// switch colour many times, but nothing needs kilobytes of it, and without a
// cap a file of nothing but SGR sequences costs that much on every cell.
const maxSGRPrefix = 256

// framePassthrough keeps the file's own SGR sequences, attaching the active
// one to each glyph so cropping and effects still work. Everything else the
// file might contain — other CSI sequences, OSC, a lone ESC — is dropped: the
// prefix is written to the terminal verbatim, so only colour may go in it.
func framePassthrough(lines []string) *render.Frame {
	rows := make([][]render.Cell, len(lines))
	w := 0
	for i, line := range lines {
		var cells []render.Cell
		var cur string
		width := 0
		rs := []rune(line)
		for x := 0; x < len(rs); x++ {
			if rs[x] == 0x1b {
				x = skipEscape(rs, x, &cur)
				continue
			}
			if !render.Safe(rs[x]) {
				continue
			}
			cells = append(cells, render.Cell{R: rs[x], Lvl: render.Weight(rs[x]), Raw: cur})
			width += render.RuneWidth(rs[x])
		}
		rows[i] = cells
		if width > w {
			w = width
		}
	}
	f := render.New(w, len(rows))
	for y, row := range rows {
		x := 0
		for _, c := range row {
			f.SetRune(x, y, c)
			x += render.RuneWidth(c.R)
		}
	}
	return f
}

// skipEscape consumes the escape sequence starting at i, folding it into cur
// when it is an SGR. It returns the index of the sequence's final rune, so the
// caller's loop increment lands on whatever follows.
func skipEscape(rs []rune, i int, cur *string) int {
	if i+1 >= len(rs) || rs[i+1] != '[' {
		return i // a lone ESC, or a sequence introducer we do not honour
	}
	j := i + 2
	for j < len(rs) && !(rs[j] >= '@' && rs[j] <= '~') {
		j++
	}
	if j >= len(rs) {
		return len(rs) // unterminated: the rest of the line is the sequence
	}
	if rs[j] != 'm' || !sgrBody(rs[i+2:j]) {
		return j
	}
	seq := string(rs[i : j+1])
	if seq == "\x1b[0m" || seq == "\x1b[m" {
		*cur = ""
	} else if len(*cur)+len(seq) <= maxSGRPrefix {
		*cur += seq
	}
	return j
}

// sgrBody rejects anything but the digits and separators a colour select is
// made of, so a private-parameter sequence cannot ride along in the prefix.
func sgrBody(rs []rune) bool {
	for _, r := range rs {
		if (r < '0' || r > '9') && r != ';' && r != ':' {
			return false
		}
	}
	return true
}
