// keys.go implements DOSEdit's WINDOWS-scope global accelerators (spec §8). The
// accelerator table is installed on the UIManager as its globalKey hook (see
// NewUIManager / SetGlobalKey). The UIManager only calls it in WINDOWS scope, so
// accelerators never fire while a dialog or menu owns input — that gating lives
// in the router, not here. routeGlobalKey returns true when it consumes the
// event; otherwise the UIManager forwards the key to the active editor so
// ordinary typing and the editor's own chords (Ctrl+C/V/X/Z/Y, arrows,
// Home/End, …) still work.
//
// F3 vs Find Next conflict (spec §7): F3 = Open, Find Next = Ctrl+L.
package app

import (
	"dosedit/internal/ui"

	"github.com/gdamore/tcell/v2"
)

// routeGlobalKey is the WINDOWS-scope accelerator handler. It returns true if it
// consumed ev.
func (a *App) routeGlobalKey(ev *tcell.EventKey) bool {
	// Correct any windows still sized against the reference desktop now that the
	// real layout geometry is available.
	for _, w := range a.windows {
		a.placeWindow(w)
	}

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
			a.ui.OpenMenu()
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
		// Ctrl+H also arrives as Backspace on some terminals; we give the Replace
		// command priority per the spec accelerator table.
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
				a.ui.SyncMenuActive()
				a.statusbar.SetContext(ui.CtxMenu)
				return true
			}
		}
		// Plain Alt (no rune) opens the menu bar.
		if ev.Rune() == 0 {
			a.ui.OpenMenu()
			return true
		}
	}

	return false
}

// handleMoveSize implements keyboard window move/resize while moveSize is on.
// Returns true if the event was consumed. It drives the window manager's
// MoveActive / ResizeActive helpers (Ctrl+F5 mode).
func (a *App) handleMoveSize(ev *tcell.EventKey) bool {
	if a.activeWindow() == nil {
		a.moveSize = false
		return false
	}
	shift := ev.Modifiers()&tcell.ModShift != 0
	switch ev.Key() {
	case tcell.KeyLeft:
		if shift {
			a.wm.ResizeActive(-1, 0)
		} else {
			a.wm.MoveActive(-1, 0)
		}
	case tcell.KeyRight:
		if shift {
			a.wm.ResizeActive(1, 0)
		} else {
			a.wm.MoveActive(1, 0)
		}
	case tcell.KeyUp:
		if shift {
			a.wm.ResizeActive(0, -1)
		} else {
			a.wm.MoveActive(0, -1)
		}
	case tcell.KeyDown:
		if shift {
			a.wm.ResizeActive(0, 1)
		} else {
			a.wm.MoveActive(0, 1)
		}
	case tcell.KeyEnter, tcell.KeyEscape:
		a.moveSize = false
		a.statusbar.SetContext(ui.CtxEditing)
		return true
	default:
		return false
	}
	return true
}
