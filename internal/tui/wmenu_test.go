package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// --- helpers ----------------------------------------------------------------

// readRow returns the runes on a screen row in [x0, x1) as a string.
func readRow(scr tcell.SimulationScreen, x0, x1, y int) string {
	var b strings.Builder
	for x := x0; x < x1; x++ {
		r, _, _, _ := scr.GetContent(x, y)
		if r == 0 {
			r = ' '
		}
		b.WriteRune(r)
	}
	return b.String()
}

func sampleMenus(rec *[]string) []*Menu {
	mk := func(name string) func() {
		return func() { *rec = append(*rec, name) }
	}
	return []*Menu{
		{Title: "File", Mnemonic: 'F', Items: []MenuItem{
			{Label: "New", Mnemonic: 'N', Accel: "Ctrl+N", Action: mk("New")},
			{Label: "Open", Mnemonic: 'O', Accel: "F3", Action: mk("Open")},
			{Separator: true},
			{Label: "Save", Mnemonic: 'S', Accel: "F2", Disabled: true},
			{Label: "Exit", Mnemonic: 'x', Accel: "Alt+X", Action: mk("Exit")},
		}},
		{Title: "Edit", Mnemonic: 'E', Items: []MenuItem{
			{Label: "Cut", Mnemonic: 'C', Accel: "Ctrl+X", Action: mk("Cut")},
			{Label: "Copy", Mnemonic: 'o', Accel: "Ctrl+C", Action: mk("Copy")},
		}},
		{Title: "Help", Mnemonic: 'H', Items: []MenuItem{
			{Label: "About", Mnemonic: 'A', Action: mk("About")},
		}},
	}
}

func wmenuApp(t *testing.T) (*App, tcell.SimulationScreen) {
	t.Helper()
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	scr.SetSize(80, 25)
	return NewApp(scr), scr
}

// drawAll renders root + modals to the simulation screen (mirrors App.draw).
func drawAll(a *App, scr tcell.SimulationScreen) {
	scr.Clear()
	sw, sh := scr.Size()
	full := Rect{X: 0, Y: 0, W: sw, H: sh}
	if a.root != nil {
		a.root.Draw(NewScreenSurface(scr, full))
	}
	for _, m := range a.modals {
		m.Draw(NewScreenSurface(scr, full))
	}
	scr.Show()
}

// --- menubar rendering ------------------------------------------------------

func TestMenuBarRendersTitlesAndMnemonics(t *testing.T) {
	a, scr := wmenuApp(t)
	var rec []string
	mb := NewMenuBar(sampleMenus(&rec))
	mb.SetBounds(Rect{X: 0, Y: 0, W: 80, H: 1})
	a.SetRoot(mb)
	drawAll(a, scr)

	row := readRow(scr, 0, 80, 0)
	if !strings.Contains(row, "File") || !strings.Contains(row, "Edit") {
		t.Fatalf("bar row missing titles: %q", row)
	}
	// Help is right-aligned.
	if !strings.HasSuffix(strings.TrimRight(row, " "), "Help") {
		t.Fatalf("Help not at right edge: %q", row)
	}

	// Mnemonic of File ('F') is drawn in the MenuMnemonic style (white fg).
	_, _, st, _ := scr.GetContent(2, 0) // 'F'
	fg, _, _ := st.Decompose()
	wf, _, _ := tcellRGB(fg)
	if wf < 200 {
		t.Fatalf("File mnemonic not highlighted (fg=%v)", fg)
	}
}

// tcellRGB returns the RGB components of a tcell.Color.
func tcellRGB(c tcell.Color) (int32, int32, int32) {
	r, g, b := c.RGB()
	return r, g, b
}

// --- activate / dropdown / esc ----------------------------------------------

