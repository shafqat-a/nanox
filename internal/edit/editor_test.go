package edit

import (
	"testing"

	"dosedit/internal/buffer"
	"dosedit/internal/tui"

	"github.com/gdamore/tcell/v2"
)

// newTestEditor returns an Editor over a buffer seeded with the given lines,
// sized to a bounds rect.
func newTestEditor(t *testing.T, lines []string, w, h int) *Editor {
	t.Helper()
	b := buffer.NewUntitled()
	if lines != nil {
		b.Lines = append([]string(nil), lines...)
	}
	e := NewEditor(b)
	e.SetBounds(tui.Rect{X: 0, Y: 0, W: w, H: h})
	return e
}

func key(k tcell.Key, r rune, mod tcell.ModMask) *tcell.EventKey {
	return tcell.NewEventKey(k, r, mod)
}

func typeRunes(e *Editor, s string) {
	for _, r := range s {
		e.HandleKey(key(tcell.KeyRune, r, tcell.ModNone))
	}
}

func TestTypingInserts(t *testing.T) {
	e := newTestEditor(t, []string{""}, 40, 10)
	typeRunes(e, "hello")
	if got := e.Buffer().Line(0); got != "hello" {
		t.Fatalf("line0 = %q, want %q", got, "hello")
	}
	if e.curCol != 5 {
		t.Fatalf("curCol = %d, want 5", e.curCol)
	}
}

func TestEnterSplits(t *testing.T) {
	e := newTestEditor(t, []string{"abcdef"}, 40, 10)
	// cursor at col 3
	for i := 0; i < 3; i++ {
		e.HandleKey(key(tcell.KeyRight, 0, tcell.ModNone))
	}
	e.HandleKey(key(tcell.KeyEnter, 0, tcell.ModNone))
	if e.Buffer().LineCount() != 2 {
		t.Fatalf("lineCount = %d, want 2", e.Buffer().LineCount())
	}
	if e.Buffer().Line(0) != "abc" || e.Buffer().Line(1) != "def" {
		t.Fatalf("split = %q / %q", e.Buffer().Line(0), e.Buffer().Line(1))
	}
	if e.curLine != 1 || e.curCol != 0 {
		t.Fatalf("cursor = %d,%d want 1,0", e.curLine, e.curCol)
	}
}

func TestEnterAutoIndent(t *testing.T) {
	e := newTestEditor(t, []string{"    foo"}, 40, 10)
	e.HandleKey(key(tcell.KeyEnd, 0, tcell.ModNone))
	e.HandleKey(key(tcell.KeyEnter, 0, tcell.ModNone))
	if e.Buffer().Line(1) != "    " {
		t.Fatalf("indent line = %q, want 4 spaces", e.Buffer().Line(1))
	}
	if e.curCol != 4 {
		t.Fatalf("curCol = %d, want 4", e.curCol)
	}
}

func TestBackspaceJoins(t *testing.T) {
	e := newTestEditor(t, []string{"abc", "def"}, 40, 10)
	e.HandleKey(key(tcell.KeyDown, 0, tcell.ModNone)) // line1 col0
	e.HandleKey(key(tcell.KeyBackspace2, 0, tcell.ModNone))
	if e.Buffer().LineCount() != 1 || e.Buffer().Line(0) != "abcdef" {
		t.Fatalf("after join: %d lines, line0=%q", e.Buffer().LineCount(), e.Buffer().Line(0))
	}
	if e.curLine != 0 || e.curCol != 3 {
		t.Fatalf("cursor = %d,%d want 0,3", e.curLine, e.curCol)
	}
}

func TestArrowMovementClamps(t *testing.T) {
	e := newTestEditor(t, []string{"ab", "cdef"}, 40, 10)
	// Up at top stays at line 0.
	e.HandleKey(key(tcell.KeyUp, 0, tcell.ModNone))
	if e.curLine != 0 {
		t.Fatalf("up at top: curLine=%d", e.curLine)
	}
	// Left at 0,0 stays.
	e.HandleKey(key(tcell.KeyLeft, 0, tcell.ModNone))
	if e.curLine != 0 || e.curCol != 0 {
		t.Fatalf("left at origin: %d,%d", e.curLine, e.curCol)
	}
	// End then Down: col should clamp to shorter? line1 is longer; go to end of line1.
	e.HandleKey(key(tcell.KeyEnd, 0, tcell.ModNone)) // col2 on line0
	e.HandleKey(key(tcell.KeyDown, 0, tcell.ModNone))
	if e.curLine != 1 || e.curCol != 2 {
		t.Fatalf("down keeps col: %d,%d want 1,2", e.curLine, e.curCol)
	}
	// Down at bottom stays.
	e.HandleKey(key(tcell.KeyDown, 0, tcell.ModNone))
	if e.curLine != 1 {
		t.Fatalf("down at bottom: curLine=%d", e.curLine)
	}
}

