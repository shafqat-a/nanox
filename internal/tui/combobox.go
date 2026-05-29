package tui

import (
	"github.com/gdamore/tcell/v2"

	"dosedit/internal/theme"
)

// ComboBox is a collapsed drop-down selector. Collapsed it renders an
// input-like field (white-on-black, like a TextBox) showing the selected item
// plus a ▼ drop button on the right. Opening (Enter/Space/Down/click the ▼)
// pushes a full-screen modal popup overlay (cmbPopup) that draws a bordered
// list at the field's position; the popup routes all mouse input. A click on an
// item selects and closes; a click outside closes without change; Esc closes;
// Up/Down/Enter navigate and select. If no App is set, opening is a no-op.
type ComboBox struct {
	BaseWidget
	items    []string
	selected int
	app      *App
	popup    *cmbPopup
	onChange func(int)
}

// NewComboBox returns a ComboBox over items, selecting the first if any.
func NewComboBox(items []string) *ComboBox {
	cmb := &ComboBox{items: items, selected: 0}
	if len(items) == 0 {
		cmb.selected = -1
	}
	return cmb
}

// Focusable reports that a ComboBox can hold keyboard focus.
func (cmb *ComboBox) Focusable() bool { return true }

// SetApp wires the App used to host the popup overlay.
func (cmb *ComboBox) SetApp(a *App) { cmb.app = a }

// SetItems replaces the items, clamping the selection.
func (cmb *ComboBox) SetItems(items []string) {
	cmb.items = items
	if cmb.selected >= len(items) {
		cmb.selected = len(items) - 1
	}
	if cmb.selected < 0 && len(items) > 0 {
		cmb.selected = 0
	}
}

// Selected returns the selected index, or -1 when empty.
func (cmb *ComboBox) Selected() int { return cmb.selected }

// SelectedText returns the selected item's text, or "" when empty.
func (cmb *ComboBox) SelectedText() string {
	if cmb.selected < 0 || cmb.selected >= len(cmb.items) {
		return ""
	}
	return cmb.items[cmb.selected]
}

// SetOnChange installs a callback fired when the selection changes.
func (cmb *ComboBox) SetOnChange(fn func(int)) { cmb.onChange = fn }

// cmbSetSelected updates the selection and fires onChange when it changes.
func (cmb *ComboBox) cmbSetSelected(i int) {
	if i < 0 || i >= len(cmb.items) || i == cmb.selected {
		return
	}
	cmb.selected = i
	if cmb.onChange != nil {
		cmb.onChange(i)
	}
}

// Draw renders the collapsed field and the ▼ drop button.
func (cmb *ComboBox) Draw(surf Surface) {
	b := cmb.bounds
	if b.Empty() {
		return
	}
	field := theme.InputField()
	surf.Fill(b, ' ', field)

	textW := b.W - 1
	if textW < 0 {
		textW = 0
	}
	if textW > 0 {
		surf.Text(b.X, b.Y, lbFit(cmb.SelectedText(), textW), field)
	}
	// Drop button cell on the right.
	btnStyle := field
	if cmb.focused {
		btnStyle = field.Foreground(theme.Yellow).Reverse(true)
	}
	surf.Set(b.X+b.W-1, b.Y, theme.SbDown, btnStyle)
}

// HandleKey opens the popup on Enter/Space/Down. Returns true when consumed.
func (cmb *ComboBox) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEnter, tcell.KeyDown:
		cmb.cmbOpen()
		return true
	case tcell.KeyRune:
		if ev.Rune() == ' ' {
			cmb.cmbOpen()
			return true
		}
	}
	return false
}

// HandleMouse opens the popup on a click anywhere in the field.
func (cmb *ComboBox) HandleMouse(ev MouseEvent) bool {
	if ev.Action != MouseDown {
		return false
	}
	if !cmb.bounds.Contains(ev.X, ev.Y) {
		return false
	}
	cmb.cmbOpen()
	return true
}

// cmbOpen pushes the modal popup overlay. No-op when no App is set or the list
// is empty.
func (cmb *ComboBox) cmbOpen() {
	if cmb.app == nil || len(cmb.items) == 0 {
		return
	}
	cmb.popup = newCmbPopup(cmb)
	cmb.app.PushModal(cmb.popup)
}

