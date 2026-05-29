package tui

import (
	"github.com/gdamore/tcell/v2"

	"dosedit/internal/theme"
)

// TextBox is a single-line text input. It renders the VBDOS recessed input
// field: white-on-black text bracketed by dark-gray edge glyphs that read as a
// sunken frame, with the caret cell drawn yellow when focused. The widget owns
// a horizontal scroll offset so text longer than the field scrolls.
//
// The bounds are the full widget rectangle; the leftmost and rightmost cells
// are the recessed frame edges, and the interior is the editable field.
type TextBox struct {
	BaseWidget
	runes    []rune // text content
	width    int    // requested field width (interior columns)
	caret    int    // caret index in runes [0,len]
	scroll   int    // first visible rune index
	onChange func(string)
}

// NewTextBox returns a TextBox initialised with text and the given interior
// width (the visible editable columns; the frame edges are drawn outside it).
// width is clamped to a minimum of 1.
func NewTextBox(text string, width int) *TextBox {
	if width < 1 {
		width = 1
	}
	tb := &TextBox{runes: []rune(text), width: width}
	tb.caret = len(tb.runes)
	return tb
}

// Focusable reports that a TextBox can hold keyboard focus.
func (t *TextBox) Focusable() bool { return true }

// Text returns the current contents.
func (t *TextBox) Text() string { return string(t.runes) }

// SetText replaces the contents, clamps the caret to the end, and fires
// onChange.
func (t *TextBox) SetText(s string) {
	t.runes = []rune(s)
	if t.caret > len(t.runes) {
		t.caret = len(t.runes)
	}
	t.tbClampScroll()
	t.tbNotify()
}

// SetOnChange installs a callback invoked whenever the text changes.
func (t *TextBox) SetOnChange(fn func(string)) { t.onChange = fn }

// tbNotify fires the onChange callback if installed.
func (t *TextBox) tbNotify() {
	if t.onChange != nil {
		t.onChange(string(t.runes))
	}
}

// tbInner returns the interior editable rectangle (between the frame edges).
func (t *TextBox) tbInner() Rect {
	b := t.bounds
	if b.W <= 2 {
		return Rect{X: b.X, Y: b.Y, W: 0, H: b.H}
	}
	return Rect{X: b.X + 1, Y: b.Y, W: b.W - 2, H: b.H}
}

// tbClampScroll keeps the caret visible within the interior width.
func (t *TextBox) tbClampScroll() {
	w := t.tbInner().W
	if w <= 0 {
		t.scroll = 0
		return
	}
	if t.caret < t.scroll {
		t.scroll = t.caret
	}
	if t.caret > t.scroll+w-1 {
		t.scroll = t.caret - w + 1
	}
	if t.scroll < 0 {
		t.scroll = 0
	}
}

// Draw renders the recessed frame edges and the visible slice of text with the
// caret highlighted when focused.
func (t *TextBox) Draw(surf Surface) {
	b := t.bounds
	if b.Empty() {
		return
	}
	field := theme.InputField()
	edge := tcell.StyleDefault.Foreground(theme.DGray).Background(theme.Black)
	y := b.Y

	// Recessed frame edges read the field as sunken.
	if b.W >= 1 {
		surf.Set(b.X, y, theme.VSingle, edge)
	}
	if b.W >= 2 {
		surf.Set(b.X+b.W-1, y, theme.VSingle, edge)
	}

	inner := t.tbInner()
	if inner.W <= 0 {
		return
	}
	t.tbClampScroll()

	// Background fill for the interior.
	surf.Fill(inner, ' ', field)

	for col := 0; col < inner.W; col++ {
		idx := t.scroll + col
		r := ' '
		if idx < len(t.runes) {
			r = t.runes[idx]
		}
		st := field
		if t.focused && idx == t.caret {
			st = field.Foreground(theme.Yellow).Reverse(true)
		}
		surf.Set(inner.X+col, y, r, st)
	}
}

// HandleKey edits the text: printable insert, Backspace, Delete, arrows, Home,
// End. Returns true when consumed.
func (t *TextBox) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyRune:
		t.tbInsert(ev.Rune())
		return true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if t.caret > 0 {
			t.runes = append(t.runes[:t.caret-1], t.runes[t.caret:]...)
			t.caret--
			t.tbClampScroll()
			t.tbNotify()
		}
		return true
	case tcell.KeyDelete:
		if t.caret < len(t.runes) {
			t.runes = append(t.runes[:t.caret], t.runes[t.caret+1:]...)
			t.tbNotify()
		}
		return true
	case tcell.KeyLeft:
		if t.caret > 0 {
			t.caret--
			t.tbClampScroll()
		}
		return true
	case tcell.KeyRight:
		if t.caret < len(t.runes) {
			t.caret++
			t.tbClampScroll()
		}
		return true
	case tcell.KeyHome:
		t.caret = 0
		t.tbClampScroll()
		return true
	case tcell.KeyEnd:
		t.caret = len(t.runes)
		t.tbClampScroll()
		return true
	}
	return false
}

// tbInsert inserts r at the caret and advances it.
func (t *TextBox) tbInsert(r rune) {
	t.runes = append(t.runes, 0)
	copy(t.runes[t.caret+1:], t.runes[t.caret:])
	t.runes[t.caret] = r
	t.caret++
	t.tbClampScroll()
	t.tbNotify()
}

// HandleMouse focuses (handled by App) and positions the caret on MouseDown.
func (t *TextBox) HandleMouse(ev MouseEvent) bool {
	if ev.Action != MouseDown {
		return false
	}
	inner := t.tbInner()
	if !t.bounds.Contains(ev.X, ev.Y) {
		return false
	}
	col := ev.X - inner.X
	if col < 0 {
		col = 0
	}
	idx := t.scroll + col
	if idx > len(t.runes) {
		idx = len(t.runes)
	}
	t.caret = idx
	t.tbClampScroll()
	return true
}