func TestSelectCopyPaste(t *testing.T) {
	e := newTestEditor(t, []string{"hello"}, 40, 10)
	// Select "hel" with Shift+Right x3.
	for i := 0; i < 3; i++ {
		e.HandleKey(key(tcell.KeyRight, 0, tcell.ModShift))
	}
	if !e.edHasSelection() {
		t.Fatal("expected selection after shift+right")
	}
	if got := e.edSelectedText(); got != "hel" {
		t.Fatalf("selected = %q, want %q", got, "hel")
	}
	e.HandleKey(key(tcell.KeyCtrlC, 0, tcell.ModCtrl))
	// Move to end and paste.
	e.HandleKey(key(tcell.KeyEnd, 0, tcell.ModNone))
	e.HandleKey(key(tcell.KeyCtrlV, 0, tcell.ModCtrl))
	if got := e.Buffer().Line(0); got != "hellohel" {
		t.Fatalf("after paste: %q, want %q", got, "hellohel")
	}
}

func TestSelectCut(t *testing.T) {
	e := newTestEditor(t, []string{"hello"}, 40, 10)
	for i := 0; i < 3; i++ {
		e.HandleKey(key(tcell.KeyRight, 0, tcell.ModShift))
	}
	e.HandleKey(key(tcell.KeyCtrlX, 0, tcell.ModCtrl))
	if got := e.Buffer().Line(0); got != "lo" {
		t.Fatalf("after cut: %q, want %q", got, "lo")
	}
	if e.edHasSelection() {
		t.Fatal("selection should be cleared after cut")
	}
}

func TestUndoRedo(t *testing.T) {
	e := newTestEditor(t, []string{""}, 40, 10)
	typeRunes(e, "abc")
	e.HandleKey(key(tcell.KeyCtrlZ, 0, tcell.ModCtrl)) // undo last 'c'
	if got := e.Buffer().Line(0); got != "ab" {
		t.Fatalf("after undo: %q, want %q", got, "ab")
	}
	e.HandleKey(key(tcell.KeyCtrlY, 0, tcell.ModCtrl)) // redo 'c'
	if got := e.Buffer().Line(0); got != "abc" {
		t.Fatalf("after redo: %q, want %q", got, "abc")
	}
}

func TestGotoLine(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	e := newTestEditor(t, lines, 40, 10)
	e.GotoLine(30)
	if e.curLine != 29 {
		t.Fatalf("GotoLine(30): curLine=%d want 29", e.curLine)
	}
	// Clamp beyond end.
	e.GotoLine(1000)
	if e.curLine != 49 {
		t.Fatalf("GotoLine(1000): curLine=%d want 49", e.curLine)
	}
}

func TestFindMovesCursor(t *testing.T) {
	e := newTestEditor(t, []string{"alpha", "beta", "gamma beta"}, 40, 10)
	ok := e.Find("beta", false, false, true)
	if !ok {
		t.Fatal("Find should succeed")
	}
	if e.curLine != 1 {
		t.Fatalf("Find landed on line %d, want 1", e.curLine)
	}
	// FindNext should advance to the second "beta" on line 2.
	if !e.FindNext() {
		t.Fatal("FindNext should succeed")
	}
	if e.curLine != 2 {
		t.Fatalf("FindNext landed on line %d, want 2", e.curLine)
	}
}

func TestReplaceAll(t *testing.T) {
	e := newTestEditor(t, []string{"foo foo", "foo"}, 40, 10)
	n := e.ReplaceAll("foo", "bar", false, false)
	if n != 3 {
		t.Fatalf("ReplaceAll count = %d, want 3", n)
	}
	if e.Buffer().Line(0) != "bar bar" || e.Buffer().Line(1) != "bar" {
		t.Fatalf("after replaceall: %q / %q", e.Buffer().Line(0), e.Buffer().Line(1))
	}
}

func TestTabIndentBlock(t *testing.T) {
	e := newTestEditor(t, []string{"a", "b", "c"}, 40, 10)
	e.buf.UseSpaces = false
	// Select lines 0..1 with Shift+Down.
	e.HandleKey(key(tcell.KeyDown, 0, tcell.ModShift))
	e.HandleKey(key(tcell.KeyTab, 0, tcell.ModNone))
	if e.Buffer().Line(0) != "\ta" || e.Buffer().Line(1) != "\tb" {
		t.Fatalf("indent: %q / %q", e.Buffer().Line(0), e.Buffer().Line(1))
	}
	// Outdent with Shift+Tab.
	e.HandleKey(key(tcell.KeyBacktab, 0, tcell.ModNone))
	if e.Buffer().Line(0) != "a" || e.Buffer().Line(1) != "b" {
		t.Fatalf("outdent: %q / %q", e.Buffer().Line(0), e.Buffer().Line(1))
	}
}

