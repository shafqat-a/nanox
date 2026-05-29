package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// newTestScreen returns an initialized simulation screen sized to fit widgets.
func newTestScreen(t *testing.T) tcell.Screen {
	t.Helper()
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	scr.SetSize(40, 20)
	t.Cleanup(scr.Fini)
	return scr
}

// cellRune returns the rune drawn at (x, y) on a simulation screen.
func cellRune(t *testing.T, scr tcell.Screen, x, y int) rune {
	t.Helper()
	sim, ok := scr.(tcell.SimulationScreen)
	if !ok {
		t.Fatalf("not a simulation screen")
	}
	cells, w, _ := sim.GetContents()
	c := cells[y*w+x]
	if len(c.Runes) == 0 {
		return ' '
	}
	return c.Runes[0]
}

// cellStyle returns the style at (x, y) on a simulation screen.
func cellStyle(t *testing.T, scr tcell.Screen, x, y int) tcell.Style {
	t.Helper()
	sim := scr.(tcell.SimulationScreen)
	cells, w, _ := sim.GetContents()
	return cells[y*w+x].Style
}

func drawWidget(scr tcell.Screen, w Widget) {
	s := NewScreenSurface(scr, w.Bounds())
	w.Draw(s)
	scr.Show()
}

// --- Button -----------------------------------------------------------------

func TestButtonFiresOnEnter(t *testing.T) {
	fired := false
	b := NewButton("OK", func() { fired = true })
	if !b.Focusable() {
		t.Fatal("button should be focusable")
	}
	ev := tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)
	if !b.HandleKey(ev) {
		t.Fatal("Enter not consumed")
	}
	if !fired {
		t.Fatal("action not fired on Enter")
	}
}

func TestButtonFiresOnSpaceAndMouse(t *testing.T) {
	count := 0
	b := NewButton("OK", func() { count++ })
	b.SetBounds(Rect{X: 2, Y: 2, W: 5, H: 2})

	sp := tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone)
	if !b.HandleKey(sp) {
		t.Fatal("Space not consumed")
	}
	me := MouseEvent{X: 3, Y: 2, Action: MouseDown, Button: tcell.Button1}
	if !b.HandleMouse(me) {
		t.Fatal("MouseDown not consumed")
	}
	if count != 2 {
		t.Fatalf("want 2 activations, got %d", count)
	}

	// Press outside bounds: no activation.
	out := MouseEvent{X: 30, Y: 18, Action: MouseDown, Button: tcell.Button1}
	if b.HandleMouse(out) {
		t.Fatal("press outside bounds should not consume")
	}
	if count != 2 {
		t.Fatalf("outside press fired action: %d", count)
	}
}

func TestButtonDefaultAndFocusRenderDiffer(t *testing.T) {
	scr := newTestScreen(t)
	bnds := Rect{X: 1, Y: 1, W: 6, H: 2}

	plain := NewButton("OK", nil)
	plain.SetBounds(bnds)
	drawWidget(scr, plain)
	plainEdge := cellRune(t, scr, bnds.X, bnds.Y)

	scr.Clear()
	def := NewButton("OK", nil)
	def.SetDefault(true)
	if !def.IsDefault() {
		t.Fatal("IsDefault should be true")
	}
	def.SetBounds(bnds)
	drawWidget(scr, def)
	defEdge := cellRune(t, scr, bnds.X, bnds.Y)

	if plainEdge == defEdge {
		t.Fatalf("default outline not drawn: plain=%q default=%q", plainEdge, defEdge)
	}

	// Focused render: label row style differs from unfocused.
	scr.Clear()
	plain2 := NewButton("OK", nil)
	plain2.SetBounds(bnds)
	drawWidget(scr, plain2)
	unfStyle := cellStyle(t, scr, bnds.X+2, bnds.Y)

	scr.Clear()
	foc := NewButton("OK", nil)
	foc.SetBounds(bnds)
	foc.SetFocused(true)
	drawWidget(scr, foc)
	focStyle := cellStyle(t, scr, bnds.X+2, bnds.Y)

	if unfStyle == focStyle {
		t.Fatal("focused label row should render in reverse video")
	}
}

func TestButtonPreferredSize(t *testing.T) {
	b := NewButton("OK", nil)
	w, h := b.PreferredSize()
	// face = "OK" (2) + 2 pad = 4; +1 shadow col = 5; height 2.
	if w != 5 || h != 2 {
		t.Fatalf("PreferredSize = %d,%d want 5,2", w, h)
	}
	if b.PreferredWidth() != 5 {
		t.Fatalf("PreferredWidth = %d want 5", b.PreferredWidth())
	}
}

// --- Checkbox ---------------------------------------------------------------

