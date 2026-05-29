// Package tui is a from-scratch, event-driven, retained-mode terminal widget
// toolkit built directly on tcell (no tview). It provides the core foundation:
// geometry, a clipped drawing Surface, mouse/keyboard events, the Widget /
// Container interfaces with embeddable base implementations, and the App that
// owns the screen, event loop, focus and modality.
//
// See internal/tui/DESIGN.md for the authoritative contract.
package tui

// Rect is an axis-aligned rectangle in absolute screen coordinates. X,Y is the
// top-left cell; W,H are width and height in cells.
type Rect struct{ X, Y, W, H int }

// Contains reports whether the absolute cell (x, y) lies within r.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Inset returns r shrunk by dx on the left/right and dy on the top/bottom. The
// result is clamped so width and height never go negative.
func (r Rect) Inset(dx, dy int) Rect {
	out := Rect{X: r.X + dx, Y: r.Y + dy, W: r.W - 2*dx, H: r.H - 2*dy}
	if out.W < 0 {
		out.W = 0
	}
	if out.H < 0 {
		out.H = 0
	}
	return out
}

// Empty reports whether r encloses no cells.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// intersect returns the overlap of a and b (possibly empty).
func intersect(a, b Rect) Rect {
	x0 := a.X
	if b.X > x0 {
		x0 = b.X
	}
	y0 := a.Y
	if b.Y > y0 {
		y0 = b.Y
	}
	x1 := a.X + a.W
	if b.X+b.W < x1 {
		x1 = b.X + b.W
	}
	y1 := a.Y + a.H
	if b.Y+b.H < y1 {
		y1 = b.Y + b.H
	}
	if x1 <= x0 || y1 <= y0 {
		return Rect{X: x0, Y: y0, W: 0, H: 0}
	}
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}
