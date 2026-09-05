package tui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/verdantran/wakeart/internal/awake"
	"github.com/verdantran/wakeart/internal/config"
	"github.com/verdantran/wakeart/internal/effect"
	"github.com/verdantran/wakeart/internal/palette"
	"github.com/verdantran/wakeart/internal/render"
	"github.com/verdantran/wakeart/internal/scene"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func deck(t *testing.T) *scene.Registry {
	t.Helper()
	reg := scene.Load(os.DirFS("../../scenes"), nil)
	if reg.Len() == 0 {
		t.Fatal("no embedded scenes found")
	}
	for _, e := range reg.Errs {
		t.Errorf("scene failed to load: %v", e)
	}
	return reg
}

// fixtureDeck loads the checked-in .scene files. No art ships with the binary
// any more, but the file format is still supported for scenes users add, so
// the parse-fit-paint path needs something to exercise it.
func fixtureDeck(t *testing.T) *scene.Registry {
	t.Helper()
	reg := scene.Load(nil, []string{filepath.Join("testdata", "scenes")})
	if reg.Len() == 0 {
		t.Fatal("no fixture scenes found")
	}
	for _, e := range reg.Errs {
		t.Errorf("fixture failed to load: %v", e)
	}
	return reg
}

func newModel(t *testing.T, reg *scene.Registry, w, h int, mut func(*config.Config)) tea.Model {
	t.Helper()
	cfg := config.Default()
	if mut != nil {
		mut(&cfg)
	}
	var m tea.Model = New(Options{Registry: reg, Config: cfg, Mode: palette.TrueColor, Seed: 7})
	m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

// stripSGR removes colour so the goldens track layout, not palette tuning.
func stripSGR(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && !(s[i] >= '@' && s[i] <= '~') {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return strings.ReplaceAll(b.String(), "\r\n", "\n")
}

func TestGoldenScenes(t *testing.T) {
	var all []*scene.Scene
	all = append(all, deck(t).Scenes...)
	all = append(all, fixtureDeck(t).Scenes...)
	sizes := []struct{ w, h int }{{80, 24}, {120, 40}}
	for _, sz := range sizes {
		for _, s := range all {
			name := fmt.Sprintf("%s_%dx%d", scene.SlugOf(s), sz.w, sz.h)
			t.Run(name, func(t *testing.T) {
				if s.Live() {
					t.Skip("live scene: its output depends on the machine, not on time")
				}
				var b strings.Builder
				if s.Procedural() {
					// Sampled at fixed moments: the geometry is a pure
					// function of time, so this pins the projection.
					for _, ms := range []int{0, 700, 2300} {
						f := s.Render(time.Duration(ms)*time.Millisecond, 1, sz.w, sz.h)
						fmt.Fprintf(&b, "--- t=%dms ---\n%s\n", ms, render.Plain(f))
					}
					compareGolden(t, name+".txt", b.String())
					return
				}
				frames, err := s.Frames()
				if err != nil {
					t.Fatal(err)
				}
				for i, f := range frames {
					fitted, cropped := render.Fit(f, sz.w, sz.h, s.Meta.Align)
					fmt.Fprintf(&b, "--- frame %d (cropped=%v) ---\n", i, cropped)
					b.WriteString(render.Plain(fitted))
					b.WriteString("\n")
				}
				compareGolden(t, name+".txt", b.String())
			})
		}
	}
}

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run: go test ./internal/tui -update)", err)
	}
	if string(want) != got {
		t.Errorf("output differs from %s; run with -update if the change is intended", path)
	}
}

// Every scene in the deck, at several sizes, must paint exactly the viewport:
// no short frames, and no row wider than the pane once double-width glyphs are
// counted properly.
func TestViewFillsTheViewport(t *testing.T) {
	for _, reg := range []*scene.Registry{deck(t), fixtureDeck(t)} {
		for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}, {40, 12}, {200, 60}, {90, 26}} {
			for i := range reg.Scenes {
				m := newModel(t, reg, sz.w, sz.h, nil).(Model)
				m.pos = i
				checkViewport(t, m, sz.w, sz.h)
			}
		}
	}
}

