package render

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/verdantran/wakeart/internal/palette"
)

func lines(s string) []string { return strings.Split(strings.TrimPrefix(s, "\n"), "\n") }

func TestWeightOrdering(t *testing.T) {
	// The ramp only reads well if ink weight rises with visual density.
	ramp := []rune{' ', '.', ':', '-', '=', '+', '*', '#', '%', '@'}
	for i := 1; i < len(ramp); i++ {
		if Weight(ramp[i]) <= Weight(ramp[i-1]) {
			t.Errorf("Weight(%q)=%d not greater than Weight(%q)=%d",
				ramp[i], Weight(ramp[i]), ramp[i-1], Weight(ramp[i-1]))
		}
	}
	if Weight(' ') != 0 {
		t.Errorf("space must weigh nothing, got %d", Weight(' '))
	}
	if Weight('█') <= Weight('░') {
		t.Error("full block must outweigh light shade")
	}
}

func TestFitCentres(t *testing.T) {
	src := FromLines([]string{"ab", "cd"}, nil)
	got, cropped := Fit(src, 6, 4, Center)
	if cropped {
		t.Error("art smaller than the viewport must not report a crop")
	}
	if got.W != 6 || got.H != 4 {
		t.Fatalf("size = %dx%d", got.W, got.H)
	}
	if got.At(2, 1).R != 'a' {
		t.Errorf("expected 'a' at 2,1, got %q", got.At(2, 1).R)
	}
}

func TestFitCropsFromCentre(t *testing.T) {
	src := FromLines([]string{"abcde", "fghij", "klmno"}, nil)
	got, cropped := Fit(src, 3, 1, Center)
	if !cropped {
		t.Error("oversized art must report a crop")
	}
	if Plain(got) != "ghi" {
		t.Errorf("crop = %q, want %q", Plain(got), "ghi")
	}
}

func TestFitAlign(t *testing.T) {
	src := FromLines([]string{"xy"}, nil)
	tl, _ := Fit(src, 5, 3, TopLeft)
	if tl.At(0, 0).R != 'x' {
		t.Errorf("topleft put %q at origin", tl.At(0, 0).R)
	}
	bl, _ := Fit(src, 5, 3, BottomLeft)
	if bl.At(0, 2).R != 'x' {
		t.Errorf("bottomleft put %q on the last row", bl.At(0, 2).R)
	}
}

func TestScaleKeepsHeaviestCell(t *testing.T) {
	src := FromLines([]string{"@ @ @ @ @ @", "           ", "@ @ @ @ @ @"}, nil)
	got := Scale(src, 4, 2)
	if got.W > 4 || got.H > 2 {
		t.Fatalf("scaled to %dx%d, want no larger than 4x2", got.W, got.H)
	}
	if strings.TrimSpace(Plain(got)) == "" {
		t.Error("downscaling must not erase the art")
	}
}

func TestScaleLeavesSmallArtAlone(t *testing.T) {
	src := FromLines([]string{"ab"}, nil)
	if got := Scale(src, 40, 10); got != src {
		t.Error("art smaller than the viewport must be returned untouched")
	}
}

func TestPaintEmitsNoEscapesForBlanks(t *testing.T) {
	f := FromLines([]string{"  ", "  "}, nil)
	var b strings.Builder
	Paint(f, palette.All[0], palette.TrueColor, &b)
	if strings.Contains(b.String(), "\x1b") {
		t.Errorf("blank frame emitted escapes: %q", b.String())
	}
}

func TestPaintCoalescesRuns(t *testing.T) {
	// Identical adjacent glyphs share one colour, so one escape should cover
	// the whole run.
	f := FromLines([]string{strings.Repeat("@", 40)}, nil)
	var b strings.Builder
	Paint(f, palette.All[0], palette.TrueColor, &b)
	if n := strings.Count(b.String(), "\x1b[38"); n != 1 {
		t.Errorf("colour changes = %d, want 1 for a uniform run", n)
	}
}

func TestPaintMonoUsesAttributesOnly(t *testing.T) {
	f := FromLines([]string{".@"}, nil)
	var b strings.Builder
	Paint(f, palette.All[0], palette.Mono, &b)
	if strings.Contains(b.String(), "38;2") || strings.Contains(b.String(), "38;5") {
		t.Errorf("mono mode emitted colour: %q", b.String())
	}
}

