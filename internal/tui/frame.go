package tui

import (
	"dosedit/internal/theme"
)

// Frame is a GroupBox: a single-line border drawn in theme.DialogBody() with a
// caption embedded in the top border (" Caption "). It hosts child widgets in
// the area inset by one cell on every side. Layout of children (their bounds)
// is the caller's responsibility; Frame simply draws its chrome and then each
// child clipped to the inner rect.
type Frame struct {
	BaseContainer
	caption string
}

// NewFrame returns a GroupBox with the given caption.
func NewFrame(caption string) *Frame {
	return &Frame{caption: caption}
}

// Caption returns the frame's caption text.
func (f *Frame) Caption() string { return f.caption }

// SetCaption updates the frame's caption text.
func (f *Frame) SetCaption(c string) { f.caption = c }

// GetInnerRect returns the content area inside the border (inset by one cell).
func (f *Frame) GetInnerRect() Rect { return f.Bounds().Inset(1, 1) }

// Draw renders the single-line border, the embedded caption, and the children.
func (f *Frame) Draw(s Surface) {
	b := f.Bounds()
	if b.W < 2 || b.H < 2 {
		return
	}
	st := theme.DialogBody()

	left := b.X
	right := b.X + b.W - 1
	top := b.Y
	bottom := b.Y + b.H - 1

	// Corners.
	s.Set(left, top, theme.TLSingle, st)
	s.Set(right, top, theme.TRSingle, st)
	s.Set(left, bottom, theme.BLSingle, st)
	s.Set(right, bottom, theme.BRSingle, st)

	// Top and bottom horizontal runs.
	for x := left + 1; x < right; x++ {
		s.Set(x, top, theme.HSingle, st)
		s.Set(x, bottom, theme.HSingle, st)
	}
	// Left and right vertical runs.
	for y := top + 1; y < bottom; y++ {
		s.Set(left, y, theme.VSingle, st)
		s.Set(right, y, theme.VSingle, st)
	}

	// Embedded caption: " Caption " starting two cells in from the left corner.
	if f.caption != "" {
		cap := " " + f.caption + " "
		s.Text(left+2, top, frmTruncate(cap, b.W-3), st)
	}

	// Draw children clipped to the inner content rect.
	inner := f.GetInnerRect()
	if inner.Empty() {
		return
	}
	cs := s.Clip(inner)
	for _, ch := range f.Children() {
		ch.Draw(cs.Clip(ch.Bounds()))
	}
}

// frmTruncate trims s so its rune length does not exceed max (max < 0 → "").
func frmTruncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
