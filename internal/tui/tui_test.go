package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// --- stub widgets -----------------------------------------------------------

// focusBox is a minimal focusable leaf widget. It records the last mouse event
// and can optionally consume keys.
type focusBox struct {
	BaseWidget
	consumeKey bool
	gotMouse   bool
	lastMouse  MouseEvent
}

func (b *focusBox) Focusable() bool { return true }

func (b *focusBox) HandleKey(*tcell.EventKey) bool { return b.consumeKey }

func (b *focusBox) HandleMouse(ev MouseEvent) bool {
	b.gotMouse = true
	b.lastMouse = ev
	return true
}

// panel is a non-focusable container used to build trees.
type panel struct {
	BaseContainer
}

func newApp(t *testing.T) *App {
	t.Helper()
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	scr.SetSize(80, 25)
	return NewApp(scr)
}

func keyTab(shift bool) *tcell.EventKey {
	mod := tcell.ModNone
	if shift {
		return tcell.NewEventKey(tcell.KeyBacktab, 0, mod)
	}
	return tcell.NewEventKey(tcell.KeyTab, 0, mod)
}

func keyRune(r rune) *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)
}

// --- geometry / surface ----------------------------------------------------

func TestRect(t *testing.T) {
	r := Rect{X: 2, Y: 3, W: 4, H: 5}
	if !r.Contains(2, 3) || !r.Contains(5, 7) {
		t.Fatal("Contains edge cells failed")
	}
	if r.Contains(6, 3) || r.Contains(2, 8) || r.Contains(1, 3) {
		t.Fatal("Contains should reject outside cells")
	}
	in := r.Inset(1, 1)
	if in != (Rect{X: 3, Y: 4, W: 2, H: 3}) {
		t.Fatalf("Inset = %+v", in)
	}
	if (Rect{W: 0, H: 5}).Empty() != true || (Rect{W: 1, H: 1}).Empty() != false {
		t.Fatal("Empty failed")
	}
}

func TestSurfaceClipping(t *testing.T) {
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	scr.SetSize(20, 10)
	full := NewScreenSurface(scr, Rect{X: 0, Y: 0, W: 20, H: 10})
	sub := full.Clip(Rect{X: 5, Y: 5, W: 3, H: 3})

	if sub.Bounds() != (Rect{X: 5, Y: 5, W: 3, H: 3}) {
		t.Fatalf("sub bounds = %+v", sub.Bounds())
	}
	// Write inside and outside the clip.
	sub.Set(6, 6, 'A', tcell.StyleDefault)
	sub.Set(0, 0, 'B', tcell.StyleDefault) // outside clip -> dropped
	scr.Show()

	if r, _, _, _ := scr.GetContent(6, 6); r != 'A' {
		t.Fatalf("expected A at (6,6), got %q", r)
	}
	if r, _, _, _ := scr.GetContent(0, 0); r == 'B' {
		t.Fatal("write outside clip was not dropped")
	}

	// Clip intersection with parent: a clip larger than parent is trimmed.
	nested := sub.Clip(Rect{X: 0, Y: 0, W: 100, H: 100})
	if nested.Bounds() != (Rect{X: 5, Y: 5, W: 3, H: 3}) {
		t.Fatalf("nested clip not intersected: %+v", nested.Bounds())
	}

	// Text clipping: only the in-clip portion is written.
	sub.Text(4, 6, "xyz", tcell.StyleDefault) // x=4 dropped, 5,6,7 in clip
	scr.Show()
	if r, _, _, _ := scr.GetContent(4, 6); r == 'x' {
		t.Fatal("text before clip not dropped")
	}
	if r, _, _, _ := scr.GetContent(5, 6); r != 'y' {
		t.Fatalf("text at clip start = %q want y", r)
	}
}

// --- focus ring ------------------------------------------------------------

