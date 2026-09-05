package config

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestRejectsBadValues(t *testing.T) {
	cases := map[string]string{
		"nan speed":          "speed = nan\n",
		"inf speed":          "speed = inf\n",
		"negative duration":  "duration = \"-5s\"\n",
		"zero duration":      "duration = \"0s\"\n",
		"unknown palette":    "palette = \"bogus\"\n",
		"unknown intensity":  "intensity = \"bogus\"\n",
		"unknown on_blur":    "on_blur = \"bogus\"\n",
		"unknown effect":     "effects = [\"bogus\"]\n",
		"unknown transition": "transitions = [\"bogus\"]\n",
	}
	for name, body := range cases {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Errorf("%s: accepted %q", name, body)
		}
	}
}

func TestAcceptsGoodValues(t *testing.T) {
	body := `duration = "5s"
speed = 2.5
palette = "acid"
intensity = "heavy"
on_blur = "stop"
effects = ["glitch", "chroma"]
transitions = ["cut", "dissolve"]
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("rejected a valid config: %v", err)
	}
	if c.HoldDuration != 5*time.Second || c.Speed != 2.5 || c.Palette != "acid" {
		t.Errorf("values not carried through: %+v", c)
	}
}

func TestRejectsDuplicateKeyBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[keys]\nnext = [\"n\"]\nprev = [\"n\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("a key bound to two actions was accepted")
	}
}

func TestAcceptsDistinctKeyBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[keys]\nnext = [\"l\", \"right\"]\nprev = [\"h\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Errorf("distinct bindings rejected: %v", err)
	}
}

// The check walks Keys by reflection so a newly added binding is covered
// without anyone remembering to list it. This asserts that coverage directly:
// every field gets a distinct key, and every pair must then collide.
func TestKeyCheckCoversEveryBinding(t *testing.T) {
	v := reflect.ValueOf(&Keys{}).Elem()
	n := v.Type().NumField()
	if n == 0 {
		t.Fatal("Keys has no fields")
	}
	for i := 0; i < n; i++ {
		name := v.Type().Field(i).Name
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			k := Keys{}
			kv := reflect.ValueOf(&k).Elem()
			kv.Field(i).Set(reflect.ValueOf([]string{"z"}))
			kv.Field(j).Set(reflect.ValueOf([]string{"z"}))
			if err := k.check(); err == nil {
				t.Errorf("a key shared by %s and %s was not reported",
					name, v.Type().Field(j).Name)
			}
		}
	}
}

// Every field needs a toml tag, or reflection skips it and the binding silently
// falls out of both the config file and the collision check.
func TestEveryKeyFieldIsTagged(t *testing.T) {
	t3 := reflect.TypeOf(Keys{})
	for i := 0; i < t3.NumField(); i++ {
		if t3.Field(i).Tag.Get("toml") == "" {
			t.Errorf("Keys.%s has no toml tag", t3.Field(i).Name)
		}
	}
}
