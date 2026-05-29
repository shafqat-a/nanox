package buffer

import (
	"strings"
	"testing"
)

func TestUndoRedoSingleLine(t *testing.T) {
	b := &Buffer{Lines: []string{"hello"}, EOL: "\n"}
	s := NewUndoStack(0)

	before := b.Snapshot(0, 1)
	b.InsertString(0, 5, " world")
	after := b.Snapshot(0, 1)
	s.Record(Op{Line: 0, Before: before, After: after, CursorL: 0, CursorC: 5, NewCursorL: 0, NewCursorC: 11})

	if b.Lines[0] != "hello world" {
		t.Fatalf("setup wrong: %q", b.Lines[0])
	}

	op, ok := s.Undo()
	if !ok {
		t.Fatal("expected undo available")
	}
	l, c := op.Revert(b)
	if b.Lines[0] != "hello" || l != 0 || c != 5 {
		t.Fatalf("undo wrong: %q cursor=%d,%d", b.Lines[0], l, c)
	}

	op, ok = s.Redo()
	if !ok {
		t.Fatal("expected redo available")
	}
	l, c = op.Apply(b)
	if b.Lines[0] != "hello world" || l != 0 || c != 11 {
		t.Fatalf("redo wrong: %q cursor=%d,%d", b.Lines[0], l, c)
	}
}

func TestUndoRedoMultiLineSequence(t *testing.T) {
	b := &Buffer{Lines: []string{"start"}, EOL: "\n"}
	s := NewUndoStack(0)

	// Edit 1: split the line.
	before1 := b.Snapshot(0, 1)
	b.SplitLine(0, 2) // "st", "art"
	after1 := b.Snapshot(0, 2)
	s.Record(Op{Line: 0, Before: before1, After: after1, CursorL: 0, CursorC: 2, NewCursorL: 1, NewCursorC: 0})

	// Edit 2: multi-line paste at start of line 1.
	before2 := b.Snapshot(1, 1)
	el, ec := b.InsertText(1, 0, "X\nY")
	after2 := b.Snapshot(1, el-1+1) // lines 1..el inclusive
	s.Record(Op{Line: 1, Before: before2, After: after2, CursorL: 1, CursorC: 0, NewCursorL: el, NewCursorC: ec})

	if strings.Join(b.Lines, "|") != "st|X|Yart" {
		t.Fatalf("after edits: %#v", b.Lines)
	}

	// Undo edit 2.
	op, _ := s.Undo()
	op.Revert(b)
	if strings.Join(b.Lines, "|") != "st|art" {
		t.Fatalf("after undo 2: %#v", b.Lines)
	}

	// Undo edit 1.
	op, _ = s.Undo()
	op.Revert(b)
	if strings.Join(b.Lines, "|") != "start" {
		t.Fatalf("after undo 1: %#v", b.Lines)
	}

	if _, ok := s.Undo(); ok {
		t.Fatal("expected no more undo")
	}

	// Redo both.
	op, _ = s.Redo()
	op.Apply(b)
	op, _ = s.Redo()
	op.Apply(b)
	if strings.Join(b.Lines, "|") != "st|X|Yart" {
		t.Fatalf("after redo all: %#v", b.Lines)
	}
}

func TestRecordClearsRedo(t *testing.T) {
	s := NewUndoStack(0)
	s.Record(Op{Line: 0, Before: []string{"a"}, After: []string{"ab"}})
	s.Undo()
	if !s.CanRedo() {
		t.Fatal("expected redo available before new record")
	}
	s.Record(Op{Line: 0, Before: []string{"a"}, After: []string{"ac"}})
	if s.CanRedo() {
		t.Fatal("new Record must clear redo history")
	}
}

func TestUndoStackLimit(t *testing.T) {
	s := NewUndoStack(3)
	for i := 0; i < 10; i++ {
		s.Record(Op{Line: i})
	}
	if len(s.undo) != 3 {
		t.Fatalf("expected bounded to 3, got %d", len(s.undo))
	}
	// most recent should be the last recorded (Line 9)
	op, _ := s.Undo()
	if op.Line != 9 {
		t.Fatalf("expected most-recent op Line=9, got %d", op.Line)
	}
}

func TestClear(t *testing.T) {
	s := NewUndoStack(0)
	s.Record(Op{Line: 0})
	s.Undo()
	s.Clear()
	if s.CanUndo() || s.CanRedo() {
		t.Fatal("Clear should empty both histories")
	}
}

func TestSnapshotRangeSafe(t *testing.T) {
	b := &Buffer{Lines: []string{"a", "b"}, EOL: "\n"}
	if got := b.Snapshot(1, 10); strings.Join(got, "|") != "b" {
		t.Fatalf("Snapshot overrun wrong: %#v", got)
	}
	if got := b.Snapshot(5, 1); len(got) != 0 {
		t.Fatalf("Snapshot out-of-range should be empty, got %#v", got)
	}
}
