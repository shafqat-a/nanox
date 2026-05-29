package buffer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewUntitledNaming(t *testing.T) {
	a := NewUntitled()
	b := NewUntitled()
	if a.Title == b.Title {
		t.Fatalf("expected distinct titles, got %q twice", a.Title)
	}
	if !strings.HasPrefix(a.Title, "Untitled") || !strings.HasPrefix(b.Title, "Untitled") {
		t.Fatalf("unexpected titles: %q %q", a.Title, b.Title)
	}
	if len(a.Lines) != 1 || a.Lines[0] != "" {
		t.Fatalf("new buffer should be one empty line, got %#v", a.Lines)
	}
	if a.EOL != "\n" || a.TabWidth != 4 {
		t.Fatalf("defaults wrong: EOL=%q TabWidth=%d", a.EOL, a.TabWidth)
	}
	if a.DisplayName() != a.Title {
		t.Fatalf("DisplayName mismatch: %q vs %q", a.DisplayName(), a.Title)
	}
}

func TestLoadDetectLF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "lf.txt")
	if err := os.WriteFile(p, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if b.EOL != "\n" {
		t.Fatalf("expected LF, got %q", b.EOL)
	}
	if got := strings.Join(b.Lines, "|"); got != "a|b|c" {
		t.Fatalf("lines wrong: %q", got)
	}
	if b.Modified {
		t.Fatal("freshly loaded buffer must not be modified")
	}
	if b.DisplayName() != "lf.txt" {
		t.Fatalf("DisplayName wrong: %q", b.DisplayName())
	}
}

func TestLoadDetectCRLF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "crlf.txt")
	if err := os.WriteFile(p, []byte("x\r\ny\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if b.EOL != "\r\n" {
		t.Fatalf("expected CRLF, got %q", b.EOL)
	}
	if got := strings.Join(b.Lines, "|"); got != "x|y" {
		t.Fatalf("lines wrong: %q", got)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Lines) != 1 || b.Lines[0] != "" {
		t.Fatalf("empty file should be one empty line, got %#v", b.Lines)
	}
}

func TestSaveRoundTripLF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	b := &Buffer{Lines: []string{"one", "two", "three"}, Path: p, EOL: "\n", Modified: true}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if b.Modified {
		t.Fatal("Save should clear Modified")
	}
	data, _ := os.ReadFile(p)
	if string(data) != "one\ntwo\nthree" {
		t.Fatalf("unexpected file content: %q", string(data))
	}
	rb, _ := Load(p)
	if strings.Join(rb.Lines, "|") != "one|two|three" || rb.EOL != "\n" {
		t.Fatalf("round trip failed: %#v eol=%q", rb.Lines, rb.EOL)
	}
}

func TestSaveRoundTripCRLF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	b := &Buffer{Lines: []string{"one", "two"}, Path: p, EOL: "\r\n"}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "one\r\ntwo" {
		t.Fatalf("unexpected content: %q", string(data))
	}
	rb, _ := Load(p)
	if rb.EOL != "\r\n" {
		t.Fatalf("CRLF not preserved: %q", rb.EOL)
	}
}

func TestSaveNoPath(t *testing.T) {
	b := NewUntitled()
	if err := b.Save(); err == nil {
		t.Fatal("expected error saving buffer with no path")
	}
}

func TestSaveAsUpdatesPathTitle(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "saved.bas")
	b := NewUntitled()
	b.Lines = []string{"hi"}
	if err := b.SaveAs(p); err != nil {
		t.Fatal(err)
	}
	if b.Path != p || b.Title != "saved.bas" || b.Modified {
		t.Fatalf("SaveAs state wrong: path=%q title=%q mod=%v", b.Path, b.Title, b.Modified)
	}
	if b.EOL != "\n" {
		t.Fatalf("new file should default to LF, got %q", b.EOL)
	}
}

func TestInsertRuneAndString(t *testing.T) {
	b := &Buffer{Lines: []string{"héllo"}, EOL: "\n"}
	b.InsertRune(0, 1, 'X') // rune-aware: after 'h'
	if b.Lines[0] != "hXéllo" {
		t.Fatalf("InsertRune wrong: %q", b.Lines[0])
	}
	if !b.Modified {
		t.Fatal("InsertRune should set Modified")
	}
	b.InsertString(0, 0, ">>")
	if b.Lines[0] != ">>hXéllo" {
		t.Fatalf("InsertString wrong: %q", b.Lines[0])
	}
	// clamping past end
	b.InsertRune(0, 999, '!')
	if !strings.HasSuffix(b.Lines[0], "!") {
		t.Fatalf("clamp insert wrong: %q", b.Lines[0])
	}
}

