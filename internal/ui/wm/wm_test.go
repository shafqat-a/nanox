package wm

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// newTestManager builds a manager with a standard 80x23 desktop (rows 1..23,
// the area between a row-0 menu bar and the status bar) holding n windows. The
// windows are NOT activated yet; callers do that explicitly.
func newTestManager(n int) (*Manager, []*Window) {
	m := NewManager()
	m.SetRect(0, 1, 80, 23)
	ws := make([]*Window, n)
	for i := range ws {
		w := NewWindow(tview.NewBox(), "win")
		w.SetRect(2+i*2, 2+i*2, 20, 10)
		m.Add(w)
		ws[i] = w
	}
	return m, ws
}

func TestActivateRaisesZOrder(t *testing.T) {
	m, ws := newTestManager(3)
	m.Activate(ws[0])

	got := m.Windows()
	if got[len(got)-1] != ws[0] {
		t.Fatalf("Activate did not raise window to top of z-order")
	}
	if m.Active() != ws[0] {
		t.Fatalf("Active() = %p, want %p", m.Active(), ws[0])
	}
	if !ws[0].IsActive() {
		t.Fatalf("activated window is not marked active")
	}
	for _, w := range []*Window{ws[1], ws[2]} {
		if w.IsActive() {
			t.Fatalf("non-activated window still marked active")
		}
	}
}

func TestNextPrevCycle(t *testing.T) {
	m, ws := newTestManager(3)
	m.Activate(ws[2]) // top order: [0,1,2], active = 2

	m.Next() // raises bottom (ws[0]) to top
	if m.Active() != ws[0] {
		t.Fatalf("Next: active = %p, want ws[0]", m.Active())
	}
	m.Next() // order now [1,2,0]; bottom = ws[1]
	if m.Active() != ws[1] {
		t.Fatalf("Next x2: active = %p, want ws[1]", m.Active())
	}

	// Prev should activate the window just below the current top.
	m, ws = newTestManager(3)
	m.Activate(ws[2]) // order [0,1,2]
	m.Prev()          // raises ws[1] (n-2)
	if m.Active() != ws[1] {
		t.Fatalf("Prev: active = %p, want ws[1]", m.Active())
	}

	// Single-window wrap: Next/Prev keep it active without panicking.
	m1, w1 := newTestManager(1)
	m1.Activate(w1[0])
	m1.Next()
	m1.Prev()
	if m1.Active() != w1[0] {
		t.Fatalf("single-window cycle lost the active window")
	}
}

func TestTileNonOverlappingWithinDesktop(t *testing.T) {
	m, ws := newTestManager(4)
	m.Tile()

	dx, dy, dw, dh := m.GetRect()
	rects := make([][4]int, len(ws))
	for i, w := range ws {
		x, y, ww, wh := w.GetRect()
		if ww <= 0 || wh <= 0 {
			t.Fatalf("tiled window %d has non-positive size %dx%d", i, ww, wh)
		}
		if x < dx || y < dy || x+ww > dx+dw || y+wh > dy+dh {
			t.Fatalf("tiled window %d rect (%d,%d,%d,%d) escapes desktop (%d,%d,%d,%d)",
				i, x, y, ww, wh, dx, dy, dw, dh)
		}
		rects[i] = [4]int{x, y, ww, wh}
	}
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if overlap(rects[i], rects[j]) {
				t.Fatalf("tiled windows %d and %d overlap: %v %v", i, j, rects[i], rects[j])
			}
		}
	}
}

func overlap(a, b [4]int) bool {
	return a[0] < b[0]+b[2] && b[0] < a[0]+a[2] &&
		a[1] < b[1]+b[3] && b[1] < a[1]+a[3]
}

func TestCascadeOffsets(t *testing.T) {
	m, ws := newTestManager(3)
	m.Cascade()

	x0, y0, w0, h0 := ws[0].GetRect()
	x1, y1, _, _ := ws[1].GetRect()
	if x1 <= x0 || y1 <= y0 {
		t.Fatalf("cascade did not offset window 1 down-right of window 0: w0=(%d,%d) w1=(%d,%d)",
			x0, y0, x1, y1)
	}
	// All windows should be the same ~2/3 size.
	if w0 <= 0 || h0 <= 0 {
		t.Fatalf("cascade window 0 has bad size %dx%d", w0, h0)
	}
	dx, dy, dw, dh := m.GetRect()
	for i, w := range ws {
		x, y, ww, wh := w.GetRect()
		if x < dx || y < dy || x+ww > dx+dw || y+wh > dy+dh {
			t.Fatalf("cascade window %d escapes desktop", i)
		}
	}
}

