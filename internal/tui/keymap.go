package tui

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/verdantran/wakeart/internal/config"
)

type KeyMap struct {
	Next      key.Binding
	Prev      key.Binding
	Pause     key.Binding
	Shuffle   key.Binding
	Palette   key.Binding
	Effects   key.Binding
	EffectSet key.Binding
	Faster    key.Binding
	Slower    key.Binding
	Fit       key.Binding
	StatusBar key.Binding
	Carousel  key.Binding
	Awake     key.Binding
	Help      key.Binding
	Quit      key.Binding
}

func bind(keys []string, help string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(keys[0], help))
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Next:      bind([]string{"n", "right", "l"}, "next scene"),
		Prev:      bind([]string{"p", "left"}, "previous scene"),
		Pause:     bind([]string{" "}, "pause / resume"),
		Shuffle:   bind([]string{"s"}, "toggle shuffle"),
		Palette:   bind([]string{"c"}, "cycle palette"),
		Effects:   bind([]string{"e"}, "cycle effect intensity"),
		EffectSet: bind([]string{"E", "x"}, "cycle which effect runs"),
		Faster:    bind([]string{"+", "="}, "speed up"),
		Slower:    bind([]string{"-", "_"}, "slow down"),
		Fit:       bind([]string{"f"}, "toggle fit"),
		StatusBar: bind([]string{"i"}, "toggle status bar"),
		Carousel:  bind([]string{"a"}, "toggle carousel (auto-advance)"),
		Awake:     bind([]string{"k"}, "keep the system awake"),
		Help:      bind([]string{"h", "?"}, "toggle this help"),
		Quit:      bind([]string{"q", "esc", "ctrl+c"}, "quit"),
	}
}

// Apply overlays user-configured keys; an empty list keeps the default.
func (k *KeyMap) Apply(c config.Keys) {
	set := func(b *key.Binding, keys []string) {
		if len(keys) == 0 {
			return
		}
		help := b.Help()
		*b = key.NewBinding(key.WithKeys(keys...), key.WithHelp(keys[0], help.Desc))
	}
	set(&k.Next, c.Next)
	set(&k.Prev, c.Prev)
	set(&k.Pause, c.Pause)
	set(&k.Shuffle, c.Shuffle)
	set(&k.Palette, c.Palette)
	set(&k.Effects, c.Effects)
	set(&k.EffectSet, c.EffectSet)
	set(&k.Faster, c.Faster)
	set(&k.Slower, c.Slower)
	set(&k.Fit, c.Fit)
	set(&k.StatusBar, c.StatusBar)
	set(&k.Carousel, c.Carousel)
	set(&k.Awake, c.Awake)
	set(&k.Help, c.Help)
	set(&k.Quit, c.Quit)
}

// Rows drives the help overlay, so the overlay cannot drift from the bindings.
func (k KeyMap) Rows() [][2]string {
	all := []key.Binding{
		k.Pause, k.Next, k.Prev, k.Carousel, k.Shuffle, k.Palette, k.Effects,
		k.EffectSet, k.Faster, k.Slower, k.Fit, k.StatusBar, k.Awake, k.Help, k.Quit,
	}
	rows := make([][2]string, 0, len(all))
	for _, b := range all {
		keys := b.Keys()
		label := ""
		for i, s := range keys {
			if i > 0 {
				label += ", "
			}
			if s == " " {
				s = "space"
			}
			label += s
		}
		rows = append(rows, [2]string{label, b.Help().Desc})
	}
	return rows
}
