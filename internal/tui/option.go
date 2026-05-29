package tui

import (
	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// Option is a single radio item. It renders "( ) label" when unselected and
// "(•) label" when selected, in the dialog body style. Options are owned by an
// OptionGroup, which is the actual focus stop; an individual Option is not
// focusable on its own.
type Option struct {
	BaseWidget
	label    string
	selected bool
}

// NewOption returns an option item with the given label.
func NewOption(label string) *Option {
	return &Option{label: label}
}

// Label returns the option's label.
func (o *Option) Label() string { return o.label }

// IsSelected reports whether this option is the selected one in its group.
func (o *Option) IsSelected() bool { return o.selected }

// PreferredWidth returns the cell width of "( ) label".
func (o *Option) PreferredWidth() int { return 4 + lblRuneLen(o.label) }

// Draw renders the option marker and label. reverse draws the selected marker
// in reverse video (used by the group when it is focused).
func (o *Option) Draw(s Surface, reverse bool) {
	b := o.Bounds()
	if b.Empty() {
		return
	}
	body := theme.DialogBody()
	mark := body
	if o.selected && reverse {
		mark = tcell.StyleDefault.Foreground(theme.LGray).Background(theme.Black)
	}
	glyph := ' '
	if o.selected {
		glyph = '•'
	}
	x := b.X
	y := b.Y
	s.Set(x, y, '(', mark)
	s.Set(x+1, y, glyph, mark)
	s.Set(x+2, y, ')', mark)
	s.Set(x+3, y, ' ', body)
	s.Text(x+4, y, o.label, body)
}

// OptionGroup is a focusable, container-like widget that owns a vertical stack
// of radio Options and acts as a SINGLE focus stop. Exactly one option is
// selected. When focused, Up/Left move the selection toward the first item and
// Down/Right move it toward the last; a left-click selects the clicked option.
// The selected marker is drawn in reverse video while the group is focused.
type OptionGroup struct {
	BaseWidget
	options  []*Option
	selected int
	onChange func(int)
}

// NewOptionGroup builds an option group with one Option per label; the first
// option is selected by default.
func NewOptionGroup(labels []string) *OptionGroup {
	g := &OptionGroup{}
	for _, lab := range labels {
		g.options = append(g.options, NewOption(lab))
	}
	if len(g.options) > 0 {
		g.options[0].selected = true
		g.selected = 0
	}
	return g
}

// Selected returns the index of the selected option, or -1 if the group is
// empty.
func (g *OptionGroup) Selected() int {
	if len(g.options) == 0 {
		return -1
	}
	return g.selected
}

// SetSelected selects the option at index i (clamped to range) without firing
// the change callback.
func (g *OptionGroup) SetSelected(i int) {
	if len(g.options) == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(g.options) {
		i = len(g.options) - 1
	}
	g.optSet(i, false)
}

// SelectedLabel returns the label of the selected option, or "" if empty.
func (g *OptionGroup) SelectedLabel() string {
	if g.selected < 0 || g.selected >= len(g.options) {
		return ""
	}
	return g.options[g.selected].label
}

// SetOnChange registers a callback invoked with the new index whenever the
// selection changes via keyboard or mouse.
func (g *OptionGroup) SetOnChange(fn func(int)) { g.onChange = fn }

// Options returns the group's option items.
func (g *OptionGroup) Options() []*Option { return g.options }

// Focusable reports that the group is a focus stop.
func (g *OptionGroup) Focusable() bool { return true }

// PreferredHeight returns the number of option rows.
func (g *OptionGroup) PreferredHeight() int { return len(g.options) }

// PreferredWidth returns the widest option's cell width.
func (g *OptionGroup) PreferredWidth() int {
	w := 0
	for _, o := range g.options {
		if pw := o.PreferredWidth(); pw > w {
			w = pw
		}
	}
	return w
}

// PreferredSize returns the group's natural width and height.
func (g *OptionGroup) PreferredSize() (int, int) {
	return g.PreferredWidth(), g.PreferredHeight()
}

// SetBounds positions the group and lays its options out vertically, one per
// row from the top of the group's rectangle.
func (g *OptionGroup) SetBounds(r Rect) {
	g.BaseWidget.SetBounds(r)
	for i, o := range g.options {
		o.SetBounds(Rect{X: r.X, Y: r.Y + i, W: r.W, H: 1})
	}
}

// optSet selects index i, updates option flags, and optionally fires onChange.
func (g *OptionGroup) optSet(i int, fire bool) {
	if i < 0 || i >= len(g.options) {
		return
	}
	changed := i != g.selected
	for j, o := range g.options {
		o.selected = j == i
	}
	g.selected = i
	if fire && changed && g.onChange != nil {
		g.onChange(i)
	}
}

// Draw renders each owned option, reversing the selected marker when focused.
func (g *OptionGroup) Draw(s Surface) {
	for _, o := range g.options {
		o.Draw(s, g.focused)
	}
}

// HandleKey moves the selection with the arrow keys while focused.
func (g *OptionGroup) HandleKey(ev *tcell.EventKey) bool {
	if len(g.options) == 0 {
		return false
	}
	switch ev.Key() {
	case tcell.KeyUp, tcell.KeyLeft:
		if g.selected > 0 {
			g.optSet(g.selected-1, true)
		}
		return true
	case tcell.KeyDown, tcell.KeyRight:
		if g.selected < len(g.options)-1 {
			g.optSet(g.selected+1, true)
		}
		return true
	}
	return false
}

// HandleMouse selects the option under a left-button press.
func (g *OptionGroup) HandleMouse(ev MouseEvent) bool {
	if ev.Action != MouseDown {
		return false
	}
	for i, o := range g.options {
		if o.Bounds().Contains(ev.X, ev.Y) {
			g.optSet(i, true)
			return true
		}
	}
	return false
}
