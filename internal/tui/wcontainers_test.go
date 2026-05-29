package tui

import (
	"testing"

	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// --- helpers ----------------------------------------------------------------

// wcMouse builds a MouseEvent at (x, y) with the given action and the primary
// button mask.
func wcMouse(x, y int, action MouseAction) MouseEvent {
	return MouseEvent{X: x, Y: y, Action: action, Button: tcell.Button1}
}

// wcContent is a minimal content widget that records the mouse events it gets.
type wcContent struct {
	BaseWidget
	mouseCount int
	last       MouseEvent
}

func (c *wcContent) HandleMouse(ev MouseEvent) bool {
	c.mouseCount++
	c.last = ev
	return true
}

// wcButton is a focusable button stub that records Enter and click activations.
type wcButton struct {
	BaseWidget
	activations int
}

func (b *wcButton) Focusable() bool { return true }

func (b *wcButton) HandleKey(ev *tcell.EventKey) bool {
	if ev.Key() == tcell.KeyEnter {
		b.activations++
		return true
	}
	return false
}

func (b *wcButton) HandleMouse(ev MouseEvent) bool {
	if ev.Action == MouseDown {
		b.activations++
		return true
	}
	return false
}

// wcMakeWindow returns a window with the given bounds.
func wcMakeWindow(title string, r Rect) *Window {
	w := NewWindow(&wcContent{}, title)
	w.SetBounds(r)
	return w
}

// --- Desktop ----------------------------------------------------------------

func TestDesktopActivateZOrder(t *testing.T) {
	d := NewDesktop()
	d.SetBounds(Rect{X: 0, Y: 0, W: 80, H: 24})
	w1 := wcMakeWindow("w1", Rect{X: 0, Y: 0, W: 20, H: 8})
	w2 := wcMakeWindow("w2", Rect{X: 5, Y: 2, W: 20, H: 8})
	d.AddWindow(w1)
	d.AddWindow(w2)

	// Last added is active and topmost.
	if d.Active() != w2 {
		t.Fatalf("expected w2 active, got %v", d.Active())
	}
	if !w2.IsActive() || w1.IsActive() {
		t.Fatalf("active flags wrong: w1=%v w2=%v", w1.IsActive(), w2.IsActive())
	}

	// Activating w1 raises it to topmost (last in z-order) and flips flags.
	d.Activate(w1)
	wins := d.Windows()
	if wins[len(wins)-1] != w1 {
		t.Fatalf("expected w1 topmost after Activate")
	}
	if !w1.IsActive() || w2.IsActive() {
		t.Fatalf("flags after Activate wrong: w1=%v w2=%v", w1.IsActive(), w2.IsActive())
	}
	// Children z-order must match window z-order.
	ch := d.Children()
	if ch[len(ch)-1] != w1 {
		t.Fatalf("children z-order does not match windows z-order")
	}
}

func TestDesktopNextPrev(t *testing.T) {
	d := NewDesktop()
	d.SetBounds(Rect{X: 0, Y: 0, W: 80, H: 24})
	w1 := wcMakeWindow("w1", Rect{X: 0, Y: 0, W: 20, H: 8})
	w2 := wcMakeWindow("w2", Rect{X: 5, Y: 2, W: 20, H: 8})
	w3 := wcMakeWindow("w3", Rect{X: 10, Y: 4, W: 20, H: 8})
	d.AddWindow(w1)
	d.AddWindow(w2)
	d.AddWindow(w3) // active

	d.Next()
	if d.Active() == w3 {
		t.Fatalf("Next should move off w3")
	}
	first := d.Active()
	d.Prev()
	if d.Active() != w3 {
		t.Fatalf("Prev should return to w3, got %v", d.Active().Title())
	}
	_ = first
}

func TestDesktopTileNonOverlapping(t *testing.T) {
	d := NewDesktop()
	db := Rect{X: 0, Y: 1, W: 80, H: 22}
	d.SetBounds(db)
	for i := 0; i < 4; i++ {
		d.AddWindow(wcMakeWindow("w", Rect{X: 0, Y: 0, W: 10, H: 5}))
	}
	d.Tile()
	wins := d.Windows()
	for _, w := range wins {
		b := w.Bounds()
		if b.X < db.X || b.Y < db.Y || b.X+b.W > db.X+db.W || b.Y+b.H > db.Y+db.H {
			t.Fatalf("tiled window %v outside desktop %v", b, db)
		}
	}
	// No two windows overlap.
	for i := 0; i < len(wins); i++ {
		for j := i + 1; j < len(wins); j++ {
			if wcOverlap(wins[i].Bounds(), wins[j].Bounds()) {
				t.Fatalf("tiled windows overlap: %v %v", wins[i].Bounds(), wins[j].Bounds())
			}
		}
	}
}

func wcOverlap(a, b Rect) bool {
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}

func TestDesktopCascadeOffsets(t *testing.T) {
	d := NewDesktop()
	d.SetBounds(Rect{X: 0, Y: 1, W: 80, H: 22})
	w1 := wcMakeWindow("w1", Rect{X: 0, Y: 0, W: 10, H: 5})
	w2 := wcMakeWindow("w2", Rect{X: 0, Y: 0, W: 10, H: 5})
	d.AddWindow(w1)
	d.AddWindow(w2)
	d.Cascade()
	if w1.Bounds().X == w2.Bounds().X && w1.Bounds().Y == w2.Bounds().Y {
		t.Fatalf("cascade did not offset windows: %v %v", w1.Bounds(), w2.Bounds())
	}
}

// --- Window drag / clamp ----------------------------------------------------

func TestWindowDragMovesAndClamps(t *testing.T) {
	d := NewDesktop()
	db := Rect{X: 0, Y: 1, W: 80, H: 22}
	d.SetBounds(db)
	w := wcMakeWindow("w", Rect{X: 10, Y: 5, W: 20, H: 8})
	d.AddWindow(w)

	// Press on the title bar (top row), away from buttons.
	tb := w.Bounds()
	d.HandleMouse(wcMouse(tb.X+5, tb.Y, MouseDown))
	// Drag down-right by (4, 3).
	d.HandleMouse(wcMouse(tb.X+9, tb.Y+3, MouseDrag))
	d.HandleMouse(wcMouse(tb.X+9, tb.Y+3, MouseUp))
	got := w.Bounds()
	if got.X != 14 || got.Y != 8 {
		t.Fatalf("expected window moved to (14,8), got (%d,%d)", got.X, got.Y)
	}

	// Drag far past the desktop edge → must clamp inside.
	d.HandleMouse(wcMouse(got.X+5, got.Y, MouseDown))
	d.HandleMouse(wcMouse(1000, 1000, MouseDrag))
	d.HandleMouse(wcMouse(1000, 1000, MouseUp))
	got = w.Bounds()
	if got.X+got.W > db.X+db.W || got.Y+got.H > db.Y+db.H {
		t.Fatalf("window not clamped: %v in %v", got, db)
	}
	if got.X < db.X || got.Y < db.Y {
		t.Fatalf("window clamped past top-left: %v in %v", got, db)
	}
}

// --- Window buttons ---------------------------------------------------------

func TestWindowCloseAndMaxButtons(t *testing.T) {
	w := wcMakeWindow("w", Rect{X: 0, Y: 0, W: 20, H: 8})
	closed := false
	toggled := false
	w.SetOnClose(func() { closed = true })
	w.SetOnToggleMax(func() { toggled = true })

	b := w.Bounds()
	// Close button is at left+1, top.
	if !w.HandleMouse(wcMouse(b.X+1, b.Y, MouseDown)) {
		t.Fatalf("close button click not consumed")
	}
	if !closed {
		t.Fatalf("onClose not fired")
	}
	// Max button is at right-1, top.
	if !w.HandleMouse(wcMouse(b.X+b.W-2, b.Y, MouseDown)) {
		t.Fatalf("max button click not consumed")
	}
	if !toggled {
		t.Fatalf("onToggleMax not fired")
	}
}

func TestWindowToggleMaximize(t *testing.T) {
	w := wcMakeWindow("w", Rect{X: 3, Y: 3, W: 20, H: 8})
	w.ToggleMaximize()
	if !w.IsMaximized() {
		t.Fatalf("expected maximized")
	}
	if w.RestoreRect() != (Rect{X: 3, Y: 3, W: 20, H: 8}) {
		t.Fatalf("restore rect not captured: %v", w.RestoreRect())
	}
	w.ToggleMaximize()
	if w.IsMaximized() {
		t.Fatalf("expected restored")
	}
}

// --- Dialog -----------------------------------------------------------------

func TestDialogEnterRoutesToDefault(t *testing.T) {
	scr := newTestScreen(t)
	app := NewApp(scr)
	dlg := NewDialog("Confirm")
	ok := &wcButton{}
	cancel := &wcButton{}
	dlg.AddButton(ok)
	dlg.AddButton(cancel)
	dlg.SetDefault(ok)
	app.PushModal(dlg)

	// Move focus off the buttons to a non-consuming state by focusing cancel,
	// then send Enter at dialog level: default (ok) should fire.
	if !dlg.HandleKey(tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone)) {
		t.Fatalf("dialog Enter not consumed")
	}
	if ok.activations != 1 {
		t.Fatalf("expected default button activated once, got %d", ok.activations)
	}
	if cancel.activations != 0 {
		t.Fatalf("cancel should not activate, got %d", cancel.activations)
	}
}