func checkViewport(t *testing.T, m Model, w, h int) {
	t.Helper()
	name := m.current().Meta.Name
	for step := 0; step < 6; step++ {
		var mm tea.Model = m
		out := stripSGR(mm.View())
		if n := strings.Count(out, "\n") + 1; n != h {
			t.Errorf("%s at %dx%d: rendered %d lines, want %d", name, w, h, n, h)
		}
		for i, line := range strings.Split(out, "\n") {
			if cw := runewidth.StringWidth(line); cw > w {
				t.Errorf("%s at %dx%d: line %d is %d cells, overflows", name, w, h, i, cw)
			}
		}
		next, _ := m.Update(tickMsg(time.Now().Add(time.Duration(step) * 90 * time.Millisecond)))
		m = next.(Model)
	}
}

func TestTooSmall(t *testing.T) {
	m := newModel(t, deck(t), 10, 4, nil)
	if !strings.Contains(m.View(), "too small") {
		t.Error("a tiny viewport should say so rather than render garbage")
	}
}

// Art on a second monitor should keep playing when the terminal is not the
// focused window; only the opt-in modes go quiet.
func TestBlurKeepsRunningByDefault(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil).(Model)
	blurred, _ := m.Update(tea.BlurMsg{})
	if got := blurred.(Model).interval(); got <= 0 {
		t.Error("the default should keep animating while unfocused")
	}
}

func TestBlurThrottle(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, func(c *config.Config) { c.OnBlur = "throttle" }).(Model)
	focused := m.interval()
	blurred, _ := m.Update(tea.BlurMsg{})
	got := blurred.(Model).interval()
	if got <= 0 {
		t.Fatal("throttle should keep ticking, just slower")
	}
	if got <= focused {
		t.Errorf("unfocused interval %v should be longer than the focused %v", got, focused)
	}
	if got < time.Second/blurThrottleFPS {
		t.Errorf("throttled interval = %v, want at least %v", got, time.Second/blurThrottleFPS)
	}
}

func TestBlurPause(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, func(c *config.Config) { c.OnBlur = "pause" }).(Model)
	if m.interval() <= 0 {
		t.Fatal("a focused model should schedule ticks")
	}
	blurred, cmd := m.Update(tea.BlurMsg{})
	if cmd != nil {
		t.Error("blur should not schedule work of its own")
	}
	bm := blurred.(Model)
	if bm.interval() != 0 {
		t.Error("pause mode must not wake up while unfocused")
	}
	bm.ticking = false // the pending tick has now lapsed
	refocused, cmd := bm.Update(tea.FocusMsg{})
	if cmd == nil {
		t.Error("regaining focus should restart the loop")
	}
	if refocused.(Model).interval() <= 0 {
		t.Error("a refocused model should schedule ticks again")
	}
}

func TestFocusDoesNotDoubleTheTickChain(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil).(Model)
	m.ticking = true
	blurred, _ := m.Update(tea.BlurMsg{})
	refocused, cmd := blurred.(Model).Update(tea.FocusMsg{})
	if cmd != nil {
		t.Error("refocusing while a tick is pending must not start a second chain")
	}
	if !refocused.(Model).focused {
		t.Error("focus was not recorded")
	}
}

func TestKeypressDoesNotDoubleTheTickChain(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil).(Model)
	m.ticking = true
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd != nil {
		t.Error("a keypress while a tick is pending must not start a second chain")
	}
}

func TestBlurModeRoundTrip(t *testing.T) {
	for _, s := range []string{"run", "throttle", "pause"} {
		if got := ParseBlurMode(s).String(); got != s {
			t.Errorf("ParseBlurMode(%q).String() = %q", s, got)
		}
	}
	if ParseBlurMode("nonsense") != BlurRun {
		t.Error("an unknown blur mode should fall back to running")
	}
}

