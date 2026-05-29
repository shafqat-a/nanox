package tui

import (
	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// Button is a 3-D raised VBDOS push button. Its face shows " label " in the
// button-face style (black-on-light-gray) with a one-cell solid-black drop
// shadow along the right and bottom edges (an L-shape).
//
// A DEFAULT button draws a black single-line outline around its face so it
// stands out as the Enter target. When FOCUSED, the label row is drawn in
// reverse video and the outline is kept so the active target is unmistakable.
//
// The face occupies one row; including the shadow row the widget is two rows
// tall. The face width is len(label)+2 (one pad cell each side); including the
// shadow column the widget is one cell wider.
type Button struct {
	BaseWidget
	label     string
	action    func()
	isDefault bool
}

// NewButton returns a push button with the given label and click/activate
// action. The action may be nil.
func NewButton(label string, action func()) *Button {
	return &Button{label: label, action: action}
}

// SetAction replaces the button's activation callback.
func (b *Button) SetAction(action func()) { b.action = action }

// SetLabel replaces the button's label text.
func (b *Button) SetLabel(label string) { b.label = label }

// Label returns the button's label text.
func (b *Button) Label() string { return b.label }

// SetDefault marks (or unmarks) the button as the dialog's default button.
func (b *Button) SetDefault(v bool) { b.isDefault = v }

// IsDefault reports whether the button is the default button.
func (b *Button) IsDefault() bool { return b.isDefault }

// Focusable reports that buttons accept keyboard focus.
func (b *Button) Focusable() bool { return true }

// btnFaceWidth returns the width of the button face in cells (label + padding).
func (b *Button) btnFaceWidth() int { return lblRuneLen(b.label) + 2 }

// PreferredWidth returns the full widget width: face width plus one shadow
// column.
func (b *Button) PreferredWidth() int { return b.btnFaceWidth() + 1 }

// PreferredSize returns the full widget size including the shadow: width =
// face+1 (shadow column), height = 2 (face row + shadow row).
func (b *Button) PreferredSize() (int, int) { return b.PreferredWidth(), 2 }

// Draw renders the raised button face, its L-shaped black shadow, and (for the
// default/focused states) the outline and reverse-video label row.
func (b *Button) Draw(s Surface) {
	bnds := b.Bounds()
	if bnds.Empty() {
		return
	}

	faceW := b.btnFaceWidth()
	if faceW > bnds.W {
		faceW = bnds.W
	}
	// Face occupies the top row, left faceW columns.
	faceX := bnds.X
	faceY := bnds.Y

	face := theme.ButtonFace()
	if b.focused {
		// Reverse video: swap fg/bg of the button face.
		face = tcell.StyleDefault.
			Foreground(theme.LGray).
			Background(theme.Black)
	}

	// Paint the face row.
	for x := 0; x < faceW; x++ {
		s.Set(faceX+x, faceY, ' ', face)
	}
	// Centre the label within the face (one pad cell each side nominally).
	labelW := lblRuneLen(b.label)
	lx := faceX + 1
	if labelW < faceW-2 {
		lx = faceX + (faceW-labelW)/2
	}
	s.Text(lx, faceY, b.label, face)

	// Default / focused outline around the face (single-line black box drawn
	// on the perimeter cells of the face row — VBDOS draws a thin outline that
	// hugs the face). We render the outline as bracket-style edges so it does
	// not erase the label on a single-row face.
	if b.isDefault || b.focused {
		outline := theme.ButtonFace().Foreground(theme.Black)
		s.Set(faceX, faceY, theme.VSingle, outline)
		s.Set(faceX+faceW-1, faceY, theme.VSingle, outline)
	}

	// L-shaped solid-black shadow: right edge of the face row + the entire
	// bottom row beneath the face (offset one cell right, classic DOS look).
	sh := theme.Shadow()
	// Right shadow column, beside the face row.
	if faceW < bnds.W {
		s.Set(faceX+faceW, faceY, ' ', sh)
	}
	// Bottom shadow row, starting one cell in from the left.
	if bnds.H > 1 {
		shY := faceY + 1
		for x := 1; x <= faceW && x < bnds.W; x++ {
			s.Set(faceX+x, shY, ' ', sh)
		}
	}
}

// btnActivate invokes the action if set.
func (b *Button) btnActivate() {
	if b.action != nil {
		b.action()
	}
}

// HandleKey activates the button on Enter or Space.
func (b *Button) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEnter:
		b.btnActivate()
		return true
	case tcell.KeyRune:
		if ev.Rune() == ' ' {
			b.btnActivate()
			return true
		}
	}
	return false
}

// HandleMouse activates the button on a press inside its face/bounds.
func (b *Button) HandleMouse(ev MouseEvent) bool {
	if ev.Action == MouseDown && b.Bounds().Contains(ev.X, ev.Y) {
		b.btnActivate()
		return true
	}
	return false
}
