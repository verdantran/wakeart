// Package config merges defaults, the TOML file and flags, in that order.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/verdantran/wakeart/internal/effect"
	"github.com/verdantran/wakeart/internal/palette"
	"github.com/verdantran/wakeart/internal/render"
)

type Keys struct {
	Next      []string `toml:"next"`
	Prev      []string `toml:"prev"`
	Pause     []string `toml:"pause"`
	Shuffle   []string `toml:"shuffle"`
	Palette   []string `toml:"palette"`
	Effects   []string `toml:"effects"`
	EffectSet []string `toml:"effect_set"`
	Faster    []string `toml:"faster"`
	Slower    []string `toml:"slower"`
	Fit       []string `toml:"fit"`
	StatusBar []string `toml:"status_bar"`
	Carousel  []string `toml:"carousel"`
	Awake     []string `toml:"awake"`
	Help      []string `toml:"help"`
	Quit      []string `toml:"quit"`
}

type Config struct {
	Duration    string   `toml:"duration"`
	Shuffle     bool     `toml:"shuffle"`
	Palette     string   `toml:"palette"`
	Effects     []string `toml:"effects"`
	Intensity   string   `toml:"intensity"`
	Speed       float64  `toml:"speed"`
	StatusBar   bool     `toml:"status_bar"`
	Carousel    bool     `toml:"carousel"`
	Awake       bool     `toml:"awake"`
	OnBlur      string   `toml:"on_blur"`
	Transitions []string `toml:"transitions"`
	SceneDirs   []string `toml:"scene_dirs"`
	Keys        Keys     `toml:"keys"`

	HoldDuration time.Duration `toml:"-"`
	// HoldForced is set when --duration was given on the command line. A
	// scene's own hold overrides the configured default, but not a duration
	// the user just typed.
	HoldForced bool `toml:"-"`
}

func Default() Config {
	return Config{
		Duration:    "20s",
		Shuffle:     false,
		Palette:     "neon",
		Effects:     []string{"scanlines", "glitch", "chroma"},
		Intensity:   "subtle",
		Speed:       1.0,
		StatusBar:   true,
		Carousel:    true,
		Awake:       false,
		OnBlur:      "run",
		Transitions: []string{"cut", "scanwipe", "glitchcut"},

		HoldDuration: 20 * time.Second,
	}
}

// Dir is the config root, honouring XDG_CONFIG_HOME.
func Dir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "wakeart")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".wakeart"
	}
	return filepath.Join(home, ".config", "wakeart")
}

func SceneDir() string { return filepath.Join(Dir(), "scenes") }
func Path() string     { return filepath.Join(Dir(), "config.toml") }

// Load reads the config file if present. A missing file is not an error.
func Load(path string) (Config, error) {
	c := Default()
	if path == "" {
		path = Path()
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := toml.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if c.Duration != "" {
		d, err := time.ParseDuration(c.Duration)
		if err != nil {
			return c, fmt.Errorf("%s: duration: %w", path, err)
		}
		c.HoldDuration = d
	}
	if err := c.validate(); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// validate rejects what the flag parser would reject. Without it a typo in the
// file is silent: an unknown palette falls back to the default, an unknown
// effect never runs, and the user is left wondering.
func (c *Config) validate() error {
	// NaN and Inf slip past a `<= 0` test and then poison every duration
	// computed from them.
	if math.IsNaN(c.Speed) || math.IsInf(c.Speed, 0) {
		return fmt.Errorf("speed: %v is not a usable multiplier", c.Speed)
	}
	if c.Speed <= 0 {
		c.Speed = 1
	}
	if c.HoldDuration <= 0 {
		return fmt.Errorf("duration: %s must be positive", c.HoldDuration)
	}
	if _, ok := palette.Get(c.Palette); !ok {
		return fmt.Errorf("unknown palette %q (have: %s)", c.Palette, strings.Join(palette.Names(), ", "))
	}
	switch strings.ToLower(c.Intensity) {
	case "off", "none", "subtle", "heavy":
	default:
		return fmt.Errorf("unknown intensity %q (have: off, subtle, heavy)", c.Intensity)
	}
	switch strings.ToLower(c.OnBlur) {
	case "run", "throttle", "pause", "stop":
	default:
		return fmt.Errorf("unknown on_blur %q (have: run, throttle, pause)", c.OnBlur)
	}
	for _, n := range c.Effects {
		if !effect.Valid(n) {
			return fmt.Errorf("unknown effect %q (have: %s)", n, strings.Join(effect.Names, ", "))
		}
	}
	for _, t := range c.Transitions {
		if !render.ValidTransition(t) {
			return fmt.Errorf("unknown transition %q", t)
		}
	}
	return c.Keys.check()
}

// check catches a key bound to two actions. Nothing downstream would report
// it: the key handler is a switch, so the first arm silently wins and one of
// the two rebindings just does nothing.
func (k Keys) check() error {
	owner := map[string]string{}
	for _, b := range []struct {
		action string
		keys   []string
	}{
		{"next", k.Next}, {"prev", k.Prev}, {"pause", k.Pause},
		{"shuffle", k.Shuffle}, {"palette", k.Palette}, {"effects", k.Effects},
		{"effect_set", k.EffectSet},
		{"faster", k.Faster}, {"slower", k.Slower}, {"fit", k.Fit},
		{"status_bar", k.StatusBar}, {"carousel", k.Carousel}, {"awake", k.Awake},
		{"help", k.Help}, {"quit", k.Quit},
	} {
		for _, key := range b.keys {
			if prev, ok := owner[key]; ok {
				return fmt.Errorf("keys: %q is bound to both %s and %s", key, prev, b.action)
			}
			owner[key] = b.action
		}
	}
	return nil
}
