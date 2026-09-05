// Package tui is the Bubble Tea front end: the render loop, key handling and
// the status bar.
package tui

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/verdantran/wakeart/internal/awake"
	"github.com/verdantran/wakeart/internal/config"
	"github.com/verdantran/wakeart/internal/effect"
	"github.com/verdantran/wakeart/internal/palette"
	"github.com/verdantran/wakeart/internal/render"
	"github.com/verdantran/wakeart/internal/scene"
)

const (
	minWidth  = 20
	minHeight = 10
	maxFPS    = 30
)

type tickMsg time.Time

// BlurMode decides what happens when the terminal loses focus. Art on a second
// monitor should keep playing, so running is the default; the quieter modes are
// there for laptops on battery.
type BlurMode int

const (
	BlurRun BlurMode = iota
	BlurThrottle
	BlurPause
)

const blurThrottleFPS = 2

// noticeLife is how long a toggle's confirmation sits in the status bar.
const noticeLife = 2500 * time.Millisecond

func ParseBlurMode(s string) BlurMode {
	switch strings.ToLower(s) {
	case "throttle":
		return BlurThrottle
	case "pause", "stop":
		return BlurPause
	}
	return BlurRun
}

func (b BlurMode) String() string {
	switch b {
	case BlurThrottle:
		return "throttle"
	case BlurPause:
		return "pause"
	}
	return "run"
}

type Options struct {
	Registry  *scene.Registry
	Config    config.Config
	Mode      palette.Mode
	Start     int
	Seed      int64
	NoEffects bool
}

type Model struct {
	reg  *scene.Registry
	cfg  config.Config
	keys KeyMap
	mode palette.Mode

	order []int
	pos   int

	w, h     int
	paused   bool
	focused  bool
	shuffle  bool
	showBar  bool
	carousel bool
	showHelp bool
	fitMode  bool
	speed    float64

	palIdx      int
	palOverride bool
	effects     effect.Set
	effState    *effect.State
	// effSel is 0 for the configured (or scene's own) set, otherwise the
	// 1-based index into effect.Names of the single effect being previewed.
	effSel int

	animElapsed time.Duration
	holdElapsed time.Duration
	lastTick    time.Time

	from       *render.Frame
	trans      render.Transition
	transTicks int
	transTotal int

	onBlur      BlurMode
	awake       *awake.Keeper
	notice      string
	noticeAt    time.Time
	ticking     bool
	seed        int64
	transitions []render.Transition
	rng         *rand.Rand
	cache       *frameCache
	quitting    bool
}

func New(o Options) Model {
	keys := DefaultKeyMap()
	keys.Apply(o.Config.Keys)

	pal := 0
	if i := indexOfPalette(o.Config.Palette); i >= 0 {
		pal = i
	}
	if o.Mode == palette.Mono {
		pal = indexOfPalette("mono")
	}

	in := effect.ParseIntensity(o.Config.Intensity)
	if o.NoEffects || o.Mode == palette.Mono {
		in = effect.Off
	}

	var trans []render.Transition
	for _, s := range o.Config.Transitions {
		if render.ValidTransition(s) {
			trans = append(trans, render.Transition(s))
		}
	}
	if len(trans) == 0 {
		trans = []render.Transition{render.Cut}
	}

	m := Model{
		reg:         o.Registry,
		cfg:         o.Config,
		keys:        keys,
		mode:        o.Mode,
		paused:      false,
		focused:     true,
		shuffle:     o.Config.Shuffle,
		showBar:     o.Config.StatusBar,
		carousel:    o.Config.Carousel,
		awake:       awake.New(),
		speed:       o.Config.Speed,
		palIdx:      pal,
		effects:     effect.Set{Names: o.Config.Effects, Intensity: in},
		effState:    effect.NewState(o.Seed),
		onBlur:      ParseBlurMode(o.Config.OnBlur),
		transitions: trans,
		seed:        o.Seed,
		rng:         rand.New(rand.NewSource(o.Seed)),
		cache:       newFrameCache(64),
	}
	m.buildOrder(o.Start)
	// Must agree with what Init will schedule; claiming a pending tick that
	// never arrives would wedge every later keypress on the ticking check.
	m.ticking = m.interval() > 0
	if o.Config.Awake {
		if err := m.awake.Start(); err != nil {
			m.notify(awakeMessage(false, err))
		}
	}
	return m
}

// Cleanup releases anything the model holds outside the terminal.
func (m Model) Cleanup() { m.awake.Stop() }

