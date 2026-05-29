package ui

import (
	"fmt"

	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// StatusContext selects which context-sensitive hint set the status bar shows.
type StatusContext int

const (
	// CtxEditing is the normal editing context (also shows Ln/Col + INS/OVR).
	CtxEditing StatusContext = iota
	// CtxMenu is shown while a menu is open.
	CtxMenu
	// CtxDialog is shown while a modal dialog is active.
	CtxDialog
	// CtxMove is shown while moving/sizing a window.
	CtxMove
)

// sbMinReflowWidth is the smallest width at which the right-aligned cursor
// readout is still drawn; below it the bar shows hints only.
const sbMinReflowWidth = 60

// sbHints maps each context to its left/centre hint string. Per spec §4.3 the
// status bar is uniform Black-on-Cyan with no red labels.
var sbHints = map[StatusContext]string{
	CtxEditing: "F1=Help  F2=Save  F3=Open  F6=Window  F10=Menu  Alt+X=Exit",
	CtxMenu:    "F1=Help  Up/Dn=Move  Enter=Select  Esc=Cancel",
	CtxDialog:  "Tab=Next  Enter=OK  Esc=Cancel",
	CtxMove:    "Arrows=Move  Shift+Arrows=Size  Enter=Done  Esc=Cancel",
}

// StatusBar is the full-width context-aware status / function-key bar (spec
// §6.5). It is a custom primitive embedding tview.Box.
type StatusBar struct {
	*tview.Box

	sbCtx      StatusContext
	sbLine     int
	sbCol      int
	sbInsert   bool
	sbModified bool
}

// NewStatusBar creates a status bar in the editing context.
func NewStatusBar() *StatusBar {
	return &StatusBar{
		Box:      tview.NewBox(),
		sbCtx:    CtxEditing,
		sbLine:   1,
		sbCol:    1,
		sbInsert: true,
	}
}

// SetContext swaps the active hint set.
func (s *StatusBar) SetContext(ctx StatusContext) {
	s.sbCtx = ctx
}

// SetCursor updates the line/column readout. ins=true => INS, false => OVR.
func (s *StatusBar) SetCursor(ln, col int, ins bool) {
	s.sbLine = ln
	s.sbCol = col
	s.sbInsert = ins
}

// SetModified controls the leading "*" dirty indicator on the cursor readout.
func (s *StatusBar) SetModified(mod bool) {
	s.sbModified = mod
}

// sbRightText builds the right-aligned cursor readout for the editing context.
func (s *StatusBar) sbRightText() string {
	ind := "OVR"
	if s.sbInsert {
		ind = "INS"
	}
	mark := ""
	if s.sbModified {
		mark = "*"
	}
	return fmt.Sprintf("Ln %d  Col %d  %s%s", s.sbLine, s.sbCol, mark, ind)
}

// Draw renders the bar: the whole row is filled with the status-bar style,
// the context hint is drawn at the left, and (only when editing and the
// terminal is wide enough) the cursor readout is right-justified. It reflows
// and clips gracefully and never panics on small sizes.
func (s *StatusBar) Draw(screen tcell.Screen) {
	s.Box.DrawForSubclass(screen, s)

	x, y, width, height := s.GetRect()
	if width <= 0 || height <= 0 {
		return
	}

	style := theme.StatusBar()

	// Fill the entire row with the uniform background.
	for col := 0; col < width; col++ {
		screen.SetContent(x+col, y, ' ', nil, style)
	}

	hint := []rune(sbHints[s.sbCtx])

	// Right-aligned cursor readout (editing context, wide enough terminals).
	rightStart := width
	if s.sbCtx == CtxEditing && width >= sbMinReflowWidth {
		right := []rune(s.sbRightText())
		if len(right) < width {
			rightStart = width - len(right)
			sbPutRunes(screen, x, y, rightStart, right, width, style)
		}
	}

	// Left hint, clipped so it never overruns the right readout.
	limit := rightStart
	if limit > width {
		limit = width
	}
	sbPutRunes(screen, x, y, 1, hint, limit, style)
}

// sbPutRunes writes rs starting at column off (0-based, relative to x),
// clipping at limit columns. Safe for any width.
func sbPutRunes(screen tcell.Screen, x, y, off int, rs []rune, limit int, style tcell.Style) {
	for i, r := range rs {
		col := off + i
		if col < 0 {
			continue
		}
		if col >= limit {
			break
		}
		screen.SetContent(x+col, y, r, nil, style)
	}
}
