package proc

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/verdantran/wakeart/internal/audio"
	"github.com/verdantran/wakeart/internal/render"
)

// stubLevels stands in for a capture, so the drawing is exercised on any
// machine rather than only where the recorder exists.
type stubLevels struct {
	lv      [audio.Bands]float64
	live    bool
	err     error
	stopped int
}

func (s *stubLevels) Levels() ([audio.Bands]float64, bool) { return s.lv, s.live }
func (s *stubLevels) Err() error                           { return s.err }
func (s *stubLevels) Stop()                                { s.stopped++ }

func spectrumWith(s *stubLevels) *Spectrum { return &Spectrum{src: s} }

func TestSpectrumDrawsBarsFromLevels(t *testing.T) {
	st := &stubLevels{live: true}
	for i := range st.lv {
		st.lv[i] = 1 // every band full
	}
	f := spectrumWith(st).Frame(0, 80, 24)
	out := render.Plain(f)
	if strings.TrimSpace(out) == "" {
		t.Fatal("full levels drew nothing")
	}
	// The bottom row must be solid where the bars are.
	if !strings.Contains(out, "█") {
		t.Errorf("no block glyphs in the output:\n%s", out)
	}
}

// A quiet band must draw shorter than a loud one, or the scene is not showing
// the audio at all.
func TestSpectrumHeightTracksLevel(t *testing.T) {
	inked := func(level float64) int {
		st := &stubLevels{live: true}
		for i := range st.lv {
			st.lv[i] = level
		}
		f := spectrumWith(st).Frame(0, 80, 24)
		n := 0
		for _, c := range f.Cells {
			if !c.Blank() {
				n++
			}
		}
		return n
	}
	quiet, loud := inked(0.15), inked(0.95)
	if quiet >= loud {
		t.Errorf("quiet drew %d cells, loud drew %d; height is not tracking level", quiet, loud)
	}
	if inked(0) != 0 {
		t.Errorf("silence drew %d cells, want an empty pane", inked(0))
	}
}

func TestSpectrumReportsCaptureFailure(t *testing.T) {
	st := &stubLevels{err: errors.New("parec exploded")}
	out := render.Plain(spectrumWith(st).Frame(0, 80, 24))
	if !strings.Contains(out, "parec exploded") {
		t.Errorf("the failure was not surfaced to the pane:\n%s", out)
	}
}

func TestSpectrumWaitsForAudio(t *testing.T) {
	out := render.Plain(spectrumWith(&stubLevels{live: false}).Frame(0, 80, 24))
	if !strings.Contains(out, "waiting for audio") {
		t.Errorf("expected a waiting notice, got:\n%s", out)
	}
}

// Peaks fall over time and never sink below the current level.
func TestSpectrumPeaksFallAndCapTheBar(t *testing.T) {
	st := &stubLevels{live: true}
	for i := range st.lv {
		st.lv[i] = 1
	}
	sp := spectrumWith(st)
	sp.Frame(0, 80, 24)
	for i := range st.lv {
		st.lv[i] = 0
	}
	before := sp.peaks[0]
	sp.Frame(500*time.Millisecond, 80, 24)
	after := sp.peaks[0]
	if !(after < before) {
		t.Errorf("peak did not fall: %v then %v", before, after)
	}
	sp.Frame(10*time.Second, 80, 24)
	for i, p := range sp.peaks {
		if p < 0 {
			t.Errorf("peak %d went negative: %v", i, p)
		}
	}
}

// The renderer is handed whatever viewport the terminal is, including absurd
// ones, and must never draw outside the frame.
func TestSpectrumSurvivesOddViewports(t *testing.T) {
	st := &stubLevels{live: true}
	for i := range st.lv {
		st.lv[i] = 1
	}
	for _, wh := range [][2]int{{0, 0}, {1, 1}, {0, 24}, {80, 0}, {2, 2}, {1, 200}, {200, 1}, {audio.Bands, 10}, {audio.Bands*2 - 1, 10}} {
		f := spectrumWith(st).Frame(0, wh[0], wh[1])
		if f == nil {
			t.Fatalf("%dx%d: nil frame", wh[0], wh[1])
		}
		if f.W != max0(wh[0]) || f.H != max0(wh[1]) {
			t.Errorf("%dx%d: frame is %dx%d", wh[0], wh[1], f.W, f.H)
		}
	}
}

// Hostile levels reach here only through a bug, but a bar height is an index.
func TestSpectrumClampsRogueLevels(t *testing.T) {
	for _, v := range []float64{-1, 2, 1e9} {
		st := &stubLevels{live: true}
		for i := range st.lv {
			st.lv[i] = v
		}
		spectrumWith(st).Frame(0, 80, 24) // must not panic or draw out of bounds
	}
}

func TestSpectrumStopReleasesTheCapture(t *testing.T) {
	st := &stubLevels{}
	spectrumWith(st).Stop()
	if st.stopped != 1 {
		t.Errorf("Stop reached the source %d times, want 1", st.stopped)
	}
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}
