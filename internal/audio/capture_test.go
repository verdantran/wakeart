package audio

import (
	"encoding/binary"
	"errors"
	"math"
	"os"
	"os/exec"
	"testing"
	"time"
)

// The capture backend is Linux-only, so the pipeline is exercised here with
// the test binary standing in for the recorder. That covers the parts most
// likely to be wrong wherever this runs: the decode, the ring buffer, the
// goroutine lifecycle. What it cannot cover is parec itself.
const helperEnv = "WAKEART_AUDIO_TEST_HELPER"

func TestMain(m *testing.M) {
	if freq := os.Getenv(helperEnv); freq != "" {
		emitTone(freq)
		return
	}
	os.Exit(m.Run())
}

// emitTone writes a float32le sine wave to stdout until it is killed, the way
// a recorder streams. "silence" writes zeroes instead.
func emitTone(mode string) {
	buf := make([]byte, 4*256)
	phase := 0.0
	step := 2 * math.Pi * 440 / Rate
	for {
		for i := 0; i < len(buf); i += 4 {
			v := 0.0
			if mode != "silence" {
				v = math.Sin(phase)
				phase += step
			}
			binary.LittleEndian.PutUint32(buf[i:], math.Float32bits(float32(v)))
		}
		if _, err := os.Stdout.Write(buf); err != nil {
			return
		}
	}
}

func fakeSource(t *testing.T, mode string) *Source {
	t.Helper()
	s := NewSource("")
	orig := newCaptureCmd
	newCaptureCmd = func(string) (*exec.Cmd, error) {
		c := exec.Command(os.Args[0])
		c.Env = append(os.Environ(), helperEnv+"="+mode)
		return c, nil
	}
	t.Cleanup(func() { newCaptureCmd = orig; s.Stop() })
	return s
}

// waitFor polls until cond holds, so the test does not depend on how quickly
// the helper is scheduled.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestCapturedToneReachesTheBands(t *testing.T) {
	s := fakeSource(t, "tone")
	s.Prime(5 * time.Second)
	var peak float64
	waitFor(t, "a band to respond to the tone", func() bool {
		lv, live := s.Levels()
		if !live {
			return false // still filling; the deadline in waitFor bounds this
		}
		peak = 0
		for _, v := range lv {
			if v > peak {
				peak = v
			}
		}
		return peak > 0.5
	})
}

func TestCapturedSilenceStaysFlat(t *testing.T) {
	s := fakeSource(t, "silence")
	s.Prime(5 * time.Second)
	for i := 0; i < 40; i++ {
		s.Levels()
	}
	lv, _ := s.Levels()
	for i, v := range lv {
		if v > 0.01 {
			t.Errorf("band %d reads %v on silence", i, v)
		}
	}
}

// A machine with no recorder must not respawn a process every frame.
func TestUnsupportedBackendIsRememberedNotRetried(t *testing.T) {
	s := NewSource("")
	orig := newCaptureCmd
	calls := 0
	newCaptureCmd = func(string) (*exec.Cmd, error) {
		calls++
		return nil, ErrUnsupported
	}
	defer func() { newCaptureCmd = orig }()

	for i := 0; i < 50; i++ {
		if _, live := s.Levels(); live {
			t.Fatal("an unsupported source reported itself live")
		}
	}
	if calls != 1 {
		t.Errorf("tried to start the recorder %d times, want 1", calls)
	}
	if !errors.Is(s.Err(), ErrUnsupported) {
		t.Errorf("Err = %v, want ErrUnsupported", s.Err())
	}
}

func TestStopEndsTheProcess(t *testing.T) {
	s := fakeSource(t, "tone")
	s.Levels()
	waitFor(t, "capture to start", s.Running)

	s.mu.Lock()
	proc := s.cmd.Process
	s.mu.Unlock()

	s.Stop()
	if s.Running() {
		t.Error("still running after Stop")
	}
	waitFor(t, "the child to exit", func() bool {
		return proc.Signal(os.Signal(nil)) != nil
	})
}

func TestStopIsIdempotent(t *testing.T) {
	s := fakeSource(t, "tone")
	s.Levels()
	waitFor(t, "capture to start", s.Running)
	for i := 0; i < 5; i++ {
		s.Stop()
	}
}

func TestLevelsRestartsAfterStop(t *testing.T) {
	s := fakeSource(t, "tone")
	s.Levels()
	waitFor(t, "capture to start", s.Running)
	s.Stop()
	s.Levels()
	waitFor(t, "capture to restart", s.Running)
}

// A scene file names the device, so it is untrusted. argv means it cannot
// become a command, but it could still become a flag.
func TestDeviceValidation(t *testing.T) {
	if got, err := cleanDevice(""); err != nil || got != defaultMonitor {
		t.Errorf("empty device = %q, %v; want the default monitor", got, err)
	}
	for _, ok := range []string{"@DEFAULT_MONITOR@", "alsa_output.pci-0000_00_1f.3.analog-stereo.monitor", "easyeffects_sink.monitor"} {
		if _, err := cleanDevice(ok); err != nil {
			t.Errorf("cleanDevice(%q) rejected a real device name: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"-d", "--format=s16le", "-/dev/zero",
		"a b", "a;rm -rf /", "a$(id)", "a`id`", "a\nb", "a|b", "a&b", "a>f", "a'b", "a\"b",
	} {
		if _, err := cleanDevice(bad); err == nil {
			t.Errorf("cleanDevice(%q) was accepted", bad)
		}
	}
}

// The one-shot paths draw a single frame, so an unprimed source would render
// an empty pane and call it silence.
func TestPrimeWaitsForAFullWindow(t *testing.T) {
	s := fakeSource(t, "tone")
	if _, live := s.Levels(); live {
		t.Error("reported live before any samples had arrived")
	}
	s.Prime(5 * time.Second)
	lv, live := s.Levels()
	if !live {
		t.Fatal("still not live after priming")
	}
	peak := 0.0
	for _, v := range lv {
		if v > peak {
			peak = v
		}
	}
	if peak == 0 {
		t.Error("primed but every band is zero")
	}
}

// Priming must give up rather than hang when there is no recorder.
func TestPrimeGivesUpOnAnUnsupportedBackend(t *testing.T) {
	s := NewSource("")
	orig := newCaptureCmd
	newCaptureCmd = func(string) (*exec.Cmd, error) { return nil, ErrUnsupported }
	defer func() { newCaptureCmd = orig }()

	start := time.Now()
	s.Prime(5 * time.Second)
	if d := time.Since(start); d > time.Second {
		t.Errorf("Prime waited %v for a backend that does not exist", d)
	}
}