func TestPauseStopsTheClock(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil).(Model)
	paused, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	pm := paused.(Model)
	if !pm.paused {
		t.Fatal("space should pause")
	}
	if pm.interval() != 0 {
		t.Error("a paused model must not tick")
	}
	before := pm.holdElapsed
	after, _ := pm.Update(tickMsg(time.Now().Add(time.Second)))
	if after.(Model).holdElapsed != before {
		t.Error("the hold timer must not advance while paused")
	}
}

func TestStaticSceneSleepsUntilTheHoldExpires(t *testing.T) {
	reg := fixtureDeck(t)
	s, i := reg.Find("still")
	if s == nil {
		t.Fatal("still fixture missing")
	}
	cfg := config.Default()
	cfg.StatusBar = false
	cfg.Effects = nil
	var m tea.Model = New(Options{Registry: reg, Config: cfg, Mode: palette.TrueColor, Start: i, Seed: 1})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	got := m.(Model).interval()
	if got < time.Second {
		t.Errorf("a still scene with no motion effects woke after %v; it should sleep until the hold expires", got)
	}
}

func TestMotionEffectsForceATick(t *testing.T) {
	reg := fixtureDeck(t)
	s, i := reg.Find("still")
	if s == nil {
		t.Fatal("still fixture missing")
	}
	cfg := config.Default()
	cfg.StatusBar = false
	cfg.Effects = []string{"glitch"}
	var m tea.Model = New(Options{Registry: reg, Config: cfg, Mode: palette.TrueColor, Start: i, Seed: 1})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := m.(Model).interval(); got > 100*time.Millisecond {
		t.Errorf("interval = %v, want a motion-rate tick", got)
	}
}

func TestNavigationWraps(t *testing.T) {
	reg := deck(t)
	m := newModel(t, reg, 80, 24, nil).(Model)
	first := m.current().Meta.Name
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if next.(Model).current().Meta.Name == first && reg.Len() > 1 {
		t.Error("n should move to another scene")
	}
	back, _ := next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if back.(Model).current().Meta.Name != first {
		t.Error("p should return to the previous scene")
	}
	// Wrapping backwards off the front must not go negative.
	wrapped, _ := back.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if wrapped.(Model).pos < 0 {
		t.Error("wrapping backwards produced a negative position")
	}
}

func TestSpeedClamps(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil).(Model)
	var mm tea.Model = m
	for i := 0; i < 10; i++ {
		mm, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
	}
	if got := mm.(Model).speed; got != 4 {
		t.Errorf("speed = %v, want it clamped to 4", got)
	}
	for i := 0; i < 20; i++ {
		mm, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
	}
	if got := mm.(Model).speed; got != 0.25 {
		t.Errorf("speed = %v, want it clamped to 0.25", got)
	}
}

func TestEffectsKeyCycles(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil).(Model)
	start := m.effects.Intensity
	var mm tea.Model = m
	for i := 0; i < 3; i++ {
		mm, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	}
	if mm.(Model).effects.Intensity != start {
		t.Error("three presses should return to the starting intensity")
	}
}

func TestMonoModeDisablesColourAndEffects(t *testing.T) {
	reg := deck(t)
	var m tea.Model = New(Options{Registry: reg, Config: config.Default(), Mode: palette.Mono, Seed: 1})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.(Model).effects.Intensity != effect.Off {
		t.Error("mono terminals should not run effects")
	}
	if strings.Contains(m.View(), "38;2") {
		t.Error("mono mode emitted truecolor")
	}
}

func TestHelpOverlay(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	out := stripSGR(m.View())
	if !strings.Contains(out, "next scene") || !strings.Contains(out, "quit") {
		t.Errorf("help overlay missing bindings:\n%s", out)
	}
}

