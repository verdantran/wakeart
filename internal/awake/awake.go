// Package awake holds off system sleep for as long as the carousel is on
// screen, by running the platform's own inhibitor as a child process.
package awake

import (
	"errors"
	"os/exec"
	"sync"
	"time"
)

var ErrUnsupported = errors.New("no sleep inhibitor on this platform")

// Keeper owns at most one inhibitor process. The zero value is unusable; call New.
type Keeper struct {
	mu  sync.Mutex
	cmd *exec.Cmd
}

func New() *Keeper { return &Keeper{} }

// Supported reports whether an inhibitor binary is on PATH.
func Supported() bool { return inhibitor() != "" }

// Name is the inhibitor this platform would use, for the doctor report.
func Name() string {
	if p := inhibitor(); p != "" {
		return p
	}
	return "none"
}

func (k *Keeper) On() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.cmd != nil
}

// Toggle flips the inhibitor and reports the state it settled on. It decides
// and acts under one lock, so two toggles cannot both read "off" and leave a
// second inhibitor running with no handle to it.
func (k *Keeper) Toggle() (bool, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.cmd != nil {
		k.stop()
		return false, nil
	}
	if err := k.start(); err != nil {
		return false, err
	}
	return true, nil
}

func (k *Keeper) Start() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.start()
}

func (k *Keeper) Stop() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.stop()
}

// start and stop carry the work; the exported pair only takes the lock.
func (k *Keeper) start() error {
	if k.cmd != nil {
		return nil
	}
	c := inhibitCmd()
	if c == nil {
		return ErrUnsupported
	}
	detach(c)
	if err := c.Start(); err != nil {
		return err
	}
	k.cmd = c
	return nil
}

// stop will not wait forever on a helper that ignores the signal: quitting the
// carousel must not hang on it.
func (k *Keeper) stop() {
	if k.cmd == nil {
		return
	}
	c := k.cmd
	k.cmd = nil
	kill(c)
	done := make(chan struct{})
	go func() { _ = c.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		if c.Process != nil {
			_ = c.Process.Kill()
		}
		go func() { _ = c.Wait() }() // reap whenever it finally exits
	}
}
