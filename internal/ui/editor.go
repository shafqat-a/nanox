package ui

import (
	"strings"
	"unicode"

	"dosedit/internal/buffer"
	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// edClipboard is the package-level internal clipboard shared by all editors.
// It holds the most recently cut/copied text (may contain "\n" line breaks).
var edClipboard string

// edPos is a (line, col) cursor/anchor position; col is a 0-based rune offset.
type edPos struct {
	Line, Col int
}

// Editor is the custom text-editing primitive for DOSEdit (spec §6.4,
// Appendix B). It embeds tview.Box and renders the inner text area plus
// scrollbars, driving the buffer and undo stack directly.
type Editor struct {
	*tview.Box

	buf     *buffer.Buffer
	topLine int // vertical scroll offset (first visible line)
	leftCol int // horizontal scroll offset (first visible display column)
	curLine int
	curCol  int

	selAnchor *edPos // nil when no selection
	overwrite bool   // INS/OVR mode
	undo      *buffer.UndoStack

	onCursorMove func(ln, col int, ins bool)
	onChange     func()
}

// NewEditor creates an Editor over buffer b (a fresh untitled buffer if nil).
func NewEditor(b *buffer.Buffer) *Editor {
	if b == nil {
		b = buffer.NewUntitled()
	}
	if b.TabWidth <= 0 {
		b.TabWidth = 4
	}
	e := &Editor{
		Box:  tview.NewBox(),
		buf:  b,
		undo: buffer.NewUndoStack(500),
	}
	return e
}

// --- exported integration API (called by APP / window) ---

// Buffer returns the underlying document buffer.
func (e *Editor) Buffer() *buffer.Buffer { return e.buf }

// SetOnCursorMove registers a callback fired after every cursor/mode/buffer
// change. ln/col are 1-based for display; ins is true in insert (not overwrite)
// mode.
func (e *Editor) SetOnCursorMove(fn func(ln, col int, ins bool)) {
	e.onCursorMove = fn
	e.edNotifyCursor()
}

// SetOnChange registers a callback fired when the buffer's Modified state may
// have changed.
func (e *Editor) SetOnChange(fn func()) { e.onChange = fn }

// --- notifications ---

func (e *Editor) edNotifyCursor() {
	if e.onCursorMove != nil {
		e.onCursorMove(e.curLine+1, e.curCol+1, !e.overwrite)
	}
}

func (e *Editor) edNotifyChange() {
	if e.onChange != nil {
		e.onChange()
	}
}

// --- small helpers (all prefixed ed*) ---

func edMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func edMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func edClamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// edLineRunes returns the rune slice for the given buffer line.
func (e *Editor) edLineRunes(line int) []rune {
	return []rune(e.buf.Line(line))
}

// edLineLen returns the rune length of the given buffer line.
func (e *Editor) edLineLen(line int) int {
	return len(e.edLineRunes(line))
}

// edTabWidth returns the (positive) display width of a tab.
func (e *Editor) edTabWidth() int {
	if e.buf.TabWidth <= 0 {
		return 4
	}
	return e.buf.TabWidth
}

// edDisplayCol converts a rune column on a line to its display column,
// expanding tabs to the configured tab stop.
func (e *Editor) edDisplayCol(line, col int) int {
	rs := e.edLineRunes(line)
	tw := e.edTabWidth()
	dc := 0
	for i := 0; i < col && i < len(rs); i++ {
		if rs[i] == '\t' {
			dc += tw - (dc % tw)
		} else {
			dc++
		}
	}
	return dc
}

// edLineDisplayWidth returns the full display width of a line.
func (e *Editor) edLineDisplayWidth(line int) int {
	return e.edDisplayCol(line, e.edLineLen(line))
}

// --- selection helpers ---

// edHasSelection reports whether a non-empty selection exists.
func (e *Editor) edHasSelection() bool {
	if e.selAnchor == nil {
		return false
	}
	return e.selAnchor.Line != e.curLine || e.selAnchor.Col != e.curCol
}

// edSelRange returns the normalized selection range (start <= end) in document
// order. Only valid when edHasSelection is true.
func (e *Editor) edSelRange() (sl, sc, el, ec int) {
	a := *e.selAnchor
	b := edPos{Line: e.curLine, Col: e.curCol}
	if b.Line < a.Line || (b.Line == a.Line && b.Col < a.Col) {
		a, b = b, a
	}
	return a.Line, a.Col, b.Line, b.Col
}

// edStartSelection sets the anchor at the current cursor if none exists.
func (e *Editor) edStartSelection() {
	if e.selAnchor == nil {
		e.selAnchor = &edPos{Line: e.curLine, Col: e.curCol}
	}
}

// edClearSelection drops any active selection.
func (e *Editor) edClearSelection() { e.selAnchor = nil }

// edSelectedText returns the currently selected text, or "" if no selection.
func (e *Editor) edSelectedText() string {
	if !e.edHasSelection() {
		return ""
	}
	sl, sc, el, ec := e.edSelRange()
	if sl == el {
		rs := e.edLineRunes(sl)
		sc = edClamp(sc, 0, len(rs))
		ec = edClamp(ec, 0, len(rs))
		return string(rs[sc:ec])
	}
	var sb strings.Builder
	first := e.edLineRunes(sl)
	sc = edClamp(sc, 0, len(first))
	sb.WriteString(string(first[sc:]))
	for l := sl + 1; l < el; l++ {
		sb.WriteByte('\n')
		sb.WriteString(e.buf.Line(l))
	}
	sb.WriteByte('\n')
	last := e.edLineRunes(el)
	ec = edClamp(ec, 0, len(last))
	sb.WriteString(string(last[:ec]))
	return sb.String()
}

// --- undo plumbing ---

// edRecord captures a before/after snapshot of [line, line+beforeCount) and
// records an undo Op. oldCount lines were affected before; the new count is
// computed from the live buffer relative to the trailing lines that were
// unchanged. Callers pass the affected line range explicitly.
func (e *Editor) edRecord(line, beforeCount, afterCount, curL, curC, newL, newC int, before []string) {
	after := e.buf.Snapshot(line, afterCount)
	e.undo.Record(buffer.Op{
		Line:       line,
		Before:     before,
		After:      after,
		CursorL:    curL,
		CursorC:    curC,
		NewCursorL: newL,
		NewCursorC: newC,
	})
}

// --- editing operations ---

// edDeleteSelection removes the active selection (recording undo) and places
// the cursor at the start. Returns true if anything was deleted.
func (e *Editor) edDeleteSelection() bool {
	if !e.edHasSelection() {
		return false
	}
	sl, sc, el, ec := e.edSelRange()
	beforeCount := el - sl + 1
	before := e.buf.Snapshot(sl, beforeCount)
	curL, curC := e.curLine, e.curCol
	e.buf.DeleteRange(sl, sc, el, ec)
	e.curLine, e.curCol = sl, sc
	e.edClearSelection()
	e.edRecord(sl, beforeCount, 1, curL, curC, e.curLine, e.curCol, before)
	e.edNotifyChange()
	return true
}

// edInsertRune inserts (or overwrites) a single printable rune at the cursor.
func (e *Editor) edInsertRune(r rune) {
	e.edDeleteSelection()
	line := e.curLine
	before := e.buf.Snapshot(line, 1)
	curL, curC := e.curLine, e.curCol
	rs := e.edLineRunes(line)
	if e.overwrite && e.curCol < len(rs) {
		e.buf.DeleteRune(line, e.curCol)
	}
	e.buf.InsertRune(line, e.curCol, r)
	e.curCol++
	e.edRecord(line, 1, 1, curL, curC, e.curLine, e.curCol, before)
	e.edClearSelection()
	e.edNotifyChange()
}

// edInsertText inserts arbitrary (possibly multi-line) text at the cursor,
// replacing any selection.
func (e *Editor) edInsertText(text string) {
	if text == "" {
		return
	}
	e.edDeleteSelection()
	line := e.curLine
	curL, curC := e.curLine, e.curCol
	nl := strings.Count(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	before := e.buf.Snapshot(line, 1)
	endLine, endCol := e.buf.InsertText(line, e.curCol, text)
	e.curLine, e.curCol = endLine, endCol
	e.edRecord(line, 1, nl+1, curL, curC, e.curLine, e.curCol, before)
	e.edClearSelection()
	e.edNotifyChange()
}

// edSplitLine handles Enter: split the current line, auto-indenting the new
// line to match the leading whitespace of the line being split.
func (e *Editor) edSplitLine() {
	e.edDeleteSelection()
	line := e.curLine
	curL, curC := e.curLine, e.curCol
	rs := e.edLineRunes(line)

	// Compute leading whitespace for auto-indent.
	indent := 0
	for indent < len(rs) && (rs[indent] == ' ' || rs[indent] == '\t') {
		indent++
	}
	if indent > e.curCol {
		indent = e.curCol
	}
	lead := string(rs[:indent])

	before := e.buf.Snapshot(line, 1)
	e.buf.SplitLine(line, e.curCol)
	e.curLine = line + 1
	e.curCol = 0
	if lead != "" {
		e.buf.InsertString(e.curLine, 0, lead)
		e.curCol = len([]rune(lead))
	}
	e.edRecord(line, 1, 2, curL, curC, e.curLine, e.curCol, before)
	e.edClearSelection()
	e.edNotifyChange()
}

// edBackspace handles Backspace: delete left, joining lines at column 0.
func (e *Editor) edBackspace() {
	if e.edDeleteSelection() {
		return
	}
	if e.curCol > 0 {
		line := e.curLine
		before := e.buf.Snapshot(line, 1)
		curL, curC := e.curLine, e.curCol
		e.buf.DeleteRune(line, e.curCol-1)
		e.curCol--
		e.edRecord(line, 1, 1, curL, curC, e.curLine, e.curCol, before)
		e.edNotifyChange()
		return
	}
	if e.curLine > 0 {
		prev := e.curLine - 1
		before := e.buf.Snapshot(prev, 2)
		curL, curC := e.curLine, e.curCol
		newCol := e.buf.JoinLine(e.curLine)
		e.curLine = prev
		e.curCol = newCol
		e.edRecord(prev, 2, 1, curL, curC, e.curLine, e.curCol, before)
		e.edNotifyChange()
	}
}

// edDelete handles Delete: delete selection, or the char right (joining lines).
func (e *Editor) edDelete() {
	if e.edDeleteSelection() {
		return
	}
	line := e.curLine
	rs := e.edLineRunes(line)
	if e.curCol < len(rs) {
		before := e.buf.Snapshot(line, 1)
		curL, curC := e.curLine, e.curCol
		e.buf.DeleteRune(line, e.curCol)
		e.edRecord(line, 1, 1, curL, curC, e.curLine, e.curCol, before)
		e.edNotifyChange()
		return
	}
	if line+1 < e.buf.LineCount() {
		before := e.buf.Snapshot(line, 2)
		curL, curC := e.curLine, e.curCol
		e.buf.DeleteRune(line, e.curCol) // joins next line up
		e.edRecord(line, 2, 1, curL, curC, e.curLine, e.curCol, before)
		e.edNotifyChange()
	}
}

// edTabString returns the text inserted for a Tab keystroke.
func (e *Editor) edTabString() string {
	if e.buf.UseSpaces {
		tw := e.edTabWidth()
		n := tw - (e.edDisplayCol(e.curLine, e.curCol) % tw)
		if n <= 0 {
			n = tw
		}
		return strings.Repeat(" ", n)
	}
	return "\t"
}

// edIndentBlock indents (dir>0) or outdents (dir<0) the selected line block.
func (e *Editor) edIndentBlock(dir int) {
	sl, _, el, _ := e.edSelRange()
	beforeCount := el - sl + 1
	before := e.buf.Snapshot(sl, beforeCount)
	curL, curC := e.curLine, e.curCol
	anchor := *e.selAnchor

	unit := "\t"
	width := 1
	if e.buf.UseSpaces {
		width = e.edTabWidth()
		unit = strings.Repeat(" ", width)
	}

	for l := sl; l <= el; l++ {
		rs := e.edLineRunes(l)
		if dir > 0 {
			e.buf.InsertString(l, 0, unit)
		} else {
			// Outdent: remove up to one tab or `width` leading spaces.
			removed := 0
			if len(rs) > 0 && rs[0] == '\t' {
				e.buf.DeleteRune(l, 0)
				removed = 1
			} else {
				for removed < width && removed < len(rs) && rs[removed] == ' ' {
					removed++
				}
				for k := 0; k < removed; k++ {
					e.buf.DeleteRune(l, 0)
				}
			}
		}
	}

	// Adjust cursor/anchor columns by the change applied to their own line.
	adjust := func(p *edPos) {
		if p.Line < sl || p.Line > el {
			return
		}
		if dir > 0 {
			p.Col += width
		} else {
			p.Col = edMax(0, p.Col-width)
		}
		p.Col = edClamp(p.Col, 0, e.edLineLen(p.Line))
	}
	cur := edPos{Line: e.curLine, Col: e.curCol}
	adjust(&cur)
	adjust(&anchor)
	e.curLine, e.curCol = cur.Line, cur.Col
	e.selAnchor = &anchor

	e.edRecord(sl, beforeCount, beforeCount, curL, curC, e.curLine, e.curCol, before)
	e.edNotifyChange()
}

// edTab handles Tab / Shift+Tab.
func (e *Editor) edTab(shift bool) {
	if e.edHasSelection() {
		sl, _, el, _ := e.edSelRange()
		if sl != el {
			if shift {
				e.edIndentBlock(-1)
			} else {
				e.edIndentBlock(1)
			}
			return
		}
	}
	if shift {
		return // single-line Shift+Tab: no-op (outdent handled for blocks)
	}
	e.edInsertText(e.edTabString())
}

// --- clipboard ---

func (e *Editor) edCopy() {
	if e.edHasSelection() {
		edClipboard = e.edSelectedText()
	}
}

func (e *Editor) edCut() {
	if e.edHasSelection() {
		edClipboard = e.edSelectedText()
		e.edDeleteSelection()
	}
}

func (e *Editor) edPaste() {
	if edClipboard != "" {
		e.edInsertText(edClipboard)
	}
}

// --- undo / redo ---

func (e *Editor) edUndo() {
	op, ok := e.undo.Undo()
	if !ok {
		return
	}
	l, c := op.Revert(e.buf)
	e.curLine = edClamp(l, 0, e.buf.LineCount()-1)
	e.curCol = edClamp(c, 0, e.edLineLen(e.curLine))
	e.edClearSelection()
	e.edNotifyChange()
}

func (e *Editor) edRedo() {
	op, ok := e.undo.Redo()
	if !ok {
		return
	}
	l, c := op.Apply(e.buf)
	e.curLine = edClamp(l, 0, e.buf.LineCount()-1)
	e.curCol = edClamp(c, 0, e.edLineLen(e.curLine))
	e.edClearSelection()
	e.edNotifyChange()
}

// --- movement ---

// edClampCursor keeps the cursor within the buffer's valid range.
func (e *Editor) edClampCursor() {
	e.curLine = edClamp(e.curLine, 0, e.buf.LineCount()-1)
	e.curCol = edClamp(e.curCol, 0, e.edLineLen(e.curLine))
}

func (e *Editor) edMoveLeft() {
	if e.curCol > 0 {
		e.curCol--
	} else if e.curLine > 0 {
		e.curLine--
		e.curCol = e.edLineLen(e.curLine)
	}
}

func (e *Editor) edMoveRight() {
	if e.curCol < e.edLineLen(e.curLine) {
		e.curCol++
	} else if e.curLine < e.buf.LineCount()-1 {
		e.curLine++
		e.curCol = 0
	}
}

func (e *Editor) edMoveUp() {
	if e.curLine > 0 {
		e.curLine--
		e.curCol = edMin(e.curCol, e.edLineLen(e.curLine))
	}
}

func (e *Editor) edMoveDown() {
	if e.curLine < e.buf.LineCount()-1 {
		e.curLine++
		e.curCol = edMin(e.curCol, e.edLineLen(e.curLine))
	}
}

func edIsWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// edWordLeft moves to the start of the previous word.
func (e *Editor) edWordLeft() {
	if e.curCol == 0 {
		if e.curLine > 0 {
			e.curLine--
			e.curCol = e.edLineLen(e.curLine)
		}
		return
	}
	rs := e.edLineRunes(e.curLine)
	i := e.curCol
	for i > 0 && !edIsWord(rs[i-1]) {
		i--
	}
	for i > 0 && edIsWord(rs[i-1]) {
		i--
	}
	e.curCol = i
}

// edWordRight moves to the start of the next word.
func (e *Editor) edWordRight() {
	rs := e.edLineRunes(e.curLine)
	if e.curCol >= len(rs) {
		if e.curLine < e.buf.LineCount()-1 {
			e.curLine++
			e.curCol = 0
		}
		return
	}
	i := e.curCol
	for i < len(rs) && edIsWord(rs[i]) {
		i++
	}
	for i < len(rs) && !edIsWord(rs[i]) {
		i++
	}
	e.curCol = i
}

func (e *Editor) edHome()    { e.curCol = 0 }
func (e *Editor) edEnd()     { e.curCol = e.edLineLen(e.curLine) }
func (e *Editor) edDocHome() { e.curLine, e.curCol = 0, 0 }
func (e *Editor) edDocEnd() {
	e.curLine = e.buf.LineCount() - 1
	e.curCol = e.edLineLen(e.curLine)
}

// edPageRows returns the number of text rows in the view (>=1).
func (e *Editor) edPageRows() int {
	_, _, _, h := e.GetInnerRect()
	if h < 1 {
		return 1
	}
	return h
}

func (e *Editor) edPageUp() {
	n := e.edPageRows()
	e.curLine = edMax(0, e.curLine-n)
	e.topLine = edMax(0, e.topLine-n)
	e.curCol = edMin(e.curCol, e.edLineLen(e.curLine))
}

func (e *Editor) edPageDown() {
	n := e.edPageRows()
	e.curLine = edMin(e.buf.LineCount()-1, e.curLine+n)
	e.topLine = edMin(edMax(0, e.buf.LineCount()-1), e.topLine+n)
	e.curCol = edMin(e.curCol, e.edLineLen(e.curLine))
}

// --- drawing ---

// Draw renders the editor's inner text area and scrollbars.
func (e *Editor) Draw(screen tcell.Screen) {
	e.DrawForSubclass(screen, e.Box)
	x, y, w, h := e.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}

	lineCount := e.buf.LineCount()
	maxWidth := 0
	for i := 0; i < lineCount; i++ {
		if dw := e.edLineDisplayWidth(i); dw > maxWidth {
			maxWidth = dw
		}
	}

	// Decide whether scrollbars are needed. Account for the space they take.
	textW, textH := w, h
	needV := lineCount > textH
	needH := maxWidth > textW
	if needV {
		textW = w - 1
	}
	if needH {
		textH = h - 1
	}
	// Re-evaluate after reserving (a bar may become needed once the other shrinks).
	if !needV && lineCount > textH {
		needV = true
		textW = w - 1
	}
	if !needH && maxWidth > textW {
		needH = true
		textH = h - 1
	}
	if textW < 1 {
		textW = 1
	}
	if textH < 1 {
		textH = 1
	}

	e.edScrollToCursorWith(textW, textH)

	base := theme.EditorText()
	sel := theme.Selection()
	cursorStyle := theme.Cursor()

	var sl, sc, el, ec int
	hasSel := e.edHasSelection()
	if hasSel {
		sl, sc, el, ec = e.edSelRange()
	}

	for row := 0; row < textH; row++ {
		lineIdx := e.topLine + row
		sy := y + row
		// Blank the row first.
		for col := 0; col < textW; col++ {
			screen.SetContent(x+col, sy, ' ', nil, base)
		}
		if lineIdx >= lineCount {
			continue
		}
		e.edDrawLine(screen, x, sy, textW, lineIdx, base, sel, cursorStyle,
			hasSel, sl, sc, el, ec)
	}

	if needV {
		e.edDrawVScroll(screen, x+textW, y, textH, lineCount)
	}
	if needH {
		e.edDrawHScroll(screen, x, y+textH, textW, maxWidth)
	}
}

// edScrollToCursorWith keeps the cursor visible given the text area dimensions.
func (e *Editor) edScrollToCursorWith(textW, textH int) {
	if e.curLine < e.topLine {
		e.topLine = e.curLine
	}
	if e.curLine >= e.topLine+textH {
		e.topLine = e.curLine - textH + 1
	}
	if e.topLine < 0 {
		e.topLine = 0
	}
	dc := e.edDisplayCol(e.curLine, e.curCol)
	if dc < e.leftCol {
		e.leftCol = dc
	}
	if dc >= e.leftCol+textW {
		e.leftCol = dc - textW + 1
	}
	if e.leftCol < 0 {
		e.leftCol = 0
	}
}

// edDrawLine renders one visible line with selection and cursor styling.
func (e *Editor) edDrawLine(screen tcell.Screen, x, sy, textW, lineIdx int,
	base, sel, cursorStyle tcell.Style, hasSel bool, sl, sc, el, ec int) {

	rs := e.edLineRunes(lineIdx)
	tw := e.edTabWidth()

	// Build the display cells for this line: each rune may expand (tab).
	// We iterate runes, tracking the display column, and paint those that fall
	// within [leftCol, leftCol+textW).
	dc := 0
	col := 0
	isCursorLine := lineIdx == e.curLine

	for col <= len(rs) {
		// Determine selection state at this rune position.
		inSel := false
		if hasSel {
			inSel = edPosInRange(lineIdx, col, sl, sc, el, ec)
		}
		isCursorCell := isCursorLine && col == e.curCol

		if col == len(rs) {
			// Past end of text: only the cursor (and trailing selection) matter.
			if isCursorCell {
				vx := dc - e.leftCol
				if vx >= 0 && vx < textW {
					screen.SetContent(x+vx, sy, ' ', nil, cursorStyle)
				}
			}
			break
		}

		r := rs[col]
		style := base
		if inSel {
			style = sel
		}
		if isCursorCell {
			style = cursorStyle
		}

		if r == '\t' {
			width := tw - (dc % tw)
			for k := 0; k < width; k++ {
				vx := dc + k - e.leftCol
				if vx >= 0 && vx < textW {
					screen.SetContent(x+vx, sy, ' ', nil, style)
				}
			}
			dc += width
		} else {
			vx := dc - e.leftCol
			if vx >= 0 && vx < textW {
				screen.SetContent(x+vx, sy, r, nil, style)
			}
			dc++
		}
		col++
	}
}

// edPosInRange reports whether rune position (line,col) lies within the
// half-open selection [(sl,sc),(el,ec)).
func edPosInRange(line, col, sl, sc, el, ec int) bool {
	if line < sl || line > el {
		return false
	}
	afterStart := line > sl || col >= sc
	beforeEnd := line < el || col < ec
	return afterStart && beforeEnd
}

// edDrawVScroll renders the vertical scrollbar in column vx.
func (e *Editor) edDrawVScroll(screen tcell.Screen, vx, y, textH, lineCount int) {
	st := theme.EditorText()
	screen.SetContent(vx, y, theme.SbUp, nil, st)
	screen.SetContent(vx, y+textH-1, theme.SbDown, nil, st)
	trackTop := y + 1
	trackH := textH - 2
	for i := 0; i < trackH; i++ {
		screen.SetContent(vx, trackTop+i, theme.SbTrack, nil, st)
	}
	if trackH > 0 {
		maxTop := edMax(1, lineCount-textH)
		pos := 0
		if maxTop > 0 {
			pos = e.topLine * (trackH - 1) / maxTop
		}
		pos = edClamp(pos, 0, trackH-1)
		screen.SetContent(vx, trackTop+pos, theme.SbThumb, nil, st)
	}
}

// edDrawHScroll renders the horizontal scrollbar in row hy.
func (e *Editor) edDrawHScroll(screen tcell.Screen, x, hy, textW, maxWidth int) {
	st := theme.EditorText()
	screen.SetContent(x, hy, theme.SbLeft, nil, st)
	screen.SetContent(x+textW-1, hy, theme.SbRight, nil, st)
	trackLeft := x + 1
	trackW := textW - 2
	for i := 0; i < trackW; i++ {
		screen.SetContent(trackLeft+i, hy, theme.SbTrack, nil, st)
	}
	if trackW > 0 {
		maxLeft := edMax(1, maxWidth-textW)
		pos := 0
		if maxLeft > 0 {
			pos = e.leftCol * (trackW - 1) / maxLeft
		}
		pos = edClamp(pos, 0, trackW-1)
		screen.SetContent(trackLeft+pos, hy, theme.SbThumb, nil, st)
	}
}

// --- input ---

// edIsMovementKey reports whether key is a cursor-movement key (used to decide
// when a Shift modifier should extend the selection).
func edIsMovementKey(key tcell.Key) bool {
	switch key {
	case tcell.KeyLeft, tcell.KeyRight, tcell.KeyUp, tcell.KeyDown,
		tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn:
		return true
	}
	return false
}

// InputHandler routes editing/movement keys. Keys it does not handle are left
// unconsumed so they bubble up to the application.
func (e *Editor) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return e.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		mod := event.Modifiers()
		shift := mod&tcell.ModShift != 0
		ctrl := mod&tcell.ModCtrl != 0
		key := event.Key()

		moved := false   // a cursor move that should manage selection
		handled := false // whether we consumed the event
		mutated := false // whether the buffer/mode changed (refresh cursor)

		// For a shifted movement key, anchor the selection at the CURRENT cursor
		// position before the move happens (so the first shifted move extends
		// from where we started).
		if shift && edIsMovementKey(key) {
			e.edStartSelection()
		}

		switch key {
		// --- movement ---
		case tcell.KeyLeft:
			if ctrl {
				e.edWordLeft()
			} else {
				e.edMoveLeft()
			}
			moved, handled = true, true
		case tcell.KeyRight:
			if ctrl {
				e.edWordRight()
			} else {
				e.edMoveRight()
			}
			moved, handled = true, true
		case tcell.KeyUp:
			e.edMoveUp()
			moved, handled = true, true
		case tcell.KeyDown:
			e.edMoveDown()
			moved, handled = true, true
		case tcell.KeyHome:
			if ctrl {
				e.edDocHome()
			} else {
				e.edHome()
			}
			moved, handled = true, true
		case tcell.KeyEnd:
			if ctrl {
				e.edDocEnd()
			} else {
				e.edEnd()
			}
			moved, handled = true, true
		case tcell.KeyPgUp:
			if ctrl {
				e.leftCol = 0
				e.edHome()
			} else {
				e.edPageUp()
			}
			moved, handled = true, true
		case tcell.KeyPgDn:
			if ctrl {
				e.edEnd()
			} else {
				e.edPageDown()
			}
			moved, handled = true, true

		// --- editing ---
		case tcell.KeyEnter:
			e.edSplitLine()
			handled, mutated = true, true
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			e.edBackspace()
			handled, mutated = true, true
		case tcell.KeyDelete:
			if shift {
				e.edCut()
			} else {
				e.edDelete()
			}
			handled, mutated = true, true
		case tcell.KeyTab:
			e.edTab(false)
			handled, mutated = true, true
		case tcell.KeyBacktab:
			e.edTab(true)
			handled, mutated = true, true
		case tcell.KeyInsert:
			if ctrl {
				e.edCopy()
			} else if shift {
				e.edPaste()
				mutated = true
			} else {
				e.overwrite = !e.overwrite
				mutated = true
			}
			handled = true

		// --- control chords ---
		case tcell.KeyCtrlC:
			e.edCopy()
			handled = true
		case tcell.KeyCtrlX:
			e.edCut()
			handled, mutated = true, true
		case tcell.KeyCtrlV:
			e.edPaste()
			handled, mutated = true, true
		case tcell.KeyCtrlZ:
			e.edUndo()
			handled, mutated = true, true
		case tcell.KeyCtrlY:
			e.edRedo()
			handled, mutated = true, true

		// --- printable runes ---
		case tcell.KeyRune:
			if ctrl {
				return // let app handle Ctrl+letter chords we don't own
			}
			e.edInsertRune(event.Rune())
			handled, mutated = true, true
		}

		if !handled {
			return // bubble up
		}

		if moved {
			if !shift {
				e.edClearSelection()
			}
			e.edClampCursor()
		}
		if mutated {
			e.edClampCursor()
		}
		e.edNotifyCursor()
	})
}
