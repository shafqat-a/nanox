// ui_manager.go defines UIManager, DOSEdit's single focus authority. It is the
// sole tview-focused primitive (installed via SetRoot + SetFocus): every
// keyboard and mouse event tview delivers to the focused leaf arrives here, and
// UIManager routes it according to one derived "scope":
//
//	len(dialogStack) > 0 -> DIALOG  (top dialog owns all input; true modality)
//	menuActive           -> MENU    (the menu bar owns all input)
//	otherwise            -> WINDOWS (global accelerators, then the active editor)
//
// This replaces the old scattered focus logic (app-level SetInputCapture /
// SetMouseCapture, ad-hoc tapp.SetFocus calls gated by modalOpen /
// menubar.IsActive() / moveSize booleans, and the full-screen modalLayer). No
// child control (editor, dialog field, menu) is ever tview-focused directly;
// they are driven through the forwarded InputHandlers below, and each already
// manages its own internal focus (the Dialog traps Tab/Enter/Esc itself; the
// editor draws its own block cursor regardless of tview focus).
package app

import (
	"dosedit/internal/ui"
	"dosedit/internal/ui/wm"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// UIManager is the root primitive and sole focus holder. It owns the window
// manager, the menu bar, the status bar and a stack of open modal dialogs, and
// derives the active input scope from that state.
type UIManager struct {
	*tview.Box

	wm        *wm.Manager
	menubar   *ui.MenuBar
	statusbar *ui.StatusBar

	dialogStack []*ui.Dialog
	menuActive  bool

	// globalKey is the window-scope accelerator hook installed by the App. It is
	// consulted ONLY in WINDOWS scope; returning true consumes the event. While a
	// dialog or menu is active it is never called, so accelerators cannot fire
	// over a dialog/menu.
	globalKey func(*tcell.EventKey) bool

	// menuRow / statusRow record the laid-out screen rows for the bars so the
	// mouse router can classify clicks without re-deriving geometry.
	menuRow   int
	statusRow int
}

// NewUIManager builds the root UIManager over the supplied collaborators. The
// App wires the globalKey hook and keeps menuActive in sync via the menu bar's
// SetOnActivate/SetOnClose callbacks.
func NewUIManager(manager *wm.Manager, menubar *ui.MenuBar, statusbar *ui.StatusBar) *UIManager {
	u := &UIManager{
		Box:       tview.NewBox(),
		wm:        manager,
		menubar:   menubar,
		statusbar: statusbar,
	}
	return u
}

// SetGlobalKey installs the WINDOWS-scope accelerator hook.
func (u *UIManager) SetGlobalKey(fn func(*tcell.EventKey) bool) { u.globalKey = fn }

// scope is the single derivation of the current input scope.
type scope int

const (
	scopeWindows scope = iota
	scopeMenu
	scopeDialog
)

// currentScope derives the active scope from the dialog stack and menu state.
func (u *UIManager) currentScope() scope {
	if len(u.dialogStack) > 0 {
		return scopeDialog
	}
	if u.menuActive {
		return scopeMenu
	}
	return scopeWindows
}

// topDialog returns the dialog currently owning input, or nil.
func (u *UIManager) topDialog() *ui.Dialog {
	if n := len(u.dialogStack); n > 0 {
		return u.dialogStack[n-1]
	}
	return nil
}

// DialogOpen reports whether any modal dialog is on the stack.
func (u *UIManager) DialogOpen() bool { return len(u.dialogStack) > 0 }

// MenuActive reports whether the menu bar currently owns input.
func (u *UIManager) MenuActive() bool { return u.menuActive }

// --- scope transitions -----------------------------------------------------

// PushDialog adds d to the top of the dialog stack, switches the status bar to
// the dialog context and lays the new dialog out centred.
func (u *UIManager) PushDialog(d *ui.Dialog) {
	if d == nil {
		return
	}
	u.dialogStack = append(u.dialogStack, d)
	u.statusbar.SetContext(ui.CtxDialog)
	u.centreDialog(d)
}

// PopDialog removes the top dialog and restores the scope below it (another
// dialog, the menu, or the windows). The status bar context is updated to match.
func (u *UIManager) PopDialog() {
	if n := len(u.dialogStack); n > 0 {
		u.dialogStack = u.dialogStack[:n-1]
	}
	u.restoreContext()
}

// OpenMenu activates the menu bar (menuActive is kept in sync by the bar's
// SetOnActivate callback, but we set it here too so the scope flips immediately
// even before the callback chain runs).
func (u *UIManager) OpenMenu() {
	u.menuActive = true
	u.menubar.Activate()
}

// SyncMenuActive mirrors the menu bar's active flag onto the UIManager scope.
// The App installs SetOnActivate/SetOnClose callbacks that call this so the
// scope always matches the bar.
func (u *UIManager) SyncMenuActive() {
	u.menuActive = u.menubar.IsActive()
	if u.currentScope() == scopeWindows {
		u.restoreContext()
	}
}

// restoreContext sets the status-bar context appropriate to the current scope.
func (u *UIManager) restoreContext() {
	switch u.currentScope() {
	case scopeDialog:
		u.statusbar.SetContext(ui.CtxDialog)
	case scopeMenu:
		u.statusbar.SetContext(ui.CtxMenu)
	default:
		u.statusbar.SetContext(ui.CtxEditing)
	}
}

// --- layout ----------------------------------------------------------------

// SetRect lays out the three regions: the menu bar on the top row, the status
// bar on the bottom row, and the window manager in between. Each open dialog is
// re-centred against its Size().
func (u *UIManager) SetRect(x, y, width, height int) {
	u.Box.SetRect(x, y, width, height)
	if width <= 0 || height <= 0 {
		return
	}

	u.menuRow = y
	u.statusRow = y + height - 1

	u.menubar.SetRect(x, y, width, 1)
	if height >= 2 {
		u.statusbar.SetRect(x, u.statusRow, width, 1)
	}
	// Desktop occupies the rows between the bars.
	deskY := y + 1
	deskH := height - 2
	if deskH < 0 {
		deskH = 0
	}
	u.wm.SetRect(x, deskY, width, deskH)

	for _, d := range u.dialogStack {
		u.centreDialog(d)
	}
}

// centreDialog positions d centred in the full UIManager rect using its Size().
func (u *UIManager) centreDialog(d *ui.Dialog) {
	x, y, width, height := u.GetRect()
	if width <= 0 || height <= 0 {
		return
	}
	dw, dh := d.Size()
	if dw > width {
		dw = width
	}
	if dh > height {
		dh = height
	}
	dx := x + (width-dw)/2
	dy := y + (height-dh)/2
	d.SetRect(dx, dy, dw, dh)
}

// --- drawing ---------------------------------------------------------------

// Draw paints, bottom-to-top: the desktop + windows, the status bar, the menu
// bar (which draws its own dropdown over the desktop when active), then every
// open dialog (bottom..top), each re-centred so a resize keeps them in place.
func (u *UIManager) Draw(screen tcell.Screen) {
	u.wm.Draw(screen)
	u.statusbar.Draw(screen)
	u.menubar.Draw(screen)
	for _, d := range u.dialogStack {
		u.centreDialog(d)
		d.Draw(screen)
	}
}

// --- focus -----------------------------------------------------------------

// Focus marks the UIManager as the focused primitive. It never delegates to a
// child: UIManager is the sole tview focus and routes input itself.
func (u *UIManager) Focus(delegate func(p tview.Primitive)) {
	u.Box.Focus(delegate)
}

// HasFocus reports the UIManager's own focus flag.
func (u *UIManager) HasFocus() bool { return u.Box.HasFocus() }

// --- key routing -----------------------------------------------------------

// InputHandler is the single key router. It dispatches by current scope:
//
//	DIALOG  -> the top dialog (it traps Tab/Enter/Esc internally).
//	MENU    -> the menu bar (arrows/Enter/Esc/mnemonics; its SetOnClose pops us
//	           out of the menu scope).
//	WINDOWS -> globalKey(ev) first; if consumed, done. Otherwise the active
//	           window's content InputHandler (so the editor types). Dropped when
//	           there is no active window.
func (u *UIManager) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return func(ev *tcell.EventKey, setFocus func(p tview.Primitive)) {
		switch u.currentScope() {
		case scopeDialog:
			if d := u.topDialog(); d != nil {
				if h := d.InputHandler(); h != nil {
					h(ev, func(tview.Primitive) {})
				}
			}
		case scopeMenu:
			if h := u.menubar.InputHandler(); h != nil {
				h(ev, func(tview.Primitive) {})
			}
			// The menu bar may have closed itself (Esc / item action); resync.
			u.menuActive = u.menubar.IsActive()
		default: // WINDOWS
			if u.globalKey != nil && u.globalKey(ev) {
				return
			}
			w := u.wm.Active()
			if w == nil {
				return
			}
			content := w.Content()
			if content == nil {
				return
			}
			if h := content.InputHandler(); h != nil {
				h(ev, func(tview.Primitive) {})
			}
		}
	}
}