func TestDialogEscCallsCancel(t *testing.T) {
	dlg := NewDialog("Confirm")
	cancelled := false
	dlg.SetCancel(func() { cancelled = true })
	if !dlg.HandleKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)) {
		t.Fatalf("Esc not consumed")
	}
	if !cancelled {
		t.Fatalf("cancel not called on Esc")
	}
}

func TestDialogFocusableButtonsReachable(t *testing.T) {
	dlg := NewDialog("D")
	a := &wcButton{}
	b := &wcButton{}
	dlg.AddButton(a)
	dlg.AddButton(b)
	dlg.SetBounds(Rect{X: 0, Y: 0, W: 40, H: 20})
	focus := dlg.FocusableDescendants()
	if len(focus) != 2 {
		t.Fatalf("expected 2 focusable descendants, got %d", len(focus))
	}
}

// --- Frame ------------------------------------------------------------------

func TestFrameDrawsCaption(t *testing.T) {
	scr := newTestScreen(t)
	f := NewFrame("Opts")
	f.SetBounds(Rect{X: 0, Y: 0, W: 20, H: 6})
	f.Draw(NewScreenSurface(scr, Rect{X: 0, Y: 0, W: 40, H: 20}))
	scr.Show()

	// Corner glyph.
	if got := cellRune(t, scr, 0, 0); got != theme.TLSingle {
		t.Fatalf("expected TLSingle at corner, got %q", got)
	}
	// Caption " Opts " begins at x=2: leading space then 'O' at x=3.
	if got := cellRune(t, scr, 3, 0); got != 'O' {
		t.Fatalf("expected caption 'O' at (3,0), got %q", got)
	}
}

// --- Panel ------------------------------------------------------------------

func TestPanelVStackLayout(t *testing.T) {
	p := NewVStack(2, 1)
	c1 := &wcContent{}
	c2 := &wcContent{}
	p.Add(c1)
	p.Add(c2)
	p.SetBounds(Rect{X: 4, Y: 4, W: 10, H: 10})
	if c1.Bounds() != (Rect{X: 4, Y: 4, W: 10, H: 2}) {
		t.Fatalf("c1 bounds wrong: %v", c1.Bounds())
	}
	if c2.Bounds() != (Rect{X: 4, Y: 7, W: 10, H: 2}) {
		t.Fatalf("c2 bounds wrong: %v", c2.Bounds())
	}
}
