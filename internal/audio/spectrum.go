// Package audio turns the system's own output into band levels for the
// spectrum scene. It captures what the machine is playing, never a microphone.
package audio

import "math"

// Bands is how many bars the analyser produces, and so the resolution of the
// spectrum scene.
const Bands = 24

// Window is the sample count fed to each transform. At 22.05 kHz it spans
// ~46 ms: long enough to resolve a bass note, short enough that a drum hit
// still lands inside one frame.
const Window = 1024

// Rate is the sample rate we ask the capture backend for. Halving CD rate
// costs nothing visible — the top band sits at 11 kHz — and halves the work.
const Rate = 22050

// Analyser turns a window of PCM into band levels, smoothed over time so the
// bars fall rather than flicker.
type Analyser struct {
	rate   float64
	levels [Bands]float64
	re     [Window]float64
	im     [Window]float64
	hann   [Window]float64
	edges  [Bands + 1]int
}

func NewAnalyser(rate int) *Analyser {
	if rate <= 0 {
		rate = Rate
	}
	a := &Analyser{rate: float64(rate)}
	for i := range a.hann {
		a.hann[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(Window-1)))
	}
	a.buildEdges()
	return a
}

// buildEdges spaces the bands logarithmically. Linear spacing would put almost
// every bar above 5 kHz, where music has little to show, and squeeze the bass —
// the part you actually see moving — into a single bar.
//
// The top is pinned below Nyquist rather than at a fixed 16 kHz, so a low
// capture rate narrows the range instead of collapsing every high band onto
// the same bin.
func (a *Analyser) buildEdges() {
	nyquist := a.rate / 2
	lo := 40.0
	hi := math.Min(16000, nyquist*0.95)

	// Below about 2 kHz of range there is nothing to spread logarithmically.
	if hi <= lo*2 {
		hi = lo * 2
	}
	maxBin := Window / 2
	for i := 0; i <= Bands; i++ {
		f := lo * math.Pow(hi/lo, float64(i)/float64(Bands))
		bin := int(f / nyquist * float64(maxBin))
		if i > 0 && bin <= a.edges[i-1] {
			bin = a.edges[i-1] + 1 // every band owns at least one bin
		}
		if bin > maxBin {
			bin = maxBin
		}
		a.edges[i] = bin
	}
	// If clamping collapsed the top bands, walk back down so each still owns a
	// bin. Only a very low capture rate can reach this.
	for i := Bands; i > 0; i-- {
		if a.edges[i] <= a.edges[i-1] {
			a.edges[i-1] = a.edges[i] - 1
		}
	}
}

// Levels folds one window of samples into the band levels and returns them.
// Fewer samples than a full window is not an error: the tail is zero-padded,
// which costs a little resolution and nothing else.
func (a *Analyser) Levels(samples []float64) [Bands]float64 {
	n := copy(a.re[:], samples)
	for i := n; i < Window; i++ {
		a.re[i] = 0
	}
	for i := 0; i < Window; i++ {
		if math.IsNaN(a.re[i]) || math.IsInf(a.re[i], 0) {
			a.re[i] = 0 // a garbled sample must not poison the whole window
		}
		a.re[i] *= a.hann[i]
		a.im[i] = 0
	}
	fft(a.re[:], a.im[:])

	for b := 0; b < Bands; b++ {
		lo, hi := a.edges[b], a.edges[b+1]
		peak := 0.0
		for k := lo; k < hi && k <= Window/2; k++ {
			// The strongest bin, not the mean: averaging over a wide high band
			// buries a tone among its quiet neighbours.
			if m := math.Hypot(a.re[k], a.im[k]); m > peak {
				peak = m
			}
		}

		// dB across a 60 dB floor. Magnitude alone leaves everything quiet in
		// the bottom row; ears are logarithmic and the bars should be too.
		v := 0.0
		if peak > 0 {
			v = (20*math.Log10(peak/float64(Window/4)) + 60) / 60
		}
		v = clamp01(v)

		// Rise at once so a transient is not missed, fall gradually so a bar
		// reads as a decaying level rather than a strobe.
		if v > a.levels[b] {
			a.levels[b] = v
		} else {
			a.levels[b] += (v - a.levels[b]) * 0.28
		}
	}
	return a.levels
}

// BandRange is the frequency span a band covers, for the doctor report and
// for tests.
func (a *Analyser) BandRange(b int) (lo, hi float64) {
	per := (a.rate / 2) / float64(Window/2)
	return float64(a.edges[b]) * per, float64(a.edges[b+1]) * per
}

func clamp01(v float64) float64 {
	if !(v > 0) { // also catches NaN
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// fft is an in-place radix-2 Cooley-Tukey transform. len(re) must be a power
// of two, which Window is by construction.
func fft(re, im []float64) {
	n := len(re)
	if n <= 1 {
		return
	}
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		ang := -2 * math.Pi / float64(length)
		wr, wi := math.Cos(ang), math.Sin(ang)
		for i := 0; i < n; i += length {
			cr, ci := 1.0, 0.0
			for j := 0; j < length/2; j++ {
				ur, ui := re[i+j], im[i+j]
				vr := re[i+j+length/2]*cr - im[i+j+length/2]*ci
				vi := re[i+j+length/2]*ci + im[i+j+length/2]*cr
				re[i+j], im[i+j] = ur+vr, ui+vi
				re[i+j+length/2], im[i+j+length/2] = ur-vr, ui-vi
				cr, ci = cr*wr-ci*wi, cr*wi+ci*wr
			}
		}
	}
}
