package wm

import (
	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// dragMode is the manager's pointer-interaction state.
type dragMode int

const (
	dragNone   dragMode = iota
	dragMove            // moving a window by its title bar
	dragResize          // resizing a window by its bottom-right grip
)

// Manager owns the textured desktop region and a z-ordered stack of windows.
// It embeds *tview.Box: the box rect IS the desktop area (the rows between the
// menu bar and the status bar). Windows are stored bottom..top; the last entry
// is the topmost / active one.
type Manager struct {
	*tview.Box

	windows []*Window

	// pointer drag/resize state.
	mode      dragMode
	dragWin   *Window
	grabDX    int // pointer offset from the window origin at grab time
	grabDY    int
	grabRight int // for resize: pointer offset from the window's right/bottom edge
	grabBot   int
}

// NewManager creates an empty window manager.
func NewManager() *Manager {
	return &Manager{Box: tview.NewBox()}
}

// Add pushes w on top of the z-order (without changing the active flags;
// callers typically follow with Activate).
func (m *Manager) Add(w *Window) {
	if w == nil {
		return
	}
	m.windows = append(m.windows, w)
}

// Remove unlinks w from the stack. If it was active, the new topmost window
// (if any) becomes active.
func (m *Manager) Remove(w *Window) {
	for i, x := range m.windows {
		if x == w {
			m.windows = append(m.windows[:i], m.windows[i+1:]...)
			break
		}
	}
	if m.dragWin == w {
		m.mode = dragNone
		m.dragWin = nil
	}
	if w.IsActive() && len(m.windows) > 0 {
		m.Activate(m.windows[len(m.windows)-1])
	}
}

// Windows returns the z-order slice (bottom..top). The returned slice is a
// copy so callers cannot mutate internal ordering.
func (m *Manager) Windows() []*Window {
	out := make([]*Window, len(m.windows))
	copy(out, m.windows)
	return out
}

// Active returns the topmost active window, or nil.
func (m *Manager) Active() *Window {
	for i := len(m.windows) - 1; i >= 0; i-- {
		if m.windows[i].IsActive() {
			return m.windows[i]
		}
	}
	return nil
}

// Activate raises w to the top of the z-order, marks it active and all others
// inactive.
func (m *Manager) Activate(w *Window) {
	if w == nil {
		return
	}
	found := false
	for i, x := range m.windows {
		if x == w {
			m.windows = append(m.windows[:i], m.windows[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return
	}
	m.windows = append(m.windows, w)
	for _, x := range m.windows {
		x.SetActive(x == w)
	}
}

// Next activates the next window in z-order, wrapping. With the active window
// on top, "next" is the bottom-most window raised to the top (classic F6
// cycling through the stack).
func (m *Manager) Next() {
	if len(m.windows) == 0 {
		return
	}
	// Raise the bottom window: it becomes the new top/active.
	m.Activate(m.windows[0])
}

// Prev activates the previous window in z-order, wrapping. It raises the window
// just below the current top so repeated Prev cycles backwards.
func (m *Manager) Prev() {
	n := len(m.windows)
	if n == 0 {
		return
	}
	if n == 1 {
		m.Activate(m.windows[0])
		return
	}
	m.Activate(m.windows[n-2])
}

// Cascade offset-stacks all windows at ~2/3 of the desktop size, each shifted
// one column/row down-right from the previous. Maximized windows are restored.
func (m *Manager) Cascade() {
	dx, dy, dw, dh := m.GetRect()
	if dw <= 0 || dh <= 0 {
		return
	}
	ww := dw * 2 / 3
	wh := dh * 2 / 3
	if ww < MinWindowWidth {
		ww = MinWindowWidth
	}
	if wh < MinWindowHeight {
		wh = MinWindowHeight
	}
	for i, w := range m.windows {
		w.maximized = false
		off := i % maxCascade(dw, dh, ww, wh)
		x := dx + off
		y := dy + off
		if x+ww > dx+dw {
			x = dx + dw - ww
		}
		if y+wh > dy+dh {
			y = dy + dh - wh
		}
		w.SetRect(x, y, ww, wh)
	}
}

// maxCascade returns how many one-cell steps fit before the window would run
// off the desktop, with a floor of 1 so the modulo is always valid.
func maxCascade(dw, dh, ww, wh int) int {
	stepsX := dw - ww
	stepsY := dh - wh
	n := stepsX
	if stepsY < n {
		n = stepsY
	}
	if n < 1 {
		n = 1
	}
	return n
}

// Tile arranges all windows in a grid that fills the desktop with no overlap.
// Maximized windows are restored. With n windows it uses ceil(sqrt(n)) columns.
func (m *Manager) Tile() {
	n := len(m.windows)
	if n == 0 {
		return
	}
	dx, dy, dw, dh := m.GetRect()
	if dw <= 0 || dh <= 0 {
		return
	}

	cols := 1
	for cols*cols < n {
		cols++
	}
	rows := (n + cols - 1) / cols

	cellW := dw / cols
	cellH := dh / rows
	if cellW < 1 {
		cellW = 1
	}
	if cellH < 1 {
		cellH = 1
	}

	for i, w := range m.windows {
		w.maximized = false
		c := i % cols
		r := i / cols
		x := dx + c*cellW
		y := dy + r*cellH
		ww := cellW
		wh := cellH
		// Last column/row absorbs the remainder so the grid fills the desktop.
		if c == cols-1 {
			ww = dx + dw - x
		}
		if r == rows-1 {
			wh = dy + dh - y
		}
		w.SetRect(x, y, ww, wh)
	}
}

// Draw paints the desktop hatch across the whole manager rect, then draws every
// window bottom->top so the active/topmost window renders last. Windows fully
// outside the desktop are skipped; partially-outside windows draw clipped by
// the screen.
func (m *Manager) Draw(screen tcell.Screen) {
	x, y, width, height := m.GetRect()
	if width <= 0 || height <= 0 {
		return
	}

	style := theme.Desktop()
	for ry := y; ry < y+height; ry++ {
		for rx := x; rx < x+width; rx++ {
			screen.SetContent(rx, ry, theme.Texture, nil, style)
		}
	}

	for _, w := range m.windows {
		w.setDesktop(x, y, width, height)
		wx, wy, ww, wh := w.GetRect()
		if ww <= 0 || wh <= 0 {
			continue
		}
		// Skip windows entirely outside the desktop rect.
		if wx >= x+width || wy >= y+height || wx+ww <= x || wy+wh <= y {
			continue
		}
		w.Draw(screen)
	}
}

// windowAt returns the topmost window whose rect contains (x,y), or nil.
func (m *Manager) windowAt(x, y int) *Window {
	for i := len(m.windows) - 1; i >= 0; i-- {
		w := m.windows[i]
		wx, wy, ww, wh := w.GetRect()
		if x >= wx && x < wx+ww && y >= wy && y < wy+wh {
			return w
		}
	}
	return nil
}

// HandleMouse is the manager's mouse entry point. It returns whether the event
// was consumed and, when a content click should route focus, the content
// primitive that received the forwarded event.
//
// Behaviour:
//   - LeftDown on a window: Activate it, then dispatch by HitTest region —
//     close fires onClose; max/restore toggles and fires onToggleMax; title
//     begins a move-drag; resize grip begins a resize-drag; content forwards
//     the event to the content's MouseHandler (absolute coords) and returns it.
//   - Move while dragging/resizing updates the window rect (clamped to the
//     desktop, min size enforced).
//   - LeftUp ends any drag/resize.
//   - Clicks on the empty desktop are not consumed.
func (m *Manager) HandleMouse(action tview.MouseAction, event *tcell.EventMouse) (bool, tview.Primitive) {
	x, y := event.Position()

	switch action {
	case tview.MouseLeftDown:
		w := m.windowAt(x, y)
		if w == nil {
			return false, nil
		}
		m.Activate(w)
		switch w.HitTest(x, y) {
		case RegionClose:
			if w.onClose != nil {
				w.onClose()
			}
			return true, nil
		case RegionMaxRestore:
			w.setDesktop(m.GetRect())
			w.ToggleMaximize()
			if w.onToggleMax != nil {
				w.onToggleMax()
			}
			return true, nil
		case RegionTitle:
			wx, wy, _, _ := w.GetRect()
			m.mode = dragMove
			m.dragWin = w
			m.grabDX = x - wx
			m.grabDY = y - wy
			return true, nil
		case RegionResize:
			wx, wy, ww, wh := w.GetRect()
			m.mode = dragResize
			m.dragWin = w
			m.grabRight = (wx + ww - 1) - x
			m.grabBot = (wy + wh - 1) - y
			return true, nil
		case RegionContent:
			return m.forwardContent(w, action, event)
		}
		return true, nil

	case tview.MouseMove:
		switch m.mode {
		case dragMove:
			m.moveTo(m.dragWin, x-m.grabDX, y-m.grabDY)
			return true, nil
		case dragResize:
			m.resizeTo(m.dragWin, x+m.grabRight, y+m.grabBot)
			return true, nil
		}
		return false, nil

	case tview.MouseLeftUp:
		if m.mode != dragNone {
			m.mode = dragNone
			m.dragWin = nil
			return true, nil
		}
		// Forward releases to content of the topmost window under the cursor so
		// the editor can end its own drags.
		if w := m.windowAt(x, y); w != nil && w.HitTest(x, y) == RegionContent {
			return m.forwardContent(w, action, event)
		}
		return false, nil
	}

	// Other actions (wheel, etc.) forward to the content under the cursor.
	if w := m.windowAt(x, y); w != nil && w.HitTest(x, y) == RegionContent {
		return m.forwardContent(w, action, event)
	}
	return false, nil
}

// forwardContent positions the window's content at its inner rect (so absolute
// coordinates line up) and dispatches the event to the content's MouseHandler.
func (m *Manager) forwardContent(w *Window, action tview.MouseAction, event *tcell.EventMouse) (bool, tview.Primitive) {
	if w.content == nil {
		return true, nil
	}
	ix, iy, iw, ih := w.GetInnerRect()
	w.content.SetRect(ix, iy, iw, ih)
	if mh := w.content.MouseHandler(); mh != nil {
		consumed, _ := mh(action, event, func(tview.Primitive) {})
		_ = consumed
	}
	return true, w.content
}

// moveTo sets the window origin to (nx,ny), clamped so the window stays fully
// within the desktop rect.
func (m *Manager) moveTo(w *Window, nx, ny int) {
	if w == nil {
		return
	}
	dx, dy, dw, dh := m.GetRect()
	_, _, ww, wh := w.GetRect()
	nx = clamp(nx, dx, dx+dw-ww)
	ny = clamp(ny, dy, dy+dh-wh)
	w.SetRect(nx, ny, ww, wh)
}

// resizeTo sets the window's bottom-right corner to (rx,ry), enforcing the min
// size and clamping the corner to the desktop rect.
func (m *Manager) resizeTo(w *Window, rx, ry int) {
	if w == nil {
		return
	}
	dx, dy, dw, dh := m.GetRect()
	wx, wy, _, _ := w.GetRect()
	rx = clamp(rx, wx+MinWindowWidth-1, dx+dw-1)
	ry = clamp(ry, wy+MinWindowHeight-1, dy+dh-1)
	w.SetRect(wx, wy, rx-wx+1, ry-wy+1)
}

// MoveActive nudges the active window by (dx,dy), clamped to the desktop. For
// the keyboard move mode (Ctrl+F5).
func (m *Manager) MoveActive(dx, dy int) {
	w := m.Active()
	if w == nil {
		return
	}
	wx, wy, _, _ := w.GetRect()
	m.moveTo(w, wx+dx, wy+dy)
}

// ResizeActive grows/shrinks the active window by (dw,dh) at its bottom-right
// corner, clamped to the desktop and the min size. For the keyboard size mode.
func (m *Manager) ResizeActive(dw, dh int) {
	w := m.Active()
	if w == nil {
		return
	}
	wx, wy, ww, wh := w.GetRect()
	m.resizeTo(w, wx+ww-1+dw, wy+wh-1+dh)
}

// clamp constrains v to [lo,hi]; if the range is empty (hi<lo) it returns lo.
func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