func TestSplitLine(t *testing.T) {
	b := &Buffer{Lines: []string{"abcdef"}, EOL: "\n"}
	b.SplitLine(0, 3)
	if len(b.Lines) != 2 || b.Lines[0] != "abc" || b.Lines[1] != "def" {
		t.Fatalf("SplitLine wrong: %#v", b.Lines)
	}
	// split at end of first line
	b.SplitLine(0, 3)
	if b.Lines[0] != "abc" || b.Lines[1] != "" {
		t.Fatalf("SplitLine at end wrong: %#v", b.Lines)
	}
}

func TestDeleteRune(t *testing.T) {
	b := &Buffer{Lines: []string{"abc", "def"}, EOL: "\n"}
	b.DeleteRune(0, 1) // delete 'b'
	if b.Lines[0] != "ac" {
		t.Fatalf("DeleteRune wrong: %q", b.Lines[0])
	}
	// delete at end of line joins next
	b.DeleteRune(0, 2)
	if len(b.Lines) != 1 || b.Lines[0] != "acdef" {
		t.Fatalf("DeleteRune join wrong: %#v", b.Lines)
	}
	// delete at end of final line is a no-op
	before := b.Lines[0]
	b.DeleteRune(0, 99)
	if b.Lines[0] != before {
		t.Fatalf("expected no-op, got %q", b.Lines[0])
	}
}

func TestJoinLine(t *testing.T) {
	b := &Buffer{Lines: []string{"abc", "def"}, EOL: "\n"}
	col := b.JoinLine(1)
	if col != 3 {
		t.Fatalf("JoinLine returned col %d, want 3", col)
	}
	if len(b.Lines) != 1 || b.Lines[0] != "abcdef" {
		t.Fatalf("JoinLine wrong: %#v", b.Lines)
	}
	if c := b.JoinLine(0); c != 0 {
		t.Fatalf("JoinLine(0) should be no-op returning 0, got %d", c)
	}
}

func TestDeleteRangeSingleLine(t *testing.T) {
	b := &Buffer{Lines: []string{"hello world"}, EOL: "\n"}
	removed := b.DeleteRange(0, 5, 0, 11) // " world"
	if removed != " world" {
		t.Fatalf("removed wrong: %q", removed)
	}
	if b.Lines[0] != "hello" {
		t.Fatalf("line wrong: %q", b.Lines[0])
	}
}

func TestDeleteRangeMultiLine(t *testing.T) {
	b := &Buffer{Lines: []string{"abcXX", "middle", "YYdef"}, EOL: "\n"}
	removed := b.DeleteRange(0, 3, 2, 2)
	if removed != "XX\nmiddle\nYY" {
		t.Fatalf("removed wrong: %q", removed)
	}
	if len(b.Lines) != 1 || b.Lines[0] != "abcdef" {
		t.Fatalf("result wrong: %#v", b.Lines)
	}
}

func TestDeleteRangeReversedEndpoints(t *testing.T) {
	b := &Buffer{Lines: []string{"hello world"}, EOL: "\n"}
	// endpoints given backwards should behave the same
	removed := b.DeleteRange(0, 11, 0, 5)
	if removed != " world" || b.Lines[0] != "hello" {
		t.Fatalf("reversed range wrong: removed=%q line=%q", removed, b.Lines[0])
	}
}

func TestInsertTextSingleLine(t *testing.T) {
	b := &Buffer{Lines: []string{"abef"}, EOL: "\n"}
	el, ec := b.InsertText(0, 2, "cd")
	if b.Lines[0] != "abcdef" {
		t.Fatalf("line wrong: %q", b.Lines[0])
	}
	if el != 0 || ec != 4 {
		t.Fatalf("end pos wrong: %d,%d", el, ec)
	}
}

func TestInsertTextMultiLine(t *testing.T) {
	b := &Buffer{Lines: []string{"abXcd"}, EOL: "\n"}
	el, ec := b.InsertText(0, 2, "1\n2\n3")
	want := []string{"ab1", "2", "3Xcd"}
	if strings.Join(b.Lines, "|") != strings.Join(want, "|") {
		t.Fatalf("lines wrong: %#v", b.Lines)
	}
	if el != 2 || ec != 1 {
		t.Fatalf("end pos wrong: %d,%d (want 2,1)", el, ec)
	}
}

func TestInsertTextCRLFNormalised(t *testing.T) {
	b := &Buffer{Lines: []string{""}, EOL: "\n"}
	el, _ := b.InsertText(0, 0, "p\r\nq")
	if len(b.Lines) != 2 || b.Lines[0] != "p" || b.Lines[1] != "q" || el != 1 {
		t.Fatalf("CRLF paste wrong: %#v end=%d", b.Lines, el)
	}
}

func TestLineHelpers(t *testing.T) {
	b := &Buffer{Lines: []string{"a", "b"}, EOL: "\n"}
	if b.LineCount() != 2 {
		t.Fatalf("LineCount wrong")
	}
	if b.Line(1) != "b" || b.Line(99) != "" {
		t.Fatalf("Line helper wrong")
	}
}
