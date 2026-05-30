// Package app wires DOSEdit's tui toolkit primitives, MDI window management,
// command dispatch and global key routing into a runnable application on the
// tcell-only tui toolkit (replacing the old tview/winman shell).
//
// Focus & modality architecture: the tui.App is the single authority for the
// event loop, focus and modality. The App installs one root layout (appRoot:
// MenuBar / Desktop / StatusBar) via SetRoot, and a global key hook via
// SetKeyHook. The hook is consulted ONLY when no modal is open and BEFORE the
// focused widget, so accelerators never fire over a dialog or menu. Dialogs are
// shown with tui.App.PushModal and dismissed by the dialog's onOK/onCancel
// callbacks via tui.App.PopModal — the App makes them truly modal (background
// clicks are swallowed). The active editor receives keys because it is the
// focused widget; the menu bar drives its own dropdown overlay as a modal.
package app

import (
	"fmt"

	"dosedit/internal/buffer"
	"dosedit/internal/edit"
	"dosedit/internal/tui"
)

// App holds the whole running application's state and collaborators.
type App struct {
	app       *tui.App
	root      *appRoot
	desktop   *tui.Desktop
	menubar   *tui.MenuBar
	statusbar *tui.StatusBar

	// windows is the bookkeeping list of MDI editor windows in creation order
	// (stable for Alt+1..9 and the dirty-prompt walks). editorOf maps each window
	// to its editor; numberOf to its MDI number.
	windows  []*tui.Window
	editorOf map[*tui.Window]*edit.Editor
	numberOf map[*tui.Window]int

	nextWindowNumber int
	moveSize         bool // keyboard move/size mode active (Ctrl+F5)

	// lineNumbers is the app-wide preference for showing line numbers in editor
	// windows. New windows inherit it; setLineNumbers applies it to all open
	// editors. Default false. Settable before windows are created via
	// SetLineNumbersDefault, and changeable at runtime via Edit > Options.
	lineNumbers bool
}

// New constructs the App over the supplied tui.App, builds the menu bar from its
// own command tree, assembles the root layout and installs the global key hook.
// The first editor window is opened separately via OpenInitialWindow so the
// layout has run before a window is focused.
func New(tapp *tui.App) *App {
	a := &App{
		app:              tapp,
		desktop:          tui.NewDesktop(),
		statusbar:        tui.NewStatusBar(),
		editorOf:         map[*tui.Window]*edit.Editor{},
		numberOf:         map[*tui.Window]int{},
		nextWindowNumber: 1,
	}

	// BuildMenus closes over the App's command methods, so the bar can be built
	// only after the App value exists.
	a.menubar = tui.NewMenuBar(a.BuildMenus())
	a.menubar.SetApp(tapp)

	// Drive the status bar context off the menu bar's transitions, and refocus the
	// active editor when the menu closes.
	a.menubar.SetOnActivate(func() { a.statusbar.SetContext(tui.CtxMenu) })
	a.menubar.SetOnClose(func() {
		a.statusbar.SetContext(tui.CtxEditing)
		a.focusActiveEditor()
	})

	a.root = newAppRoot(a.menubar, a.desktop, a.statusbar)
	tapp.SetRoot(a.root)
	tapp.SetKeyHook(a.routeGlobalKey)

	return a
}

// OpenInitialWindow opens the single untitled editor window the app starts with,
// focused. Call after the root layout is installed.
func (a *App) OpenInitialWindow() {
	a.newEditorWindow(buffer.NewUntitled())
}

// SetLineNumbersDefault sets the app-wide line-numbers preference. Call before
// windows are created (e.g. from the CLI flag) so the initial window honours it.
func (a *App) SetLineNumbersDefault(on bool) { a.lineNumbers = on }

// Run runs the underlying tui.App event loop until Stop.
func (a *App) Run() error { return a.app.Run() }

// setLineNumbers sets the line-numbers preference and applies it to every open
// editor, then forces a redraw so the change shows immediately.
func (a *App) setLineNumbers(on bool) {
	a.lineNumbers = on
	for _, w := range a.windows {
		a.editorOf[w].SetLineNumbers(on)
	}
	a.app.Redraw()
}

// --- window lifecycle ------------------------------------------------------

