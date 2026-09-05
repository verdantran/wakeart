package render

type Align int

const (
	Center Align = iota
	TopLeft
	BottomLeft
)

func ParseAlign(s string) Align {
	switch s {
	case "topleft":
		return TopLeft
	case "bottomleft":
		return BottomLeft
	}
	return Center
}

func (a Align) String() string {
	switch a {
	case TopLeft:
		return "topleft"
	case BottomLeft:
		return "bottomleft"
	}
	return "center"
}

// Fit places src into a w*h viewport. Oversized art is cropped from the
// centre rather than scaled; scaling ASCII always looks worse than losing an
// edge. Cropped reports whether anything was lost.
func Fit(src *Frame, w, h int, a Align) (dst *Frame, cropped bool) {
	dst = New(w, h)
	if src == nil || w <= 0 || h <= 0 {
		return dst, false
	}
	cropped = src.W > w || src.H > h

	var ox, oy int
	switch a {
	case TopLeft:
		ox, oy = 0, 0
	case BottomLeft:
		ox, oy = 0, h-src.H
	default:
		ox, oy = (w-src.W)/2, (h-src.H)/2
	}
	// A negative offset is the crop: start reading from inside the source.
	sx0, sy0 := 0, 0
	if ox < 0 {
		sx0, ox = -ox, 0
	}
	if oy < 0 {
		sy0, oy = -oy, 0
	}
	for y := 0; oy+y < h && sy0+y < src.H; y++ {
		for x := 0; ox+x < w && sx0+x < src.W; x++ {
			dst.Set(ox+x, oy+y, src.At(sx0+x, sy0+y))
		}
	}
	return dst, cropped
}

// Scale downsamples src to fit within w*h, choosing the heaviest cell in each
// source block so thin strokes survive. Upscaling is never done; art smaller
// than the viewport is returned untouched.
func Scale(src *Frame, w, h int) *Frame {
	if src == nil || src.W == 0 || src.H == 0 || w <= 0 || h <= 0 {
		return src
	}
	if src.W <= w && src.H <= h {
		return src
	}
	fx := float64(src.W) / float64(w)
	fy := float64(src.H) / float64(h)
	f := fx
	if fy > f {
		f = fy
	}
	nw := int(float64(src.W) / f)
	nh := int(float64(src.H) / f)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := New(nw, nh)
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			sx0, sy0 := int(float64(x)*f), int(float64(y)*f)
			sx1, sy1 := int(float64(x+1)*f), int(float64(y+1)*f)
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			if sy1 <= sy0 {
				sy1 = sy0 + 1
			}
			// A continuation cell carries its head's level but no glyph of its
			// own, so letting one win leaves an empty column behind.
			if dst.At(x, y).Cont {
				continue
			}
			best := Cell{}
			for sy := sy0; sy < sy1 && sy < src.H; sy++ {
				for sx := sx0; sx < sx1 && sx < src.W; sx++ {
					if c := src.At(sx, sy); !c.Cont && !c.Blank() && c.Lvl >= best.Lvl {
						best = c
					}
				}
			}
			dst.SetRune(x, y, best)
		}
	}
	return dst
}
