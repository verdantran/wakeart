package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/verdantran/wakeart/internal/config"
	"github.com/verdantran/wakeart/internal/palette"
	"github.com/verdantran/wakeart/internal/scene"
)

// A scene name comes from TOML frontmatter in any file under the scene dirs,
// and TOML \u escapes make control characters trivial to embed. The status bar
// renders it, so it must not become a way to drive the terminal.
func TestStatusBarDoesNotLeakEscapes(t *testing.T) {
	body := "---\nname = \"\\u001B]0;pwned\\u0007\\u001B[31mEVIL\"\n---\nart\n"
	s, err := scene.Parse("evil.scene", []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	reg := &scene.Registry{Scenes: []*scene.Scene{s}}

	m := New(Options{Registry: reg, Config: config.Default(), Mode: palette.TrueColor, Seed: 1})
	m.w, m.h = 80, 24
	out := m.statusBar(false)

	for _, bad := range []string{"\x1b]", "\x1bP", "\x07", "\x00", "\n"} {
		if strings.Contains(out, bad) {
			t.Errorf("%q reached the status bar: %q", bad, out)
		}
	}
	if !strings.Contains(out, "EVIL") {
		t.Errorf("the printable part of the name was lost: %q", out)
	}
}

// Init runs on a copy of the model, so it cannot record that a tick is
// pending. If New claims one anyway and Init schedules none, every later
// keypress short-circuits on the ticking check and the app is frozen.
func TestInitAndNewAgreeOnTicking(t *testing.T) {
	reg := deck(t)
	cfg := config.Default()
	cfg.Carousel = false
	cfg.Effects = nil
	cfg.StatusBar = false

	m := New(Options{Registry: reg, Config: cfg, Mode: palette.TrueColor, Seed: 1})
	scheduled := m.Init() != nil
	if m.ticking != scheduled {
		t.Fatalf("New set ticking=%v but Init scheduled=%v", m.ticking, scheduled)
	}
}

// With no tick pending, a keypress has to start the chain rather than assume
// one is already running.
func TestKeypressRecoversAStalledTickChain(t *testing.T) {
	reg := deck(t)
	m := New(Options{Registry: reg, Config: config.Default(), Mode: palette.TrueColor, Seed: 1})
	m.ticking = false
	m.w, m.h = 80, 24

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil {
		t.Error("a keypress with no tick pending returned no command")
	}
	if !next.(Model).ticking {
		t.Error("the model still reports no tick pending")
	}
}

// Toggling the bar changes the viewport height, and a transition holding a
// frame of the old height used to panic when the two were blended.
func TestBarToggleMidTransitionDoesNotPanic(t *testing.T) {
	reg := deck(t)
	m := New(Options{Registry: reg, Config: config.Default(), Mode: palette.TrueColor, Seed: 3})
	m.w, m.h = 80, 24

	var mm tea.Model = m
	mm, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	mm, _ = mm.Update(tickMsg{})
	mm, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	_ = mm.(Model).View()
}

// View runs on a copy of the model at moments bubbletea chooses, so drawing
// from the shared RNG there made a --seed run unreproducible.
func TestViewIsPureForTheSameState(t *testing.T) {
	reg := deck(t)
	m := New(Options{Registry: reg, Config: config.Default(), Mode: palette.TrueColor, Seed: 11})
	m.w, m.h = 80, 24

	var mm tea.Model = m
	mm, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	mm, _ = mm.Update(tickMsg{})

	first := mm.(Model).View()
	for i := 0; i < 5; i++ {
		if got := mm.(Model).View(); got != first {
			t.Fatalf("View call %d differs from the first for identical state", i+2)
		}
	}
}

// The cache is walked in frame order, so evicting the oldest insertion throws
// away exactly the entry wanted next.
func TestFrameCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := newFrameCache(2)
	c.put("a", cacheEntry{})
	c.put("b", cacheEntry{})
	if _, ok := c.get("a"); !ok {
		t.Fatal("a should still be cached")
	}
	c.put("c", cacheEntry{}) // evicts b, the least recently used

	if _, ok := c.get("a"); !ok {
		t.Error("a was evicted despite being used most recently")
	}
	if _, ok := c.get("b"); ok {
		t.Error("b should have been evicted")
	}
	if _, ok := c.get("c"); !ok {
		t.Error("c should be cached")
	}
	if len(c.items) != len(c.order) {
		t.Errorf("index and order disagree: %d items, %d order entries", len(c.items), len(c.order))
	}
}

// The tick delta was capped above but not below, so a clock that steps
// backwards drove holdElapsed negative and the progress bar's repeat count
// with it. Go's monotonic clock keeps this off the shipped path, but the
// guard was asymmetric and the package API can reach it.
func TestBackwardsClockDoesNotBreakTheProgressBar(t *testing.T) {
	reg := deck(t)
	m := New(Options{Registry: reg, Config: config.Default(), Mode: palette.TrueColor, Seed: 5})
	m.w, m.h = 80, 24
	m.lastTick = time.Now()

	var mm tea.Model = m
	mm, _ = mm.Update(tickMsg(time.Now().Add(-time.Hour)))
	if got := mm.(Model).holdElapsed; got < 0 {
		t.Errorf("holdElapsed went negative: %v", got)
	}
	_ = mm.(Model).View()
}

func TestProgressBarStaysInRange(t *testing.T) {
	reg := deck(t)
	m := New(Options{Registry: reg, Config: config.Default(), Mode: palette.TrueColor, Seed: 5})
	m.w, m.h = 80, 24
	for _, e := range []time.Duration{-time.Hour, -1, 0, time.Second, time.Hour} {
		m.holdElapsed = e
		if got := len([]rune(m.progress(8))); got != 8 {
			t.Errorf("holdElapsed %v gave a %d-cell bar, want 8", e, got)
		}
	}
}
