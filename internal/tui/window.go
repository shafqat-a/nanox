package tui

import (
	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// winMinW and winMinH are the smallest a window may be sized to (enough for the
// frame plus a little content).
const (
	winMinW = 8
	winMinH = 4
)

// Window is an MDI child window hosting exactly one content Widget. When active
// it draws a double-line frame in theme.ActiveFrame() with a magenta title bar
// (theme.ActiveTitle()); when inactive a single-line frame in
// theme.InactiveFrame()/InactiveTitle(). The caller supplies the title text
// (e.g. "[1] HELLO.BAS" or "*Untitled1"). A close button sits top-left and a
// maximize/restore button top-right.
//
// Window manages its own mouse interaction in HandleMouse: dragging the title
// bar moves it, dragging the bottom-right corner resizes it, clicking the
// close/max buttons fires the registered callbacks, and any other click inside
// is forwarded to the content widget. The Desktop clamps a window to its area.
type Window struct {
	BaseContainer
	content Widget
	title   string
	active  bool

	maximized   bool
	restoreRect Rect // bounds to restore to when un-maximizing

	onClose     func()
	onToggleMax func()

	// drag/resize interaction state.
	dragging bool
	resizing bool
	dragDX   int // pointer offset from window origin while dragging
	dragDY   int
}

// NewWindow returns a Window wrapping content with the given title. The content
// widget is also registered as the window's single child.
func NewWindow(content Widget, title string) *Window {
	w := &Window{content: content, title: title}
	if content != nil {
		w.Add(content)
	}
	return w
}

// SetTitle sets the title text shown in the title bar.
func (w *Window) SetTitle(t string) { w.title = t }

// Title returns the current title text.
func (w *Window) Title() string { return w.title }

// Content returns the hosted content widget.
func (w *Window) Content() Widget { return w.content }

// SetActive sets whether the window is the active (focused) MDI window.
func (w *Window) SetActive(v bool) { w.active = v }

// IsActive reports whether the window is active.
func (w *Window) IsActive() bool { return w.active }

// IsMaximized reports whether the window is maximized.
func (w *Window) IsMaximized() bool { return w.maximized }

// SetMaximized sets the maximized flag directly (without firing callbacks). The
// Desktop is responsible for re-laying maximized windows to fill its area.
func (w *Window) SetMaximized(v bool) {
	if v && !w.maximized {
		w.restoreRect = w.Bounds()
	}
	w.maximized = v
}

// ToggleMaximize flips the maximized flag, remembering the pre-maximize bounds
// so a later restore returns the window to its prior size/position.
func (w *Window) ToggleMaximize() {
	w.SetMaximized(!w.maximized)
}

// RestoreRect returns the bounds the window should return to when un-maximized.
func (w *Window) RestoreRect() Rect { return w.restoreRect }

// SetOnClose registers the callback fired when the close button is clicked.
func (w *Window) SetOnClose(fn func()) { w.onClose = fn }

// SetOnToggleMax registers the callback fired when the maximize/restore button
// is clicked.
func (w *Window) SetOnToggleMax(fn func()) { w.onToggleMax = fn }

// GetInnerRect returns the content area inside the window frame (inset by one
// cell on every side; the top inset is the title bar row).
func (w *Window) GetInnerRect() Rect {
	b := w.Bounds()
	inner := Rect{X: b.X + 1, Y: b.Y + 1, W: b.W - 2, H: b.H - 2}
	if inner.W < 0 {
		inner.W = 0
	}
	if inner.H < 0 {
		inner.H = 0
	}
	return inner
}

// frameStyles returns the frame/title styles and the box glyphs to use for the
// current active state.
func (w *Window) frameStyles() (frame, title tcell.Style, tl, tr, bl, br, h, v rune) {
	if w.active {
		return theme.ActiveFrame(), theme.ActiveTitle(),
			theme.TLDouble, theme.TRDouble, theme.BLDouble, theme.BRDouble,
			theme.HDouble, theme.VDouble
	}
	return theme.InactiveFrame(), theme.InactiveTitle(),
		theme.TLSingle, theme.TRSingle, theme.BLSingle, theme.BRSingle,
		theme.HSingle, theme.VSingle
}

// Draw renders the frame, title bar, buttons, and the content widget laid into
// the inner rect.
func (w *Window) Draw(s Surface) {
	b := w.Bounds()
	if b.W < 2 || b.H < 2 {
		return
	}
	frame, title, tl, tr, bl, br, hRune, vRune := w.frameStyles()

	left := b.X
	right := b.X + b.W - 1
	top := b.Y
	bottom := b.Y + b.H - 1

	// Top border row acts as the title bar (filled in the title style).
	for x := left; x <= right; x++ {
		s.Set(x, top, hRune, frame)
	}
	s.Set(left, top, tl, frame)
	s.Set(right, top, tr, frame)

	// Title bar fill + centered title text.
	if b.W > 2 {
		s.Fill(Rect{X: left + 1, Y: top, W: b.W - 2, H: 1}, ' ', title)
		t := winTruncate(w.title, b.W-4)
		tx := left + 1 + (b.W-2-len([]rune(t)))/2
		if tx < left+1 {
			tx = left + 1
		}
		s.Text(tx, top, t, title)
	}

	// Title-bar buttons: close top-left, max/restore top-right.
	s.Set(left+1, top, theme.BtnClose, title)
	maxGlyph := theme.BtnMax
	if w.maximized {
		maxGlyph = theme.BtnRestore
	}
	s.Set(right-1, top, maxGlyph, title)

	// Side borders.
	for y := top + 1; y < bottom; y++ {
		s.Set(left, y, vRune, frame)
		s.Set(right, y, vRune, frame)
	}
	// Bottom border.
	for x := left + 1; x < right; x++ {
		s.Set(x, bottom, hRune, frame)
	}
	s.Set(left, bottom, bl, frame)
	s.Set(right, bottom, br, frame)

	// Lay the content widget into the inner rect and draw it.
	inner := w.GetInnerRect()
	if w.content != nil && !inner.Empty() {
		w.content.SetBounds(inner)
		w.content.Draw(s.Clip(inner))
	}
}

// Focusable reports true so a click on the window focuses it (and the content
// receives keys via bubbling). Tab traversal still reaches focusable content
// descendants through FocusableDescendants.
func (w *Window) Focusable() bool { return true }

// HandleKey forwards unconsumed keys to the content widget.
func (w *Window) HandleKey(ev *tcell.EventKey) bool {
	if w.content != nil {
		return w.content.HandleKey(ev)
	}
	return false
}

// HandleMouse implements title-drag, bottom-right resize, the close/max
// buttons, and forwards other clicks to the content widget. It returns true if
// it consumed the event.
func (w *Window) HandleMouse(ev MouseEvent) bool {
	b := w.Bounds()
	top := b.Y
	left := b.X
	right := b.X + b.W - 1
	bottom := b.Y + b.H - 1

	switch ev.Action {
	case MouseDown:
		// Close button (top-left, just inside the corner).
		if ev.Y == top && ev.X == left+1 {
			if w.onClose != nil {
				w.onClose()
			}
			return true
		}
		// Max/restore button (top-right, just inside the corner).
		if ev.Y == top && ev.X == right-1 {
			if w.onToggleMax != nil {
				w.onToggleMax()
			} else {
				w.ToggleMaximize()
			}
			return true
		}
		// Bottom-right corner → start resize.
		if ev.X == right && ev.Y == bottom {
			w.resizing = true
			return true
		}
		// Title bar (top row) → start drag.
		if ev.Y == top {
			w.dragging = true
			w.dragDX = ev.X - left
			w.dragDY = ev.Y - top
			return true
		}
		// Otherwise forward to content.
		if w.content != nil && w.content.Bounds().Contains(ev.X, ev.Y) {
			return w.content.HandleMouse(ev)
		}
		return true

	case MouseDrag:
		if w.dragging {
			nx := ev.X - w.dragDX
			ny := ev.Y - w.dragDY
			w.SetBounds(Rect{X: nx, Y: ny, W: b.W, H: b.H})
			return true
		}
		if w.resizing {
			nw := ev.X - left + 1
			nh := ev.Y - top + 1
			if nw < winMinW {
				nw = winMinW
			}
			if nh < winMinH {
				nh = winMinH
			}
			w.SetBounds(Rect{X: left, Y: top, W: nw, H: nh})
			return true
		}
		if w.content != nil && w.content.Bounds().Contains(ev.X, ev.Y) {
			return w.content.HandleMouse(ev)
		}
		return false

	case MouseUp:
		if w.dragging || w.resizing {
			w.dragging = false
			w.resizing = false
			return true
		}
		if w.content != nil && w.content.Bounds().Contains(ev.X, ev.Y) {
			return w.content.HandleMouse(ev)
		}
		return false

	default:
		if w.content != nil && w.content.Bounds().Contains(ev.X, ev.Y) {
			return w.content.HandleMouse(ev)
		}
		return false
	}
}

// winTruncate trims s so its rune length does not exceed max (max < 0 → "").
func winTruncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