func TestFocusRingTraversalAndWrap(t *testing.T) {
	a := newApp(t)
	root := &panel{}
	b1, b2, b3 := &focusBox{}, &focusBox{}, &focusBox{}
	root.Add(b1)
	root.Add(b2)
	root.Add(b3)
	a.SetRoot(root)

	if a.Focused() != b1 {
		t.Fatal("SetRoot should focus first focusable")
	}
	a.FocusNext()
	if a.Focused() != b2 {
		t.Fatal("FocusNext -> b2")
	}
	a.FocusNext()
	if a.Focused() != b3 {
		t.Fatal("FocusNext -> b3")
	}
	a.FocusNext()
	if a.Focused() != b1 {
		t.Fatal("FocusNext should wrap to b1")
	}
	a.FocusPrev()
	if a.Focused() != b3 {
		t.Fatal("FocusPrev should wrap to b3")
	}
	// Only the focused widget carries the flag.
	if !b3.Focused() || b1.Focused() || b2.Focused() {
		t.Fatal("focused flag not exclusive")
	}
}

func TestTabKeyMovesFocus(t *testing.T) {
	a := newApp(t)
	root := &panel{}
	b1, b2 := &focusBox{}, &focusBox{}
	root.Add(b1)
	root.Add(b2)
	a.SetRoot(root)

	a.dispatchKey(keyTab(false))
	if a.Focused() != b2 {
		t.Fatal("Tab should advance focus to b2")
	}
	a.dispatchKey(keyTab(true))
	if a.Focused() != b1 {
		t.Fatal("Shift+Tab should move focus to b1")
	}
}

// --- key hook --------------------------------------------------------------

func TestKeyHookOnlyWhenNoModal(t *testing.T) {
	a := newApp(t)
	root := &panel{}
	b := &focusBox{}
	root.Add(b)
	a.SetRoot(root)

	hookCalls := 0
	a.SetKeyHook(func(ev *tcell.EventKey) bool {
		hookCalls++
		return true // consume
	})

	// No modal: hook is consulted.
	a.dispatchKey(keyRune('x'))
	if hookCalls != 1 {
		t.Fatalf("hook calls = %d, want 1", hookCalls)
	}

	// Modal open: hook must NOT be consulted.
	modal := &panel{}
	mb := &focusBox{}
	modal.Add(mb)
	a.PushModal(modal)
	a.dispatchKey(keyRune('x'))
	if hookCalls != 1 {
		t.Fatalf("hook called while modal open: calls = %d", hookCalls)
	}
}

// --- modal trap ------------------------------------------------------------

func TestModalTrapAndRestore(t *testing.T) {
	a := newApp(t)
	root := &panel{}
	rb := &focusBox{}
	root.Add(rb)
	a.SetRoot(root)
	if a.Focused() != rb {
		t.Fatal("root focus")
	}

	modal := &panel{}
	m1, m2 := &focusBox{}, &focusBox{}
	modal.Add(m1)
	modal.Add(m2)
	a.PushModal(modal)

	if a.TopLayer() != modal {
		t.Fatal("TopLayer should be modal")
	}
	if a.Focused() != m1 {
		t.Fatal("modal should focus its first child")
	}

	// Focus traversal stays inside the modal subtree.
	a.FocusNext()
	if a.Focused() != m2 {
		t.Fatal("FocusNext within modal -> m2")
	}
	a.FocusNext()
	if a.Focused() != m1 {
		t.Fatal("modal focus should wrap and never escape to root")
	}
	// Attempting to focus a root widget is rejected (not in modal ring).
	a.Focus(rb)
	if a.Focused() == rb {
		t.Fatal("focus escaped modal to root widget")
	}

	a.PopModal()
	if a.TopLayer() != root {
		t.Fatal("PopModal should restore root layer")
	}
	if a.Focused() != rb {
		t.Fatal("PopModal should restore focus to root ring")
	}
}