// cmbClose pops the popup overlay if it is the top modal.
func (cmb *ComboBox) cmbClose() {
	if cmb.app != nil && cmb.popup != nil {
		cmb.app.PopModal()
		cmb.popup = nil
	}
}

// cmbPopup is the full-screen modal overlay that hosts the open dropdown list.
// It is full-screen so every click reaches it (true modality): clicks on a list
// row select+close; clicks elsewhere close without change.
type cmbPopup struct {
	BaseContainer
	owner   *ComboBox
	listRc  Rect // bordered list rectangle (absolute)
	list    *ListBox
	maxRows int
}

// newCmbPopup builds the popup, seeding the inner ListBox with the owner's
// items and current selection.
func newCmbPopup(owner *ComboBox) *cmbPopup {
	p := &cmbPopup{owner: owner, maxRows: 8}
	p.list = NewListBox(owner.items)
	if owner.selected >= 0 {
		p.list.SetSelected(owner.selected)
	}
	p.Add(p.list)
	return p
}

// cmbLayout computes the bordered list rectangle just below the owner field
// and assigns the inner ListBox bounds (inside the single-line border).
func (p *cmbPopup) cmbLayout() {
	fb := p.owner.bounds
	rows := len(p.owner.items)
	if rows > p.maxRows {
		rows = p.maxRows
	}
	if rows < 1 {
		rows = 1
	}
	w := fb.W
	if w < 3 {
		w = 3
	}
	x := fb.X
	y := fb.Y + 1
	// Clamp vertically within the screen.
	scr := p.bounds // full-screen bounds set by App
	if !scr.Empty() {
		if y+rows+2 > scr.Y+scr.H {
			// Open upward if it would overflow the bottom.
			y = fb.Y - (rows + 2)
			if y < scr.Y {
				y = scr.Y
			}
		}
		if x+w > scr.X+scr.W {
			x = scr.X + scr.W - w
		}
		if x < scr.X {
			x = scr.X
		}
	}
	p.listRc = Rect{X: x, Y: y, W: w, H: rows + 2}
	p.list.SetBounds(p.listRc.Inset(1, 1))
}

// Draw paints the single-line border and the inner list.
func (p *cmbPopup) Draw(surf Surface) {
	p.cmbLayout()
	rc := p.listRc
	frame := theme.ListBox()
	// Border.
	surf.Set(rc.X, rc.Y, theme.TLSingle, frame)
	surf.Set(rc.X+rc.W-1, rc.Y, theme.TRSingle, frame)
	surf.Set(rc.X, rc.Y+rc.H-1, theme.BLSingle, frame)
	surf.Set(rc.X+rc.W-1, rc.Y+rc.H-1, theme.BRSingle, frame)
	for x := rc.X + 1; x < rc.X+rc.W-1; x++ {
		surf.Set(x, rc.Y, theme.HSingle, frame)
		surf.Set(x, rc.Y+rc.H-1, theme.HSingle, frame)
	}
	for y := rc.Y + 1; y < rc.Y+rc.H-1; y++ {
		surf.Set(rc.X, y, theme.VSingle, frame)
		surf.Set(rc.X+rc.W-1, y, theme.VSingle, frame)
	}
	p.list.Draw(surf)
}

// HandleKey routes navigation to the list; Enter selects+closes, Esc closes.
func (p *cmbPopup) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEscape:
		p.owner.cmbClose()
		return true
	case tcell.KeyEnter:
		p.owner.cmbSetSelected(p.list.Selected())
		p.owner.cmbClose()
		return true
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyPgUp, tcell.KeyPgDn,
		tcell.KeyHome, tcell.KeyEnd:
		return p.list.HandleKey(ev)
	}
	return false
}

// HandleMouse: a click inside the list selects+closes; a click outside closes
// without changing the selection. Wheel events scroll the list.
func (p *cmbPopup) HandleMouse(ev MouseEvent) bool {
	switch ev.Action {
	case WheelUp, WheelDown:
		return p.list.HandleMouse(ev)
	case MouseDown:
		if p.list.Bounds().Contains(ev.X, ev.Y) {
			row := ev.Y - p.list.Bounds().Y
			idx := p.list.top + row
			if idx >= 0 && idx < len(p.owner.items) {
				p.owner.cmbSetSelected(idx)
			}
			p.owner.cmbClose()
			return true
		}
		// Click outside the list: close without change.
		p.owner.cmbClose()
		return true
	}
	return false
}
