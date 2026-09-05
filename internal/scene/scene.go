// Package scene parses .scene files and holds the deck.
package scene

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/verdantran/wakeart/internal/proc"
	"github.com/verdantran/wakeart/internal/render"
)

type LoopMode string

const (
	Forward  LoopMode = "forward"
	PingPong LoopMode = "pingpong"
	Once     LoopMode = "once"
)

type Meta struct {
	Name    string
	Author  string
	Palette string
	FPS     float64
	Loop    LoopMode
	Hold    time.Duration
	Align   render.Align
	Effects []string
	Tags    []string
	Source  string // filesystem path, or "embedded:<name>"

	Kind  string // empty for art read from the file; otherwise a generator
	Shape string
	Scale float64
}

type Scene struct {
	Meta Meta

	proc   proc.Renderer
	body   string
	once   sync.Once
	frames []*render.Frame
	err    error
}

// Procedural reports whether the scene generates its geometry per frame
// instead of reading it from the file.
func (s *Scene) Procedural() bool { return s.proc != nil }

// Render draws a procedural scene straight into the viewport. It is already
// the right size, so it needs no fitting.
func (s *Scene) Render(elapsed time.Duration, speed float64, w, h int) *render.Frame {
	if s.proc == nil {
		return render.New(w, h)
	}
	return s.proc.Frame(time.Duration(float64(elapsed)*speed), w, h)
}

// Live reports whether the scene's output depends on something outside the
// program — the spectrum scene reads the machine's audio — so it cannot be
// compared against a golden file.
func (s *Scene) Live() bool {
	l, ok := s.proc.(interface{ Live() bool })
	return ok && l.Live()
}

// Stop releases whatever the scene holds open. Only a live scene has anything
// to release.
func (s *Scene) Stop() {
	if st, ok := s.proc.(interface{ Stop() }); ok {
		st.Stop()
	}
}

// Loop is the period after which a procedural scene exactly repeats, or 0.
func (s *Scene) Loop() time.Duration {
	if s.proc == nil {
		return 0
	}
	return s.proc.Loop()
}

func (s *Scene) Describe() string {
	if s.proc == nil {
		return ""
	}
	return s.proc.Describe()
}

// Frames parses the body on first display; startup only reads frontmatter.
func (s *Scene) Frames() ([]*render.Frame, error) {
	if s.proc != nil {
		return nil, nil
	}
	s.once.Do(func() {
		s.frames, s.err = parseFrames(s.body)
	})
	return s.frames, s.err
}

// FrameCount reports the frame total without forcing a full parse. A
// procedural scene has no fixed count.
func (s *Scene) FrameCount() int {
	if s.proc != nil {
		return 0
	}
	return strings.Count(s.body, "\n===") + 1
}

// Size reports the largest frame, since a progressive scene grows as it plays.
func (s *Scene) Size() (w, h int) {
	if s.proc != nil {
		return 0, 0 // sized to the viewport
	}
	fs, err := s.Frames()
	if err != nil {
		return 0, 0
	}
	for _, f := range fs {
		if f.W > w {
			w = f.W
		}
		if f.H > h {
			h = f.H
		}
	}
	return w, h
}

// FrameAt maps elapsed time to a frame under the scene's loop mode.
func (s *Scene) FrameAt(elapsed time.Duration, speed float64) *render.Frame {
	fs, err := s.Frames()
	if err != nil || len(fs) == 0 {
		return render.New(0, 0)
	}
	return fs[s.FrameIndexAt(elapsed, speed)]
}

// FrameIndexAt is the same mapping, exposed so the render cache can key on it.
func (s *Scene) FrameIndexAt(elapsed time.Duration, speed float64) int {
	fs, err := s.Frames()
	if err != nil || len(fs) <= 1 || s.Meta.FPS <= 0 {
		return 0
	}
	n := int(elapsed.Seconds() * s.Meta.FPS * speed)
	if n < 0 {
		n = 0
	}
	switch s.Meta.Loop {
	case Once:
		if n >= len(fs) {
			n = len(fs) - 1
		}
	case PingPong:
		if len(fs) > 1 {
			period := 2*len(fs) - 2
			n %= period
			if n >= len(fs) {
				n = period - n
			}
		}
	default:
		n %= len(fs)
	}
	return n
}

// HasANSI reports whether the scene carries its own escape sequences, which
// take priority over gradient colouring.
func (s *Scene) HasANSI() bool { return strings.Contains(s.body, "\x1b[") }

func (s *Scene) ColourPath() string {
	if s.proc != nil {
		return "procedural"
	}
	switch {
	case s.HasANSI():
		return "ansi passthrough"
	case strings.Contains(s.body, "\n~~~"):
		return "colour mask"
	}
	return "density ramp"
}

func titleFromFilename(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.NewReplacer("-", " ", "_", " ").Replace(base)
	words := strings.Fields(base)
	if len(words) == 0 {
		return "Untitled"
	}
	for i, w := range words {
		r, n := utf8.DecodeRuneInString(w)
		words[i] = string(unicode.ToUpper(r)) + w[n:]
	}
	return strings.Join(words, " ")
}

func (m Meta) String() string {
	return fmt.Sprintf("%s (%s)", m.Name, m.Source)
}
