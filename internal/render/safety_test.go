package render

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/verdantran/wakeart/internal/palette"
)

// Blend used to index `from` with `to`'s geometry, so a viewport that changed
// mid-transition (the status bar appearing) panicked.
func TestBlendSurvivesMismatchedSizes(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, tr := range AllTransitions {
		small, big := New(80, 23), New(80, 24)
		for _, p := range []float64{0.1, 0.5, 0.9} {
			if got := Blend(small, big, tr, p, rng); got == nil {
				t.Errorf("%s at %v: nil frame", tr, p)
			}
			if got := Blend(big, small, tr, p, rng); got == nil {
				t.Errorf("%s at %v reversed: nil frame", tr, p)
			}
		}
	}
}

// Scale picked the continuation half of a wide glyph as a region's winner, and
// placed it with Set, so the whole row came out empty.
func TestScaleKeepsWideGlyphRows(t *testing.T) {
	src := FromLines([]string{"⚡⚡⚡⚡⚡⚡", "############"}, nil)
	dst := Scale(src, 6, 2)
	if got := strings.TrimSpace(Plain(dst)); got == "" {
		t.Fatalf("scaling a row of wide glyphs produced nothing: %q", Plain(dst))
	}
	for i, c := range dst.Cells {
		if c.Cont && (i == 0 || !dst.Cells[i-1].Wide()) {
			t.Errorf("cell %d is an orphaned continuation", i)
		}
	}
}

func TestSafeString(t *testing.T) {
	cases := map[string]string{
		"plain":  "plain",
		"⚡ keep": "⚡ keep",
		// The ESC and BEL go; the printable body of the sequence stays.
		"\x1b]0;x\x07": "]0;x",
		"a\x00b":       "ab",
		"a\tb":         "ab",
		"a\x7fb":       "ab",
		"a\u009bb":     "ab",
		"a\u0085b":     "ab",
	}
	for in, want := range cases {
		if got := SafeString(in); got != want {
			t.Errorf("SafeString(%q) = %q, want %q", in, got, want)
		}
	}
}

// A raw C1 byte is not valid UTF-8, so decoding turns it into the replacement
// character long before it could reach the terminal as a control code.
func TestRawInvalidBytesAreNeutralised(t *testing.T) {
	if got := SafeString("a\x9bb"); strings.ContainsRune(got, 0x9b) {
		t.Errorf("raw C1 byte survived SafeString: %q", got)
	}
	var b strings.Builder
	Paint(FromLines([]string{"a\x9b[31mb"}, nil), palette.All[0], palette.TrueColor, &b)
	if strings.ContainsRune(b.String(), 0x9b) {
		t.Errorf("raw C1 byte painted: %q", b.String())
	}
}