func TestOverwriteMode(t *testing.T) {
	e := newTestEditor(t, []string{"abc"}, 40, 10)
	e.HandleKey(key(tcell.KeyInsert, 0, tcell.ModNone)) // toggle overwrite
	if !e.overwrite {
		t.Fatal("expected overwrite mode on")
	}
	typeRunes(e, "X")
	if got := e.Buffer().Line(0); got != "Xbc" {
		t.Fatalf("overwrite type: %q, want %q", got, "Xbc")
	}
}

func TestMouseClickPositionsCursor(t *testing.T) {
	e := newTestEditor(t, []string{"hello world"}, 40, 10)
	e.HandleMouse(tui.MouseEvent{X: 6, Y: 0, Action: tui.MouseDown, Button: tcell.ButtonPrimary})
	if e.curLine != 0 || e.curCol != 6 {
		t.Fatalf("click pos: %d,%d want 0,6", e.curLine, e.curCol)
	}
}

func TestMouseDragSelects(t *testing.T) {
	e := newTestEditor(t, []string{"hello world"}, 40, 10)
	e.HandleMouse(tui.MouseEvent{X: 0, Y: 0, Action: tui.MouseDown, Button: tcell.ButtonPrimary})
	e.HandleMouse(tui.MouseEvent{X: 5, Y: 0, Action: tui.MouseDrag, Button: tcell.ButtonPrimary})
	if !e.edHasSelection() {
		t.Fatal("expected selection after drag")
	}
	if got := e.edSelectedText(); got != "hello" {
		t.Fatalf("drag selected %q, want %q", got, "hello")
	}
	e.HandleMouse(tui.MouseEvent{X: 5, Y: 0, Action: tui.MouseUp, Button: tcell.ButtonPrimary})
	if !e.edHasSelection() {
		t.Fatal("selection should persist after mouse up")
	}
}

func TestGutterShiftsMapping(t *testing.T) {
	e := newTestEditor(t, []string{"hello world"}, 40, 10)
	e.SetLineNumbers(true)
	gw := e.edGutterWidth()
	if gw <= 0 {
		t.Fatal("expected positive gutter width")
	}
	// Force geometry/scroll to settle by rendering once.
	renderToSim(t, e, 40, 10)
	// A click at screen x=gw should map to doc col 0 (start of text).
	e.HandleMouse(tui.MouseEvent{X: gw, Y: 0, Action: tui.MouseDown, Button: tcell.ButtonPrimary})
	if e.curCol != 0 {
		t.Fatalf("click at gutter edge: curCol=%d want 0", e.curCol)
	}
	// A click at screen x=gw+6 should land on doc col 6.
	e.HandleMouse(tui.MouseEvent{X: gw + 6, Y: 0, Action: tui.MouseDown, Button: tcell.ButtonPrimary})
	if e.curCol != 6 {
		t.Fatalf("click with gutter: curCol=%d want 6", e.curCol)
	}
}

// renderToSim draws the editor onto a SimulationScreen-backed surface and
// returns the rune grid; it also exercises the Draw path headlessly.
func renderToSim(t *testing.T, e *Editor, w, h int) {
	t.Helper()
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	scr.SetSize(w, h)
	surf := tui.NewScreenSurface(scr, e.Bounds())
	e.Draw(surf)
	scr.Show()
}

func TestDrawRendersText(t *testing.T) {
	e := newTestEditor(t, []string{"hello"}, 20, 5)
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	scr.SetSize(20, 5)
	surf := tui.NewScreenSurface(scr, e.Bounds())
	e.Draw(surf)
	scr.Show()
	for i, want := range "hello" {
		r, _, _, _ := scr.GetContent(i, 0)
		if r != want {
			t.Fatalf("cell %d = %q, want %q", i, r, want)
		}
	}
}

func TestNilBufferUntitled(t *testing.T) {
	e := NewEditor(nil)
	if e.Buffer() == nil {
		t.Fatal("expected non-nil buffer")
	}
	if !e.Focusable() {
		t.Fatal("editor must be focusable")
	}
}

func TestVScrollbarShownAndScrolls(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "x"
	}
	e := newTestEditor(t, lines, 20, 10)
	renderToSim(t, e, 20, 10)
	shown, col, _, downY, _, _, _, _, _ := e.edVScrollGeom()
	if !shown {
		t.Fatal("expected vertical scrollbar")
	}
	before := e.topLine
	// Click the down arrow.
	e.HandleMouse(tui.MouseEvent{X: col, Y: downY, Action: tui.MouseDown, Button: tcell.ButtonPrimary})
	if e.topLine != before+1 {
		t.Fatalf("down arrow: topLine=%d want %d", e.topLine, before+1)
	}
}