// newEditorWindow builds an editor window over buf, wires its callbacks, adds it
// to the desktop and focuses it. The new window becomes active.
func (a *App) newEditorWindow(buf *buffer.Buffer) *tui.Window {
	ed := edit.NewEditor(buf)
	ed.SetLineNumbers(a.lineNumbers)
	number := a.nextWindowNumber
	a.nextWindowNumber++

	w := tui.NewWindow(ed, "")
	a.editorOf[w] = ed
	a.numberOf[w] = number

	w.SetOnClose(func() { a.closeWindowPrompt(w, func() { a.closeWindow(w) }) })
	w.SetOnToggleMax(func() { w.ToggleMaximize(); a.app.Redraw() })

	// Cursor moves drive the status bar (1-based ln/col + INS/OVR).
	ed.SetOnCursorMove(func(ln, col int, ins bool) {
		a.statusbar.SetCursor(ln, col, ins)
		if !a.menubar.IsActive() && !a.moveSize {
			a.statusbar.SetContext(tui.CtxEditing)
		}
	})
	// Buffer changes refresh the title and dirty indicator.
	ed.SetOnChange(func() {
		a.updateTitle(w)
		a.statusbar.SetModified(ed.Buffer().Modified)
	})

	a.windows = append(a.windows, w)
	a.placeWindow(w)
	a.desktop.AddWindow(w)
	a.activate(w)
	return w
}

// placeWindow gives w a cascaded rect sized to ~2/3 of the desktop. The desktop
// clamps any oversize rect on each layout.
func (a *App) placeWindow(w *tui.Window) {
	b := a.desktop.Bounds()
	dx, dy, dw, dh := b.X, b.Y, b.W, b.H
	if dw < 40 || dh < 15 {
		// Not laid out yet: use the 80x23 reference desktop (rows 1..23).
		dx, dy, dw, dh = 0, 1, 80, 23
	}
	ww := dw * 2 / 3
	wh := dh * 2 / 3
	off := ((a.numberOf[w] - 1) % 6) * 2
	w.SetBounds(tui.Rect{X: dx + off, Y: dy + off, W: ww, H: wh})
}

// updateTitle formats and applies w's title: "[n] name" with a leading "*" on
// the name when the buffer is modified.
func (a *App) updateTitle(w *tui.Window) {
	ed := a.editorOf[w]
	name := ed.Buffer().DisplayName()
	if ed.Buffer().Modified {
		name = "*" + name
	}
	w.SetTitle(fmt.Sprintf("[%d] %s", a.numberOf[w], name))
}

// activate raises w to the top of the z-order, makes it active, refreshes the
// status bar and focuses its editor.
func (a *App) activate(w *tui.Window) {
	if w == nil {
		return
	}
	a.desktop.Activate(w)
	a.updateTitle(w)
	ed := a.editorOf[w]
	a.statusbar.SetModified(ed.Buffer().Modified)
	if !a.menubar.IsActive() && !a.moveSize {
		a.statusbar.SetContext(tui.CtxEditing)
	}
	a.app.Focus(ed)
	a.app.Redraw()
}

// focusActiveEditor focuses the active window's editor (used after a menu or
// dialog closes).
func (a *App) focusActiveEditor() {
	if ed := a.activeEditor(); ed != nil {
		a.app.Focus(ed)
	}
}

// activeWindow returns the desktop's active window, or nil.
func (a *App) activeWindow() *tui.Window { return a.desktop.Active() }

// activeEditor returns the active window's editor, or nil.
func (a *App) activeEditor() *edit.Editor {
	w := a.activeWindow()
	if w == nil {
		return nil
	}
	return a.editorOf[w]
}

// closeWindow removes w from the desktop and bookkeeping, then activates a
// remaining window if any. Callers needing a dirty-prompt route through
// cmdCloseActive / closeWindowPrompt instead.
func (a *App) closeWindow(w *tui.Window) {
	if w == nil {
		return
	}
	a.desktop.RemoveWindow(w)
	for i, x := range a.windows {
		if x == w {
			a.windows = append(a.windows[:i], a.windows[i+1:]...)
			break
		}
	}
	delete(a.editorOf, w)
	delete(a.numberOf, w)
	if cur := a.desktop.Active(); cur != nil {
		a.activate(cur)
	}
	a.app.Redraw()
}

// cycleWindow activates the next (dir>0) or previous (dir<0) window.
func (a *App) cycleWindow(dir int) {
	if len(a.windows) == 0 {
		return
	}
	if dir < 0 {
		a.desktop.Prev()
	} else {
		a.desktop.Next()
	}
	if cur := a.desktop.Active(); cur != nil {
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
	a.desktop.Cascade()
	if cur := a.desktop.Active(); cur != nil {
		a.activate(cur)
	}
}

// tileWindows lays the windows out in a non-overlapping grid (F5).
func (a *App) tileWindows() {
	a.desktop.Tile()
	if cur := a.desktop.Active(); cur != nil {
		a.activate(cur)
	}
}
