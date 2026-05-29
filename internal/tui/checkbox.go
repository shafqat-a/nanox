package tui

import (
	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// Checkbox is a VBDOS check box rendered as "[ ] label" (unchecked) or
// "[X] label" (checked) in the dialog body style (black-on-light-gray). When
// focused, the three box cells ("[ ]"/"[X]") are drawn in reverse video so the
// focus target is obvious. Space or Enter toggles it; a mouse press toggles it.
type Checkbox struct {
	BaseWidget
	label    string
	checked  bool
	onChange func(bool)
}

// NewCheckbox returns a check box with the given label and initial state.
func NewCheckbox(label string, checked bool) *Checkbox {
	return &Checkbox{label: label, checked: checked}
}

// IsChecked reports whether the box is checked.
func (c *Checkbox) IsChecked() bool { return c.checked }

// SetChecked sets the checked state without firing the change callback.
func (c *Checkbox) SetChecked(v bool) { c.checked = v }

// SetOnChange registers a callback invoked with the new state on every toggle.
func (c *Checkbox) SetOnChange(fn func(bool)) { c.onChange = fn }

// SetLabel replaces the check box label.
func (c *Checkbox) SetLabel(label string) { c.label = label }

// Label returns the check box label.
func (c *Checkbox) Label() string { return c.label }

// Focusable reports that check boxes accept keyboard focus.
func (c *Checkbox) Focusable() bool { return true }

// PreferredWidth returns the cell width of "[ ] label".
func (c *Checkbox) PreferredWidth() int { return 4 + lblRuneLen(c.label) }

// PreferredSize returns the natural width and height (one row).
func (c *Checkbox) PreferredSize() (int, int) { return c.PreferredWidth(), 1 }

// Draw renders the box and label at the top-left of the bounds.
func (c *Checkbox) Draw(s Surface) {
	b := c.Bounds()
	if b.Empty() {
		return
	}
	body := theme.DialogBody()
	box := body
	if c.focused {
		box = tcell.StyleDefault.Foreground(theme.LGray).Background(theme.Black)
	}
	mark := ' '
	if c.checked {
		mark = 'X'
	}
	x := b.X
	y := b.Y
	// The three box cells in (possibly reversed) box style.
	s.Set(x, y, '[', box)
	s.Set(x+1, y, mark, box)
	s.Set(x+2, y, ']', box)
	// Space + label in normal body style.
	s.Set(x+3, y, ' ', body)
	s.Text(x+4, y, c.label, body)
}

// cbToggle flips the state and fires the change callback.
func (c *Checkbox) cbToggle() {
	c.checked = !c.checked
	if c.onChange != nil {
		c.onChange(c.checked)
	}
}

// HandleKey toggles on Space or Enter.
func (c *Checkbox) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEnter:
		c.cbToggle()
		return true
	case tcell.KeyRune:
		if ev.Rune() == ' ' {
			c.cbToggle()
			return true
		}
	}
	return false
}

// HandleMouse toggles on a press inside the bounds.
func (c *Checkbox) HandleMouse(ev MouseEvent) bool {
	if ev.Action == MouseDown && c.Bounds().Contains(ev.X, ev.Y) {
		c.cbToggle()
		return true
	}
	return false
}
