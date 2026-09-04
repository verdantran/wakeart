// Package config merges defaults, the TOML file and flags, in that order.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Keys struct {
	Next      []string `toml:"next"`
	Prev      []string `toml:"prev"`
	Pause     []string `toml:"pause"`
	Shuffle   []string `toml:"shuffle"`
	Palette   []string `toml:"palette"`
	Effects   []string `toml:"effects"`
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
	if c.Speed <= 0 {
		c.Speed = 1
	}
	return c, nil
}
