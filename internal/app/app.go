// Package app wires DOSEdit's UI primitives, window management, command
// dispatch and global key routing into a runnable tview application (spec §6.1,
// §7, §8, Appendix A).
//
// Focus architecture: a single root primitive, UIManager (ui_manager.go), is
// the sole tview-focused primitive and routes ALL keyboard and mouse input via
// a derived scope (DIALOG / MENU / WINDOWS). The App owns the collaborators
// (window manager, menu bar, status bar), the set of MDI editor windows and the
// command tree; it installs the WINDOWS-scope accelerator hook and the dialog
// push/pop flows on the UIManager. There is no app-level SetInputCapture /
// SetMouseCapture, no modal overlay layer and no winman dependency — the
// UIManager replaces all of that.
package app

import (
	"fmt"

	"dosedit/internal/buffer"
	"dosedit/internal/ui"
	"dosedit/internal/ui/wm"

	"github.com/rivo/tview"
)

// App holds the whole running application's state and collaborators.
type App struct {
	tapp      *tview.Application
	wm        *wm.Manager
	menubar   *ui.MenuBar
	statusbar *ui.StatusBar
	ui        *UIManager

	// windows is the App's bookkeeping list of MDI editor windows, parallel to
	// the window manager's z-order but kept in creation order so Alt+1..9 and the
	// dirty-prompt walks are stable. editorOf maps each window to its editor.
	windows  []*wm.Window
	editorOf map[*wm.Window]*ui.Editor
	numberOf map[*wm.Window]int

	nextWindowNumber int
	moveSize         bool // keyboard move/size mode active (Ctrl+F5)

	// lineNumbers is the app-wide preference for showing line numbers in editor
	// windows. New windows inherit it; setLineNumbers applies it to all open
	// editors. Default false. Settable before windows are created via
	// SetLineNumbersDefault, and changeable at runtime via the Edit > Options
	// dialog.
	lineNumbers bool

	// placed records windows already sized against real desktop geometry, so a
	// window created before the first layout (when the manager's rect is still a
	// tview default) is re-sized once on first activation.
	placed map[*wm.Window]bool
}

// New constructs an App over the supplied collaborators, builds the menu bar
// from its own command tree and assembles the root UIManager. The first editor
// window is opened separately via OpenInitialWindow so the layout has run before
// a window is focused.
func New(tapp *tview.Application, manager *wm.Manager, statusbar *ui.StatusBar) *App {
	a := &App{
		tapp:             tapp,
		wm:               manager,
		statusbar:        statusbar,
		editorOf:         map[*wm.Window]*ui.Editor{},
		numberOf:         map[*wm.Window]int{},
		nextWindowNumber: 1,
		placed:           map[*wm.Window]bool{},
	}

	// BuildMenus closes over the App's command methods, so the bar can be built
	// only after the App value exists.
	a.menubar = ui.NewMenuBar(a.BuildMenus())

	a.ui = NewUIManager(manager, a.menubar, statusbar)
	a.ui.SetGlobalKey(a.routeGlobalKey)

	// Keep the UIManager's menu scope in sync with the bar, and drive the status
	// bar context off the bar's transitions.
	a.menubar.SetOnActivate(func() {
		a.statusbar.SetContext(ui.CtxMenu)
		a.ui.SyncMenuActive()
	})
	a.menubar.SetOnClose(func() {
		a.ui.SyncMenuActive()
		a.statusbar.SetContext(ui.CtxEditing)
	})

	return a
}

// Root returns the top-level primitive to install via SetRoot (the UIManager,
// which is also the sole focus target).
func (a *App) Root() tview.Primitive { return a.ui }

// OpenInitialWindow opens the single untitled editor window the app starts
// with, focused. Call after the root layout is installed.
func (a *App) OpenInitialWindow() {
	a.newEditorWindow(buffer.NewUntitled())
}

// SetLineNumbersDefault sets the app-wide line-numbers preference. Call before
// windows are created (e.g. from the CLI flag) so the initial window honours it.
func (a *App) SetLineNumbersDefault(on bool) {
	a.lineNumbers = on
}

// setLineNumbers sets the line-numbers preference and applies it to every open
// editor, then forces a redraw so the change shows immediately.
func (a *App) setLineNumbers(on bool) {
	a.lineNumbers = on
	for _, w := range a.windows {
		a.editorOf[w].SetLineNumbers(on)
	}
	a.tapp.Draw()
}

// --- window lifecycle ------------------------------------------------------

