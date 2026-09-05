package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sync"
	"time"
)

var ErrUnsupported = errors.New("no system-audio capture on this platform")

// newCaptureCmd is the recorder factory, swapped out by tests so the pipeline
// can be exercised without a sound server.
var newCaptureCmd = captureCmd

// idleStop is how long a source keeps capturing after the last frame asked for
// levels. The carousel moves off the spectrum scene for a minute at a time, and
// holding a capture stream open through all of it would be rude.
const idleStop = 3 * time.Second

// ring holds a few windows of recent samples, so a frame always has a full
// window to transform even if it lands between reads.
const ring = Window * 4

// Source captures the system's audio output and hands out band levels. It
// starts on the first request and stops itself once nothing is asking.
//
// Everything here is safe to call from the render loop: the capture runs in its
// own goroutine, and no method blocks on it.
type Source struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stopped chan struct{}
	buf     [ring]float64
	pos     int
	written int64 // total samples ever pushed; primed once it reaches Window
	last    time.Time
	err     error

	device   string
	analyser *Analyser
	scratch  []float64
}

func NewSource(device string) *Source {
	return &Source{
		device:   device,
		analyser: NewAnalyser(Rate),
		scratch:  make([]float64, Window),
	}
}

// Levels returns the current band levels, starting capture if it is not
// already running. The second result is false when nothing is being captured,
// which is the caller's cue to draw something else.
func (s *Source) Levels() ([Bands]float64, bool) {
	s.mu.Lock()
	s.last = time.Now()
	if s.cmd == nil && s.err == nil {
		s.start()
	}
	if s.err != nil || s.written < Window {
		s.mu.Unlock()
		// Not yet primed is not the same as silence, and drawing empty bars
		// would say the wrong thing. The caller shows its waiting state.
		return [Bands]float64{}, false
	}
	// Copy the newest window out from under the lock, so the transform does not
	// hold up the reader goroutine.
	for i := 0; i < Window; i++ {
		s.scratch[i] = s.buf[(s.pos+ring-Window+i)%ring]
	}
	s.mu.Unlock()

	return s.analyser.Levels(s.scratch), true
}

// Prime starts the capture and waits for the first full window, so a caller
// that will only ever draw one frame does not draw an empty one. The carousel
// does not need it: it is already drawing again in 30ms.
func (s *Source) Prime(d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if _, live := s.Levels(); live {
			return
		}
		if s.Err() != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Err is the reason capture is not running, or nil.
func (s *Source) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Running reports whether a capture process is alive.
func (s *Source) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cmd != nil
}

// start assumes the lock. A failure is recorded rather than retried, so a
// machine with no capture tool does not respawn a process every frame.
func (s *Source) start() {
	cmd, err := newCaptureCmd(s.device)
	if err != nil {
		s.err = err
		return
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		s.err = err
		return
	}
	detach(cmd)
	if err := cmd.Start(); err != nil {
		s.err = err
		return
	}
	s.cmd = cmd
	s.stopped = make(chan struct{})
	go s.read(out, s.stopped)
	go s.reapWhenIdle(s.stopped)
}

// read decodes the little-endian float32 stream the backend writes.
func (s *Source) read(out io.ReadCloser, stopped <-chan struct{}) {
	defer out.Close()
	buf := make([]byte, 4096)
	for {
		select {
		case <-stopped:
			return
		default:
		}
		n, err := out.Read(buf)
		if n > 0 {
			s.push(buf[:n-n%4])
		}
		if err != nil {
			return
		}
	}
}

func (s *Source) push(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i+4 <= len(b); i += 4 {
		v := float64(math.Float32frombits(binary.LittleEndian.Uint32(b[i:])))
		if math.IsNaN(v) || math.IsInf(v, 0) {
			v = 0
		}
		s.buf[s.pos] = v
		s.pos = (s.pos + 1) % ring
		s.written++
	}
}

// reapWhenIdle stops the capture once no frame has asked for levels recently.
func (s *Source) reapWhenIdle(stopped <-chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-stopped:
			return
		case <-t.C:
			s.mu.Lock()
			idle := time.Since(s.last) > idleStop
			s.mu.Unlock()
			if idle {
				s.Stop()
				return
			}
		}
	}
}

// Stop ends the capture. It is safe to call when nothing is running, and a
// later Levels will start a fresh one.
func (s *Source) Stop() {
	s.mu.Lock()
	cmd := s.cmd
	stopped := s.stopped
	s.cmd, s.stopped = nil, nil
	s.written, s.pos = 0, 0
	s.mu.Unlock()

	if cmd == nil {
		return
	}
	close(stopped)
	kill(cmd)

	// Never block the caller on a child that ignores the signal: quitting the
	// carousel must not hang on it.
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		go func() { _ = cmd.Wait() }()
	}
}

// defaultMonitor is PulseAudio's name for the monitor of whatever sink is
// currently the default — that is, the machine's own output. PipeWire honours
// it too, through pipewire-pulse.
//
// Every backend here must name a monitor. A capture source that could resolve
// to a microphone does not belong in this file: the scene shows what the
// machine is playing, and nothing else.
const defaultMonitor = "@DEFAULT_MONITOR@"

// ValidDevice reports whether a device name from frontmatter is usable, so a
// typo is caught when the scene is parsed rather than when it first draws.
func ValidDevice(device string) error {
	_, err := cleanDevice(device)
	return err
}

// cleanDevice keeps a scene file from injecting an argument. A leading dash
// would be read as a flag by the recorder however carefully we avoid a shell.
func cleanDevice(device string) (string, error) {
	if device == "" {
		return defaultMonitor, nil
	}
	if device[0] == '-' {
		return "", fmt.Errorf("audio device %q may not begin with a dash", device)
	}
	for _, r := range device {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == ':' || r == '@':
		default:
			return "", fmt.Errorf("audio device %q contains an unexpected character %q", device, r)
		}
	}
	return device, nil
}
