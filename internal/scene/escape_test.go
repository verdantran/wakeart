package scene

import (
	"strings"
	"testing"

	"github.com/verdantran/wakeart/internal/palette"
	"github.com/verdantran/wakeart/internal/render"
)

// paintArt renders a one-frame scene the way the carousel would. The uniform
// colour mask matters: it gives every cell the same style, so Paint emits no
// SGR between them and nothing breaks up a sequence that leaked through.
func paintArt(t *testing.T, art string) string {
	t.Helper()
	body := art + "\n~~~\n" + strings.Repeat("5", len([]rune(art))) + "\n"
	s, err := Parse("test.scene", []byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	frames, err := s.Frames()
	if err != nil {
		t.Fatalf("frames: %v", err)
	}
	var b strings.Builder
	render.Paint(frames[0], palette.All[0], palette.TrueColor, &b)
	return b.String()
}

// A scene file is just a file, and `wakeart add` invites pulling one in from
// elsewhere. Nothing in it may reach the terminal as a control sequence: OSC 52
// alone would let a scene write the user's clipboard.
func TestArtCannotDriveTheTerminal(t *testing.T) {
	art := map[string]string{
		"osc window title": "\x1b]0;pwned\x07",
		"osc 52 clipboard": "\x1b]52;c;cm0gLXJmIH4K\x07",
		"osc between csi":  "AB\x1b]0;pwned\x07CD\x1b[31mX",
		"dcs":              "\x1bP0;1|hello\x1b\\",
		"private csi":      "\x1b[?1049h\x1b[31mA",
		"csi in sgr guise": "\x1b[?25lm\x1b[31mA",
		"lone esc":         "A\x1bB\x1b[31mC",
		"del and nul":      "A\x7fB\x00C\x1b[31mD",
		"backspace":        "A\bB",
		"tab":              "A\tB",
	}
	banned := []string{"\x1b]", "\x1bP", "\x1b\\", "\x1b[?", "\x07", "\x7f", "\x00", "\b", "\t"}

	for name, in := range art {
		out := paintArt(t, in)
		for _, bad := range banned {
			if strings.Contains(out, bad) {
				t.Errorf("%s: %q reached the terminal in %q", name, bad, out)
			}
		}
	}
}

// A truncated escape sequence used to index one past the rune slice.
func TestMalformedEscapesDoNotPanic(t *testing.T) {
	for n := 0; n < 200; n++ {
		pre := strings.Repeat("a", n)
		for _, body := range []string{
			pre + "\x1b[3",
			pre + "\x1b[",
			pre + "\x1b",
			pre + "\x1b[31m",
			pre + "\x1b[;;;;m\x1b[",
			"\x1b[\n" + pre,
		} {
			s, err := Parse("test.scene", []byte(body))
			if err != nil {
				continue
			}
			f, err := s.Frames()
			if err != nil || len(f) == 0 {
				continue
			}
			var b strings.Builder
			render.Paint(f[0], palette.All[0], palette.TrueColor, &b)
			_ = render.Plain(f[0])
		}
	}
}

// The style prefix is copied onto every cell, so an unbounded one is paid for
// per glyph rather than once.
func TestSGRPrefixIsBounded(t *testing.T) {
	s, err := Parse("test.scene", []byte(strings.Repeat("\x1b[31m", 20000)+"X"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Frames()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range f[0].Cells {
		if len(c.Raw) > maxSGRPrefix {
			t.Fatalf("style prefix is %d bytes, cap is %d", len(c.Raw), maxSGRPrefix)
		}
	}
}

// Colour the file does carry must still survive, or the fix broke the feature.
func TestSGRPassthroughStillWorks(t *testing.T) {
	out := paintArt(t, "\x1b[31mRED\x1b[0m")
	if !strings.Contains(out, "\x1b[31m") {
		t.Errorf("scene colour was dropped: %q", out)
	}
	if !strings.Contains(out, "RED") {
		t.Errorf("art was dropped: %q", out)
	}
}