func (m *Model) notify(s string) { m.notice, m.noticeAt = s, time.Now() }

func (m Model) noticeLeft() time.Duration {
	if m.notice == "" {
		return 0
	}
	if r := noticeLife - time.Since(m.noticeAt); r > 0 {
		return r
	}
	return 0
}

func (m Model) noticeText() string {
	if m.noticeLeft() == 0 {
		return ""
	}
	return m.notice
}

func awakeMessage(on bool, err error) string {
	switch {
	case errors.Is(err, awake.ErrUnsupported):
		return "no sleep inhibitor available here"
	case err != nil:
		return "sleep inhibitor failed: " + err.Error()
	case on:
		return "awake on — system will not sleep"
	}
	return "awake off"
}

func indexOfPalette(name string) int {
	for i, p := range palette.All {
		if p.Name == name {
			return i
		}
	}
	return -1
}

func (m *Model) buildOrder(start int) {
	n := m.reg.Len()
	m.order = make([]int, n)
	for i := range m.order {
		m.order[i] = i
	}
	if m.shuffle {
		m.rng.Shuffle(n, func(i, j int) { m.order[i], m.order[j] = m.order[j], m.order[i] })
	}
	m.pos = 0
	for i, idx := range m.order {
		if idx == start {
			m.pos = i
			break
		}
	}
}

// Init cannot record anything: bubbletea keeps the model it was handed, and
// this receiver is a copy. It schedules the first tick directly, and New sets
// ticking to match so the two agree from the start.
func (m Model) Init() tea.Cmd {
	d := m.interval()
	if d <= 0 {
		return nil
	}
	return schedule(d)
}

func (m Model) current() *scene.Scene {
	if len(m.order) == 0 {
		return nil
	}
	return m.reg.Scenes[m.order[m.pos%len(m.order)]]
}

func (m Model) hold() time.Duration {
	if m.cfg.HoldForced {
		return m.cfg.HoldDuration // an explicit --duration beats every scene
	}
	if s := m.current(); s != nil && s.Meta.Hold > 0 {
		return s.Meta.Hold
	}
	return m.cfg.HoldDuration
}

func (m Model) activePalette() palette.Palette {
	if !m.palOverride {
		if s := m.current(); s != nil && s.Meta.Palette != "" && m.mode != palette.Mono {
			if p, ok := palette.Get(s.Meta.Palette); ok {
				return p
			}
		}
	}
	return palette.All[m.palIdx]
}

func (m Model) activeEffects() effect.Set {
	// A single effect picked with the cycle key beats both the scene and the
	// config, the way a cycled palette does.
	if m.effSel > 0 {
		return effect.Set{Names: []string{effect.Names[m.effSel-1]}, Intensity: m.effects.Intensity}
	}
	s := m.current()
	if s != nil && len(s.Meta.Effects) > 0 {
		return effect.Set{Names: s.Meta.Effects, Intensity: m.effects.Intensity}
	}
	return m.effects
}

// effectMessage names what the effect cycle just selected, for the status bar.
func (m Model) effectMessage() string {
	names := m.activeEffects().Names
	what := "none"
	if m.effSel > 0 {
		what = effect.Names[m.effSel-1]
	} else if len(names) > 0 {
		what = strings.Join(names, " + ")
	}
	if m.effects.Intensity == effect.Off {
		return "effects " + what + " — intensity off"
	}
	return "effects " + what
}

// interval decides the next wake-up. A static scene with no motion effects
// sleeps until the hold expires rather than ticking for nothing.
func (m Model) interval() time.Duration {
	if m.quitting {
		return 0
	}
	if m.paused || (!m.focused && m.onBlur == BlurPause) {
		return m.noticeLeft() // still wake once, to clear a toggle's notice
	}
	fps := 0.0
	if m.transTotal > 0 {
		fps = maxFPS
	}
	if s := m.current(); s != nil {
		if s.Procedural() {
			if f := s.Meta.FPS * m.speed; f > fps {
				fps = f
			}
		}
		if frames, err := s.Frames(); err == nil && len(frames) > 1 {
			f := s.Meta.FPS * m.speed
			if s.Meta.Loop == scene.Once && m.animElapsed.Seconds()*f > float64(len(frames)) {
				f = 0
			}
			if f > fps {
				fps = f
			}
		}
	}
	e := m.activeEffects()
	if e.Has("glitch") || e.Has("flicker") {
		if fps < 12 {
			fps = 12
		}
	}
	// The bar's only moving part is the hold progress, which is frozen when
	// the carousel is off.
	if m.showBar && m.carousel && fps < 4 {
		fps = 4
	}
	if m.noticeLeft() > 0 && fps < 4 {
		fps = 4
	}
	if fps <= 0 {
		if !m.carousel {
			return 0 // nothing moves and nothing is due to advance
		}
		remaining := m.hold() - m.holdElapsed
		if remaining < 50*time.Millisecond {
			remaining = 50 * time.Millisecond
		}
		return remaining
	}
	if !m.focused && m.onBlur == BlurThrottle && fps > blurThrottleFPS {
		fps = blurThrottleFPS
	}
	if fps > maxFPS {
		fps = maxFPS
	}
	return time.Duration(float64(time.Second) / fps)
}

