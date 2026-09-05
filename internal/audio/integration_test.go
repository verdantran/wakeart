package audio

import (
	"fmt"
	"math"
	"os"
	"testing"
	"time"
)

// TestRealCaptureFromMonitor drives the actual recorder, not a stand-in. It is
// skipped unless a sound server is present, because the machine this is
// usually written on has none: set WAKEART_AUDIO_INTEGRATION=1 and, if the
// sink is not the default, WAKEART_AUDIO_DEVICE to its monitor.
//
// It is the only test that covers parec's own arguments. Everything else in
// this package substitutes a process of its own.
func TestRealCaptureFromMonitor(t *testing.T) {
	if os.Getenv("WAKEART_AUDIO_INTEGRATION") == "" {
		t.Skip("set WAKEART_AUDIO_INTEGRATION=1 with a sound server running")
	}
	if !Supported() {
		t.Fatalf("no capture backend found: %s", Backend())
	}
	t.Logf("backend: %s", Backend())

	s := NewSource(os.Getenv("WAKEART_AUDIO_DEVICE"))
	defer s.Stop()

	var best float64
	var bestBand int
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		lv, live := s.Levels()
		if err := s.Err(); err != nil {
			t.Fatalf("capture failed: %v", err)
		}
		if !live {
			t.Fatal("source reported not live")
		}
		for i, v := range lv {
			if v > best {
				best, bestBand = v, i
			}
		}
		if best > 0.5 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if best <= 0.5 {
		t.Fatalf("nothing was captured: strongest band reached only %.3f", best)
	}

	lo, hi := NewAnalyser(Rate).BandRange(bestBand)
	t.Logf("strongest band %d (%.0f-%.0f Hz) reached %.3f", bestBand, lo, hi, best)

	// The container plays a 440 Hz tone; anything far from it means we are
	// reading the stream wrongly rather than merely reading something.
	if want := os.Getenv("WAKEART_AUDIO_TONE_HZ"); want != "" {
		var f float64
		if _, err := fmt.Sscan(want, &f); err == nil && f > 0 {
			if f < lo*0.6 || f > hi*1.7 {
				t.Errorf("a %.0f Hz tone peaked in the band covering %.0f-%.0f Hz", f, lo, hi)
			}
		}
	}
	if math.IsNaN(best) {
		t.Error("level is NaN")
	}
}
