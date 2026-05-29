// Package app wires DOSEdit's UI primitives, window management, command
// dispatch and global key routing into a runnable tview application (spec §6.1,
// §7, §8, Appendix A). It owns the application loop, the desktop / menu bar /
// status bar, the set of MDI editor windows, and the modal-dialog overlay.
package app

import (
	"dosedit/internal/buffer"
	"dosedit/internal/ui"

	"github.com/epiclabs-io/winman"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// modalPageName is the tview.Pages page used to host a centred modal dialog
// over the main layout.
const modalPageName = "modal"

// mainPageName is the tview.Pages page hosting the master row layout.
const mainPageName = "main"

// App holds the whole running application's state and collaborators.
type App struct {
	tapp      *tview.Application
	desktop   *ui.Desktop
	menubar   *ui.MenuBar
	statusbar *ui.StatusBar

	root  *tview.Flex  // menu / desktop / status row layout
	pages *tview.Pages // main layout + modal overlay

	windows []*ui.EditorWindow
	active  *ui.EditorWindow

	nextWindowNumber int
	modalOpen        bool
	moveSize         bool // keyboard move/size mode active (Ctrl+F5)

	// placed records windows already sized against real desktop geometry, so a
	// window created before the first layout (when the manager's inner rect is
	// still a tview default) is re-sized once on first activation.
	placed map[*ui.EditorWindow]bool
}

// New constructs an App over the supplied collaborators, builds the menu bar
// from its own command tree and assembles the root layout and modal-overlay
// pages. The first editor window is opened separately via OpenInitialWindow so
// the layout has been installed before a window is focused.
func New(tapp *tview.Application, desktop *ui.Desktop, statusbar *ui.StatusBar) *App {
	a := &App{
		tapp:             tapp,
		desktop:          desktop,
		statusbar:        statusbar,
		nextWindowNumber: 1,
		placed:           map[*ui.EditorWindow]bool{},
	}

	// BuildMenus closes over the App's command methods, so the bar can be built
	// only after the App value exists.
	a.menubar = ui.NewMenuBar(a.BuildMenus())
	menubar := a.menubar

	a.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(menubar, 1, 0, false).
		AddItem(desktop.Primitive(), 0, 1, true).
		AddItem(statusbar, 1, 0, false)

	a.pages = tview.NewPages()
	a.pages.AddPage(mainPageName, a.root, true, true)

	// When the menu bar closes, return focus to the active editor and reset
	// the status-bar context.
	menubar.SetOnClose(func() {
		a.statusbar.SetContext(ui.CtxEditing)
		a.focusActiveEditor()
	})

	// When the menu bar becomes active (keyboard or mouse), switch the status
	// bar to the menu context.
	menubar.SetOnActivate(func() {
		a.statusbar.SetContext(ui.CtxMenu)
	})

	// Mouse support for the menu bar. The open dropdown is drawn below row 0 and
	// thus outside the MenuBar primitive's rect, so tview's per-primitive routing
	// never delivers dropdown clicks to it. Capture mouse events at the app level
	// and hit-test them against the bar in absolute coordinates. Only events the
	// menu actually handles are swallowed; everything else passes through so
	// winman window drag/resize and the editor keep working.
	tapp.SetMouseCapture(func(ev *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		if a.modalOpen {
			return ev, action
		}
		x, y := ev.Position()
		if a.menubar.HandleMouse(action, x, y) {
			// The bar consumed it. If it became active, give it keyboard focus so
			// arrow-key navigation takes over.
			if a.menubar.IsActive() {
				a.tapp.SetFocus(a.menubar)
			}
			return nil, action
		}
		return ev, action
	})

	return a
}

// Root returns the top-level primitive to install via SetRoot (the pages
// container so modal dialogs can overlay the main layout).
func (a *App) Root() tview.Primitive { return a.pages }

// OpenInitialWindow opens the single untitled editor window the app starts
// with, focused. Call after the root layout is installed.
func (a *App) OpenInitialWindow() {
	a.newEditorWindow(buffer.NewUntitled())
}

// --- window lifecycle ------------------------------------------------------

// newEditorWindow builds an editor window over buf, wires its callbacks and
// adds it to the desktop. The new window becomes the active window.
func (a *App) newEditorWindow(buf *buffer.Buffer) *ui.EditorWindow {
	ed := ui.NewEditor(buf)
	number := a.nextWindowNumber
	a.nextWindowNumber++

	var w *ui.EditorWindow
	w = ui.NewEditorWindow(a.desktop, ed, number,
		func() { a.closeWindow(w) },
		func() { w.ToggleMaximize() },
	)

	// Cursor moves drive the status bar (1-based ln/col + INS/OVR).
	ed.SetOnCursorMove(func(ln, col int, ins bool) {
		a.statusbar.SetCursor(ln, col, ins)
		if !a.menubar.IsActive() && !a.modalOpen {
			a.statusbar.SetContext(ui.CtxEditing)
		}
	})
	// Buffer changes refresh the title and dirty indicator.
	ed.SetOnChange(func() {
		w.Update()
		a.statusbar.SetModified(ed.Buffer().Modified)
	})

	a.windows = append(a.windows, w)
	a.activate(w)
	return w
}

// activate makes w the active (focused, top-most) window and updates the
// status bar to reflect its editor state.
func (a *App) activate(w *ui.EditorWindow) {
	if w == nil {
		return
	}
	a.active = w
	a.placeWindow(w)
	a.desktop.Manager().SetZ(w, winman.WindowZTop)
	if !a.modalOpen && !a.menubar.IsActive() {
		a.tapp.SetFocus(w)
		a.statusbar.SetContext(ui.CtxEditing)
	}
	w.Update()
	a.statusbar.SetModified(w.Editor().Buffer().Modified)
	ed := w.Editor()
	// Re-emit the cursor position for the freshly focused editor.
	ed.SetOnCursorMove(func(ln, col int, ins bool) {
		a.statusbar.SetCursor(ln, col, ins)
		if !a.menubar.IsActive() && !a.modalOpen {
			a.statusbar.SetContext(ui.CtxEditing)
		}
	})
}

// placeWindow gives w a sensible cascaded rect sized to ~2/3 of the desktop.
//
// Windows may be created before the layout has run, when the manager's inner
// rect is still a tview default (around 15x10). In that case we size against
// the 80x23 reference desktop and leave w unmarked, so the next placement
// attempt (on activation or a global key) corrects it once real geometry is
// available. The window manager clamps any oversize rect to the real desktop on
// each draw, so an interim reference-sized rect always renders sensibly. A
// window is only marked placed once it has been sized against real geometry; it
// is never moved again afterwards.
func (a *App) placeWindow(w *ui.EditorWindow) {
	if a.placed[w] {
		return
	}
	dx, dy, dw, dh := a.desktop.Manager().GetInnerRect()
	real := dw >= 40 && dh >= 15
	if !real {
		// Not laid out yet: use the 80x23 reference desktop.
		dx, dy, dw, dh = 0, 1, 80, 23
	}
	ww := dw * 2 / 3
	wh := dh * 2 / 3
	if ww < winman.MinWindowWidth {
		ww = winman.MinWindowWidth
	}
	if wh < winman.MinWindowHeight {
		wh = winman.MinWindowHeight
	}
	off := ((w.Number() - 1) % 6) * 2
	w.SetRect(dx+off, dy+off, ww, wh)
	if real {
		a.placed[w] = true
	}
}

// focusActiveEditor returns keyboard focus to the active editor window (or the
// desktop if there are none).
func (a *App) focusActiveEditor() {
	if a.modalOpen {
		return
	}
	if a.active != nil {
		a.tapp.SetFocus(a.active)
		return
	}
	a.tapp.SetFocus(a.desktop.Primitive())
}

// closeWindow removes w from the desktop and the window list, then activates a
// remaining window if any. Callers that need a dirty-prompt should route
// through cmdCloseWindow instead.
func (a *App) closeWindow(w *ui.EditorWindow) {
	if w == nil {
		return
	}
	a.desktop.Manager().RemoveWindow(w)
	for i, x := range a.windows {
		if x == w {
			a.windows = append(a.windows[:i], a.windows[i+1:]...)
			break
		}
	}
	if a.active == w {
		a.active = nil
	}
	if len(a.windows) > 0 {
		a.activate(a.windows[len(a.windows)-1])
	} else {
		a.focusActiveEditor()
	}
}

// windowIndex returns the position of w in the window slice, or -1.
func (a *App) windowIndex(w *ui.EditorWindow) int {
	for i, x := range a.windows {
		if x == w {
			return i
		}
	}
	return -1
}

// cycleWindow activates the next (dir>0) or previous (dir<0) window.
func (a *App) cycleWindow(dir int) {
	n := len(a.windows)
	if n == 0 {
		return
	}
	i := a.windowIndex(a.active)
	if i < 0 {
		i = 0
	}
	i = (i + dir + n) % n
	a.activate(a.windows[i])
}

// activateByNumber activates the window whose MDI number is num (Alt+1..9).
func (a *App) activateByNumber(num int) {
	for _, w := range a.windows {
		if w.Number() == num {
			a.activate(w)
			return
		}
	}
}

// --- window arrangement ----------------------------------------------------

// cascadeWindows lays the windows out in an overlapping cascade.
func (a *App) cascadeWindows() {
	dx, dy, dw, dh := a.desktop.Manager().GetInnerRect()
	if dw <= 0 || dh <= 0 {
		return
	}
	ww := dw * 2 / 3
	wh := dh * 2 / 3
	if ww < winman.MinWindowWidth {
		ww = winman.MinWindowWidth
	}
	if wh < winman.MinWindowHeight {
		wh = winman.MinWindowHeight
	}
	for i, w := range a.windows {
		if w.IsMaximized() {
			w.Restore()
		}
		off := (i % 6) * 2
		w.SetRect(dx+off, dy+off, ww, wh)
	}
	if a.active != nil {
		a.activate(a.active)
	}
}

// tileWindows lays the windows out in a non-overlapping grid (F5).
func (a *App) tileWindows() {
	n := len(a.windows)
	if n == 0 {
		return
	}
	dx, dy, dw, dh := a.desktop.Manager().GetInnerRect()
	if dw <= 0 || dh <= 0 {
		return
	}
	// Choose a near-square grid.
	cols := 1
	for cols*cols < n {
		cols++
	}
	rows := (n + cols - 1) / cols
	cw := dw / cols
	ch := dh / rows
	if cw < winman.MinWindowWidth {
		cw = winman.MinWindowWidth
	}
	if ch < winman.MinWindowHeight {
		ch = winman.MinWindowHeight
	}
	for i, w := range a.windows {
		if w.IsMaximized() {
			w.Restore()
		}
		c := i % cols
		r := i / cols
		w.SetRect(dx+c*cw, dy+r*ch, cw, ch)
	}
	if a.active != nil {
		a.activate(a.active)
	}
}

// --- modal overlay ---------------------------------------------------------

// showModal centres prim (sized w×h) over the main layout, blocks the windows
// beneath it and gives it focus. The status bar switches to the dialog context.
func (a *App) showModal(prim tview.Primitive, w, h int) {
	a.modalOpen = true
	a.statusbar.SetContext(ui.CtxDialog)

	// Centre using a Flex sandwich (flexible spacers around a fixed cell).
	row := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(prim, w, 0, true).
		AddItem(nil, 0, 1, false)
	center := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(row, h, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage(modalPageName, center, true, true)
	a.tapp.SetFocus(prim)
}

// closeModal removes the modal overlay and restores focus to the active editor.
func (a *App) closeModal() {
	if !a.modalOpen {
		return
	}
	a.pages.RemovePage(modalPageName)
	a.modalOpen = false
	a.statusbar.SetContext(ui.CtxEditing)
	a.focusActiveEditor()
}