// --- mouse routing ---------------------------------------------------------

// MouseHandler is the single mouse entry point. By scope:
//
//	DIALOG  -> forward to the top dialog when the point is inside its rect;
//	           otherwise swallow the event (true modality — the background gets
//	           nothing).
//	else    -> a click on the menu row (or while the menu is active) goes to the
//	           menu bar; if it consumes, sync menuActive and return consumed.
//	           Otherwise the window manager handles it; a content click activates
//	           that window (routing future keyboard to it).
func (u *UIManager) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		x, y := event.Position()

		if u.currentScope() == scopeDialog {
			d := u.topDialog()
			if d == nil {
				return true, nil
			}
			dx, dy, dw, dh := d.GetRect()
			inside := x >= dx && x < dx+dw && y >= dy && y < dy+dh
			if inside {
				if h := d.MouseHandler(); h != nil {
					return h(action, event, func(tview.Primitive) {})
				}
				return true, nil
			}
			// Outside the dialog: swallow so the background never sees it.
			return true, nil
		}

		// Menu bar: clicks on its row, or any click while it is active (so a click
		// elsewhere can close it).
		if y == u.menuRow || u.menuActive {
			if u.menubar.HandleMouse(action, x, y) {
				u.menuActive = u.menubar.IsActive()
				return true, nil
			}
		}

		// Window manager handles the rest (activate/move/resize/close/content).
		consumed, content := u.wm.HandleMouse(action, event)
		if content != nil {
			// A content click already activated the window in the manager; future
			// keyboard now routes to wm.Active() automatically.
			return consumed, nil
		}
		return consumed, nil
	}
}
