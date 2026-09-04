package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/verdantran/wakeart/internal/config"
	"github.com/verdantran/wakeart/internal/palette"
	"github.com/verdantran/wakeart/internal/scene"
)

// BenchmarkStartup covers what happens before the first frame: reading the
// deck and parsing every frontmatter.
func BenchmarkStartup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		reg := scene.Load(os.DirFS("../../scenes"), nil)
		if reg.Len() == 0 {
			b.Fatal("no scenes")
		}
	}
}

// BenchmarkFrame is the per-tick cost: compose, clone, run effects, paint.
func BenchmarkFrame(b *testing.B) {
	reg := scene.Load(os.DirFS("../../scenes"), nil)
	var m tea.Model = New(Options{Registry: reg, Config: config.Default(), Mode: palette.TrueColor, Seed: 1})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	now := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m, _ = m.Update(tickMsg(now.Add(time.Duration(i) * 33 * time.Millisecond)))
		if len(m.View()) == 0 {
			b.Fatal("empty view")
		}
	}
}

// BenchmarkFrameNoEffects isolates the composition and paint cost.
func BenchmarkFrameNoEffects(b *testing.B) {
	reg := scene.Load(os.DirFS("../../scenes"), nil)
	cfg := config.Default()
	cfg.Effects = nil
	cfg.Intensity = "off"
	var m tea.Model = New(Options{Registry: reg, Config: cfg, Mode: palette.TrueColor, Seed: 1})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(m.View()) == 0 {
			b.Fatal("empty view")
		}
	}
}

func BenchmarkPaintOnly(b *testing.B) {
	reg := scene.Load(os.DirFS("../../scenes"), nil)
	var m tea.Model = New(Options{Registry: reg, Config: config.Default(), Mode: palette.TrueColor, Seed: 1})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	mm := m.(Model)
	f, _ := mm.composed()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sb strings.Builder
		sb.Grow(f.W*f.H*8 + 256)
		renderPaint(f, mm, &sb)
	}
}
