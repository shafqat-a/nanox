package tui

import (
	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// Label is a static, non-focusable text widget rendered in the dialog body
// style (black-on-light-gray) at its bounds. An optional mnemonic character may
// be highlighted (drawn brighter) to advertise an accelerator.
type Label struct {
	BaseWidget
	text     string
	style    tcell.Style
	hasStyle bool
	mnemonic int // byte index of the mnemonic rune within text, or -1
}

// NewLabel returns a Label displaying text.
func NewLabel(text string) *Label {
	return &Label{text: text, mnemonic: -1}
}

// SetText replaces the label's text.
func (l *Label) SetText(text string) { l.text = text }

// Text returns the label's text.
func (l *Label) Text() string { return l.text }

// SetStyle overrides the default DialogBody style used to draw the text.
func (l *Label) SetStyle(st tcell.Style) {
	l.style = st
	l.hasStyle = true
}

// SetMnemonic marks the rune at the given index (in runes) as the mnemonic; it
// is drawn brighter. A negative index clears the mnemonic.
func (l *Label) SetMnemonic(runeIndex int) { l.mnemonic = runeIndex }

// PreferredWidth returns the number of cells the text occupies.
func (l *Label) PreferredWidth() int { return lblRuneLen(l.text) }

// PreferredSize returns the label's natural width and height (one row).
func (l *Label) PreferredSize() (int, int) { return l.PreferredWidth(), 1 }

// Draw renders the label text at the top-left of its bounds.
func (l *Label) Draw(s Surface) {
	b := l.Bounds()
	if b.Empty() {
		return
	}
	base := theme.DialogBody()
	if l.hasStyle {
		base = l.style
	}
	mnem := base.Foreground(theme.White).Bold(true)

	x := b.X
	y := b.Y
	idx := 0
	for _, r := range l.text {
		if idx == l.mnemonic {
			s.Set(x, y, r, mnem)
		} else {
			s.Set(x, y, r, base)
		}
		x++
		idx++
	}
}

// lblRuneLen returns the number of runes in s.
func lblRuneLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