// tick schedules the next wake-up and records whether the loop is still
// running, so regaining focus never starts a second concurrent tick chain.
func (m *Model) tick() tea.Cmd {
	d := m.interval()
	if d <= 0 {
		m.ticking = false
		return nil
	}
	m.ticking = true
	return schedule(d)
}

func schedule(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) advance(delta int) {
	if len(m.order) == 0 {
		return
	}
	if m.transTotal == 0 {
		t := m.transitions[m.rng.Intn(len(m.transitions))]
		// A Cut draws nothing from the outgoing scene, so holding on to it
		// would pin a frame until the next resize.
		if n := t.Frames(maxFPS); n > 0 {
			if f, _ := m.composed(); f != nil {
				m.from, m.trans, m.transTotal, m.transTicks = f, t, n, 0
			}
		}
	}
	m.pos = ((m.pos+delta)%len(m.order) + len(m.order)) % len(m.order)
	m.animElapsed = 0
	m.holdElapsed = 0
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.cache.reset()
		m.from = nil
		m.transTotal = 0
		return m, nil

	case tea.FocusMsg:
		if !m.focused {
			m.focused = true
			m.lastTick = time.Now()
			if !m.ticking {
				return m, m.tick()
			}
		}
		return m, nil

	case tea.BlurMsg:
		m.focused = false
		// The pending tick carries the chain onward; only the pause mode
		// lets it lapse, which m.tick decides on the next wake-up.
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tickMsg:
		now := time.Time(msg)
		if !m.lastTick.IsZero() && !m.paused {
			d := now.Sub(m.lastTick)
			if d > time.Second {
				d = time.Second // a suspended terminal must not fast-forward the deck
			}
			if d < 0 {
				d = 0 // nor may a clock that steps backwards rewind it
			}
			m.animElapsed += d
			m.holdElapsed += d
		}
		m.lastTick = now

		e := m.activeEffects()
		m.effState.Tick(e, maxFPS)

		if m.transTotal > 0 {
			m.transTicks++
			if m.transTicks >= m.transTotal {
				m.transTotal, m.transTicks = 0, 0
				m.from = nil
			}
		}
		if m.carousel && m.holdElapsed >= m.hold() {
			m.advance(1)
		}
		return m, m.tick()
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.quitting = true
		m.awake.Stop()
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp

	case key.Matches(msg, m.keys.Pause):
		m.paused = !m.paused
		m.lastTick = time.Now()

	case key.Matches(msg, m.keys.Next):
		m.advance(1)

	case key.Matches(msg, m.keys.Prev):
		m.advance(-1)

	case key.Matches(msg, m.keys.Shuffle):
		if len(m.order) == 0 {
			break
		}
		m.shuffle = !m.shuffle
		cur := m.order[m.pos]
		m.buildOrder(cur)

	case key.Matches(msg, m.keys.Palette):
		m.palOverride = true
		m.palIdx = (m.palIdx + 1) % len(palette.All)

	case key.Matches(msg, m.keys.Effects):
		m.effects.Intensity = m.effects.Intensity.Next()
		m.notify("effect intensity " + m.effects.Intensity.String())

	case key.Matches(msg, m.keys.EffectSet):
		m.effSel = (m.effSel + 1) % (len(effect.Names) + 1)
		m.notify(m.effectMessage())

	case key.Matches(msg, m.keys.Faster):
		m.speed = clampSpeed(m.speed * 2)

	case key.Matches(msg, m.keys.Slower):
		m.speed = clampSpeed(m.speed / 2)

	case key.Matches(msg, m.keys.Fit):
		m.fitMode = !m.fitMode

	case key.Matches(msg, m.keys.StatusBar):
		m.showBar = !m.showBar
		// The bar owns a row, so the viewport just changed height and a
		// half-finished transition is holding a frame of the old size.
		m.from = nil
		m.transTotal = 0

	case key.Matches(msg, m.keys.Carousel):
		m.carousel = !m.carousel
		if m.carousel {
			m.holdElapsed = 0
			m.notify("carousel on")
		} else {
			m.notify("carousel off — holding on this scene")
		}

	case key.Matches(msg, m.keys.Awake):
		on, err := m.awake.Toggle()
		m.notify(awakeMessage(on, err))
	}
	if !m.paused {
		m.lastTick = time.Now()
	}
	if m.ticking {
		return m, nil // a tick is already pending; a second would double the rate
	}
	return m, m.tick()
}

