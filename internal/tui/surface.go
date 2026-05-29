package tui

import "github.com/gdamore/tcell/v2"

// Surface is a clipped drawing target onto the screen within a widget's bounds.
// All coordinates passed to Set/Fill/Text are ABSOLUTE screen coordinates;
// writes outside the surface's clip rectangle are silently dropped.
type Surface interface {
	// Set draws a single rune at the absolute cell (x, y) if it lies within
	// the surface's clip.
	Set(x, y int, r rune, style tcell.Style)
	// Fill fills the absolute rectangle r with ch/style, clipped to bounds.
	Fill(r Rect, ch rune, style tcell.Style)
	// Text draws a single line of text starting at absolute (x, y), clipped.
	Text(x, y int, s string, style tcell.Style)
	// Clip returns a sub-surface whose clip is r intersected with this
	// surface's clip. Coordinates remain absolute.
	Clip(r Rect) Surface
	// Bounds returns the surface's clip rectangle in absolute coordinates.
	Bounds() Rect
}

// screenSurface is the screen-backed Surface implementation. It draws onto a
// tcell.Screen, clipping every write to its clip rectangle.
type screenSurface struct {
	screen tcell.Screen
	clip   Rect
}

// NewScreenSurface returns a Surface backed by screen, clipped to clip.
func NewScreenSurface(screen tcell.Screen, clip Rect) Surface {
	return &screenSurface{screen: screen, clip: clip}
}

func (s *screenSurface) Set(x, y int, r rune, style tcell.Style) {
	if !s.clip.Contains(x, y) {
		return
	}
	s.screen.SetContent(x, y, r, nil, style)
}

func (s *screenSurface) Fill(r Rect, ch rune, style tcell.Style) {
	a := intersect(r, s.clip)
	if a.Empty() {
		return
	}
	for y := a.Y; y < a.Y+a.H; y++ {
		for x := a.X; x < a.X+a.W; x++ {
			s.screen.SetContent(x, y, ch, nil, style)
		}
	}
}

func (s *screenSurface) Text(x, y int, str string, style tcell.Style) {
	if y < s.clip.Y || y >= s.clip.Y+s.clip.H {
		return
	}
	cx := x
	for _, r := range str {
		if cx >= s.clip.X+s.clip.W {
			break
		}
		if cx >= s.clip.X {
			s.screen.SetContent(cx, y, r, nil, style)
		}
		cx++
	}
}

func (s *screenSurface) Clip(r Rect) Surface {
	return &screenSurface{screen: s.screen, clip: intersect(r, s.clip)}
}

func (s *screenSurface) Bounds() Rect { return s.clip }
