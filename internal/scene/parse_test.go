package scene

import (
	"strings"
	"testing"
	"time"
)

func TestParseMalformed(t *testing.T) {
	// Every one of these must degrade rather than panic.
	cases := []struct {
		name, in string
		wantErr  bool
	}{
		{"empty", "", true},
		{"whitespace only", "   \n\n  \n", true},
		{"bad frontmatter", "---\nname = \nfps = \n---\nart\n", true},
		{"bad hold", "---\nhold = \"forever\"\n---\nart\n", true},
		{"unterminated frontmatter", "---\nname = \"x\"\nart here\n", false},
		{"no frontmatter", "  ▄▄▄\n  ███\n", false},
		{"frontmatter only field", "---\nname = \"X\"\n---\nart\n", false},
		{"ragged frames", "---\n---\nab\ncdefgh\n===\nz\n", false},
		{"mask mismatch", "---\n---\nabc\ndef\n~~~\n9\n", false},
		{"trailing delimiter", "---\n---\nart\n===\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := Parse("test.scene", []byte(c.in))
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got a scene")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, err := s.Frames(); err != nil {
				t.Fatalf("frames: %v", err)
			}
		})
	}
}

func TestParseDefaults(t *testing.T) {
	s, err := Parse("scenes/neon-skyline.scene", []byte("art\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Meta.Name != "Neon Skyline" {
		t.Errorf("name from filename = %q", s.Meta.Name)
	}
	if s.Meta.FPS != 10 {
		t.Errorf("default fps = %v", s.Meta.FPS)
	}
	if s.Meta.Loop != Forward {
		t.Errorf("default loop = %v", s.Meta.Loop)
	}
}

func TestParseFrontmatter(t *testing.T) {
	in := `---
name    = "Test"
palette = "acid"
fps     = 8
loop    = "pingpong"
hold    = "30s"
align   = "topleft"
effects = ["scanlines"]
tags    = ["a", "b"]
---
one
===
two
===
three
`
	s, err := Parse("t.scene", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if s.Meta.Hold != 30*time.Second {
		t.Errorf("hold = %v", s.Meta.Hold)
	}
	if s.Meta.Loop != PingPong {
		t.Errorf("loop = %v", s.Meta.Loop)
	}
	fs, err := s.Frames()
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 3 {
		t.Fatalf("frames = %d, want 3", len(fs))
	}
	if s.FrameCount() != 3 {
		t.Errorf("FrameCount = %d, want 3", s.FrameCount())
	}
}

func TestRaggedFramesPad(t *testing.T) {
	s, _ := Parse("t.scene", []byte("ab\ncdefgh\ni\n"))
	fs, err := s.Frames()
	if err != nil {
		t.Fatal(err)
	}
	if fs[0].W != 6 {
		t.Errorf("width = %d, want 6 (padded to widest)", fs[0].W)
	}
}

func TestPingPong(t *testing.T) {
	s, _ := Parse("t.scene", []byte("---\nfps = 1\nloop = \"pingpong\"\n---\na\n===\nb\n===\nc\n"))
	want := []int{0, 1, 2, 1, 0, 1, 2}
	for i, w := range want {
		if got := s.FrameIndexAt(time.Duration(i)*time.Second, 1); got != w {
			t.Errorf("t=%ds index = %d, want %d", i, got, w)
		}
	}
}

func TestLoopOnceClamps(t *testing.T) {
	s, _ := Parse("t.scene", []byte("---\nfps = 1\nloop = \"once\"\n---\na\n===\nb\n"))
	if got := s.FrameIndexAt(60*time.Second, 1); got != 1 {
		t.Errorf("index = %d, want 1 (clamped to last)", got)
	}
}

func TestANSIPassthrough(t *testing.T) {
	in := "---\n---\n\x1b[31mRED\x1b[0m plain\n"
	s, err := Parse("t.scene", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if !s.HasANSI() || s.ColourPath() != "ansi passthrough" {
		t.Fatalf("colour path = %q", s.ColourPath())
	}
	fs, _ := s.Frames()
	f := fs[0]
	if f.W != len("RED plain") {
		t.Errorf("width = %d, want %d (escapes must not count)", f.W, len("RED plain"))
	}
	if f.At(0, 0).Raw != "\x1b[31m" {
		t.Errorf("first cell raw = %q", f.At(0, 0).Raw)
	}
	if f.At(4, 0).Raw != "" {
		t.Errorf("cell after reset raw = %q, want empty", f.At(4, 0).Raw)
	}
}

func TestColourMask(t *testing.T) {
	s, _ := Parse("t.scene", []byte("---\n---\nabc\n~~~\n090\n"))
	if s.ColourPath() != "colour mask" {
		t.Fatalf("colour path = %q", s.ColourPath())
	}
	fs, _ := s.Frames()
	if got := fs[0].At(1, 0).Lvl; got != 255 {
		t.Errorf("masked level = %d, want 255", got)
	}
	if got := fs[0].At(0, 0).Lvl; got != 0 {
		t.Errorf("masked level = %d, want 0", got)
	}
}

func TestCRLF(t *testing.T) {
	s, err := Parse("t.scene", []byte("---\r\nname = \"X\"\r\n---\r\nart\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Meta.Name != "X" {
		t.Errorf("name = %q", s.Meta.Name)
	}
}

func TestSlugAndFind(t *testing.T) {
	s, _ := Parse("embedded:skyline", []byte("art\n"))
	r := &Registry{Scenes: []*Scene{s}}
	if got := SlugOf(s); got != "skyline" {
		t.Errorf("slug = %q", got)
	}
	if f, _ := r.Find("SKYLINE"); f == nil {
		t.Error("Find should be case-insensitive on the slug")
	}
	if f, _ := r.Find("Skyline"); f == nil {
		t.Error("Find should match the display name too")
	}
	if f, _ := r.Find("nope"); f != nil {
		t.Error("Find matched a scene that does not exist")
	}
}

func TestFilterTags(t *testing.T) {
	a, _ := Parse("a", []byte("---\ntags = [\"city\"]\n---\nx\n"))
	b, _ := Parse("b", []byte("---\ntags = [\"tech\"]\n---\nx\n"))
	r := &Registry{Scenes: []*Scene{a, b}}
	r.FilterTags([]string{"CITY"})
	if r.Len() != 1 || !strings.Contains(r.Scenes[0].Meta.Source, "a") {
		t.Errorf("filter kept %d scenes", r.Len())
	}
}
