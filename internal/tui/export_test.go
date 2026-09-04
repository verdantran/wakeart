package tui

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/verdantran/wakeart/internal/config"
	"github.com/verdantran/wakeart/internal/palette"
	"github.com/verdantran/wakeart/internal/render"
	"github.com/verdantran/wakeart/internal/scene"
)

// rowDelta is one changed row. The text is one string and the colours are a
// flat [length, index, length, index, ...] list over it — a dense row can hold
// forty colour runs, and a JSON object per run costs more than the glyphs do.
// An index of -1 means uncoloured, which is how blanks travel.
type rowDelta struct {
	Y int    `json:"y"`
	T string `json:"t,omitempty"`
	C []int  `json:"c,omitempty"`
}

type expScene struct {
	Name    string       `json:"name"`
	Palette string       `json:"palette"`
	Kind    string       `json:"kind"`
	FPS     float64      `json:"fps"`
	Tags    string       `json:"tags"`
	Frames  [][]rowDelta `json:"frames"`
}

type export struct {
	Cols   int        `json:"cols"`
	Rows   int        `json:"rows"`
	Colors []string   `json:"colors"`
	Scenes []expScene `json:"scenes"`
}

var sgrRe = regexp.MustCompile(`38;2;(\d+);(\d+);(\d+)m`)

// TestExportReplay dumps real frames as structured JSON for the web replay.
// It is a tool, not a test: WAKEART_EXPORT=<path> go test ./internal/tui -run Export
func TestExportReplay(t *testing.T) {
	out := os.Getenv("WAKEART_EXPORT")
	if out == "" {
		t.Skip("set WAKEART_EXPORT=<path> to write the replay data")
	}
	const cols, rows, maxCaptureFrames = 80, 24, 120

	reg := scene.Load(os.DirFS("../../scenes"), nil)
	ex := export{Cols: cols, Rows: rows}
	idx := map[string]int{}
	colorID := func(hex string) int {
		if i, ok := idx[hex]; ok {
			return i
		}
		idx[hex] = len(ex.Colors)
		ex.Colors = append(ex.Colors, hex)
		return idx[hex]
	}

	for i, s := range reg.Scenes {
		cfg := config.Default()
		cfg.StatusBar = false
		m := New(Options{Registry: reg, Config: cfg, Mode: palette.TrueColor, Start: i, Seed: 9})
		m.w, m.h = cols, rows
		m.pos = i

		// Exactly one loop, so the replay does not cut mid-revolution. The
		// capture rate is whatever fits that span inside the frame budget.
		captureFPS := 24.0
		span := 3 * time.Second
		if s.Procedural() {
			if loop := s.Loop(); loop > 0 {
				span = loop
			} else {
				span = 3 * time.Second
			}
			captureFPS = float64(maxCaptureFrames) / span.Seconds()
			if captureFPS > 15 {
				captureFPS = 15
			}
			if captureFPS < 10 {
				captureFPS = 10
			}
		}
		if !s.Procedural() {
			if fs, err := s.Frames(); err == nil && len(fs) > 1 && s.Meta.FPS > 0 {
				span = time.Duration(float64(len(fs))/s.Meta.FPS*float64(time.Second)) + 200*time.Millisecond
			} else {
				span = 1500 * time.Millisecond
			}
		}
		n := int(math.Round(span.Seconds() * captureFPS))
		if n > maxCaptureFrames {
			n = maxCaptureFrames
		}
		if n < 1 {
			n = 1
		}

		es := expScene{Name: s.Meta.Name, Palette: m.activePalette().Name, Kind: s.Meta.Kind,
			FPS: captureFPS, Tags: fmt.Sprint(s.Meta.Tags)}
		eff := m.activeEffects()
		pal := m.activePalette()

		// Only changed rows are sent; the player keeps the rest. Most scenes
		// hold large still areas, so this is the difference between a page
		// that loads on a phone and one that does not.
		var prev []string
		for k := 0; k < n; k++ {
			m.animElapsed = time.Duration(float64(k) / captureFPS * float64(time.Second))
			m.effState.Tick(eff, captureFPS)
			f, _ := m.composed()
			if f == nil {
				break
			}
			frame := f.Clone()
			eff.Apply(frame, m.effState)

			rows := encode(frame, pal, colorID)
			var delta []rowDelta
			for y, r := range rows {
				key := fingerprint(r)
				if prev == nil || y >= len(prev) || prev[y] != key {
					delta = append(delta, r)
				}
				if prev == nil {
					continue
				}
				prev[y] = key
			}
			if prev == nil {
				prev = make([]string, len(rows))
				for y, r := range rows {
					prev[y] = fingerprint(r)
				}
			}
			es.Frames = append(es.Frames, delta)
		}
		ex.Scenes = append(ex.Scenes, es)
	}

	data, err := json.Marshal(ex)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s: %d scenes, %d colours, %.1f KB", out, len(ex.Scenes), len(ex.Colors), float64(len(data))/1024)
}