func clampSpeed(v float64) float64 {
	switch {
	case v < 0.25:
		return 0.25
	case v > 4:
		return 4
	}
	return v
}

// barVisible lets a notice borrow the status bar for a moment, so a toggle
// pressed with the bar hidden still shows what it did.
func (m Model) barVisible() bool { return m.showBar || m.noticeText() != "" }

func (m Model) viewport() (int, int) {
	h := m.h
	if m.barVisible() {
		h--
	}
	return m.w, h
}

// composed returns the current scene fitted to the viewport, before effects.
func (m Model) composed() (*render.Frame, bool) {
	s := m.current()
	if s == nil {
		return nil, false
	}
	w, h := m.viewport()
	if w <= 0 || h <= 0 {
		return nil, false
	}
	// Procedural scenes draw straight into the viewport and change every
	// frame, so there is nothing to fit and nothing worth caching.
	if s.Procedural() {
		return s.Render(m.animElapsed, m.speed, w, h), false
	}
	idx := s.FrameIndexAt(m.animElapsed, m.speed)
	ck := fmt.Sprintf("%d:%d:%dx%d:%v", m.order[m.pos], idx, w, h, m.fitMode)
	if e, ok := m.cache.get(ck); ok {
		return e.frame, e.cropped
	}
	frames, err := s.Frames()
	if err != nil || idx >= len(frames) {
		return nil, false
	}
	src := frames[idx]
	if m.fitMode {
		src = render.Scale(src, w, h)
	}
	fitted, cropped := render.Fit(src, w, h, s.Meta.Align)
	m.cache.put(ck, cacheEntry{fitted, cropped})
	return fitted, cropped
}

// blendRNG is derived rather than drawn from m.rng: View runs on a copy of the
// model at moments bubbletea picks, so consuming the shared stream there would
// make a --seed run unreproducible. Keyed on the tick, the same transition
// frame redraws identically.
func (m Model) blendRNG() *rand.Rand {
	return rand.New(rand.NewSource(m.seed + int64(m.transTicks)*2654435761))
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.w == 0 || m.h == 0 {
		return ""
	}
	if m.w < minWidth || m.h < minHeight {
		return centreText(m.w, m.h, "[ too small ]")
	}
	if m.showHelp {
		return m.helpView()
	}

	frame, cropped := m.composed()
	if frame == nil {
		return centreText(m.w, m.h, "[ no scenes ]")
	}
	if m.transTotal > 0 && m.from != nil {
		p := float64(m.transTicks) / float64(m.transTotal)
		frame = render.Blend(m.from, frame, m.trans, p, m.blendRNG())
	}

	out := frame.Clone()
	m.activeEffects().Apply(out, m.effState)

	var b strings.Builder
	b.Grow(out.W*out.H*8 + 256)
	render.Paint(out, m.activePalette(), m.mode, &b)
	if m.barVisible() {
		b.WriteString("\r\n")
		b.WriteString(m.statusBar(cropped))
	}
	return b.String()
}

func centreText(w, h int, s string) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	pad := (w - len([]rune(s))) / 2
	if pad < 0 {
		pad = 0
	}
	var b strings.Builder
	for i := 0; i < h/2; i++ {
		b.WriteString("\n") // see helpView: a trailing CR would erase the line
	}
	b.WriteString(strings.Repeat(" ", pad) + s)
	return b.String()
}

// renderPaint exists so benchmarks can measure the paint step on its own.
func renderPaint(f *render.Frame, m Model, b *strings.Builder) {
	render.Paint(f, m.activePalette(), m.mode, b)
}