func TestMenuBarActivateOpensDropdownAndEscCloses(t *testing.T) {
	a, scr := wmenuApp(t)
	var rec []string
	mb := NewMenuBar(sampleMenus(&rec))
	mb.SetApp(a)
	a.SetRoot(mb)

	if mb.IsActive() {
		t.Fatal("should start inactive")
	}
	mb.Activate()
	if !mb.IsActive() {
		t.Fatal("Activate should open the bar")
	}
	if len(a.modals) != 1 {
		t.Fatalf("Activate should push one modal, got %d", len(a.modals))
	}

	drawAll(a, scr)
	// The dropdown should render its first item label "New" somewhere below row 0.
	found := false
	for y := 1; y < 10 && !found; y++ {
		if strings.Contains(readRow(scr, 0, 40, y), "New") {
			found = true
		}
	}
	if !found {
		t.Fatal("dropdown did not render item 'New'")
	}

	// Esc closes everything.
	a.dispatchKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if mb.IsActive() {
		t.Fatal("Esc should close the bar")
	}
	if len(a.modals) != 0 {
		t.Fatalf("Esc should pop the modal, got %d", len(a.modals))
	}
}

func TestMenuBarOnCloseFires(t *testing.T) {
	a, _ := wmenuApp(t)
	var rec []string
	mb := NewMenuBar(sampleMenus(&rec))
	mb.SetApp(a)
	a.SetRoot(mb)

	closed := false
	mb.SetOnClose(func() { closed = true })
	mb.Activate()
	a.dispatchKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if !closed {
		t.Fatal("OnClose should fire on Esc")
	}
}

// --- selecting an item runs its action --------------------------------------

func TestMenuItemActionRunsViaEnter(t *testing.T) {
	a, _ := wmenuApp(t)
	var rec []string
	mb := NewMenuBar(sampleMenus(&rec))
	mb.SetApp(a)
	a.SetRoot(mb)

	mb.Activate() // File open, hi=first selectable (New)
	a.dispatchKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if len(rec) != 1 || rec[0] != "New" {
		t.Fatalf("Enter on first item should run New, rec=%v", rec)
	}
	if mb.IsActive() {
		t.Fatal("activating an item should close the bar")
	}
}

func TestMenuItemActionRunsViaMnemonic(t *testing.T) {
	a, _ := wmenuApp(t)
	var rec []string
	mb := NewMenuBar(sampleMenus(&rec))
	mb.SetApp(a)
	a.SetRoot(mb)

	mb.Activate()
	// Press 'o' -> Open.
	a.dispatchKey(tcell.NewEventKey(tcell.KeyRune, 'o', tcell.ModNone))
	if len(rec) != 1 || rec[0] != "Open" {
		t.Fatalf("mnemonic 'o' should run Open, rec=%v", rec)
	}
}

func TestMenuArrowsNavigateSkippingSeparatorAndDisabled(t *testing.T) {
	a, _ := wmenuApp(t)
	var rec []string
	mb := NewMenuBar(sampleMenus(&rec))
	mb.SetApp(a)
	a.SetRoot(mb)

	mb.Activate() // File: New, Open, ---, Save(disabled), Exit ; hi=New(0)
	// Down: Open(1)
	a.dispatchKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	// Down: should skip separator(2) and disabled Save(3) -> Exit(4)
	a.dispatchKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if mb.popup.hi != 4 {
		t.Fatalf("hi=%d, want 4 (Exit) after skipping separator/disabled", mb.popup.hi)
	}
	a.dispatchKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if len(rec) != 1 || rec[0] != "Exit" {
		t.Fatalf("rec=%v want [Exit]", rec)
	}
}

