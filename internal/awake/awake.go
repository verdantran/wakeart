// Package awake holds off system sleep for as long as the carousel is on
// screen, by running the platform's own inhibitor as a child process.
package awake

import (
	"errors"
	"os/exec"
	"sync"
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

// Toggle flips the inhibitor and reports the state it settled on.
func (k *Keeper) Toggle() (bool, error) {
	if k.On() {
		k.Stop()
		return false, nil
	}
	return true, k.Start()
}

func (k *Keeper) Start() error {
	k.mu.Lock()
	defer k.mu.Unlock()
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

func (k *Keeper) Stop() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.cmd == nil {
		return
	}
	kill(k.cmd)
	_ = k.cmd.Wait()
	k.cmd = nil
}
