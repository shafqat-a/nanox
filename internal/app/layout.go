// layout.go defines appRoot, the top-level layout container for the tui-based
// DOSEdit application. It places the menu bar on row 0 (full width), the status
// bar on the last row (full width), and the MDI Desktop in the rows between,
// clamping gracefully to small terminals. It mirrors the gallery's galRoot
// pattern (internal/gallery/gallery.go).
package app

import "dosedit/internal/tui"

// appRoot is the application's root layout container.
type appRoot struct {
	tui.BaseContainer
	menubar *tui.MenuBar
	desktop *tui.Desktop
	status  *tui.StatusBar
}

// newAppRoot assembles the root container from the three regions. The children
// are added so focus traversal and hit-testing can reach them.
func newAppRoot(menubar *tui.MenuBar, desktop *tui.Desktop, status *tui.StatusBar) *appRoot {
	r := &appRoot{menubar: menubar, desktop: desktop, status: status}
	r.Add(menubar)
	r.Add(desktop)
	r.Add(status)
	return r
}

// SetBounds lays the three regions out within r: menu bar on the top row,
// status bar on the bottom row, desktop in between.
func (r *appRoot) SetBounds(b tui.Rect) {
	r.BaseContainer.SetBounds(b)
	if b.W <= 0 || b.H <= 0 {
		return
	}
	r.menubar.SetBounds(tui.Rect{X: b.X, Y: b.Y, W: b.W, H: 1})
	if b.H >= 2 {
		r.status.SetBounds(tui.Rect{X: b.X, Y: b.Y + b.H - 1, W: b.W, H: 1})
	}
	midY := b.Y + 1
	midH := b.H - 2
	if midH < 1 {
		midH = 1
		midY = b.Y
	}
	r.desktop.SetBounds(tui.Rect{X: b.X, Y: midY, W: b.W, H: midH})
}

// Draw paints the children in order (desktop under the bars).
func (r *appRoot) Draw(s tui.Surface) {
	r.desktop.Draw(s.Clip(r.desktop.Bounds()))
	r.menubar.Draw(s.Clip(r.menubar.Bounds()))
	r.status.Draw(s.Clip(r.status.Bounds()))
}
