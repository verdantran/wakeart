package effect

import (
	"strings"
	"testing"

	"github.com/verdantran/wakeart/internal/palette"
	"github.com/verdantran/wakeart/internal/render"
)

func testFrame() *render.Frame {
	var rows []string
	for i := 0; i < 12; i++ {
		rows = append(rows, strings.Repeat("█▓▒░ @#", 9))
	}
	return render.FromLines(rows, nil)
}

func paint(f *render.Frame) string {
	var b strings.Builder
	render.Paint(f, palette.All[0], palette.TrueColor, &b)
	return b.String()
}

// A fixed seed must replay identically, which is what makes the effects
// testable at all.
func TestDeterministic(t *testing.T) {
	set := Set{Names: Names, Intensity: Heavy}
	run := func() string {
		st := NewState(42)
		var out strings.Builder
		for i := 0; i < 200; i++ {
			st.Tick(set, 30)
			f := testFrame()
			set.Apply(f, st)
			out.WriteString(paint(f))
		}
		return out.String()
	}
	if a, b := run(), run(); a != b {
		t.Fatal("same seed produced different output")
	}
}

func TestDifferentSeedsDiverge(t *testing.T) {
	set := Set{Names: Names, Intensity: Heavy}
	run := func(seed int64) string {
		st := NewState(seed)
		var out strings.Builder
		for i := 0; i < 200; i++ {
			st.Tick(set, 30)
			f := testFrame()
			set.Apply(f, st)
			out.WriteString(paint(f))
		}
		return out.String()
	}
	if run(1) == run(2) {
		t.Error("different seeds produced identical output")
	}
}

func TestOffIsANoOp(t *testing.T) {
	set := Set{Names: Names, Intensity: Off}
	st := NewState(1)
	want := paint(testFrame())
	for i := 0; i < 50; i++ {
		st.Tick(set, 30)
		f := testFrame()
		set.Apply(f, st)
		if got := paint(f); got != want {
			t.Fatal("intensity off changed the frame")
		}
	}
}

func TestScanlinesDimAlternateRows(t *testing.T) {
	f := testFrame()
	Set{Names: []string{"scanlines"}, Intensity: Subtle}.Apply(f, NewState(1))
	if f.At(0, 0).Dim != 0 {
		t.Errorf("row 0 dim = %d, want 0", f.At(0, 0).Dim)
	}
	if f.At(0, 1).Dim == 0 {
		t.Error("row 1 should be dimmed")
	}
	if f.At(0, 2).Dim != 0 {
		t.Errorf("row 2 dim = %d, want 0", f.At(0, 2).Dim)
	}
}

func TestChromaOnlyGhostsIntoBlanks(t *testing.T) {
	f := render.FromLines([]string{" █ "}, nil)
	Set{Names: []string{"chroma"}, Intensity: Subtle}.Apply(f, NewState(1))
	if !f.At(0, 0).HasFix || !f.At(2, 0).HasFix {
		t.Error("bright glyph should ghost into both neighbouring blanks")
	}
	if f.At(1, 0).HasFix {
		t.Error("the source glyph itself must keep its ramp colour")
	}
}

func TestChromaLeavesANSIPassthroughAlone(t *testing.T) {
	f := render.New(3, 1)
	f.Set(1, 0, render.Cell{R: '█', Lvl: 255, Raw: "\x1b[31m"})
	Set{Names: []string{"chroma"}, Intensity: Heavy}.Apply(f, NewState(1))
	if f.At(0, 0).HasFix || f.At(2, 0).HasFix {
		t.Error("a scene bringing its own colour must not be ghosted")
	}
}

func TestGlitchStaysInBounds(t *testing.T) {
	set := Set{Names: []string{"glitch"}, Intensity: Heavy}
	for seed := int64(0); seed < 40; seed++ {
		st := NewState(seed)
		for i := 0; i < 300; i++ {
			st.Tick(set, 30)
			f := testFrame()
			w, h := f.W, f.H
			set.Apply(f, st)
			if f.W != w || f.H != h || len(f.Cells) != w*h {
				t.Fatalf("glitch resized the frame to %dx%d", f.W, f.H)
			}
		}
	}
}

func TestIntensityRoundTrip(t *testing.T) {
	for _, s := range []string{"off", "subtle", "heavy"} {
		if got := ParseIntensity(s).String(); got != s {
			t.Errorf("ParseIntensity(%q).String() = %q", s, got)
		}
	}
	if ParseIntensity("nonsense") != Subtle {
		t.Error("unknown intensity should fall back to subtle")
	}
	if Off.Next().Next().Next() != Off {
		t.Error("Next should cycle through three states")
	}
}