// newEditorWindow builds an editor window over buf, wires its callbacks and adds
// it to the window manager. The new window becomes active.
func (a *App) newEditorWindow(buf *buffer.Buffer) *wm.Window {
	ed := ui.NewEditor(buf)
	ed.SetLineNumbers(a.lineNumbers)
	number := a.nextWindowNumber
	a.nextWindowNumber++

	w := wm.NewWindow(ed, "")
	a.editorOf[w] = ed
	a.numberOf[w] = number

	w.SetOnClose(func() { a.closeWindowPrompt(w, func() { a.closeWindow(w) }) })
	w.SetOnToggleMax(func() { /* manager already toggled; redraw on next loop */ })

	// Cursor moves drive the status bar (1-based ln/col + INS/OVR).
	ed.SetOnCursorMove(func(ln, col int, ins bool) {
		a.statusbar.SetCursor(ln, col, ins)
		if a.ui.currentScope() == scopeWindows {
			a.statusbar.SetContext(ui.CtxEditing)
		}
	})
	// Buffer changes refresh the title and dirty indicator.
	ed.SetOnChange(func() {
		a.updateTitle(w)
		a.statusbar.SetModified(ed.Buffer().Modified)
	})

	a.windows = append(a.windows, w)
	a.wm.Add(w)
	a.activate(w)
	return w
}

// updateTitle formats and applies w's title: "[n] name" with a leading "*" on
// the name when the buffer is modified.
func (a *App) updateTitle(w *wm.Window) {
	ed := a.editorOf[w]
	name := ed.Buffer().DisplayName()
	if ed.Buffer().Modified {
		name = "*" + name
	}
	w.SetTitle(fmt.Sprintf("[%d] %s", a.numberOf[w], name))
}

// activate raises w to the top of the z-order, makes it active and refreshes the
// status bar from its editor state.
func (a *App) activate(w *wm.Window) {
	if w == nil {
		return
	}
	a.placeWindow(w)
	a.wm.Activate(w)
	a.updateTitle(w)
	ed := a.editorOf[w]
	a.statusbar.SetModified(ed.Buffer().Modified)
	if a.ui.currentScope() == scopeWindows {
		a.statusbar.SetContext(ui.CtxEditing)
	}
}

// placeWindow gives w a sensible cascaded rect sized to ~2/3 of the desktop.
//
// Windows may be created before the layout has run, when the manager's rect is
// still a tview default. In that case we size against the 80x23 reference
// desktop and leave w unmarked, so the next placement attempt (on activation or
// a global key) corrects it once real geometry is available. The manager clamps
// any oversize rect to the real desktop on each draw, so an interim
// reference-sized rect always renders sensibly. A window is only marked placed
// once it has been sized against real geometry; it is never moved again after.
func (a *App) placeWindow(w *wm.Window) {
	if a.placed[w] {
		return
	}
	dx, dy, dw, dh := a.wm.GetRect()
	real := dw >= 40 && dh >= 15
	if !real {
		// Not laid out yet: use the 80x23 reference desktop (rows 1..23).
		dx, dy, dw, dh = 0, 1, 80, 23
	}
	ww := dw * 2 / 3
	wh := dh * 2 / 3
	if ww < wm.MinWindowWidth {
		ww = wm.MinWindowWidth
	}
	if wh < wm.MinWindowHeight {
		wh = wm.MinWindowHeight
	}
	off := ((a.numberOf[w] - 1) % 6) * 2
	w.SetRect(dx+off, dy+off, ww, wh)
	if real {
		a.placed[w] = true
	}
}

// activeWindow returns the manager's active window, or nil.
func (a *App) activeWindow() *wm.Window { return a.wm.Active() }

// closeWindow removes w from the manager and the bookkeeping list, then
// activates a remaining window if any. Callers that need a dirty-prompt should
// route through cmdCloseActive / closeWindowPrompt instead.
func (a *App) closeWindow(w *wm.Window) {
	if w == nil {
		return
	}
	a.wm.Remove(w)
	for i, x := range a.windows {
		if x == w {
			a.windows = append(a.windows[:i], a.windows[i+1:]...)
			break
		}
	}
	delete(a.editorOf, w)
	delete(a.numberOf, w)
	delete(a.placed, w)
	if cur := a.wm.Active(); cur != nil {
		a.activate(cur)
	}
}

// windowIndex returns the position of w in the bookkeeping slice, or -1.
func (a *App) windowIndex(w *wm.Window) int {
	for i, x := range a.windows {
		if x == w {
			return i
		}
	}
	return -1
}

// cycleWindow activates the next (dir>0) or previous (dir<0) window.
func (a *App) cycleWindow(dir int) {
	if len(a.windows) == 0 {
		return
	}
	if dir < 0 {
		a.wm.Prev()
	} else {
		a.wm.Next()
	}
	if cur := a.wm.Active(); cur != nil {
		a.activate(cur)
	}
}

// activateByNumber activates the window whose MDI number is num (Alt+1..9).
func (a *App) activateByNumber(num int) {
	for _, w := range a.windows {
		if a.numberOf[w] == num {
			a.activate(w)
			return
		}
	}
}

// --- window arrangement ----------------------------------------------------

// cascadeWindows lays the windows out in an overlapping cascade.
func (a *App) cascadeWindows() {
	for _, w := range a.windows {
		a.placed[w] = true
	}
	a.wm.Cascade()
	if cur := a.wm.Active(); cur != nil {
		a.activate(cur)
	}
}

// tileWindows lays the windows out in a non-overlapping grid (F5).
func (a *App) tileWindows() {
	for _, w := range a.windows {
		a.placed[w] = true
	}
	a.wm.Tile()
	if cur := a.wm.Active(); cur != nil {
		a.activate(cur)
	}
}
