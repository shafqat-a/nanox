package tui

import (
	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// Dialog is a modal container pushed onto the App via PushModal. It draws a
// double-line frame with a centered magenta/white title, a light-gray body, and
// a one-cell solid-black drop shadow on the right and bottom edges. Body
// controls are stacked vertically on the left; action buttons are placed in a
// vertical column on the right. Tab/Shift+Tab focus traversal is handled by the
// App across FocusableDescendants; the Dialog only routes Enter to the default
// button and Esc to the cancel function.
type Dialog struct {
	BaseContainer
	title   string
	body    []Widget
	buttons []Widget

	def    Widget // default button (Enter)
	cancel func() // Esc handler

	w, h     int // explicit content size (frame included); 0 ⇒ AutoSize at layout
	autoSize bool
}

// dlgBtnCol is the width reserved for the right-hand button column.
const (
	dlgBtnCol  = 12
	dlgPadX    = 2
	dlgPadY    = 1
	dlgMinW    = 20
	dlgMinH    = 6
	dlgBtnRowH = 1
)

// NewDialog returns a modal Dialog with the given title. It defaults to
// auto-sizing to its content until SetSize is called.
func NewDialog(title string) *Dialog {
	return &Dialog{title: title, autoSize: true}
}

// Title returns the dialog title.
func (d *Dialog) Title() string { return d.title }

// Add adds a body control, stacked vertically in the body area.
func (d *Dialog) Add(w Widget) {
	d.body = append(d.body, w)
	d.BaseContainer.Add(w)
}

// AddButton adds a button to the right-hand button column. The first button
// added becomes the default unless SetDefault overrides it.
func (d *Dialog) AddButton(b Widget) {
	d.buttons = append(d.buttons, b)
	d.BaseContainer.Add(b)
	if d.def == nil {
		d.def = b
	}
}

// SetDefault marks b as the default button (activated on Enter).
func (d *Dialog) SetDefault(b Widget) { d.def = b }

// SetCancel registers the function invoked when Esc is pressed.
func (d *Dialog) SetCancel(fn func()) { d.cancel = fn }

// SetSize sets an explicit dialog size (frame included) and disables auto-size.
func (d *Dialog) SetSize(w, h int) {
	if w < dlgMinW {
		w = dlgMinW
	}
	if h < dlgMinH {
		h = dlgMinH
	}
	d.w, d.h = w, h
	d.autoSize = false
}

// Size returns the current dialog size, computing the auto-size if needed.
func (d *Dialog) Size() (w, h int) {
	if d.autoSize || d.w == 0 || d.h == 0 {
		return d.computeSize()
	}
	return d.w, d.h
}

// AutoSize enables automatic sizing from the current content.
func (d *Dialog) AutoSize() { d.autoSize = true }

// computeSize derives a size large enough for the stacked body and the button
// column.
func (d *Dialog) computeSize() (int, int) {
	bodyW := 0
	for _, c := range d.body {
		if cw := c.Bounds().W; cw > bodyW {
			bodyW = cw
		}
	}
	if bodyW < dlgMinW-2*dlgPadX-dlgBtnCol {
		bodyW = dlgMinW - 2*dlgPadX - dlgBtnCol
	}
	w := 1 + dlgPadX + bodyW + dlgBtnCol + 1
	if w < dlgMinW {
		w = dlgMinW
	}

	bodyRows := len(d.body) + dlgPadY
	btnRows := len(d.buttons) + dlgPadY
	rows := bodyRows
	if btnRows > rows {
		rows = btnRows
	}
	h := 1 + dlgPadY + rows + 1
	if h < dlgMinH {
		h = dlgMinH
	}
	return w, h
}

// SetBounds stores the dialog rect and lays out body controls and buttons.
func (d *Dialog) SetBounds(r Rect) {
	d.BaseWidget.SetBounds(r)
	d.layout()
}

// frameRect returns the rect occupied by the dialog frame (excluding the shadow,
// which is drawn one cell to the right and below).
func (d *Dialog) frameRect() Rect {
	b := d.Bounds()
	w, h := d.Size()
	// The App gives the dialog full-screen bounds; we center our own size
	// within that. If bounds are already our size, center is a no-op.
	x := b.X + (b.W-w-1)/2
	y := b.Y + (b.H-h-1)/2
	if x < b.X {
		x = b.X
	}
	if y < b.Y {
		y = b.Y
	}
	return Rect{X: x, Y: y, W: w, H: h}
}

// layout positions body controls (left column) and buttons (right column)
// inside the frame's content area.
func (d *Dialog) layout() {
	fr := d.frameRect()
	if fr.Empty() {
		return
	}
	contentX := fr.X + 1 + dlgPadX
	contentY := fr.Y + 1 + dlgPadY
	bodyW := fr.W - 2 - dlgPadX - dlgBtnCol
	if bodyW < 1 {
		bodyW = 1
	}

	y := contentY
	for _, c := range d.body {
		ch := c.Bounds().H
		if ch < 1 {
			ch = 1
		}
		c.SetBounds(Rect{X: contentX, Y: y, W: bodyW, H: ch})
		y += ch
	}

	btnX := fr.X + fr.W - 1 - dlgBtnCol + 1
	btnW := dlgBtnCol - 2
	if btnW < 1 {
		btnW = 1
	}
	by := contentY
	for _, b := range d.buttons {
		bh := b.Bounds().H
		if bh < 1 {
			bh = dlgBtnRowH
		}
		b.SetBounds(Rect{X: btnX, Y: by, W: btnW, H: bh})
		by += bh + 1
	}
}

// Draw renders the shadow, frame, title and children.
func (d *Dialog) Draw(s Surface) {
	fr := d.frameRect()
	if fr.W < 2 || fr.H < 2 {
		return
	}
	st := theme.DialogBody()

	// Drop shadow: one cell to the right and one row below the frame.
	shadow := theme.Shadow()
	s.Fill(Rect{X: fr.X + fr.W, Y: fr.Y + 1, W: 1, H: fr.H}, ' ', shadow)
	s.Fill(Rect{X: fr.X + 1, Y: fr.Y + fr.H, W: fr.W, H: 1}, ' ', shadow)

	// Body fill.
	s.Fill(fr, ' ', st)

	left := fr.X
	right := fr.X + fr.W - 1
	top := fr.Y
	bottom := fr.Y + fr.H - 1

	// Double-line frame.
	s.Set(left, top, theme.TLDouble, st)
	s.Set(right, top, theme.TRDouble, st)
	s.Set(left, bottom, theme.BLDouble, st)
	s.Set(right, bottom, theme.BRDouble, st)
	for x := left + 1; x < right; x++ {
		s.Set(x, top, theme.HDouble, st)
		s.Set(x, bottom, theme.HDouble, st)
	}
	for y := top + 1; y < bottom; y++ {
		s.Set(left, y, theme.VDouble, st)
		s.Set(right, y, theme.VDouble, st)
	}

	// Centered title in the top border.
	if d.title != "" {
		t := " " + d.title + " "
		t = dlgTruncate(t, fr.W-2)
		tx := left + 1 + (fr.W-2-len([]rune(t)))/2
		if tx < left+1 {
			tx = left + 1
		}
		s.Text(tx, top, t, theme.DialogTitle())
	}

	// Children (clipped to the frame interior).
	inner := fr.Inset(1, 1)
	cs := s.Clip(inner)
	for _, c := range d.body {
		c.Draw(cs.Clip(c.Bounds()))
	}
	for _, b := range d.buttons {
		b.Draw(cs.Clip(b.Bounds()))
	}
}

// HandleKey routes Enter to the default button and Esc to the cancel handler.
// Tab/Shift+Tab traversal is handled centrally by the App.
func (d *Dialog) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEnter:
		if d.def != nil {
			return d.def.HandleKey(synthEnter())
		}
		return false
	case tcell.KeyEscape:
		if d.cancel != nil {
			d.cancel()
			return true
		}
		return false
	}
	return false
}

// synthEnter builds a synthetic Enter key event for forwarding to the default
// button.
func synthEnter() *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone)
}

// dlgTruncate trims s so its rune length does not exceed max (max < 0 → "").
func dlgTruncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
