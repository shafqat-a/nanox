package tui

import (
	"dosedit/internal/theme"
)

// Desktop is the windowed MDI area. It paints a grey hatch background and hosts
// a z-ordered set of Windows, drawn bottom-to-top so the active window (kept
// last) is on top. It clamps windows to its rectangle and provides window
// management (activation, cycling, cascade, tile, move/resize of the active
// window).
type Desktop struct {
	BaseContainer
	windows []*Window
}

// NewDesktop returns an empty Desktop.
func NewDesktop() *Desktop { return &Desktop{} }

// AddWindow adds win to the desktop, raises it to the top of the z-order and
// makes it the active window (deactivating the others). It is also registered as
// a container child so focus traversal can reach its content.
func (d *Desktop) AddWindow(win *Window) {
	if win == nil {
		return
	}
	d.windows = append(d.windows, win)
	d.BaseContainer.Add(win)
	d.Activate(win)
	d.clamp(win)
}

// RemoveWindow removes win from the desktop. If it was active, the new topmost
// window becomes active.
func (d *Desktop) RemoveWindow(win *Window) {
	idx := -1
	for i, w := range d.windows {
		if w == win {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	d.windows = append(d.windows[:idx], d.windows[idx+1:]...)
	d.rebuildChildren()
	if win.IsActive() && len(d.windows) > 0 {
		d.Activate(d.windows[len(d.windows)-1])
	}
}

// Windows returns the windows in z-order (last = topmost/active).
func (d *Desktop) Windows() []*Window { return d.windows }

// Active returns the active (topmost) window, or nil.
func (d *Desktop) Active() *Window {
	for _, w := range d.windows {
		if w.IsActive() {
			return w
		}
	}
	return nil
}

// Activate raises win to the top of the z-order, marks it active and marks all
// other windows inactive.
func (d *Desktop) Activate(win *Window) {
	found := false
	for _, w := range d.windows {
		if w == win {
			found = true
		}
		w.SetActive(false)
	}
	if !found {
		return
	}
	// Move win to the end (topmost).
	for i, w := range d.windows {
		if w == win {
			d.windows = append(d.windows[:i], d.windows[i+1:]...)
			break
		}
	}
	d.windows = append(d.windows, win)
	win.SetActive(true)
	d.rebuildChildren()
}

// Next activates the next window in z-order after the active one (cycling),
// raising it to the top.
func (d *Desktop) Next() { d.cycle(+1) }

// Prev activates the previous window in z-order before the active one (cycling),
// raising it to the top.
func (d *Desktop) Prev() { d.cycle(-1) }

// cycle activates the window dir steps away from the current active one.
func (d *Desktop) cycle(dir int) {
	n := len(d.windows)
	if n == 0 {
		return
	}
	cur := -1
	for i, w := range d.windows {
		if w.IsActive() {
			cur = i
			break
		}
	}
	if cur < 0 {
		d.Activate(d.windows[n-1])
		return
	}
	next := (cur + dir) % n
	if next < 0 {
		next += n
	}
	d.Activate(d.windows[next])
}

// Cascade arranges windows in an offset diagonal stack from the top-left of the
// desktop, each the same size, the active one ending on top.
func (d *Desktop) Cascade() {
	b := d.Bounds()
	if b.Empty() || len(d.windows) == 0 {
		return
	}
	const off = 2
	cw := b.W * 3 / 4
	ch := b.H * 3 / 4
	if cw < winMinW {
		cw = winMinW
	}
	if ch < winMinH {
		ch = winMinH
	}
	for i, w := range d.windows {
		x := b.X + (i*off)%maxInt(1, b.W-cw)
		y := b.Y + (i*off)%maxInt(1, b.H-ch)
		w.SetMaximized(false)
		w.SetBounds(Rect{X: x, Y: y, W: cw, H: ch})
		d.clamp(w)
	}
}

// Tile arranges windows in a non-overlapping grid that fills the desktop rect.
func (d *Desktop) Tile() {
	b := d.Bounds()
	n := len(d.windows)
	if b.Empty() || n == 0 {
		return
	}
	cols := 1
	for cols*cols < n {
		cols++
	}
	rows := (n + cols - 1) / cols
	cellW := b.W / cols
	cellH := b.H / rows
	if cellW < 1 {
		cellW = 1
	}
	if cellH < 1 {
		cellH = 1
	}
	for i, w := range d.windows {
		c := i % cols
		r := i / cols
		x := b.X + c*cellW
		y := b.Y + r*cellH
		ww := cellW
		hh := cellH
		// Last column/row absorbs any remainder so the grid fills the rect.
		if c == cols-1 {
			ww = b.X + b.W - x
		}
		if r == rows-1 {
			hh = b.Y + b.H - y
		}
		w.SetMaximized(false)
		w.SetBounds(Rect{X: x, Y: y, W: ww, H: hh})
	}
}

// MoveActive moves the active window by (dx, dy), clamped to the desktop.
func (d *Desktop) MoveActive(dx, dy int) {
	w := d.Active()
	if w == nil {
		return
	}
	b := w.Bounds()
	w.SetBounds(Rect{X: b.X + dx, Y: b.Y + dy, W: b.W, H: b.H})
	d.clamp(w)
}

// ResizeActive grows/shrinks the active window by (dw, dh), clamped to the
// desktop and to the window minimum size.
func (d *Desktop) ResizeActive(dw, dh int) {
	w := d.Active()
	if w == nil {
		return
	}
	b := w.Bounds()
	nw := b.W + dw
	nh := b.H + dh
	if nw < winMinW {
		nw = winMinW
	}
	if nh < winMinH {
		nh = winMinH
	}
	w.SetBounds(Rect{X: b.X, Y: b.Y, W: nw, H: nh})
	d.clamp(w)
}

// clamp keeps win within the desktop rect (size first, then position).
func (d *Desktop) clamp(win *Window) {
	b := d.Bounds()
	if b.Empty() {
		return
	}
	wb := win.Bounds()
	if wb.W > b.W {
		wb.W = b.W
	}
	if wb.H > b.H {
		wb.H = b.H
	}
	if wb.W < winMinW {
		wb.W = winMinW
	}
	if wb.H < winMinH {
		wb.H = winMinH
	}
	if wb.X < b.X {
		wb.X = b.X
	}
	if wb.Y < b.Y {
		wb.Y = b.Y
	}
	if wb.X+wb.W > b.X+b.W {
		wb.X = b.X + b.W - wb.W
	}
	if wb.Y+wb.H > b.Y+b.H {
		wb.Y = b.Y + b.H - wb.H
	}
	win.SetBounds(wb)
}

// rebuildChildren resyncs the container child slice to the window z-order so
// hit-testing and focus traversal match the drawn order.
func (d *Desktop) rebuildChildren() {
	d.BaseContainer.children = d.BaseContainer.children[:0]
	for _, w := range d.windows {
		d.BaseContainer.children = append(d.BaseContainer.children, w)
	}
}

// SetBounds stores the desktop rect and re-applies maximized window sizing and
// clamping.
func (d *Desktop) SetBounds(r Rect) {
	d.BaseWidget.SetBounds(r)
	for _, w := range d.windows {
		if w.IsMaximized() {
			w.SetBounds(r)
		} else {
			d.clamp(w)
		}
	}
}

// Draw paints the hatch background then the windows bottom-to-top.
func (d *Desktop) Draw(s Surface) {
	b := d.Bounds()
	s.Fill(b, theme.Texture, theme.Desktop())
	for _, w := range d.windows {
		if w.IsMaximized() {
			w.SetBounds(b)
		}
		w.Draw(s.Clip(w.Bounds()))
	}
}

// HandleMouse finds the topmost window under the cursor, activates it on a
// press, and delegates the event to it (drag/resize/buttons/content).
func (d *Desktop) HandleMouse(ev MouseEvent) bool {
	// While a window is mid drag/resize it keeps the interaction even if the
	// pointer leaves its bounds.
	for i := len(d.windows) - 1; i >= 0; i-- {
		w := d.windows[i]
		if w.dragging || w.resizing {
			handled := w.HandleMouse(ev)
			d.clamp(w)
			return handled
		}
	}
	for i := len(d.windows) - 1; i >= 0; i-- {
		w := d.windows[i]
		if w.Bounds().Contains(ev.X, ev.Y) {
			if ev.Action == MouseDown {
				d.Activate(w)
			}
			handled := w.HandleMouse(ev)
			d.clamp(w)
			return handled
		}
	}
	return false
}

// maxInt returns the larger of a, b (local helper; min/max are not redeclared).
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
