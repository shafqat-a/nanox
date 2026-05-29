// keys.go implements DOSEdit's global key router (spec §8). RouteGlobalKeys is
// installed via tview.Application.SetInputCapture. It handles function keys,
// Alt-letter menu opens and the documented accelerators, returning nil for keys
// it consumes and the original event otherwise so the focused editor still
// receives ordinary typing and its own editing chords (Ctrl+C/V/X/Z/Y, arrows,
// Home/End, …).
//
// F3 vs Find Next conflict (spec §7): F3 = Open, Find Next = Ctrl+L.
package app

import (
	"dosedit/internal/ui"

	"github.com/gdamore/tcell/v2"
)

// RouteGlobalKeys is the application-level input capture. It runs before the
// focused primitive sees the event.
func (a *App) RouteGlobalKeys(ev *tcell.EventKey) *tcell.EventKey {
	// Correct any windows still sized against the reference desktop now that the
	// real layout geometry is available.
	for _, w := range a.windows {
		a.placeWindow(w)
	}

	// While a modal dialog is open, let it own all keys.
	if a.modalOpen {
		return ev
	}

	// While the menu bar is active, let it own all keys (its InputHandler does
	// navigation/mnemonics/Esc). We still intercept nothing here.
	if a.menubar.IsActive() {
		return ev
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
			return nil
		}
	}

	switch key {
	case tcell.KeyF1:
		a.cmdKeys()
		return nil
	case tcell.KeyF2:
		a.cmdSave()
		return nil
	case tcell.KeyF3:
		a.cmdOpen()
		return nil
	case tcell.KeyF5:
		if ctrl {
			a.cmdMoveSize() // Ctrl+F5 = keyboard Move/Size
		} else {
			a.tileWindows()
		}
		return nil
	case tcell.KeyF6:
		if shift {
			a.cycleWindow(-1)
		} else {
			a.cycleWindow(1)
		}
		return nil
	case tcell.KeyF10:
		if ctrl {
			a.cmdToggleMax()
		} else {
			a.openMenu()
		}
		return nil
	case tcell.KeyF4:
		if ctrl {
			a.cmdCloseActive()
			return nil
		}
	case tcell.KeyCtrlF:
		a.cmdFind()
		return nil
	case tcell.KeyCtrlL:
		a.cmdFindNext()
		return nil
	case tcell.KeyCtrlH:
		// Ctrl+H also arrives as Backspace on some terminals; only treat it as
		// Replace when no editor would interpret it as an edit. We give the
		// Replace command priority per the spec accelerator table.
		a.cmdReplace()
		return nil
	case tcell.KeyCtrlG:
		a.cmdGotoLine()
		return nil
	}

	// Alt-based accelerators.
	if alt {
		// Alt+X = Exit.
		if r := ev.Rune(); r == 'x' || r == 'X' {
			a.cmdExit()
			return nil
		}
		// Alt+1..9 = activate window N.
		if r := ev.Rune(); r >= '1' && r <= '9' {
			a.activateByNumber(int(r - '0'))
			return nil
		}
		// Alt+F/E/S/W/H = open the matching menu.
		if r := ev.Rune(); r != 0 {
			if a.menubar.OpenByMnemonic(r) {
				a.statusbar.SetContext(ui.CtxMenu)
				a.tapp.SetFocus(a.menubar)
				return nil
			}
		}
	}

	return ev
}

// openMenu activates the menu bar (F10 / plain Alt) and focuses it.
func (a *App) openMenu() {
	a.menubar.Activate()
	a.statusbar.SetContext(ui.CtxMenu)
	a.tapp.SetFocus(a.menubar)
}

// handleMoveSize implements keyboard window move/resize while moveSize is on.
// Returns true if the event was consumed.
func (a *App) handleMoveSize(ev *tcell.EventKey) bool {
	w := a.active
	if w == nil {
		a.moveSize = false
		return false
	}
	shift := ev.Modifiers()&tcell.ModShift != 0
	x, y, ww, wh := w.GetRect()
	switch ev.Key() {
	case tcell.KeyLeft:
		if shift {
			ww--
		} else {
			x--
		}
	case tcell.KeyRight:
		if shift {
			ww++
		} else {
			x++
		}
	case tcell.KeyUp:
		if shift {
			wh--
		} else {
			y--
		}
	case tcell.KeyDown:
		if shift {
			wh++
		} else {
			y++
		}
	case tcell.KeyEnter, tcell.KeyEscape:
		a.moveSize = false
		a.statusbar.SetContext(ui.CtxEditing)
		return true
	default:
		return false
	}
	if ww < minWinW {
		ww = minWinW
	}
	if wh < minWinH {
		wh = minWinH
	}
	w.SetRect(x, y, ww, wh)
	return true
}

const (
	minWinW = 10
	minWinH = 3
)
