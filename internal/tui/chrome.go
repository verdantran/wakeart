package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/verdantran/wakeart/internal/awake"
	"github.com/verdantran/wakeart/internal/palette"
	"github.com/verdantran/wakeart/internal/render"
)

const (
	glyphPlay    = "⏵"
	glyphPause   = "⏸"
	glyphShuffle = "⤨"
	glyphCrop    = "⊕"
	glyphHold    = "⊘"
	glyphAwake   = "☼"
)

func (m Model) barStyles() (dim, bright lipgloss.Style) {
	p := m.activePalette()
	if m.mode == palette.Mono {
		return lipgloss.NewStyle().Faint(true), lipgloss.NewStyle().Bold(true)
	}
	toHex := func(c palette.RGB) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
	return lipgloss.NewStyle().Foreground(lipgloss.Color(toHex(p.At(0.35)))),
		lipgloss.NewStyle().Foreground(lipgloss.Color(toHex(p.At(0.95))))
}

func (m Model) statusBar(cropped bool) string {
	s := m.current()
	if s == nil || m.w <= 0 {
		return ""
	}
	dim, bright := m.barStyles()

	transport := glyphPlay
	if m.paused {
		transport = glyphPause
	}

	// The notice can carry a scene name or an error string from elsewhere, so
	// it is filtered here as well as at parse time.
	label := "▸ " + strings.ToUpper(s.Meta.Name)
	if n := m.noticeText(); n != "" {
		label = "● " + strings.ToUpper(n)
	}
	label = render.SafeString(label)
	left := "  " + bright.Render(label)

	var flags []string
	if !m.carousel {
		flags = append(flags, glyphHold)
	}
	if m.awake.On() {
		flags = append(flags, glyphAwake)
	}
	if m.shuffle {
		flags = append(flags, glyphShuffle)
	}
	if cropped {
		flags = append(flags, glyphCrop)
	}
	if m.speed != 1 {
		flags = append(flags, fmt.Sprintf("%gx", m.speed))
	}
	flags = append(flags, m.activePalette().Name)

	core := fmt.Sprintf("%s  %02d/%02d  %s  %s  ",
		strings.Join(flags, " "),
		m.pos+1, len(m.order),
		m.progress(8),
		transport,
	)
	// The bar is the only place the help key can announce itself, so it goes
	// in whenever the bar is wide enough to spare the room.
	hint := ""
	if k := m.helpKey(); k != "" {
		if lipgloss.Width(left)+len(k+" help  ")+lipgloss.Width(core) <= m.w-2 {
			hint = bright.Render(k) + dim.Render(" help  ")
		}
	}
	right := hint + dim.Render(core)

	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		// Drop the name to whatever room is left rather than wrapping the bar.
		avail := m.w - lipgloss.Width(right) - 4
		if avail < 1 {
			return dim.Render(strings.Repeat(" ", m.w))
		}
		r := []rune(label)
		if len(r) > avail {
			r = r[:avail]
		}
		left = "  " + bright.Render(string(r))
		gap = m.w - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 0 {
			gap = 0
		}
	}
	return left + strings.Repeat(" ", gap) + right
}

// clamp01 is written against 0 first so NaN lands there rather than becoming a
// negative repeat count.
func clamp01(v float64) float64 {
	if !(v > 0) {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func (m Model) progress(width int) string {
	if !m.carousel {
		return strings.Repeat("▯", width)
	}
	hold := m.hold()
	p := 0.0
	if hold > 0 {
		p = float64(m.holdElapsed) / float64(hold)
	}
	filled := int(clamp01(p)*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	return strings.Repeat("▮", filled) + strings.Repeat("▯", width-filled)
}

func effectList(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, " + ")
}

// helpKey is the first key bound to the help overlay, for the bar's hint.
func (m Model) helpKey() string {
	keys := m.keys.Help.Keys()
	if len(keys) == 0 {
		return ""
	}
	if keys[0] == " " {
		return "space"
	}
	return keys[0]
}

func (m Model) helpView() string {
	dim, bright := m.barStyles()
	var b strings.Builder

	rows := m.keys.Rows()
	keyw := 0
	for _, r := range rows {
		if len(r[0]) > keyw {
			keyw = len(r[0])
		}
	}

	var lines []string
	lines = append(lines, bright.Render("wakeart"), "")
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("%s  %s",
			bright.Render(fmt.Sprintf("%*s", keyw, r[0])), dim.Render(r[1])))
	}
	carousel := "on"
	if !m.carousel {
		carousel = "off"
	}
	awakeState := "off"
	if m.awake.On() {
		awakeState = "on"
	} else if !awake.Supported() {
		awakeState = "unavailable"
	}
	lines = append(lines, "",
		dim.Render(fmt.Sprintf("scenes %s  palette %s  effects %s (%s)",
			fmt.Sprint(len(m.order)), m.activePalette().Name, m.effects.Intensity,
			effectList(m.activeEffects().Names))),
		dim.Render(fmt.Sprintf("carousel %s  awake %s (%s)",
			carousel, awakeState, awake.Name())))
	if k := m.helpKey(); k != "" {
		lines = append(lines, "", dim.Render(fmt.Sprintf("press %s to close", k)))
	}

	top := (m.h - len(lines)) / 2
	if top < 0 {
		top = 0
	}
	inner := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > inner {
			inner = w
		}
	}
	pad := (m.w - inner) / 2
	if pad < 0 {
		pad = 0
	}
	// Plain newlines only: a trailing carriage return would put the cursor
	// back at column 0 before Bubble Tea's erase-to-end-of-line, which wipes
	// the line just written. Full-width scene rows never hit that, but every
	// line here is short.
	for i := 0; i < top; i++ {
		b.WriteString("\n")
	}
	for i, l := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat(" ", pad) + l)
	}
	return b.String()
}