func TestCheckboxToggleKeyAndMouse(t *testing.T) {
	var last bool
	changes := 0
	c := NewCheckbox("Wrap", false)
	c.SetOnChange(func(v bool) { last = v; changes++ })
	c.SetBounds(Rect{X: 0, Y: 0, W: 10, H: 1})

	if c.IsChecked() {
		t.Fatal("should start unchecked")
	}
	sp := tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone)
	if !c.HandleKey(sp) {
		t.Fatal("Space not consumed")
	}
	if !c.IsChecked() || !last {
		t.Fatal("Space should check the box")
	}
	// Click toggles back off.
	me := MouseEvent{X: 1, Y: 0, Action: MouseDown, Button: tcell.Button1}
	if !c.HandleMouse(me) {
		t.Fatal("MouseDown not consumed")
	}
	if c.IsChecked() {
		t.Fatal("click should uncheck the box")
	}
	if changes != 2 {
		t.Fatalf("want 2 changes, got %d", changes)
	}
}

func TestCheckboxRenderAndFocus(t *testing.T) {
	scr := newTestScreen(t)
	bnds := Rect{X: 0, Y: 0, W: 12, H: 1}

	c := NewCheckbox("Wrap", true)
	c.SetBounds(bnds)
	drawWidget(scr, c)
	if got := cellRune(t, scr, 1, 0); got != 'X' {
		t.Fatalf("checked mark = %q want X", got)
	}
	unfStyle := cellStyle(t, scr, 0, 0)

	scr.Clear()
	c.SetFocused(true)
	drawWidget(scr, c)
	focStyle := cellStyle(t, scr, 0, 0)
	if unfStyle == focStyle {
		t.Fatal("focused box cells should render in reverse video")
	}
}

// --- OptionGroup ------------------------------------------------------------

func TestOptionGroupSelectionKeyAndMouse(t *testing.T) {
	g := NewOptionGroup([]string{"Red", "Green", "Blue"})
	var last int
	g.SetOnChange(func(i int) { last = i })
	g.SetBounds(Rect{X: 0, Y: 0, W: 12, H: 3})

	if !g.Focusable() {
		t.Fatal("group should be focusable")
	}
	if g.Selected() != 0 {
		t.Fatalf("default selected = %d want 0", g.Selected())
	}
	if g.SelectedLabel() != "Red" {
		t.Fatalf("default label = %q want Red", g.SelectedLabel())
	}

	down := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	if !g.HandleKey(down) {
		t.Fatal("Down not consumed")
	}
	if g.Selected() != 1 || last != 1 {
		t.Fatalf("after Down selected=%d last=%d want 1", g.Selected(), last)
	}

	// Click on the third row (y=2) selects Blue.
	me := MouseEvent{X: 1, Y: 2, Action: MouseDown, Button: tcell.Button1}
	if !g.HandleMouse(me) {
		t.Fatal("MouseDown not consumed")
	}
	if g.Selected() != 2 || g.SelectedLabel() != "Blue" || last != 2 {
		t.Fatalf("after click selected=%d label=%q", g.Selected(), g.SelectedLabel())
	}

	// Up moves back.
	up := tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	g.HandleKey(up)
	if g.Selected() != 1 {
		t.Fatalf("after Up selected=%d want 1", g.Selected())
	}

	if g.PreferredHeight() != 3 {
		t.Fatalf("PreferredHeight = %d want 3", g.PreferredHeight())
	}
}

func TestOptionGroupRenderMarker(t *testing.T) {
	scr := newTestScreen(t)
	g := NewOptionGroup([]string{"A", "B"})
	g.SetBounds(Rect{X: 0, Y: 0, W: 6, H: 2})
	g.SetSelected(1)
	drawWidget(scr, g)
	// Row 1 marker cell should be the bullet.
	if got := cellRune(t, scr, 1, 1); got != '•' {
		t.Fatalf("selected marker = %q want bullet", got)
	}
	// Row 0 marker should be blank.
	if got := cellRune(t, scr, 1, 0); got != ' ' {
		t.Fatalf("unselected marker = %q want space", got)
	}
}

// --- Label ------------------------------------------------------------------

func TestLabelRender(t *testing.T) {
	scr := newTestScreen(t)
	l := NewLabel("Hi")
	if l.Focusable() {
		t.Fatal("label should not be focusable")
	}
	l.SetBounds(Rect{X: 0, Y: 0, W: 5, H: 1})
	drawWidget(scr, l)
	if got := cellRune(t, scr, 0, 0); got != 'H' {
		t.Fatalf("label[0] = %q want H", got)
	}
	if w, h := l.PreferredSize(); w != 2 || h != 1 {
		t.Fatalf("PreferredSize = %d,%d want 2,1", w, h)
	}
}
