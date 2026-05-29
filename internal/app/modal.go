// Package app: modalLayer is the full-screen input-absorbing layer that hosts a
// modal dialog. It exists to make dialogs (Open / Save As / Find / Replace /
// message boxes / About) *truly* modal: the layer fills the whole screen so
// tview.Pages cannot route any click through to the background main page, and
// its MouseHandler consumes every event that lands outside the centred dialog.
// Only clicks inside the dialog are forwarded to it. Keyboard input and focus
// are delegated straight to the inner dialog so its own Tab focus-group works.
package app

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// modalLayer wraps a dialog primitive in a full-screen Box. The Box receives the
// whole screen rect (so it covers everything beneath it on tview.Pages) while
// inner is positioned in a centred w×h sub-rect.
type modalLayer struct {
	*tview.Box
	inner tview.Primitive
	w, h  int
}

// newModalLayer wraps inner in a full-screen modal layer, requesting a centred
// dialog size of w×h cells.
func newModalLayer(inner tview.Primitive, w, h int) *modalLayer {
	return &modalLayer{
		Box:   tview.NewBox(),
		inner: inner,
		w:     w,
		h:     h,
	}
}

// SetRect stores the full screen rect on the Box and positions inner in a
// centred sub-rect, clamped so it never exceeds the available area.
func (m *modalLayer) SetRect(x, y, width, height int) {
	m.Box.SetRect(x, y, width, height)

	iw, ih := m.w, m.h
	if iw > width {
		iw = width
	}
	if ih > height {
		ih = height
	}
	ix := x + (width-iw)/2
	iy := y + (height-ih)/2
	m.inner.SetRect(ix, iy, iw, ih)
}

// Draw renders only the inner dialog; the surrounding area is left untouched so
// the background (windows + menu bar) shows through. The Box draws nothing.
func (m *modalLayer) Draw(screen tcell.Screen) {
	m.inner.Draw(screen)
}

// Focus delegates focus to the inner dialog so its internal focus-group (Tab /
// Shift+Tab between fields) takes over.
func (m *modalLayer) Focus(delegate func(p tview.Primitive)) {
	delegate(m.inner)
}

// HasFocus reports focus based on the inner dialog (or one of its children).
func (m *modalLayer) HasFocus() bool {
	return m.inner.HasFocus()
}

// InputHandler forwards keyboard input to the inner dialog.
func (m *modalLayer) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return m.inner.InputHandler()
}

// MouseHandler is the heart of modality. Events whose position falls inside the
// inner dialog's rect are forwarded to the dialog. Events outside it are
// consumed (returning true) so they never propagate to the background main page,
// making all clicks off the dialog dead.
func (m *modalLayer) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		x, y := event.Position()
		ix, iy, iw, ih := m.inner.GetRect()
		inside := x >= ix && x < ix+iw && y >= iy && y < iy+ih
		if inside {
			if handler := m.inner.MouseHandler(); handler != nil {
				return handler(action, event, setFocus)
			}
			return false, nil
		}
		// Outside the dialog: swallow the event so the background never sees it.
		return true, nil
	}
}
