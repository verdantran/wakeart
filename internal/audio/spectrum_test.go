package audio

import (
	"math"
	"testing"
)

func TestFFTMatchesTheNaiveTransform(t *testing.T) {
	const n = 64
	re := make([]float64, n)
	im := make([]float64, n)
	for i := range re {
		re[i] = math.Sin(2*math.Pi*3*float64(i)/n) + 0.5*math.Cos(2*math.Pi*11*float64(i)/n)
	}
	want := make([]complex128, n)
	for k := 0; k < n; k++ {
		var sum complex128
		for j := 0; j < n; j++ {
			ang := -2 * math.Pi * float64(k) * float64(j) / n
			sum += complex(re[j], 0) * complex(math.Cos(ang), math.Sin(ang))
		}
		want[k] = sum
	}
	fft(re, im)
	for k := 0; k < n; k++ {
		if math.Abs(re[k]-real(want[k])) > 1e-9 || math.Abs(im[k]-imag(want[k])) > 1e-9 {
			t.Fatalf("bin %d: got (%v,%v), want (%v,%v)", k, re[k], im[k], real(want[k]), imag(want[k]))
		}
	}
}

func tone(freq float64, rate int, n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = math.Sin(2 * math.Pi * freq * float64(i) / float64(rate))
	}
	return s
}

func settle(a *Analyser, s []float64) [Bands]float64 {
	var lv [Bands]float64
	for i := 0; i < 40; i++ {
		lv = a.Levels(s)
	}
	return lv
}

func TestToneLandsInTheRightBand(t *testing.T) {
	for _, freq := range []float64{100, 440, 2000, 8000} {
		a := NewAnalyser(Rate)
		lv := settle(a, tone(freq, Rate, Window))
		peak, idx := 0.0, -1
		for i, v := range lv {
			if v > peak {
				peak, idx = v, i
			}
		}
		if idx < 0 {
			t.Errorf("%.0f Hz: no band responded", freq)
			continue
		}
		lo, hi := a.BandRange(idx)
		if freq < lo*0.7 || freq > hi*1.4 {
			t.Errorf("%.0f Hz peaked in band %d covering %.0f-%.0f Hz", freq, idx, lo, hi)
		}
	}
}

func TestSilenceIsFlat(t *testing.T) {
	a := NewAnalyser(Rate)
	lv := settle(a, make([]float64, Window))
	for i, v := range lv {
		if v > 0.01 {
			t.Errorf("band %d reads %v on silence", i, v)
		}
	}
}

func TestLoudToneNearlyFillsItsBar(t *testing.T) {
	a := NewAnalyser(Rate)
	lv := settle(a, tone(440, Rate, Window))
	peak := 0.0
	for _, v := range lv {
		if v > peak {
			peak = v
		}
	}
	if peak < 0.6 {
		t.Errorf("a full-scale tone only reached %.2f; the bars will look dead", peak)
	}
}

// Levels drive a bar height, so anything outside 0..1 draws off the frame.
func TestLevelsStayInRange(t *testing.T) {
	a := NewAnalyser(Rate)
	loud := make([]float64, Window)
	for i := range loud {
		loud[i] = 1e6
	}
	nan := make([]float64, Window)
	for i := range nan {
		nan[i] = math.NaN()
	}
	inf := make([]float64, Window)
	for i := range inf {
		inf[i] = math.Inf(1)
	}
	for n, in := range [][]float64{{}, {1}, make([]float64, Window*4), loud, nan, inf} {
		for _, v := range a.Levels(in) {
			if !(v >= 0 && v <= 1) {
				t.Fatalf("input %d produced level %v", n, v)
			}
		}
	}
}

// Every band must own at least one bin, or it can never move.
func TestBandsAreDistinctAndOrdered(t *testing.T) {
	for _, rate := range []int{8000, 16000, 22050, 44100, 48000, 96000} {
		a := NewAnalyser(rate)
		for i := 0; i < Bands; i++ {
			if a.edges[i+1] <= a.edges[i] {
				t.Errorf("rate %d: band %d spans no bins (%d..%d)", rate, i, a.edges[i], a.edges[i+1])
			}
		}
		if a.edges[Bands] > Window/2 {
			t.Errorf("rate %d: top edge %d exceeds the Nyquist bin", rate, a.edges[Bands])
		}
		if a.edges[0] < 0 {
			t.Errorf("rate %d: negative bottom edge", rate)
		}
	}
}
