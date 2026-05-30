package app

import (
	"strings"
	"testing"

	"dosedit/internal/theme"
	"dosedit/internal/tui"

	"github.com/gdamore/tcell/v2"
)

// newTestApp builds a fully wired App over an 80x25 simulation screen, mirroring
// main.run's assembly. Tests drive the app SYNCHRONOUSLY (HandleEvent + Sync on
// the test goroutine) rather than running App.Run in a goroutine — so screen and
// state reads never race the loop. (tcell's GetContents returns the live cell
// buffer, so concurrent draw+read is a genuine race; synchronous driving avoids
// it entirely and is deterministic.)
func newTestApp(t *testing.T) (*tui.App, tcell.SimulationScreen) {
	t.Helper()
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	sim.SetSize(80, 25)
	sim.SetStyle(theme.Desktop())

	tapp := tui.NewApp(sim)
	a := New(tapp)
	a.OpenInitialWindow()
	tapp.Sync() // render the initial frame
	return tapp, sim
}

// key feeds one key event and re-renders.
func key(tapp *tui.App, k tcell.Key, r rune, mod tcell.ModMask) {
	tapp.HandleEvent(tcell.NewEventKey(k, r, mod))
	tapp.Sync()
}

// clickAt feeds a left press+release at (x,y) and re-renders.
func clickAt(tapp *tui.App, x, y int) {
	tapp.HandleEvent(tcell.NewEventMouse(x, y, tcell.Button1, 0))
	tapp.HandleEvent(tcell.NewEventMouse(x, y, tcell.ButtonNone, 0))
	tapp.Sync()
}

// moveTo feeds a bare cursor move (no buttons held) and re-renders.
func moveTo(tapp *tui.App, x, y int) {
	tapp.HandleEvent(tcell.NewEventMouse(x, y, tcell.ButtonNone, 0))
	tapp.Sync()
}

func screenText(sim tcell.SimulationScreen) string {
	cells, w, h := sim.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				b.WriteRune(c.Runes[0])
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func rowText(sim tcell.SimulationScreen, row int) string {
	cells, w, h := sim.GetContents()
	if row < 0 || row >= h {
		return ""
	}
	var b strings.Builder
	for x := 0; x < w; x++ {
		c := cells[row*w+x]
		if len(c.Runes) > 0 {
			b.WriteRune(c.Runes[0])
		} else {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func has(sim tcell.SimulationScreen, needle string) bool {
	return strings.Contains(screenText(sim), needle)
}

// TestBootsLayout proves the app boots to the three-region layout with one
// Untitled window.
func TestBootsLayout(t *testing.T) {
	_, scr := newTestApp(t)

	menu := rowText(scr, 0)
	for _, title := range []string{"File", "Edit", "Search", "Window", "Help"} {
		if !strings.Contains(menu, title) {
			t.Errorf("menu bar row missing %q; got %q", title, menu)
		}
	}
	if status := rowText(scr, 24); !strings.Contains(status, "F1=Help") {
		t.Errorf("status bar row missing hints; got %q", status)
	}
	if !has(scr, "Untitled1") {
		t.Errorf("initial window title not rendered; screen:\n%s", screenText(scr))
	}
}

// TestModalBlocksBackgroundMouse proves true modality: while a dialog is open, a
// background click (outside the centred dialog) is swallowed and the dialog
// stays open.
func TestModalBlocksBackgroundMouse(t *testing.T) {
	tapp, _ := newTestApp(t)

	key(tapp, tcell.KeyCtrlG, 0, tcell.ModCtrl) // open Go to Line dialog
	if tapp.ModalDepth() != 1 {
		t.Fatalf("expected a modal open after Ctrl+G, got depth %d", tapp.ModalDepth())
	}
	clickAt(tapp, 2, 2) // top-left of the screen, outside the centred dialog
	if tapp.ModalDepth() != 1 {
		t.Fatalf("background click changed modal depth to %d; dialog should stay modal", tapp.ModalDepth())
	}
}

// TestEscClosesDialog proves Esc cancels (pops) an open dialog.
func TestEscClosesDialog(t *testing.T) {
	tapp, _ := newTestApp(t)

	key(tapp, tcell.KeyCtrlG, 0, tcell.ModCtrl)
	if tapp.ModalDepth() != 1 {
		t.Fatalf("dialog did not open")
	}
	key(tapp, tcell.KeyEscape, 0, tcell.ModNone)
	if tapp.ModalDepth() != 0 {
		t.Fatalf("Esc did not close the dialog; depth=%d", tapp.ModalDepth())
	}
}

// TestEnterPressesDefaultButton proves Enter activates the default (OK) button
// from any focused control: typing a number in Go to Line then Enter runs OK,
// which pops the dialog.
func TestEnterPressesDefaultButton(t *testing.T) {
	tapp, _ := newTestApp(t)

	key(tapp, tcell.KeyCtrlG, 0, tcell.ModCtrl)
	if tapp.ModalDepth() != 1 {
		t.Fatalf("dialog did not open")
	}
	key(tapp, tcell.KeyRune, '1', tcell.ModNone) // type into the (focused) field
	key(tapp, tcell.KeyEnter, '\r', tcell.ModNone)
	if tapp.ModalDepth() != 0 {
		t.Fatalf("Enter (default OK) did not close the dialog; depth=%d", tapp.ModalDepth())
	}
}

// TestMenuStaysOpenOnMouseMove guards the regression where a bare cursor move
// (tcell reports motion with no buttons) was translated to a click-release and
// dismissed an open menu. Moving the mouse without clicking must keep it open.
func TestMenuStaysOpenOnMouseMove(t *testing.T) {
	tapp, _ := newTestApp(t)

	key(tapp, tcell.KeyRune, 'f', tcell.ModAlt) // open the File menu
	if tapp.ModalDepth() != 1 {
		t.Fatalf("File menu did not open on Alt+F; depth=%d", tapp.ModalDepth())
	}
	moveTo(tapp, 1, 6)   // hover off the dropdown
	moveTo(tapp, 40, 12) // hover elsewhere
	if tapp.ModalDepth() != 1 {
		t.Fatalf("menu closed on a no-click mouse move; depth=%d", tapp.ModalDepth())
	}
}

// TestGlobalKeyDoesNotFireOverDialog proves a global accelerator does NOT fire
// while a dialog is open: with Go to Line up, F3 (Open) must not push another
// dialog.
func TestGlobalKeyDoesNotFireOverDialog(t *testing.T) {
	tapp, _ := newTestApp(t)

	key(tapp, tcell.KeyCtrlG, 0, tcell.ModCtrl)
	if tapp.ModalDepth() != 1 {
		t.Fatalf("dialog did not open")
	}
	key(tapp, tcell.KeyF3, 0, tcell.ModNone) // F3=Open must be ignored while modal
	if tapp.ModalDepth() != 1 {
		t.Fatalf("a global accelerator fired over a dialog; depth=%d", tapp.ModalDepth())
	}
}
