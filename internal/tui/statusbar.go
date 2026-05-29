package tui

import (
	"fmt"

	"dosedit/internal/theme"
)

// StatusContext selects the context-sensitive hint set shown on the left of the
// status bar.
type StatusContext int

const (
	// CtxEditing is the default editing context.
	CtxEditing StatusContext = iota
	// CtxMenu is shown while a menu is open.
	CtxMenu
	// CtxDialog is shown inside a modal dialog.
	CtxDialog
	// CtxMove is shown while moving/sizing a window.
	CtxMove
)

// stHints maps each context to its left-aligned hint string.
var stHints = map[StatusContext]string{
	CtxEditing: "F1=Help  F2=Save  F3=Open  F6=Window  F10=Menu  Alt+X=Exit",
	CtxMenu:    "F1=Help  Up/Dn=Move  Enter=Select  Esc=Cancel",
	CtxDialog:  "Tab=Next  Enter=OK  Esc=Cancel",
	CtxMove:    "Arrows=Move  Shift+Arrows=Size  Enter=Done  Esc=Cancel",
}

// StatusBar is the bottom-row VBDOS status bar (black-on-cyan). It shows a
// context hint on the left and, in the editing context, a right-aligned cursor
// readout: "Ln <n>  Col <n>", the INS/OVR mode, and a leading "*" when the
// buffer is modified.
type StatusBar struct {
	BaseWidget
	ctx      StatusContext
	ln, col  int
	ins      bool
	modified bool
}

// NewStatusBar returns a status bar defaulting to the editing context with INS
// mode and the cursor at Ln 1, Col 1.
func NewStatusBar() *StatusBar {
	return &StatusBar{ctx: CtxEditing, ln: 1, col: 1, ins: true}
}

// SetContext selects which hint set is displayed.
func (st *StatusBar) SetContext(ctx StatusContext) { st.ctx = ctx }

// SetCursor updates the cursor readout: line, column and insert/overtype mode.
func (st *StatusBar) SetCursor(ln, col int, ins bool) {
	st.ln = ln
	st.col = col
	st.ins = ins
}

// SetModified sets whether the active buffer has unsaved changes (drives the
// leading "*").
func (st *StatusBar) SetModified(m bool) { st.modified = m }

// stRightText builds the right-aligned readout for the editing context.
func (st *StatusBar) stRightText() string {
	mode := "OVR"
	if st.ins {
		mode = "INS"
	}
	star := ""
	if st.modified {
		star = "*"
	}
	return fmt.Sprintf("%sLn %d  Col %d  %s", star, st.ln, st.col, mode)
}

// Draw paints the full-width bar, the left hint, and (editing context) the
// right readout. It clips safely on narrow widths and never panics.
func (st *StatusBar) Draw(s Surface) {
	b := st.Bounds()
	if b.Empty() {
		return
	}
	row := b.Y
	style := theme.StatusBar()
	s.Fill(Rect{X: b.X, Y: row, W: b.W, H: 1}, ' ', style)

	// Left hint with a 1-cell lead.
	hint := stHints[st.ctx]
	s.Text(b.X+1, row, hint, style)

	// Right readout (editing context only).
	if st.ctx == CtxEditing {
		right := st.stRightText()
		rw := mnuRuneLen(right)
		rx := b.X + b.W - rw - 1
		// Only draw if it fits without colliding with the hint lead.
		if rx > b.X {
			s.Text(rx, row, right, style)
		}
	}
}
