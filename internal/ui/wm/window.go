// Package wm is DOSEdit's native MDI window manager. It replaces the
// third-party winman dependency with a self-contained implementation that
// paints the grey hatched desktop and a set of overlapping, draggable,
// resizable, maximizable child windows in the authentic VB-for-DOS look
// (spec §6.3, §4.3 colours, §4.4 glyphs).
//
// A Manager owns the desktop region (the rows between the menu bar and the
// status bar) and a z-ordered stack of Windows. Each Window hosts a single
// tview.Primitive (the editor) as its content. Drawing, focus framing, mouse
// drag/resize/activate and keyboard move/size are all handled natively here;
// there are no post-paint hacks.
//
// The package depends only on dosedit/internal/theme, tcell, tview and the
// standard library. It never imports winman.
package wm

import (
	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// MinWindowWidth and MinWindowHeight are the smallest rect a window may shrink
// to via resize. Four columns/rows guarantee a drawable frame plus a 2x1
// interior so the title-bar buttons never overlap.
const (
	MinWindowWidth  = 8
	MinWindowHeight = 4
)

// Region identifies which part of a window an absolute screen coordinate hits.
type Region int

const (
	// RegionNone means the point is not inside the window at all.
	RegionNone Region = iota
	// RegionContent is the inner content area inside the 1-cell frame.
	RegionContent
	// RegionTitle is any title-row cell that is not a button.
	RegionTitle
	// RegionClose is the close button cell (top-left).
	RegionClose
	// RegionMaxRestore is the maximize/restore button cell (top-right).
	RegionMaxRestore
	// RegionResize is the bottom-right corner cell (the resize grip).
	RegionResize
)

// Window is one MDI child: a framed, titled box hosting a tview.Primitive.
// It embeds *tview.Box purely for rect bookkeeping (SetRect/GetRect); all
// framing, title and button drawing is done natively in Draw so the DOS
// colour treatment is exact.
type Window struct {
	*tview.Box

	content tview.Primitive
	title   string
	active  bool

	maximized bool
	restoreX  int // saved rect while maximized
	restoreY  int
	restoreW  int
	restoreH  int

	// desktop rect to fill when maximized; set by the Manager each Draw and on
	// ToggleMaximize so the window knows the area to expand into.
	deskX, deskY, deskW, deskH int

	onClose     func()
	onToggleMax func()
}

// NewWindow creates a window hosting content with the given title. The title
// string is drawn verbatim (centered); callers supply the "[n] name" / "*"
// decoration themselves.
func NewWindow(content tview.Primitive, title string) *Window {
	return &Window{
		Box:     tview.NewBox(),
		content: content,
		title:   title,
	}
}

// SetTitle sets the (already-decorated) title string.
func (w *Window) SetTitle(title string) { w.title = title }

// Title returns the current title string.
func (w *Window) Title() string { return w.title }

// Content returns the hosted primitive.
func (w *Window) Content() tview.Primitive { return w.content }

// SetActive sets whether this window draws with the active (double-line,
// magenta-title) treatment.
func (w *Window) SetActive(on bool) { w.active = on }

// IsActive reports the active flag.
func (w *Window) IsActive() bool { return w.active }

// GetInnerRect returns the content area inside the 1-cell frame.
func (w *Window) GetInnerRect() (x, y, width, height int) {
	x, y, width, height = w.GetRect()
	x++
	y++
	width -= 2
	height -= 2
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	return x, y, width, height
}

// IsMaximized reports whether the window is filling the desktop rect.
func (w *Window) IsMaximized() bool { return w.maximized }

// setDesktop records the desktop rect the window expands into when maximized.
// The Manager calls this before drawing or toggling.
func (w *Window) setDesktop(x, y, width, height int) {
	w.deskX, w.deskY, w.deskW, w.deskH = x, y, width, height
}

// SetMaximized maximizes (fills the desktop rect) or restores the saved rect.
// Idempotent.
func (w *Window) SetMaximized(on bool) {
	if on == w.maximized {
		return
	}
	if on {
		w.restoreX, w.restoreY, w.restoreW, w.restoreH = w.GetRect()
		w.maximized = true
		if w.deskW > 0 && w.deskH > 0 {
			w.SetRect(w.deskX, w.deskY, w.deskW, w.deskH)
		}
	} else {
		w.maximized = false
		w.SetRect(w.restoreX, w.restoreY, w.restoreW, w.restoreH)
	}
}

// ToggleMaximize flips between maximized and restored.
func (w *Window) ToggleMaximize() { w.SetMaximized(!w.maximized) }

// SetOnClose registers the close-button callback.
func (w *Window) SetOnClose(fn func()) { w.onClose = fn }

// SetOnToggleMax registers the maximize/restore-button callback.
func (w *Window) SetOnToggleMax(fn func()) { w.onToggleMax = fn }

// frameStyle returns the (border, title) styles for the current active state.
func (w *Window) frameStyle() (frame, title tcell.Style) {
	if w.active {
		return theme.ActiveFrame(), theme.ActiveTitle()
	}
	return theme.InactiveFrame(), theme.InactiveTitle()
}

// frameGlyphs returns the corner/edge runes for the current active state:
// double-line when active, single-line when inactive.
func (w *Window) frameGlyphs() (tl, tr, bl, br, h, v rune) {
	if w.active {
		return theme.TLDouble, theme.TRDouble, theme.BLDouble, theme.BRDouble, theme.HDouble, theme.VDouble
	}
	return theme.TLSingle, theme.TRSingle, theme.BLSingle, theme.BRSingle, theme.HSingle, theme.VSingle
}

// closeButtonX is the screen column of the close button glyph (top-left,
// column +1 inside the frame).
func (w *Window) closeButtonX() int {
	x, _, _, _ := w.GetRect()
	return x + 1
}

// maxButtonX is the screen column of the maximize/restore button glyph
// (top-right, column -2 inside the frame).
func (w *Window) maxButtonX() int {
	x, _, width, _ := w.GetRect()
	return x + width - 2
}

// Draw renders the frame, title bar and buttons, then positions and draws the
// content inside the inner rect. Active windows get a double-line frame with
// theme.ActiveFrame() and a magenta title bar (theme.ActiveTitle()); inactive
// windows get a single-line frame with theme.InactiveFrame()/InactiveTitle().
func (w *Window) Draw(screen tcell.Screen) {
	x, y, width, height := w.GetRect()
	if width < 2 || height < 2 {
		return
	}

	frame, title := w.frameStyle()
	tl, tr, bl, br, h, v := w.frameGlyphs()
	_, frameBg, _ := frame.Decompose()
	titleFg, _, _ := title.Decompose()

	// Fill the whole window interior with the frame background so any cells the
	// content does not overpaint read as the editor surface.
	fill := tcell.StyleDefault.Background(frameBg)
	for ry := y; ry < y+height; ry++ {
		for rx := x; rx < x+width; rx++ {
			screen.SetContent(rx, ry, ' ', nil, fill)
		}
	}

	right := x + width - 1
	bottom := y + height - 1

	// Title row: background paint, then frame corners over it.
	for rx := x; rx <= right; rx++ {
		screen.SetContent(rx, y, ' ', nil, title)
	}
	screen.SetContent(x, y, tl, nil, frame)
	screen.SetContent(right, y, tr, nil, frame)

	// Side edges.
	for ry := y + 1; ry < bottom; ry++ {
		screen.SetContent(x, ry, v, nil, frame)
		screen.SetContent(right, ry, v, nil, frame)
	}

	// Bottom edge + corners.
	for rx := x; rx <= right; rx++ {
		screen.SetContent(rx, bottom, h, nil, frame)
	}
	screen.SetContent(x, bottom, bl, nil, frame)
	screen.SetContent(right, bottom, br, nil, frame)

	// Centered title text within the interior of the title row.
	if width > 2 {
		tview.Print(screen, tview.Escape(w.title), x+1, y, width-2, tview.AlignCenter, titleFg)
	}

	// Buttons: close (top-left) and maximize/restore (top-right). Only draw
	// them when there is room (need columns +1 and width-2 to be distinct
	// interior cells).
	if width >= 6 {
		screen.SetContent(w.closeButtonX(), y, theme.BtnClose, nil, title)
		maxGlyph := theme.BtnMax
		if w.maximized {
			maxGlyph = theme.BtnRestore
		}
		screen.SetContent(w.maxButtonX(), y, maxGlyph, nil, title)
	}

	// Position and draw content in the inner rect.
	ix, iy, iw, ih := w.GetInnerRect()
	if iw > 0 && ih > 0 && w.content != nil {
		w.content.SetRect(ix, iy, iw, ih)
		w.content.Draw(screen)
	}
}

// HitTest classifies the absolute screen coordinate (x,y) relative to this
// window: the close/max button cells, any other title-row cell (RegionTitle),
// the bottom-right corner (RegionResize), the inner content area
// (RegionContent), or RegionNone when outside the window.
func (w *Window) HitTest(x, y int) Region {
	rx, ry, width, height := w.GetRect()
	if width < 2 || height < 2 {
		return RegionNone
	}
	if x < rx || x >= rx+width || y < ry || y >= ry+height {
		return RegionNone
	}

	right := rx + width - 1
	bottom := ry + height - 1

	// Bottom-right corner is the resize grip (takes priority over the frame).
	if x == right && y == bottom {
		return RegionResize
	}

	// Title row.
	if y == ry {
		if width >= 6 {
			if x == w.closeButtonX() {
				return RegionClose
			}
			if x == w.maxButtonX() {
				return RegionMaxRestore
			}
		}
		return RegionTitle
	}

	// Inner content area.
	ix, iy, iw, ih := w.GetInnerRect()
	if iw > 0 && ih > 0 && x >= ix && x < ix+iw && y >= iy && y < iy+ih {
		return RegionContent
	}

	// Remaining frame cells (side/bottom edges that are not the resize grip).
	return RegionNone
}
