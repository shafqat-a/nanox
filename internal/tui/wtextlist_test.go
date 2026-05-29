package tui

import (
	"strconv"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// simScreen returns an initialised SimulationScreen of the given size.
func simScreen(t *testing.T, w, h int) tcell.Screen {
	t.Helper()
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	scr.SetSize(w, h)
	return scr
}

// runeAt returns the rune at an absolute cell after Show.
func runeAt(scr tcell.Screen, x, y int) rune {
	r, _, _, _ := scr.GetContent(x, y)
	return r
}

func keyOf(k tcell.Key) *tcell.EventKey { return tcell.NewEventKey(k, 0, tcell.ModNone) }

// --- TextBox ---------------------------------------------------------------

func TestTextBoxInsertBackspaceText(t *testing.T) {
	tb := NewTextBox("", 10)
	tb.SetFocused(true)
	for _, r := range "hello" {
		if !tb.HandleKey(keyRune(r)) {
			t.Fatal("rune not consumed")
		}
	}
	if tb.Text() != "hello" {
		t.Fatalf("Text = %q want hello", tb.Text())
	}
	tb.HandleKey(keyOf(tcell.KeyBackspace))
	if tb.Text() != "hell" {
		t.Fatalf("after backspace = %q", tb.Text())
	}
	// Home then Delete removes the first char.
	tb.HandleKey(keyOf(tcell.KeyHome))
	tb.HandleKey(keyOf(tcell.KeyDelete))
	if tb.Text() != "ell" {
		t.Fatalf("after home+delete = %q", tb.Text())
	}
	// Left then insert mid-string.
	tb.HandleKey(keyOf(tcell.KeyEnd))
	tb.HandleKey(keyOf(tcell.KeyLeft))
	tb.HandleKey(keyRune('X'))
	if tb.Text() != "elXl" {
		t.Fatalf("mid insert = %q", tb.Text())
	}
}

func TestTextBoxOnChange(t *testing.T) {
	tb := NewTextBox("", 8)
	tb.SetFocused(true)
	var got string
	tb.SetOnChange(func(s string) { got = s })
	tb.HandleKey(keyRune('a'))
	if got != "a" {
		t.Fatalf("onChange = %q", got)
	}
	tb.SetText("zz")
	if got != "zz" {
		t.Fatalf("SetText onChange = %q", got)
	}
}

func TestTextBoxRenderAndCaret(t *testing.T) {
	scr := simScreen(t, 20, 3)
	tb := NewTextBox("ab", 6)
	tb.SetBounds(Rect{X: 1, Y: 1, W: 8, H: 1}) // interior width 6
	tb.SetFocused(true)
	surf := NewScreenSurface(scr, Rect{X: 0, Y: 0, W: 20, H: 3})
	tb.Draw(surf)
	scr.Show()
	// Recessed frame edges.
	if runeAt(scr, 1, 1) != VSingleEdge() {
		t.Fatalf("left edge = %q", runeAt(scr, 1, 1))
	}
	// Text glyphs in the interior.
	if runeAt(scr, 2, 1) != 'a' || runeAt(scr, 3, 1) != 'b' {
		t.Fatal("text not rendered in field interior")
	}
}

// VSingleEdge exposes the frame glyph used for the recessed edges so the test
// reads naturally without importing theme.
func VSingleEdge() rune { return '│' }

func TestTextBoxHorizontalScroll(t *testing.T) {
	tb := NewTextBox("", 4)
	tb.SetBounds(Rect{X: 0, Y: 0, W: 6, H: 1}) // interior 4
	tb.SetFocused(true)
	for _, r := range "abcdefgh" {
		tb.HandleKey(keyRune(r))
	}
	// Caret at end (8); interior width 4 -> scroll should expose the tail.
	if tb.scroll == 0 {
		t.Fatalf("expected horizontal scroll, scroll=%d", tb.scroll)
	}
	if tb.caret-tb.scroll > 3 {
		t.Fatalf("caret not visible: caret=%d scroll=%d", tb.caret, tb.scroll)
	}
}

// --- ListBox ---------------------------------------------------------------

func TestListBoxSelectionMove(t *testing.T) {
	lb := NewListBox([]string{"one", "two", "three"})
	if lb.Selected() != 0 {
		t.Fatal("initial selection")
	}
	lb.SetBounds(Rect{X: 0, Y: 0, W: 10, H: 3})
	lb.HandleKey(keyOf(tcell.KeyDown))
	if lb.Selected() != 1 {
		t.Fatalf("down -> %d", lb.Selected())
	}
	lb.HandleKey(keyOf(tcell.KeyEnd))
	if lb.Selected() != 2 {
		t.Fatalf("end -> %d", lb.Selected())
	}
	lb.HandleKey(keyOf(tcell.KeyHome))
	if lb.Selected() != 0 {
		t.Fatalf("home -> %d", lb.Selected())
	}
	// Up at top stays clamped.
	lb.HandleKey(keyOf(tcell.KeyUp))
	if lb.Selected() != 0 {
		t.Fatalf("up clamp -> %d", lb.Selected())
	}
}

func TestListBoxSetSelectedAndOnSelect(t *testing.T) {
	lb := NewListBox([]string{"a", "b", "c"})
	var fired int
	fired = -1
	lb.SetOnSelect(func(i int) { fired = i })
	lb.SetSelected(2)
	if lb.Selected() != 2 || fired != 2 {
		t.Fatalf("SetSelected sel=%d fired=%d", lb.Selected(), fired)
	}
	// Out-of-range clamps.
	lb.SetSelected(99)
	if lb.Selected() != 2 {
		t.Fatalf("clamp high -> %d", lb.Selected())
	}
}

func TestListBoxActivate(t *testing.T) {
	lb := NewListBox([]string{"a", "b"})
	lb.SetBounds(Rect{X: 0, Y: 0, W: 8, H: 2})
	activated := -1
	lb.SetOnActivate(func(i int) { activated = i })
	lb.HandleKey(keyOf(tcell.KeyEnter))
	if activated != 0 {
		t.Fatalf("enter activate -> %d", activated)
	}
}

func TestListBoxScrollbarAppears(t *testing.T) {
	// 6 items, height 3 -> scrollbar required.
	items := make([]string, 6)
	for i := range items {
		items[i] = "item" + strconv.Itoa(i)
	}
	lb := NewListBox(items)
	lb.SetBounds(Rect{X: 0, Y: 0, W: 10, H: 3})
	if !lb.lbNeedBar() {
		t.Fatal("scrollbar should be required")
	}
	scr := simScreen(t, 12, 5)
	surf := NewScreenSurface(scr, Rect{X: 0, Y: 0, W: 12, H: 5})
	lb.Draw(surf)
	scr.Show()
	// Right inner column at x=9: top arrow is SbUp '▲'.
	if runeAt(scr, 9, 0) != '▲' {
		t.Fatalf("expected up arrow at bar top, got %q", runeAt(scr, 9, 0))
	}
	if runeAt(scr, 9, 2) != '▼' {
		t.Fatalf("expected down arrow at bar bottom, got %q", runeAt(scr, 9, 2))
	}
}

func TestListBoxNoScrollbarWhenFits(t *testing.T) {
	lb := NewListBox([]string{"a", "b"})
	lb.SetBounds(Rect{X: 0, Y: 0, W: 10, H: 4})
	if lb.lbNeedBar() {
		t.Fatal("scrollbar should NOT appear when items fit")
	}
}

func TestListBoxClickSelects(t *testing.T) {
	lb := NewListBox([]string{"a", "b", "c"})
	lb.SetBounds(Rect{X: 0, Y: 0, W: 10, H: 3})
	lb.HandleMouse(MouseEvent{X: 2, Y: 1, Action: MouseDown, Button: tcell.Button1})
	if lb.Selected() != 1 {
		t.Fatalf("click row 1 -> %d", lb.Selected())
	}
}

// --- ScrollBar -------------------------------------------------------------

func TestScrollBarArrowsAndPage(t *testing.T) {
	sb := NewScrollBar(true)
	sb.SetBounds(Rect{X: 0, Y: 0, W: 1, H: 10})
	sb.SetRange(0, 100, 10)
	var last int
	sb.SetOnChange(func(v int) { last = v })

	// Down arrow at the bottom cell steps +1.
	sb.HandleMouse(MouseEvent{X: 0, Y: 9, Action: MouseDown})
	if sb.Value() != 1 || last != 1 {
		t.Fatalf("down arrow value=%d last=%d", sb.Value(), last)
	}
	// Up arrow at the top cell steps -1.
	sb.HandleMouse(MouseEvent{X: 0, Y: 0, Action: MouseDown})
	if sb.Value() != 0 {
		t.Fatalf("up arrow value=%d", sb.Value())
	}
	// Click on the track below the thumb pages down.
	sb.HandleMouse(MouseEvent{X: 0, Y: 8, Action: MouseDown})
	if sb.Value() != 10 {
		t.Fatalf("page down value=%d", sb.Value())
	}
}

func TestScrollBarDragChangesValue(t *testing.T) {
	sb := NewScrollBar(true)
	sb.SetBounds(Rect{X: 0, Y: 0, W: 1, H: 12})
	sb.SetRange(0, 100, 10)
	// Begin drag on the thumb (track index 0 -> y=1).
	sb.HandleMouse(MouseEvent{X: 0, Y: 1, Action: MouseDown})
	if !sb.dragging {
		t.Fatal("MouseDown on thumb should begin drag")
	}
	// Drag toward the bottom of the track.
	sb.HandleMouse(MouseEvent{X: 0, Y: 10, Action: MouseDrag})
	if sb.Value() <= 0 {
		t.Fatalf("drag should raise value, got %d", sb.Value())
	}
	prev := sb.Value()
	sb.HandleMouse(MouseEvent{X: 0, Y: 11, Action: MouseUp})
	if sb.dragging {
		t.Fatal("MouseUp should end drag")
	}
	// Dragging back up lowers the value.
	sb.HandleMouse(MouseEvent{X: 0, Y: 1, Action: MouseDown})
	sb.HandleMouse(MouseEvent{X: 0, Y: 2, Action: MouseDrag})
	if sb.Value() >= prev {
		t.Fatalf("drag up should lower value: now=%d prev=%d", sb.Value(), prev)
	}
}

func TestScrollBarHorizontalRender(t *testing.T) {
	scr := simScreen(t, 12, 1)
	sb := NewScrollBar(false)
	sb.SetBounds(Rect{X: 0, Y: 0, W: 10, H: 1})
	sb.SetRange(0, 50, 5)
	surf := NewScreenSurface(scr, Rect{X: 0, Y: 0, W: 12, H: 1})
	sb.Draw(surf)
	scr.Show()
	if runeAt(scr, 0, 0) != '◄' {
		t.Fatalf("left arrow = %q", runeAt(scr, 0, 0))
	}
	if runeAt(scr, 9, 0) != '►' {
		t.Fatalf("right arrow = %q", runeAt(scr, 9, 0))
	}
}

// --- ComboBox --------------------------------------------------------------

func TestComboBoxSetItemsSelected(t *testing.T) {
	cmb := NewComboBox([]string{"red", "green", "blue"})
	if cmb.Selected() != 0 || cmb.SelectedText() != "red" {
		t.Fatalf("initial sel=%d text=%q", cmb.Selected(), cmb.SelectedText())
	}
	cmb.SetItems([]string{"x", "y"})
	if cmb.Selected() != 0 {
		t.Fatalf("after SetItems sel=%d", cmb.Selected())
	}
	cmb.cmbSetSelected(1)
	if cmb.SelectedText() != "y" {
		t.Fatalf("selected text = %q", cmb.SelectedText())
	}
}

func TestComboBoxOnChange(t *testing.T) {
	cmb := NewComboBox([]string{"a", "b", "c"})
	got := -1
	cmb.SetOnChange(func(i int) { got = i })
	cmb.cmbSetSelected(2)
	if got != 2 {
		t.Fatalf("onChange = %d", got)
	}
}

func TestComboBoxCollapsedRender(t *testing.T) {
	scr := simScreen(t, 12, 2)
	cmb := NewComboBox([]string{"hi", "yo"})
	cmb.SetBounds(Rect{X: 0, Y: 0, W: 6, H: 1})
	cmb.SetFocused(true)
	surf := NewScreenSurface(scr, Rect{X: 0, Y: 0, W: 12, H: 2})
	cmb.Draw(surf)
	scr.Show()
	if runeAt(scr, 0, 0) != 'h' || runeAt(scr, 1, 0) != 'i' {
		t.Fatal("selected text not rendered")
	}
	if runeAt(scr, 5, 0) != '▼' {
		t.Fatalf("drop button = %q", runeAt(scr, 5, 0))
	}
}

func TestComboBoxOpenNoAppNoOp(t *testing.T) {
	cmb := NewComboBox([]string{"a", "b"})
	// No App set: opening must not panic and must not push anything.
	cmb.cmbOpen()
	if cmb.popup != nil {
		t.Fatal("open without App should be a no-op")
	}
}

func TestComboBoxPopupOverlay(t *testing.T) {
	scr := simScreen(t, 40, 12)
	a := NewApp(scr)
	root := &panel{}
	cmb := NewComboBox([]string{"alpha", "beta", "gamma"})
	cmb.SetApp(a)
	cmb.SetBounds(Rect{X: 2, Y: 2, W: 10, H: 1})
	root.Add(cmb)
	a.SetRoot(root)

	// Open via key (Down).
	cmb.HandleKey(keyOf(tcell.KeyDown))
	if len(a.modals) != 1 {
		t.Fatalf("popup not pushed: modals=%d", len(a.modals))
	}
	popup := cmb.popup
	// Lay out + draw so the list bounds are computed.
	a.draw()

	// Navigate down then Enter -> selects index 1 and closes.
	popup.HandleKey(keyOf(tcell.KeyDown))
	popup.HandleKey(keyOf(tcell.KeyEnter))
	if cmb.Selected() != 1 {
		t.Fatalf("popup enter selection = %d want 1", cmb.Selected())
	}
	if len(a.modals) != 0 {
		t.Fatal("popup should close on Enter")
	}
}

func TestComboBoxPopupClickSelectsAndCloses(t *testing.T) {
	scr := simScreen(t, 40, 12)
	a := NewApp(scr)
	root := &panel{}
	cmb := NewComboBox([]string{"alpha", "beta", "gamma"})
	cmb.SetApp(a)
	cmb.SetBounds(Rect{X: 2, Y: 2, W: 10, H: 1})
	root.Add(cmb)
	a.SetRoot(root)

	cmb.HandleMouse(MouseEvent{X: 3, Y: 2, Action: MouseDown})
	if len(a.modals) != 1 {
		t.Fatalf("click should open popup: modals=%d", len(a.modals))
	}
	popup := cmb.popup
	a.draw()
	lb := popup.list.Bounds()
	// Click the second visible row.
	popup.HandleMouse(MouseEvent{X: lb.X, Y: lb.Y + 1, Action: MouseDown})
	if cmb.Selected() != 1 {
		t.Fatalf("click select = %d want 1", cmb.Selected())
	}
	if len(a.modals) != 0 {
		t.Fatal("click on item should close popup")
	}
}

func TestComboBoxPopupClickOutsideCloses(t *testing.T) {
	scr := simScreen(t, 40, 12)
	a := NewApp(scr)
	root := &panel{}
	cmb := NewComboBox([]string{"alpha", "beta"})
	cmb.SetApp(a)
	cmb.SetBounds(Rect{X: 2, Y: 2, W: 10, H: 1})
	root.Add(cmb)
	a.SetRoot(root)

	cmb.cmbOpen()
	popup := cmb.popup
	a.draw()
	before := cmb.Selected()
	// Click far outside the list rectangle.
	popup.HandleMouse(MouseEvent{X: 39, Y: 11, Action: MouseDown})
	if len(a.modals) != 0 {
		t.Fatal("outside click should close popup")
	}
	if cmb.Selected() != before {
		t.Fatal("outside click must not change selection")
	}
}
