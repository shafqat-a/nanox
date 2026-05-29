// This file implements DOSEdit's dialog control primitives, styled to match
// Visual Basic for DOS 1.0 / QuickBASIC 4.5 (spec §4.3 / §4.5 / §6.6):
//
//   - Button   — a 3-D raised push button: black text on a Light-Gray face,
//     the label padded with a space on each side, a one-cell SOLID
//     BLACK drop shadow on the right edge and the bottom edge (the
//     L-shape from the Open-dialog screenshot). The DEFAULT and/or
//     FOCUSED button additionally gets a single-line black outline
//     drawn around the face, exactly as "OK" is outlined in the shot.
//   - Checkbox — "[ ] Label" / "[X] Label", black-on-light-gray; the "[ ]" box
//     is shown reverse-video when the control is focused. Space /
//     Enter / left-click toggle it.
//   - RadioGroup — "( ) Label" / "(•) Label" options where exactly one is
//     selected; Up/Down and clicks move the selection; the focused
//     option's marker is shown reverse-video.
//
// Each control embeds tview.Box (so GetRect/SetRect/Focus/Blur/HasFocus come
// for free) and overrides Draw/InputHandler/MouseHandler. All colours and
// glyphs come from package theme — nothing is hard-coded. Every unexported
// identifier is prefixed ctl to avoid collisions with sibling files.
package ui