func TestMenuLeftRightSwitchesMenus(t *testing.T) {
	a, _ := wmenuApp(t)
	var rec []string
	mb := NewMenuBar(sampleMenus(&rec))
	mb.SetApp(a)
	a.SetRoot(mb)

	mb.Activate() // File (0)
	a.dispatchKey(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if mb.active != 1 {
		t.Fatalf("Right should switch to Edit (1), active=%d", mb.active)
	}
	// Still one modal (switch reuses popup).
	if len(a.modals) != 1 {
		t.Fatalf("switching should keep one modal, got %d", len(a.modals))
	}
	if mb.popup.menu.Title != "Edit" {
		t.Fatalf("popup menu = %q want Edit", mb.popup.menu.Title)
	}
}

func TestOpenByMnemonic(t *testing.T) {
	a, _ := wmenuApp(t)
	var rec []string
	mb := NewMenuBar(sampleMenus(&rec))
	mb.SetApp(a)
	a.SetRoot(mb)

	if !mb.OpenByMnemonic('e') {
		t.Fatal("OpenByMnemonic('e') should open Edit")
	}
	if mb.active != 1 {
		t.Fatalf("active=%d want 1 (Edit)", mb.active)
	}
	if mb.OpenByMnemonic('z') {
		t.Fatal("unknown mnemonic should return false")
	}
}

func TestActivateNoAppIsNoOp(t *testing.T) {
	var rec []string
	mb := NewMenuBar(sampleMenus(&rec))
	mb.Activate() // no app set
	if mb.IsActive() {
		t.Fatal("Activate without app should be a no-op")
	}
}

func TestMenuClickOutsideCloses(t *testing.T) {
	a, _ := wmenuApp(t)
	var rec []string
	mb := NewMenuBar(sampleMenus(&rec))
	mb.SetApp(a)
	a.SetRoot(mb)

	mb.Activate()
	// Click far away from the bar/box.
	a.dispatchMouse(tcell.NewEventMouse(70, 20, tcell.Button1, tcell.ModNone))
	if mb.IsActive() {
		t.Fatal("click outside should close the dropdown")
	}
}

// --- status bar -------------------------------------------------------------

func TestStatusBarRendersContexts(t *testing.T) {
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	scr.SetSize(80, 25)
	sb := NewStatusBar()
	sb.SetBounds(Rect{X: 0, Y: 24, W: 80, H: 1})

	cases := map[StatusContext]string{
		CtxEditing: "F2=Save",
		CtxMenu:    "Enter=Select",
		CtxDialog:  "Tab=Next",
		CtxMove:    "Shift+Arrows=Size",
	}
	for ctx, want := range cases {
		sb.SetContext(ctx)
		scr.Clear()
		sb.Draw(NewScreenSurface(scr, Rect{X: 0, Y: 0, W: 80, H: 25}))
		scr.Show()
		row := readRow(scr, 0, 80, 24)
		if !strings.Contains(row, want) {
			t.Fatalf("ctx %d row %q missing %q", ctx, row, want)
		}
	}
}

func TestStatusBarRightAlignedReadout(t *testing.T) {
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	scr.SetSize(80, 25)
	sb := NewStatusBar()
	sb.SetBounds(Rect{X: 0, Y: 24, W: 80, H: 1})
	sb.SetContext(CtxEditing)
	sb.SetCursor(12, 7, true)
	sb.SetModified(true)

	scr.Clear()
	sb.Draw(NewScreenSurface(scr, Rect{X: 0, Y: 0, W: 80, H: 25}))
	scr.Show()
	row := readRow(scr, 0, 80, 24)

	if !strings.Contains(row, "Ln 12") || !strings.Contains(row, "Col 7") {
		t.Fatalf("readout missing Ln/Col: %q", row)
	}
	if !strings.Contains(row, "INS") {
		t.Fatalf("readout missing INS: %q", row)
	}
	if !strings.Contains(row, "*Ln 12") {
		t.Fatalf("modified star missing: %q", row)
	}

	// OVR mode after toggling.
	sb.SetCursor(12, 7, false)
	sb.SetModified(false)
	scr.Clear()
	sb.Draw(NewScreenSurface(scr, Rect{X: 0, Y: 0, W: 80, H: 25}))
	scr.Show()
	row = readRow(scr, 0, 80, 24)
	if !strings.Contains(row, "OVR") {
		t.Fatalf("readout should show OVR: %q", row)
	}
	if strings.Contains(row, "*") {
		t.Fatalf("unmodified should have no star: %q", row)
	}
}

func TestStatusBarNarrowNoPanic(t *testing.T) {
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	scr.SetSize(10, 3)
	sb := NewStatusBar()
	sb.SetBounds(Rect{X: 0, Y: 2, W: 10, H: 1})
	sb.SetContext(CtxEditing)
	sb.SetCursor(999, 999, true)
	// Must not panic on a narrow width.
	sb.Draw(NewScreenSurface(scr, Rect{X: 0, Y: 0, W: 10, H: 3}))
	scr.Show()
}
