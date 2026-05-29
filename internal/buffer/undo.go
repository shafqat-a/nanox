package buffer

// Undo/redo model
// ---------------
// Rather than recording fine-grained command deltas, each undoable edit is
// captured as a before/after SNAPSHOT of the contiguous block of lines it
// touched, together with the cursor position before the edit and after it. This
// "snapshot a line range" approach is deliberately simple and always correct:
// applying or reverting an Op is just splicing a slice of lines back into the
// buffer, regardless of how complex the original edit was (single keystroke,
// multi-line paste, range delete, …).
//
// An Op covers the half-open original range [Line, Line+len(Before)) which is
// replaced by After (and vice-versa to undo). Because the editor records the
// op, it is responsible for capturing Before/cursor positions before mutating
// and After/cursor positions afterward; helpers below make that ergonomic.

// Op is one reversible edit: it replaces the lines originally at
// [Line, Line+len(Before)) with After to redo, or replaces
// [Line, Line+len(After)) with Before to undo.
type Op struct {
	Line       int      // index of the first affected line (same before & after)
	Before     []string // snapshot of the affected lines BEFORE the edit
	After      []string // snapshot of the affected lines AFTER the edit
	CursorL    int      // cursor line before the edit (restored on undo)
	CursorC    int      // cursor column before the edit (restored on undo)
	NewCursorL int      // cursor line after the edit (restored on redo)
	NewCursorC int      // cursor column after the edit (restored on redo)
}

// Apply performs the edit on b (the "redo" / forward direction), returning the
// cursor position the caller should move to.
func (op Op) Apply(b *Buffer) (line, col int) {
	b.replace(op.Line, len(op.Before), op.After)
	b.Modified = true
	return op.NewCursorL, op.NewCursorC
}

// Revert undoes the edit on b, returning the cursor position to move to.
func (op Op) Revert(b *Buffer) (line, col int) {
	b.replace(op.Line, len(op.After), op.Before)
	b.Modified = true
	return op.CursorL, op.CursorC
}

// replace swaps the count lines starting at start with repl. start/count are
// clamped to the buffer. The buffer always retains at least one line.
func (b *Buffer) replace(start, count int, repl []string) {
	if start < 0 {
		start = 0
	}
	if start > len(b.Lines) {
		start = len(b.Lines)
	}
	end := start + count
	if end > len(b.Lines) {
		end = len(b.Lines)
	}
	tail := append([]string(nil), b.Lines[end:]...)
	b.Lines = append(b.Lines[:start], append(append([]string(nil), repl...), tail...)...)
	if len(b.Lines) == 0 {
		b.Lines = []string{""}
	}
}

// Snapshot copies the count lines starting at line, for use building an Op's
// Before/After. It is range-safe and always returns at least one element when
// count > 0 and the range is valid.
func (b *Buffer) Snapshot(line, count int) []string {
	if line < 0 {
		line = 0
	}
	end := line + count
	if end > len(b.Lines) {
		end = len(b.Lines)
	}
	if line >= end {
		return []string{}
	}
	out := make([]string, end-line)
	copy(out, b.Lines[line:end])
	return out
}

// UndoStack is a bounded, two-list undo/redo history. Record clears the redo
// list (a new edit invalidates any redo future). Undo/Redo hand the relevant Op
// back to the caller, which applies/reverts it against the live Buffer.
type UndoStack struct {
	limit int
	undo  []Op
	redo  []Op
}

// NewUndoStack returns a stack bounded to at most limit retained undo ops. A
// limit <= 0 is treated as the default of 500.
func NewUndoStack(limit int) *UndoStack {
	if limit <= 0 {
		limit = 500
	}
	return &UndoStack{limit: limit}
}

// Record pushes a completed edit and discards the redo history. If the undo
// list exceeds the limit, the oldest op is dropped.
func (s *UndoStack) Record(op Op) {
	s.undo = append(s.undo, op)
	if len(s.undo) > s.limit {
		// drop oldest
		s.undo = s.undo[len(s.undo)-s.limit:]
	}
	s.redo = s.redo[:0]
}

// Undo pops the most recent op for reverting and moves it onto the redo list.
// It returns (op, false) when there is nothing to undo.
func (s *UndoStack) Undo() (Op, bool) {
	if len(s.undo) == 0 {
		return Op{}, false
	}
	op := s.undo[len(s.undo)-1]
	s.undo = s.undo[:len(s.undo)-1]
	s.redo = append(s.redo, op)
	return op, true
}

// Redo pops the most recent undone op for re-applying and moves it back onto
// the undo list. It returns (op, false) when there is nothing to redo.
func (s *UndoStack) Redo() (Op, bool) {
	if len(s.redo) == 0 {
		return Op{}, false
	}
	op := s.redo[len(s.redo)-1]
	s.redo = s.redo[:len(s.redo)-1]
	s.undo = append(s.undo, op)
	return op, true
}

// Clear empties both histories.
func (s *UndoStack) Clear() {
	s.undo = s.undo[:0]
	s.redo = s.redo[:0]
}

// CanUndo reports whether there is at least one op to undo.
func (s *UndoStack) CanUndo() bool { return len(s.undo) > 0 }

// CanRedo reports whether there is at least one op to redo.
func (s *UndoStack) CanRedo() bool { return len(s.redo) > 0 }