import (
	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ----------------------------------------------------------------------------
// Shared drawing helpers
// ----------------------------------------------------------------------------

// ctlSetString draws s at (x,y) with style, clipping to maxW cells, and returns
// the number of cells written. It never draws past the row.
func ctlSetString(screen tcell.Screen, x, y, maxW int, style tcell.Style, s string) int {
	if maxW <= 0 {
		return 0
	}
	n := 0
	for _, r := range s {
		if n >= maxW {
			break
		}
		screen.SetContent(x+n, y, r, nil, style)
		n++
	}
	return n
}

// ----------------------------------------------------------------------------
// Button — 3-D raised push button with black drop shadow + default outline
// ----------------------------------------------------------------------------

// Button is a VB-for-DOS command button. It reserves one column on the right and
// one row on the bottom of its rect for the solid-black drop shadow, so the face
// is (width-1) x (height-1). The default/focused button additionally draws a
// single-line black border inside the face.
type Button struct {
	*tview.Box

	label    string
	action   func()
	isDefAlt bool // marked as the dialog's default button (Enter target)
}

// NewButton creates a command button with the given label and activation action.
func NewButton(label string, action func()) *Button {
	b := &Button{
		Box:    tview.NewBox(),
		label:  label,
		action: action,
	}
	b.Box.SetBackgroundColor(theme.LGray)
	return b
}

// SetLabel updates the button's label.
func (b *Button) SetLabel(label string) *Button { b.label = label; return b }

// GetLabel returns the button's label.
func (b *Button) GetLabel() string { return b.label }

// SetAction sets the function fired on Enter/Space/click.
func (b *Button) SetAction(fn func()) *Button { b.action = fn; return b }

// SetDefaultButton marks (or unmarks) this button as the dialog default, which
// draws the black outline even when the button is not focused.
func (b *Button) SetDefaultButton(def bool) *Button { b.isDefAlt = def; return b }

// IsDefaultButton reports whether this button is marked as the dialog default.
func (b *Button) IsDefaultButton() bool { return b.isDefAlt }

// Fire invokes the button's action (used by the Dialog for Enter on the default).
func (b *Button) Fire() {
	if b.action != nil {
		b.action()
	}
}

// faceLabel returns the label padded with one space on each side.
func (b *Button) faceLabel() string { return " " + b.label + " " }

// Draw paints the 3-D face, the right/bottom black shadow and, for the
// default/focused button, the black outline around the face.
func (b *Button) Draw(screen tcell.Screen) {
	b.Box.DrawForSubclass(screen, b)

	x, y, w, h := b.GetRect()
	if w < 2 || h < 1 {
		return
	}

	// Reserve the last column + last row for the drop shadow.
	faceW := w - 1
	faceH := h - 1
	if faceH < 1 {
		faceH = 1
	}
	faceRight := x + faceW - 1
	faceBottom := y + faceH - 1

	face := theme.ButtonFace()
	shadow := theme.Shadow()

	// Fill the face.
	for r := y; r <= faceBottom; r++ {
		for c := x; c <= faceRight; c++ {
			screen.SetContent(c, r, ' ', nil, face)
		}
	}

	outlined := b.isDefAlt || b.HasFocus()

	// Centre the label on the middle face row.
	midRow := y + faceH/2
	lbl := b.faceLabel()
	avail := faceW
	if outlined {
		avail = faceW - 2 // leave room for the outline columns
	}
	if avail < 0 {
		avail = 0
	}
	rlbl := []rune(lbl)
	if len(rlbl) > avail {
		rlbl = rlbl[:avail]
	}
	startX := x + (faceW-len(rlbl))/2
	labelStyle := face
	if b.HasFocus() {
		// Focused button: reverse the face text so it is unmistakable.
		labelStyle = tcell.StyleDefault.Foreground(theme.White).Background(theme.Black)
		for c := x; c <= faceRight; c++ {
			screen.SetContent(c, midRow, ' ', nil, labelStyle)
		}
	}
	ctlSetString(screen, startX, midRow, len(rlbl), labelStyle, string(rlbl))

	// Default / focused button: single-line black outline around the face.
	if outlined {
		ctlDrawOutline(screen, x, y, faceRight, faceBottom, theme.Black, b.faceBg())
	}

	// Solid-black drop shadow: right column (offset down by one) + bottom row
	// (offset right by one), forming the L-shape.
	for r := y + 1; r <= faceBottom+1; r++ {
		screen.SetContent(faceRight+1, r, ' ', nil, shadow)
	}
	for c := x + 1; c <= faceRight+1; c++ {
		screen.SetContent(c, faceBottom+1, ' ', nil, shadow)
	}
}

// faceBg returns the background colour currently used on the face (black under
// the focused label row, light gray otherwise) so the outline blends correctly.
func (b *Button) faceBg() tcell.Color { return theme.LGray }

// ctlDrawOutline draws a single-line frame in fg-on-bg between the given face
// corners (drawn over the face edge cells).
func ctlDrawOutline(screen tcell.Screen, x, y, right, bottom int, fg, bg tcell.Color) {
	style := tcell.StyleDefault.Foreground(fg).Background(bg)
	if right <= x || bottom < y {
		return
	}
	if bottom == y {
		// Single-row face: just bracket the label.
		screen.SetContent(x, y, theme.VSingle, nil, style)
		screen.SetContent(right, y, theme.VSingle, nil, style)
		return
	}
	screen.SetContent(x, y, theme.TLSingle, nil, style)
	screen.SetContent(right, y, theme.TRSingle, nil, style)
	screen.SetContent(x, bottom, theme.BLSingle, nil, style)
	screen.SetContent(right, bottom, theme.BRSingle, nil, style)
	for c := x + 1; c < right; c++ {
		screen.SetContent(c, y, theme.HSingle, nil, style)
		screen.SetContent(c, bottom, theme.HSingle, nil, style)
	}
	for r := y + 1; r < bottom; r++ {
		screen.SetContent(x, r, theme.VSingle, nil, style)
		screen.SetContent(right, r, theme.VSingle, nil, style)
	}
}

// InputHandler fires the action on Enter or Space.
func (b *Button) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return b.WrapInputHandler(func(event *tcell.EventKey, _ func(p tview.Primitive)) {
		switch {
		case event.Key() == tcell.KeyEnter,
			event.Key() == tcell.KeyRune && event.Rune() == ' ':
			b.Fire()
		}
	})
}

