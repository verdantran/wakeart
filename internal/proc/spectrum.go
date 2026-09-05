package proc

import (
	"time"

	"github.com/verdantran/wakeart/internal/audio"
	"github.com/verdantran/wakeart/internal/render"
)

// eighths give a bar a sub-cell top, so a level moves smoothly instead of
// jumping a whole row at a time.
var eighths = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Spectrum draws the system's own output as a bar per frequency band. It is
// the one live scene: everything else here is a function of time, and this is
// a function of what the machine is playing.
type Spectrum struct {
	src   levelSource
	peaks [audio.Bands]float64
	last  time.Duration
}

// levelSource is what the scene needs from the capture: an audio.Source in
// the running program, a stub in the tests, which is the only way to exercise
// the drawing on a machine that cannot capture.
type levelSource interface {
	Levels() ([audio.Bands]float64, bool)
	Err() error
	Stop()
}

func NewSpectrum(p Params) *Spectrum {
	return &Spectrum{src: audio.NewSource(p.Device)}
}

func (s *Spectrum) Describe() string { return "spectrum" }

// Loop is zero: live audio never repeats, so there is no period to cut on.
func (s *Spectrum) Loop() time.Duration { return 0 }

// Live marks this renderer as non-deterministic, so the golden tests know to
// leave it alone.
func (s *Spectrum) Live() bool { return true }

// Prime waits for the capture to have a full window, so a one-shot render
// shows the audio rather than an empty pane.
func (s *Spectrum) Prime(d time.Duration) {
	if p, ok := s.src.(interface{ Prime(time.Duration) }); ok {
		p.Prime(d)
	}
}

// Stop releases the capture. The idle reaper would get there on its own, but
// quitting should not leave a recorder running for another few seconds.
func (s *Spectrum) Stop() { s.src.Stop() }

func (s *Spectrum) Frame(t time.Duration, w, h int) *render.Frame {
	f := render.New(w, h)
	if w <= 0 || h <= 0 {
		return f
	}
	if err := s.src.Err(); err != nil {
		centre(f, "audio capture unavailable: "+err.Error())
		return f
	}
	levels, live := s.src.Levels()
	if !live {
		centre(f, "waiting for audio")
		return f
	}

	// Peaks fall on elapsed time rather than frame count, so the decay looks
	// the same whatever rate the carousel is running at.
	dt := (t - s.last).Seconds()
	if dt < 0 || dt > 1 {
		dt = 0 // a seek or a resumed terminal should not drop every peak at once
	}
	s.last = t
	for i := range s.peaks {
		s.peaks[i] -= 0.6 * dt
		if s.peaks[i] < levels[i] {
			s.peaks[i] = levels[i]
		}
		if s.peaks[i] < 0 {
			s.peaks[i] = 0
		}
	}

	// One column of gap between bars, as long as the pane is wide enough to
	// afford it. Below that the bars merge into a single block and the shape
	// of the spectrum is easier to read than the individual bands.
	barW, gap := 1, 0
	if w >= audio.Bands*2 {
		barW = (w - (audio.Bands - 1)) / audio.Bands
		gap = 1
	}
	if barW < 1 {
		barW = 1
	}
	total := audio.Bands*barW + (audio.Bands-1)*gap
	x0 := (w - total) / 2
	if x0 < 0 {
		x0 = 0
	}

	for b := 0; b < audio.Bands; b++ {
		x := x0 + b*(barW+gap)
		drawBar(f, x, barW, h, levels[b], s.peaks[b])
	}
	return f
}

// drawBar fills a column group from the bottom up. Height is carried in
// eighths so the top cell can be a partial block.
func drawBar(f *render.Frame, x, barW, h int, level, peak float64) {
	eighth := int(level * float64(h) * 8)
	full, part := eighth/8, eighth%8
	peakRow := h - 1 - int(peak*float64(h))

	for dx := 0; dx < barW; dx++ {
		cx := x + dx
		if cx < 0 || cx >= f.W {
			continue
		}
		for i := 0; i < full && i < h; i++ {
			y := h - 1 - i
			// Level rises with height, so the palette gradient runs up the bar.
			f.SetRune(cx, y, render.Cell{R: '█', Lvl: barLevel(i, h)})
		}
		if part > 0 && full < h {
			y := h - 1 - full
			f.SetRune(cx, y, render.Cell{R: eighths[part], Lvl: barLevel(full, h)})
		}
		// The peak marker sits above the bar as a falling cap.
		if peakRow >= 0 && peakRow < h && peakRow < h-1-full {
			f.SetRune(cx, peakRow, render.Cell{R: '▄', Lvl: 255})
		}
	}
}

// barLevel maps a row to the density ramp: quiet bars stay dim, and a bar that
// reaches the top of the pane is at full brightness.
func barLevel(row, h int) uint8 {
	if h <= 1 {
		return 255
	}
	v := 90 + 165*row/(h-1)
	if v > 255 {
		v = 255
	}
	return uint8(v)
}

// centre writes a single line of text in the middle of the frame, for the
// states where there is nothing to draw.
func centre(f *render.Frame, msg string) {
	rs := []rune(msg)
	if len(rs) > f.W {
		rs = rs[:f.W]
	}
	y := f.H / 2
	x := (f.W - len(rs)) / 2
	for i, r := range rs {
		f.SetRune(x+i, y, render.Cell{R: r, Lvl: render.Weight(r)})
	}
}