func TestHelpTogglesOnH(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if !m.(Model).showHelp {
		t.Fatal("'h' should open the help overlay")
	}
	if out := stripSGR(m.View()); !strings.Contains(out, "next scene") {
		t.Errorf("help overlay missing bindings:\n%s", out)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m.(Model).showHelp {
		t.Error("'h' should close the help overlay again")
	}
}

func TestStatusBarAdvertisesTheHelpKey(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil).(Model)
	if bar := stripSGR(m.statusBar(false)); !strings.Contains(bar, "h help") {
		t.Errorf("the bar should say which key opens help: %q", bar)
	}
	// A narrow bar drops the hint rather than wrapping.
	m.w = 30
	if bar := stripSGR(m.statusBar(false)); strings.Contains(bar, "help") {
		t.Errorf("a narrow bar should drop the hint: %q", bar)
	}
}

// Full-width scene rows can carry a trailing CR harmlessly, but a short line
// followed by one is erased by Bubble Tea's erase-to-end-of-line, which used to
// leave the overlay blank on screen.
func TestCentredScreensUseBareNewlines(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil).(Model)
	m.showHelp = true
	if strings.Contains(m.View(), "\r") {
		t.Error("the help overlay must not end its lines with a carriage return")
	}
	if strings.Contains(centreText(40, 12, "[ too small ]"), "\r") {
		t.Error("centreText must not end its lines with a carriage return")
	}
}

func TestEffectCycleKey(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, func(c *config.Config) {
		c.Effects = []string{"scanlines", "glitch"}
	}).(Model)
	start := m.activeEffects().Names

	var seen []string
	for range effect.Names {
		m = press(m, 'E').(Model)
		got := m.activeEffects().Names
		if len(got) != 1 {
			t.Fatalf("the cycle should select one effect at a time, got %v", got)
		}
		seen = append(seen, got[0])
		if !effect.Valid(got[0]) {
			t.Errorf("%q is not an effect", got[0])
		}
		if !strings.Contains(stripSGR(m.statusBar(false)), strings.ToUpper(got[0])) {
			t.Errorf("the bar should name the selected effect %q", got[0])
		}
	}
	if !reflect.DeepEqual(seen, effect.Names) {
		t.Errorf("the cycle should walk every effect in order: got %v, want %v", seen, effect.Names)
	}

	// One more press returns to the configured set rather than a fifth effect.
	m = press(m, 'E').(Model)
	if !reflect.DeepEqual(m.activeEffects().Names, start) {
		t.Errorf("the cycle should wrap back to the configured set: got %v, want %v",
			m.activeEffects().Names, start)
	}
}

func TestEffectCycleBeatsTheScenesOwnEffects(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil).(Model)
	// Some scenes (storm, scope, donut) carry their own effect list, which
	// the cycle has to override.
	found := false
	for i, idx := range m.order {
		if len(m.reg.Scenes[idx].Meta.Effects) > 0 {
			m.pos, found = i, true
			break
		}
	}
	if !found {
		t.Fatal("no scene in the deck sets its own effects")
	}
	s := m.current()
	if got := m.activeEffects().Names; !reflect.DeepEqual(got, s.Meta.Effects) {
		t.Fatalf("the scene's own effects should apply before any cycling: %v", got)
	}
	m = press(m, 'E').(Model)
	if got := m.activeEffects().Names; len(got) != 1 || got[0] != effect.Names[0] {
		t.Errorf("a cycled effect should override the scene's, got %v", got)
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyEsc},
		{Type: tea.KeyCtrlC},
	} {
		m := newModel(t, deck(t), 80, 24, nil)
		_, cmd := m.Update(k)
		if cmd == nil {
			t.Errorf("%v did not quit", k)
		}
	}
}

func TestHoldAdvancesTheDeck(t *testing.T) {
	reg := fixtureDeck(t) // fixtures set no hold of their own
	m := newModel(t, reg, 80, 24, func(c *config.Config) { c.HoldDuration = 100 * time.Millisecond }).(Model)
	first := m.current().Meta.Name
	var mm tea.Model = m
	now := time.Now()
	mm, _ = mm.Update(tickMsg(now))
	mm, _ = mm.Update(tickMsg(now.Add(200 * time.Millisecond)))
	if reg.Len() > 1 && mm.(Model).current().Meta.Name == first {
		t.Error("the deck should have advanced once the hold expired")
	}
}