func fingerprint(rd rowDelta) string {
	var b []byte
	for _, v := range rd.C {
		b = append(b, byte(v), byte(v>>8))
	}
	return string(b) + "\x00" + rd.T
}

// encode turns a painted frame into per-row text plus colour runs.
func encode(f *render.Frame, p palette.Palette, colorID func(string) int) []rowDelta {
	rows := make([]rowDelta, 0, f.H)
	for y := 0; y < f.H; y++ {
		var text []rune
		var runs []int
		cur, n := -2, 0
		flush := func() {
			if n > 0 {
				runs = append(runs, n, cur)
			}
		}
		for x := 0; x < f.W; x++ {
			c := f.Cells[y*f.W+x]
			if c.Cont && x > 0 && f.Cells[y*f.W+x-1].Wide() {
				continue // the wide glyph already carries this cell
			}
			ch, id := ' ', -1
			if !c.Blank() && !(c.Wide() && x+1 >= f.W) {
				ch = c.R
				id = colorID(hexOf(p, c))
			}
			if id != cur {
				flush()
				cur, n = id, 0
			}
			text = append(text, ch)
			n++
		}
		flush()
		// Trailing blanks cost bytes and show nothing.
		for len(runs) >= 2 && runs[len(runs)-1] == -1 {
			text = text[:len(text)-runs[len(runs)-2]]
			runs = runs[:len(runs)-2]
		}
		rows = append(rows, rowDelta{Y: y, T: string(text), C: runs})
	}
	return rows
}

// hexOf reuses the renderer's own SGR so the replay colours match the terminal
// exactly rather than re-deriving them.
func hexOf(p palette.Palette, c render.Cell) string {
	var seq string
	if c.HasFix {
		seq = palette.SGRFixed(palette.TrueColor, scaleRGB(c.Fix, c.Dim))
	} else {
		// Coarsening the gradient position before the colour lookup merges
		// neighbouring runs, which is most of the cost in a scene like the
		// cascade where every cell of a tail is a slightly different shade.
		lvl := (int(c.Lvl) + 4) / 9 * 9
		if lvl > 255 {
			lvl = 255
		}
		seq = p.SGR(palette.TrueColor, float64(lvl)/255, (c.Dim+4)/9*9)
	}
	m := sgrRe.FindStringSubmatch(seq)
	if m == nil {
		return "#d0d0d0"
	}
	var r, g, b int
	fmt.Sscanf(m[1]+" "+m[2]+" "+m[3], "%d %d %d", &r, &g, &b)
	// Quantising to 5 bits a channel is invisible on these gradients and
	// merges a great many adjacent runs.
	q := func(v int) int { return (v + 4) / 8 * 8 & 0xff }
	return fmt.Sprintf("#%02x%02x%02x", q(r), q(g), q(b))
}

func scaleRGB(c palette.RGB, dim uint8) palette.RGB {
	if dim == 0 {
		return c
	}
	k := float64(255-dim) / 255
	return palette.RGB{R: uint8(float64(c.R) * k), G: uint8(float64(c.G) * k), B: uint8(float64(c.B) * k)}
}