func TestHitTestRegions(t *testing.T) {
	w := NewWindow(tview.NewBox(), "title")
	w.SetActive(true)
	w.SetRect(10, 5, 20, 10) // x:10..29, y:5..14

	cases := []struct {
		name   string
		x, y   int
		region Region
	}{
		{"close button", 11, 5, RegionClose},             // x+1 on title row
		{"max button", 10 + 20 - 2, 5, RegionMaxRestore}, // x+width-2 on title row
		{"title cell", 15, 5, RegionTitle},               // other title-row cell
		{"resize corner", 10 + 20 - 1, 5 + 10 - 1, RegionResize},
		{"content", 15, 9, RegionContent},
		{"outside left", 9, 9, RegionNone},
		{"outside below", 15, 20, RegionNone},
	}
	for _, c := range cases {
		if got := w.HitTest(c.x, c.y); got != c.region {
			t.Errorf("HitTest(%s)=%v, want %v", c.name, got, c.region)
		}
	}
}

func TestInnerRect(t *testing.T) {
	w := NewWindow(tview.NewBox(), "t")
	w.SetRect(10, 5, 20, 10)
	ix, iy, iw, ih := w.GetInnerRect()
	if ix != 11 || iy != 6 || iw != 18 || ih != 8 {
		t.Fatalf("GetInnerRect=(%d,%d,%d,%d), want (11,6,18,8)", ix, iy, iw, ih)
	}
}

func TestMaximizeRestore(t *testing.T) {
	m, ws := newTestManager(1)
	w := ws[0]
	w.SetRect(3, 4, 20, 10)
	w.setDesktop(m.GetRect())

	w.ToggleMaximize()
	if !w.IsMaximized() {
		t.Fatalf("ToggleMaximize did not maximize")
	}
	dx, dy, dw, dh := m.GetRect()
	x, y, ww, wh := w.GetRect()
	if x != dx || y != dy || ww != dw || wh != dh {
		t.Fatalf("maximized rect (%d,%d,%d,%d) != desktop (%d,%d,%d,%d)", x, y, ww, wh, dx, dy, dw, dh)
	}

	w.ToggleMaximize()
	if w.IsMaximized() {
		t.Fatalf("ToggleMaximize did not restore")
	}
	x, y, ww, wh = w.GetRect()
	if x != 3 || y != 4 || ww != 20 || wh != 10 {
		t.Fatalf("restored rect (%d,%d,%d,%d), want (3,4,20,10)", x, y, ww, wh)
	}
}

// mouseEvent builds a left-button mouse event at (x,y).
func mouseEvent(x, y int, primary bool) *tcell.EventMouse {
	var btn tcell.ButtonMask
	if primary {
		btn = tcell.ButtonPrimary
	}
	return tcell.NewEventMouse(x, y, btn, tcell.ModNone)
}

func TestDragMovesWindow(t *testing.T) {
	m, ws := newTestManager(1)
	w := ws[0]
	w.SetRect(5, 5, 20, 10)

	// Grab the title bar at x=10 (a plain title cell), y=5.
	consumed, _ := m.HandleMouse(tview.MouseLeftDown, mouseEvent(10, 5, true))
	if !consumed {
		t.Fatalf("LeftDown on title was not consumed")
	}
	if m.Active() != w {
		t.Fatalf("LeftDown did not activate the window")
	}

	// Move pointer +7,+3.
	consumed, _ = m.HandleMouse(tview.MouseMove, mouseEvent(17, 8, true))
	if !consumed {
		t.Fatalf("MouseMove during drag not consumed")
	}
	x, y, _, _ := w.GetRect()
	if x != 12 || y != 8 {
		t.Fatalf("after drag rect origin (%d,%d), want (12,8)", x, y)
	}

	m.HandleMouse(tview.MouseLeftUp, mouseEvent(17, 8, false))
	// A move after release must not keep dragging.
	m.HandleMouse(tview.MouseMove, mouseEvent(30, 20, false))
	x, y, _, _ = w.GetRect()
	if x != 12 || y != 8 {
		t.Fatalf("window moved after LeftUp: (%d,%d)", x, y)
	}
}

func TestDragClampsAtEdge(t *testing.T) {
	m, ws := newTestManager(1)
	w := ws[0]
	w.SetRect(5, 5, 20, 10) // desktop is (0,1,80,23)

	m.HandleMouse(tview.MouseLeftDown, mouseEvent(10, 5, true))
	// Drag far up-left past the desktop origin.
	m.HandleMouse(tview.MouseMove, mouseEvent(-50, -50, true))
	x, y, _, _ := w.GetRect()
	dx, dy, _, _ := m.GetRect()
	if x != dx || y != dy {
		t.Fatalf("drag past top-left not clamped: (%d,%d), want (%d,%d)", x, y, dx, dy)
	}

	// Drag far down-right past the desktop extent.
	m.HandleMouse(tview.MouseMove, mouseEvent(500, 500, true))
	x, y, ww, wh := w.GetRect()
	dx, dy, dw, dh := m.GetRect()
	if x+ww > dx+dw || y+wh > dy+dh {
		t.Fatalf("drag past bottom-right not clamped: rect (%d,%d,%d,%d) desktop (%d,%d,%d,%d)",
			x, y, ww, wh, dx, dy, dw, dh)
	}
}