func TestModalSwallowsOutsideClick(t *testing.T) {
	a := newApp(t)
	root := &panel{}
	rb := &focusBox{}
	rb.SetBounds(Rect{X: 0, Y: 0, W: 5, H: 5})
	root.Add(rb)
	a.SetRoot(root)

	modal := &panel{}
	// Modal occupies a small inner rect (its container bounds), child inside.
	modal.SetBounds(Rect{X: 10, Y: 10, W: 10, H: 5})
	mb := &focusBox{}
	mb.SetBounds(Rect{X: 11, Y: 11, W: 3, H: 1})
	modal.Add(mb)
	a.modals = append(a.modals, modal) // push without full-screen resize for this test
	a.focusFirst()

	// Click outside the modal bounds: swallowed, root child untouched.
	outside := tcell.NewEventMouse(1, 1, tcell.Button1, tcell.ModNone)
	a.dispatchMouse(outside)
	if rb.gotMouse {
		t.Fatal("outside click reached root widget under modal")
	}

	// Click inside the modal child: routed and focuses it.
	inside := tcell.NewEventMouse(11, 11, tcell.Button1, tcell.ModNone)
	a.dispatchMouse(inside)
	if !mb.gotMouse {
		t.Fatal("inside click did not reach modal child")
	}
	if a.Focused() != mb {
		t.Fatal("click on focusable modal child should focus it")
	}
}

// --- mouse hit-test --------------------------------------------------------

func TestMouseHitTestTopmostChild(t *testing.T) {
	a := newApp(t)
	root := &panel{}

	// Two overlapping focusable boxes; b2 added later so it is topmost.
	b1 := &focusBox{}
	b1.SetBounds(Rect{X: 0, Y: 0, W: 10, H: 10})
	b2 := &focusBox{}
	b2.SetBounds(Rect{X: 5, Y: 5, W: 10, H: 10})
	root.Add(b1)
	root.Add(b2)
	a.SetRoot(root)

	// Click in the overlap region (7,7): topmost (b2) wins.
	a.dispatchMouse(tcell.NewEventMouse(7, 7, tcell.Button1, tcell.ModNone))
	if !b2.gotMouse || b1.gotMouse {
		t.Fatal("overlap click should route to topmost child b2")
	}
	if a.Focused() != b2 {
		t.Fatal("click should focus b2")
	}
	// Coordinates are translated/absolute and preserved.
	if b2.lastMouse.X != 7 || b2.lastMouse.Y != 7 {
		t.Fatalf("mouse coords = %d,%d want 7,7", b2.lastMouse.X, b2.lastMouse.Y)
	}
	if b2.lastMouse.Action != MouseDown {
		t.Fatalf("action = %v want MouseDown", b2.lastMouse.Action)
	}

	// Click only in b1's exclusive region (2,2): routes to b1.
	b1.gotMouse, b2.gotMouse = false, false
	a.dispatchMouse(tcell.NewEventMouse(2, 2, tcell.Button1, tcell.ModNone))
	if !b1.gotMouse || b2.gotMouse {
		t.Fatal("click at (2,2) should route to b1 only")
	}
}

// --- nested focusable collection -------------------------------------------

func TestFocusableDescendantsTreeOrder(t *testing.T) {
	outer := &panel{}
	a := &focusBox{}
	inner := &panel{}
	b := &focusBox{}
	c := &focusBox{}
	nonFocus := &BaseWidget{}
	outer.Add(a)
	outer.Add(nonFocus)
	inner.Add(b)
	inner.Add(c)
	outer.Add(inner)

	got := outer.FocusableDescendants()
	want := []Widget{a, b, c}
	if len(got) != len(want) {
		t.Fatalf("len = %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order[%d] mismatch", i)
		}
	}
}

// --- Stop ------------------------------------------------------------------

func TestStop(t *testing.T) {
	a := newApp(t)
	a.running = true
	a.Stop()
	if a.running {
		t.Fatal("Stop should clear running")
	}
}