func TestBlendEndpoints(t *testing.T) {
	a := FromLines([]string{"aaaa"}, nil)
	bb := FromLines([]string{"bbbb"}, nil)
	for _, tr := range AllTransitions {
		if got := Blend(a, bb, tr, 0, testRNG()); Plain(got) != "aaaa" && tr != Cut {
			t.Errorf("%s at p=0 = %q, want the outgoing frame", tr, Plain(got))
		}
		if got := Blend(a, bb, tr, 1, testRNG()); Plain(got) != "bbbb" {
			t.Errorf("%s at p=1 = %q, want the incoming frame", tr, Plain(got))
		}
	}
}

func TestBlendLeavesSourcesUntouched(t *testing.T) {
	a := FromLines([]string{"aaaa", "aaaa"}, nil)
	bb := FromLines([]string{"bbbb", "bbbb"}, nil)
	for _, tr := range AllTransitions {
		Blend(a, bb, tr, 0.5, testRNG())
		if Plain(a) != "aaaa\naaaa" || Plain(bb) != "bbbb\nbbbb" {
			t.Fatalf("%s mutated a source frame", tr)
		}
	}
}

func TestFitZeroViewport(t *testing.T) {
	src := FromLines([]string{"ab"}, nil)
	if got, _ := Fit(src, 0, 0, Center); got.W != 0 || got.H != 0 {
		t.Error("a zero viewport must yield an empty frame, not a panic")
	}
}

func TestWideRuneReservesTwoCells(t *testing.T) {
	f := New(6, 1)
	f.SetRune(1, 0, Cell{R: '⚡', Lvl: 240})
	if !f.At(1, 0).Wide() {
		t.Error("⚡ should be recognised as double-width")
	}
	if !f.At(2, 0).Cont {
		t.Error("the cell after a wide glyph should be marked as its continuation")
	}
	if got := Plain(f); got != " ⚡" {
		t.Errorf("Plain = %q, want %q", got, " ⚡")
	}
}

func TestWideRuneDroppedAtRightEdge(t *testing.T) {
	f := New(3, 1)
	f.SetRune(2, 0, Cell{R: '⚡'})
	if !f.At(2, 0).Blank() {
		t.Error("a wide glyph with no room must be dropped, not allowed to overflow")
	}
}

func TestFromLinesCountsDisplayWidth(t *testing.T) {
	f := FromLines([]string{"⚡ab", "xyzw"}, nil)
	if f.W != 4 {
		t.Fatalf("width = %d, want 4 display cells", f.W)
	}
	// ⚡ occupies cells 0 and 1, so 'a' must land at 2 and stay aligned with 'z'.
	if got := f.At(2, 0).R; got != 'a' {
		t.Errorf("cell 2 = %q, want 'a'", got)
	}
	if got := Plain(f); got != "⚡ab\nxyzw" {
		t.Errorf("Plain = %q", got)
	}
}

func TestPaintNeverExceedsFrameWidth(t *testing.T) {
	for _, line := range []string{"⚡⚡⚡", "a⚡b⚡", "⚡", "ab⚡"} {
		f := FromLines([]string{line}, nil)
		var b strings.Builder
		Paint(f, palette.All[0], palette.TrueColor, &b)
		if got := runewidth.StringWidth(stripEsc(b.String())); got != f.W {
			t.Errorf("%q: painted %d cells, frame is %d", line, got, f.W)
		}
	}
}

// A crop that lands between the halves of a wide glyph must not leave a stray
// continuation cell painting nothing.
func TestCropThroughWideRune(t *testing.T) {
	src := FromLines([]string{"ab⚡cd"}, nil)
	got, _ := Fit(src, 3, 1, Center)
	var b strings.Builder
	Paint(got, palette.All[0], palette.TrueColor, &b)
	if w := runewidth.StringWidth(stripEsc(b.String())); w != 3 {
		t.Errorf("cropped paint is %d cells, want 3", w)
	}
}

func stripEsc(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && !(s[i] >= '@' && s[i] <= '~') {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