func TestResizeViaMouse(t *testing.T) {
	m, ws := newTestManager(1)
	w := ws[0]
	w.SetRect(5, 5, 20, 10) // corner at (24,14)

	consumed, _ := m.HandleMouse(tview.MouseLeftDown, mouseEvent(24, 14, true))
	if !consumed {
		t.Fatalf("LeftDown on resize grip not consumed")
	}
	// Drag the corner to (30,18).
	m.HandleMouse(tview.MouseMove, mouseEvent(30, 18, true))
	x, y, ww, wh := w.GetRect()
	if x != 5 || y != 5 || ww != 26 || wh != 14 {
		t.Fatalf("after resize rect (%d,%d,%d,%d), want (5,5,26,14)", x, y, ww, wh)
	}

	// Shrink below min size: should clamp to MinWindowWidth/Height.
	m.HandleMouse(tview.MouseMove, mouseEvent(0, 0, true))
	_, _, ww, wh = w.GetRect()
	if ww < MinWindowWidth || wh < MinWindowHeight {
		t.Fatalf("resize below min not clamped: %dx%d", ww, wh)
	}
	m.HandleMouse(tview.MouseLeftUp, mouseEvent(0, 0, false))
}

func TestEmptyDesktopClickNotConsumed(t *testing.T) {
	m, _ := newTestManager(1)
	// Click well away from the only window (which is at 2,2,20,10).
	consumed, content := m.HandleMouse(tview.MouseLeftDown, mouseEvent(70, 20, true))
	if consumed {
		t.Fatalf("click on empty desktop was consumed")
	}
	if content != nil {
		t.Fatalf("empty-desktop click returned content")
	}
}

func TestContentClickReturnsContent(t *testing.T) {
	m := NewManager()
	m.SetRect(0, 1, 80, 23)
	content := tview.NewBox()
	w := NewWindow(content, "win")
	w.SetRect(5, 5, 20, 10)
	m.Add(w)

	// Click inside the content area.
	consumed, got := m.HandleMouse(tview.MouseLeftDown, mouseEvent(10, 9, true))
	if !consumed {
		t.Fatalf("content click not consumed")
	}
	if got != content {
		t.Fatalf("content click returned %p, want the content box %p", got, content)
	}
}

func TestCloseAndMaxButtons(t *testing.T) {
	m, ws := newTestManager(1)
	w := ws[0]
	w.SetRect(5, 5, 20, 10)
	closed := false
	toggled := false
	w.SetOnClose(func() { closed = true })
	w.SetOnToggleMax(func() { toggled = true })

	// Close button at x+1 = 6 on the title row.
	m.HandleMouse(tview.MouseLeftDown, mouseEvent(6, 5, true))
	if !closed {
		t.Fatalf("close button did not fire onClose")
	}

	// Max button at x+width-2 = 23 on the title row.
	m.HandleMouse(tview.MouseLeftDown, mouseEvent(23, 5, true))
	if !toggled {
		t.Fatalf("max button did not fire onToggleMax")
	}
	if !w.IsMaximized() {
		t.Fatalf("max button did not maximize the window")
	}
}

func TestKeyboardMoveResize(t *testing.T) {
	m, ws := newTestManager(1)
	w := ws[0]
	w.SetRect(5, 5, 20, 10)
	m.Activate(w)

	m.MoveActive(3, 2)
	x, y, _, _ := w.GetRect()
	if x != 8 || y != 7 {
		t.Fatalf("MoveActive origin (%d,%d), want (8,7)", x, y)
	}

	m.ResizeActive(4, -2)
	_, _, ww, wh := w.GetRect()
	if ww != 24 || wh != 8 {
		t.Fatalf("ResizeActive size (%d,%d), want (24,8)", ww, wh)
	}
}

func TestDrawSmokeNoPanic(t *testing.T) {
	m, ws := newTestManager(2)
	m.Activate(ws[0])
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	screen.SetSize(80, 25)
	defer screen.Fini()
	m.Draw(screen) // must not panic with active/inactive windows on the desktop
}

func TestRemoveActivatesNext(t *testing.T) {
	m, ws := newTestManager(3)
	m.Activate(ws[2])
	m.Remove(ws[2])
	if len(m.Windows()) != 2 {
		t.Fatalf("Remove left %d windows, want 2", len(m.Windows()))
	}
	if m.Active() == nil {
		t.Fatalf("Remove of active window left no active window")
	}
}
