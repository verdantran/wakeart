package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	c := Default()
	if c.HoldDuration != 20*time.Second {
		t.Errorf("hold = %v", c.HoldDuration)
	}
	if c.Speed != 1 {
		t.Errorf("speed = %v", c.Speed)
	}
	if !c.StatusBar {
		t.Error("status bar should default on")
	}
}

func TestMissingFileIsNotAnError(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("missing config should fall back to defaults, got %v", err)
	}
	if c.Palette != "neon" {
		t.Errorf("palette = %q", c.Palette)
	}
}

func TestLoadOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`
duration   = "45s"
shuffle    = true
palette    = "acid"
speed      = 2.0
status_bar = false
effects    = ["glitch"]

[keys]
quit = ["x"]
`), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.HoldDuration != 45*time.Second {
		t.Errorf("hold = %v", c.HoldDuration)
	}
	if !c.Shuffle || c.Palette != "acid" || c.Speed != 2 || c.StatusBar {
		t.Errorf("config not applied: %+v", c)
	}
	if len(c.Effects) != 1 || c.Effects[0] != "glitch" {
		t.Errorf("effects = %v", c.Effects)
	}
	if len(c.Keys.Quit) != 1 || c.Keys.Quit[0] != "x" {
		t.Errorf("keys.quit = %v", c.Keys.Quit)
	}
}

func TestBadDurationIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(path, []byte(`duration = "forever"`), 0o644)
	if _, err := Load(path); err == nil {
		t.Error("an unparseable duration should be an error, not a silent default")
	}
}

func TestBadTOMLIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(path, []byte("this is not = = toml"), 0o644)
	if _, err := Load(path); err == nil {
		t.Error("malformed TOML should be reported")
	}
}

func TestNonPositiveSpeedFallsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(path, []byte(`speed = 0`), 0o644)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Speed != 1 {
		t.Errorf("speed = %v, want 1", c.Speed)
	}
}

func TestXDGPaths(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	if got := Dir(); got != "/tmp/xdg/wakeart" {
		t.Errorf("Dir = %q", got)
	}
	if got := SceneDir(); got != "/tmp/xdg/wakeart/scenes" {
		t.Errorf("SceneDir = %q", got)
	}
	os.Unsetenv("XDG_CONFIG_HOME")
	home, _ := os.UserHomeDir()
	if got := Dir(); got != filepath.Join(home, ".config", "wakeart") {
		t.Errorf("Dir without XDG = %q", got)
	}
}