// MouseHandler fires the action on a left click inside the face.
func (b *Button) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return b.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		x, y, w, h := b.GetRect()
		mx, my := event.Position()
		if mx < x || mx >= x+w-1 || my < y || my >= y+dlgMaxInt(h-1, 1) {
			return false, nil
		}
		if action == tview.MouseLeftClick {
			setFocus(b)
			b.Fire()
			return true, nil
		}
		if action == tview.MouseLeftDown {
			setFocus(b)
			return true, nil
		}
		return false, nil
	})
}

// ----------------------------------------------------------------------------
// Checkbox — "[ ] Label" / "[X] Label"
// ----------------------------------------------------------------------------

// Checkbox is a VB-for-DOS check box. The "[ ]" box is shown reverse-video when
// the control has focus. Space/Enter/click toggle the checked state.
type Checkbox struct {
	*tview.Box

	label   string
	checked bool
	onCheck func(checked bool)
}

// NewCheckbox creates a checkbox with the given label and initial state.
func NewCheckbox(label string, checked bool) *Checkbox {
	cb := &Checkbox{
		Box:     tview.NewBox(),
		label:   label,
		checked: checked,
	}
	cb.Box.SetBackgroundColor(theme.LGray)
	return cb
}

// SetLabel sets the checkbox label.
func (cb *Checkbox) SetLabel(label string) *Checkbox { cb.label = label; return cb }

// GetLabel returns the checkbox label.
func (cb *Checkbox) GetLabel() string { return cb.label }

// IsChecked reports whether the box is checked.
func (cb *Checkbox) IsChecked() bool { return cb.checked }

// SetChecked sets the checked state.
func (cb *Checkbox) SetChecked(v bool) *Checkbox { cb.checked = v; return cb }

// SetChangedFunc registers a callback fired whenever the state toggles.
func (cb *Checkbox) SetChangedFunc(fn func(checked bool)) *Checkbox { cb.onCheck = fn; return cb }

// toggle flips the checked state and fires the change callback.
func (cb *Checkbox) toggle() {
	cb.checked = !cb.checked
	if cb.onCheck != nil {
		cb.onCheck(cb.checked)
	}
}

// Draw paints "[ ] Label" / "[X] Label", reversing the box when focused.
func (cb *Checkbox) Draw(screen tcell.Screen) {
	cb.Box.DrawForSubclass(screen, cb)

	x, y, w, _ := cb.GetRect()
	if w <= 0 {
		return
	}
	body := theme.DialogBody()
	boxStyle := body
	if cb.HasFocus() {
		boxStyle = tcell.StyleDefault.Foreground(theme.White).Background(theme.Black)
	}
	mark := ' '
	if cb.checked {
		mark = 'X'
	}
	// "[ ]" — bracket cells + mark cell, then a space + the label.
	col := x
	col += ctlSetString(screen, col, y, x+w-col, body, "[")
	col += ctlSetString(screen, col, y, x+w-col, boxStyle, string(mark))
	col += ctlSetString(screen, col, y, x+w-col, body, "] ")
	ctlSetString(screen, col, y, x+w-col, body, cb.label)
}

// InputHandler toggles on Space or Enter.
func (cb *Checkbox) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return cb.WrapInputHandler(func(event *tcell.EventKey, _ func(p tview.Primitive)) {
		switch {
		case event.Key() == tcell.KeyEnter,
			event.Key() == tcell.KeyRune && event.Rune() == ' ':
			cb.toggle()
		}
	})
}

// MouseHandler toggles on a left click on the control row.
func (cb *Checkbox) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return cb.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		x, y, w, h := cb.GetRect()
		mx, my := event.Position()
		if mx < x || mx >= x+w || my < y || my >= y+dlgMaxInt(h, 1) {
			return false, nil
		}
		if action == tview.MouseLeftClick {
			setFocus(cb)
			cb.toggle()
			return true, nil
		}
		if action == tview.MouseLeftDown {
			setFocus(cb)
			return true, nil
		}
		return false, nil
	})
}

// ----------------------------------------------------------------------------
// RadioGroup — "( ) Label" / "(•) Label", exactly one selected
// ----------------------------------------------------------------------------