// A scene's own hold beats the configured default, but an explicit
// --duration beats the scene.
func TestHoldPrecedence(t *testing.T) {
	reg := deck(t)
	m := newModel(t, reg, 80, 24, func(c *config.Config) { c.HoldDuration = 5 * time.Second }).(Model)
	own := m.current().Meta.Hold
	if own == 0 {
		t.Skip("no shipped scene sets its own hold")
	}
	if got := m.hold(); got != own {
		t.Errorf("hold = %v, want the scene's own %v", got, own)
	}
	forced := newModel(t, reg, 80, 24, func(c *config.Config) {
		c.HoldDuration = 3 * time.Second
		c.HoldForced = true
	}).(Model)
	if got := forced.hold(); got != 3*time.Second {
		t.Errorf("forced hold = %v, want 3s", got)
	}
}

func TestSuspendedTerminalDoesNotFastForward(t *testing.T) {
	reg := deck(t)
	m := newModel(t, reg, 80, 24, nil).(Model)
	var mm tea.Model = m
	now := time.Now()
	mm, _ = mm.Update(tickMsg(now))
	// A laptop lid closed for an hour must not burn through the whole deck.
	mm, _ = mm.Update(tickMsg(now.Add(time.Hour)))
	if got := mm.(Model).holdElapsed; got > time.Second {
		t.Errorf("holdElapsed jumped by %v after a long gap", got)
	}
}

func TestCacheIsBounded(t *testing.T) {
	c := newFrameCache(4)
	for i := 0; i < 50; i++ {
		c.put(fmt.Sprint(i), cacheEntry{render.New(2, 2), false})
	}
	if len(c.items) > 4 || len(c.order) > 4 {
		t.Errorf("cache holds %d entries, want no more than 4", len(c.items))
	}
}

func press(m tea.Model, r rune) tea.Model {
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	return out
}

func TestCarouselToggleHoldsTheScene(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, func(c *config.Config) {
		c.HoldDuration, c.HoldForced = 200*time.Millisecond, true
	}).(Model)
	start := m.pos

	held := press(m, 'a').(Model)
	if held.carousel {
		t.Fatal("'a' should have switched the carousel off")
	}
	now := time.Now()
	held, _ = mustModel(held.Update(tickMsg(now)))
	held, _ = mustModel(held.Update(tickMsg(now.Add(time.Second))))
	if held.pos != start {
		t.Errorf("scene advanced with the carousel off: %d -> %d", start, held.pos)
	}

	resumed := press(held, 'a').(Model)
	if !resumed.carousel {
		t.Fatal("'a' should have switched the carousel back on")
	}
	if resumed.holdElapsed != 0 {
		t.Error("resuming should restart the hold, not jump straight to the next scene")
	}
	resumed, _ = mustModel(resumed.Update(tickMsg(now)))
	resumed, _ = mustModel(resumed.Update(tickMsg(now.Add(time.Second))))
	if resumed.pos == start {
		t.Error("the carousel did not advance again after being switched back on")
	}
}

func mustModel(m tea.Model, c tea.Cmd) (Model, tea.Cmd) { return m.(Model), c }

// A held scene with nothing animating has no reason to wake up at all.
func TestCarouselOffStopsTicksOnAStillScene(t *testing.T) {
	reg := scene.Load(nil, []string{filepath.Join("testdata", "scenes")})
	s, i := reg.Find("still")
	if s == nil {
		t.Skip("no still fixture")
	}
	m := newModel(t, reg, 80, 24, func(c *config.Config) {
		c.StatusBar = false
		c.Effects = nil
	}).(Model)
	m.pos = i
	if m.interval() <= 0 {
		t.Fatal("the carousel should still be waiting for the hold to expire")
	}
	m.carousel = false
	m.notice = ""
	if got := m.interval(); got != 0 {
		t.Errorf("a held still scene should stop ticking, got %v", got)
	}
}

