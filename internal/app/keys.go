// keys.go implements DOSEdit's global accelerators (spec §8) as the tui.App's
// key hook. The App consults the hook ONLY when no modal (dialog or menu
// dropdown) is open and BEFORE the focused widget, so accelerators never fire
// over a dialog/menu. routeGlobalKey returns true when it consumes the event;
// otherwise the App forwards the key to the focused editor so ordinary typing
// and the editor's own chords (Ctrl+C/V/X/Z/Y, arrows, Home/End, …) still work.
//
// F3 vs Find Next conflict (spec §7): F3 = Open, Find Next = Ctrl+L.
//
// Tab handling: the App's Tab/Backtab normally does focus traversal. In the
// editing context Tab must INDENT the editor instead, so the hook forwards
// Tab/Backtab to the active editor and consumes them (returns true) so the App's
// traversal does not run. (Inside dialogs the hook is not consulted, so the
// App's modal Tab-trap handles Tab normally.)
package app

import (
	"dosedit/internal/tui"

	"github.com/gdamore/tcell/v2"
)

// routeGlobalKey is the global accelerator hook installed via tui.App.SetKeyHook.
// It returns true if it consumed ev.
func (a *App) routeGlobalKey(ev *tcell.EventKey) bool {
	key := ev.Key()
	mod := ev.Modifiers()
	ctrl := mod&tcell.ModCtrl != 0
	shift := mod&tcell.ModShift != 0
	alt := mod&tcell.ModAlt != 0

	// Keyboard move/size mode (Ctrl+F5): arrows move, Shift+arrows resize,
	// Enter/Esc exit. Consume movement keys so they do not reach the editor.
	if a.moveSize {
		if a.handleMoveSize(ev) {
			return true
		}
	}

	// Tab/Backtab in the editing context must indent the editor, not traverse
	// focus. Forward to the active editor and consume.
	if key == tcell.KeyTab || key == tcell.KeyBacktab {
		if ed := a.activeEditor(); ed != nil {
			ed.HandleKey(ev)
			if w := a.activeWindow(); w != nil {
				a.updateTitle(w)
			}
			a.app.Redraw()
			return true
		}
		return false
	}

	switch key {
	case tcell.KeyF1:
		a.cmdKeys()
		return true
	case tcell.KeyF2:
		a.cmdSave()
		return true
	case tcell.KeyF3:
		a.cmdOpen()
		return true
	case tcell.KeyF5:
		if ctrl {
			a.cmdMoveSize() // Ctrl+F5 = keyboard Move/Size
		} else {
			a.tileWindows()
		}
		return true
	case tcell.KeyF6:
		if shift {
			a.cycleWindow(-1)
		} else {
			a.cycleWindow(1)
		}
		return true
	case tcell.KeyF10:
		if ctrl {
			a.cmdToggleMax()
		} else {
			a.menubar.Activate()
		}
		return true
	case tcell.KeyF4:
		if ctrl {
			a.cmdCloseActive()
			return true
		}
	case tcell.KeyCtrlF:
		a.cmdFind()
		return true
	case tcell.KeyCtrlL:
		a.cmdFindNext()
		return true
	case tcell.KeyCtrlH:
		// Ctrl+H also arrives as Backspace on some terminals; the spec accelerator
		// table gives Replace priority.
		a.cmdReplace()
		return true
	case tcell.KeyCtrlG:
		a.cmdGotoLine()
		return true
	}

	// Alt-based accelerators.
	if alt {
		// Alt+X = Exit.
		if r := ev.Rune(); r == 'x' || r == 'X' {
			a.cmdExit()
			return true
		}
		// Alt+1..9 = activate window N.
		if r := ev.Rune(); r >= '1' && r <= '9' {
			a.activateByNumber(int(r - '0'))
			return true
		}
		// Alt+F/E/S/W/H = open the matching menu.
		if r := ev.Rune(); r != 0 {
			if a.menubar.OpenByMnemonic(r) {
				return true
			}
		}
		// Plain Alt (no rune) opens the menu bar.
		if ev.Rune() == 0 {
			a.menubar.Activate()
			return true
		}
	}

	return false
}

// handleMoveSize implements keyboard window move/resize while moveSize is on.
// Returns true if the event was consumed. It drives the desktop's MoveActive /
// ResizeActive helpers (Ctrl+F5 mode).
func (a *App) handleMoveSize(ev *tcell.EventKey) bool {
	if a.activeWindow() == nil {
		a.moveSize = false
		return false
	}
	shift := ev.Modifiers()&tcell.ModShift != 0
	switch ev.Key() {
	case tcell.KeyLeft:
		if shift {
			a.desktop.ResizeActive(-1, 0)
		} else {
			a.desktop.MoveActive(-1, 0)
		}
	case tcell.KeyRight:
		if shift {
			a.desktop.ResizeActive(1, 0)
		} else {
			a.desktop.MoveActive(1, 0)
		}
	case tcell.KeyUp:
		if shift {
			a.desktop.ResizeActive(0, -1)
		} else {
			a.desktop.MoveActive(0, -1)
		}
	case tcell.KeyDown:
		if shift {
			a.desktop.ResizeActive(0, 1)
		} else {
			a.desktop.MoveActive(0, 1)
		}
	case tcell.KeyEnter, tcell.KeyEscape:
		a.moveSize = false
		a.statusbar.SetContext(tui.CtxEditing)
		a.app.Redraw()
		return true
	default:
		return false
	}
	a.app.Redraw()
	return true
}