// RadioGroup is a vertical group of option (radio) buttons of which exactly one
// is selected. Up/Down (and clicks) move the selection within the group; the
// focused option's marker is shown reverse-video.
type RadioGroup struct {
	*tview.Box

	options  []string
	selected int
	onChange func(idx int)
}

// NewRadioGroup creates a radio group from the given option labels with the
// initial selection clamped into range.
func NewRadioGroup(options []string, selected int) *RadioGroup {
	rg := &RadioGroup{
		Box:     tview.NewBox(),
		options: append([]string(nil), options...),
	}
	rg.Box.SetBackgroundColor(theme.LGray)
	rg.setSelected(selected)
	return rg
}

// Selected returns the index of the currently selected option (-1 if empty).
func (rg *RadioGroup) Selected() int {
	if len(rg.options) == 0 {
		return -1
	}
	return rg.selected
}

// SelectedLabel returns the label of the selected option ("" if empty).
func (rg *RadioGroup) SelectedLabel() string {
	if rg.selected < 0 || rg.selected >= len(rg.options) {
		return ""
	}
	return rg.options[rg.selected]
}

// SetChangedFunc registers a callback fired when the selection changes.
func (rg *RadioGroup) SetChangedFunc(fn func(idx int)) *RadioGroup { rg.onChange = fn; return rg }

// OptionCount returns the number of options (== the group's height in rows).
func (rg *RadioGroup) OptionCount() int { return len(rg.options) }

// setSelected clamps and stores idx, firing the change callback when it moves.
func (rg *RadioGroup) setSelected(idx int) {
	if len(rg.options) == 0 {
		rg.selected = 0
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(rg.options) {
		idx = len(rg.options) - 1
	}
	if idx != rg.selected {
		rg.selected = idx
		if rg.onChange != nil {
			rg.onChange(idx)
		}
	} else {
		rg.selected = idx
	}
}

// Draw paints one "( ) Label" / "(•) Label" per row, reversing the marker of the
// focused (selected) option when the group has focus.
func (rg *RadioGroup) Draw(screen tcell.Screen) {
	rg.Box.DrawForSubclass(screen, rg)

	x, y, w, h := rg.GetRect()
	if w <= 0 || h <= 0 {
		return
	}
	body := theme.DialogBody()
	for i, opt := range rg.options {
		if i >= h {
			break
		}
		row := y + i
		marker := ' '
		if i == rg.selected {
			marker = '•'
		}
		markStyle := body
		if rg.HasFocus() && i == rg.selected {
			markStyle = tcell.StyleDefault.Foreground(theme.White).Background(theme.Black)
		}
		col := x
		col += ctlSetString(screen, col, row, x+w-col, body, "(")
		col += ctlSetString(screen, col, row, x+w-col, markStyle, string(marker))
		col += ctlSetString(screen, col, row, x+w-col, body, ") ")
		ctlSetString(screen, col, row, x+w-col, body, opt)
	}
}

// InputHandler moves the selection with Up/Down (and j/k); Space/Enter are no-ops
// because the selection itself is the value.
func (rg *RadioGroup) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return rg.WrapInputHandler(func(event *tcell.EventKey, _ func(p tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyUp:
			rg.setSelected(rg.selected - 1)
		case tcell.KeyDown:
			rg.setSelected(rg.selected + 1)
		case tcell.KeyRune:
			switch event.Rune() {
			case 'k':
				rg.setSelected(rg.selected - 1)
			case 'j':
				rg.setSelected(rg.selected + 1)
			}
		}
	})
}

// MouseHandler selects the clicked option row.
func (rg *RadioGroup) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return rg.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		x, y, w, h := rg.GetRect()
		mx, my := event.Position()
		if mx < x || mx >= x+w || my < y || my >= y+h {
			return false, nil
		}
		if action == tview.MouseLeftClick || action == tview.MouseLeftDown {
			setFocus(rg)
			rg.setSelected(my - y)
			return true, nil
		}
		return false, nil
	})
}