func TestCarouselOffShowsInTheStatusBar(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil).(Model)
	m.carousel = false
	bar := stripSGR(m.statusBar(false))
	if !strings.Contains(bar, glyphHold) {
		t.Errorf("status bar should flag the held carousel: %q", bar)
	}
	if strings.Contains(bar, "▮") {
		t.Errorf("the hold progress is meaningless when held: %q", bar)
	}
}

func TestNoticeBorrowsTheStatusBar(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, func(c *config.Config) { c.StatusBar = false }).(Model)
	if m.barVisible() {
		t.Fatal("the bar should be hidden")
	}
	toggled := press(m, 'a').(Model)
	if !toggled.barVisible() {
		t.Error("a notice should reveal the bar so the toggle is visible")
	}
	if !strings.Contains(stripSGR(toggled.statusBar(false)), "CAROUSEL OFF") {
		t.Error("the notice text is missing from the bar")
	}
	if toggled.interval() > noticeLife {
		t.Error("the model must wake up in time to clear its own notice")
	}
	toggled.noticeAt = time.Now().Add(-2 * noticeLife)
	if toggled.barVisible() {
		t.Error("the bar should hide again once the notice expires")
	}
}

// The notice must clear itself even when nothing else is scheduling work.
func TestNoticeWakesAPausedModel(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil).(Model)
	m.paused = true
	if m.interval() != 0 {
		t.Fatal("a paused model should be idle")
	}
	m.notify("hello")
	d := m.interval()
	if d <= 0 || d > noticeLife {
		t.Errorf("paused model should wake once to clear the notice, got %v", d)
	}
}

func TestAwakeToggleReportsItsState(t *testing.T) {
	m := newModel(t, deck(t), 80, 24, nil).(Model)
	defer m.Cleanup()
	if m.awake.On() {
		t.Fatal("the inhibitor should start off")
	}
	on := press(m, 'k').(Model)
	if awake.Supported() {
		if !on.awake.On() {
			t.Fatal("'k' should have started the inhibitor")
		}
		if !strings.Contains(stripSGR(on.statusBar(false)), "AWAKE ON") {
			t.Error("the bar should confirm the inhibitor came on")
		}
		if !strings.Contains(stripSGR(on.statusBar(false)), glyphAwake) {
			t.Error("the bar should flag the running inhibitor")
		}
		off := press(on, 'k').(Model)
		if off.awake.On() {
			t.Error("'k' should have stopped the inhibitor again")
		}
	} else if !strings.Contains(stripSGR(on.statusBar(false)), "NO SLEEP INHIBITOR") {
		t.Error("an unavailable inhibitor should say so rather than doing nothing")
	}
}

func TestKeyMapCoversEveryBinding(t *testing.T) {
	rows := DefaultKeyMap().Rows()
	want := []string{"a", "k"}
	for _, w := range want {
		found := false
		for _, r := range rows {
			if strings.HasPrefix(r[0], w+",") || r[0] == w {
				found = true
			}
		}
		if !found {
			t.Errorf("key %q is missing from the help overlay", w)
		}
	}
	seen := map[string]string{}
	for _, b := range []key.Binding{
		DefaultKeyMap().Next, DefaultKeyMap().Prev, DefaultKeyMap().Pause,
		DefaultKeyMap().Shuffle, DefaultKeyMap().Palette, DefaultKeyMap().Effects,
		DefaultKeyMap().EffectSet,
		DefaultKeyMap().Faster, DefaultKeyMap().Slower, DefaultKeyMap().Fit,
		DefaultKeyMap().StatusBar, DefaultKeyMap().Carousel, DefaultKeyMap().Awake,
		DefaultKeyMap().Help, DefaultKeyMap().Quit,
	} {
		for _, k := range b.Keys() {
			if prev, dup := seen[k]; dup {
				t.Errorf("key %q is bound twice (%s and %s)", k, prev, b.Help().Desc)
			}
			seen[k] = b.Help().Desc
		}
	}
}
