// Package buffer implements the pure text-buffer model for DOSEdit (spec §9
// and Appendix B). It holds document text as a slice of lines (no trailing
// newline stored), tracks the originating line-ending style, and provides
// rune-aware editing primitives plus UTF-8 file load/save.
//
// This package is intentionally free of any UI dependency (no tcell / tview /
// winman). It uses only the Go standard library so it can be unit-tested and
// reasoned about in isolation.
package buffer

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// untitledCounter backs NewUntitled's deterministic "UntitledN" naming. It is
// process-global and concurrency-safe.
var untitledCounter uint64

// Buffer is the in-memory representation of one open document.
//
// Lines holds one entry per logical line with NO trailing newline character;
// an N-line file is represented by N entries, and an empty document is the
// single entry []string{""}.
type Buffer struct {
	Lines     []string // one entry per line, NO trailing newline stored
	Path      string   // "" if untitled
	Title     string   // "Untitled1" etc. when Path == ""
	Modified  bool     // dirty flag; drives the "*" title prefix in the UI
	EOL       string   // "\n" (LF) or "\r\n" (CRLF); preserved from load
	TabWidth  int      // display width of a tab; default 4
	UseSpaces bool     // if true, the editor inserts spaces for Tab
}

// NewUntitled returns a fresh, empty, untitled buffer. Each call assigns the
// next "Untitled1", "Untitled2", … title via a global counter, so naming is
// deterministic within a process run and safe under concurrent use.
func NewUntitled() *Buffer {
	n := atomic.AddUint64(&untitledCounter, 1)
	return &Buffer{
		Lines:    []string{""},
		Title:    "Untitled" + itoa(int(n)),
		EOL:      "\n",
		TabWidth: 4,
	}
}

// itoa is a tiny dependency-free integer formatter (avoids importing strconv
// purely for naming, keeping intent explicit). n is always >= 1 here.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Load reads a UTF-8 text file from path into a new Buffer. The line-ending
// style is detected ("\r\n" if any CRLF is present in the file, otherwise
// "\n") and stored in EOL so it can be preserved on save. The content is split
// into Lines with no trailing newline retained; an empty file yields
// []string{""}. Modified is false on a freshly loaded buffer.
func Load(path string) (*Buffer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)

	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}

	// Normalise CRLF to LF, then split. This keeps the splitting logic simple
	// and handles a possible lone trailing newline correctly.
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	var lines []string
	if norm == "" {
		lines = []string{""}
	} else {
		lines = strings.Split(norm, "\n")
		// A file ending in a newline produces a trailing empty element from
		// Split; that represents "no content after the final newline" and is
		// dropped so a 1-line file "abc\n" round-trips to ["abc"].
		if len(lines) > 1 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}

	return &Buffer{
		Lines:    lines,
		Path:     path,
		Title:    filepath.Base(path),
		EOL:      eol,
		TabWidth: 4,
	}, nil
}

// Save writes the buffer to its current Path, joining Lines with EOL, and
// clears Modified. It is an error to Save a buffer that has no Path (use
// SaveAs). The file is written with 0644 permissions.
func (b *Buffer) Save() error {
	if b.Path == "" {
		return &os.PathError{Op: "save", Path: "", Err: os.ErrInvalid}
	}
	eol := b.EOL
	if eol == "" {
		eol = "\n"
	}
	data := strings.Join(b.Lines, eol)
	if err := os.WriteFile(b.Path, []byte(data), 0o644); err != nil {
		return err
	}
	b.Modified = false
	return nil
}

// SaveAs writes the buffer to path, updates Path and Title to match, and
// clears Modified. If EOL is empty it defaults to LF.
func (b *Buffer) SaveAs(path string) error {
	if b.EOL == "" {
		b.EOL = "\n"
	}
	data := strings.Join(b.Lines, b.EOL)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		return err
	}
	b.Path = path
	b.Title = filepath.Base(path)
	b.Modified = false
	return nil
}

// DisplayName returns the bare document name for the title bar / window list:
// Title for untitled buffers, otherwise the base name of Path. The "*"
// modified prefix is applied by the UI layer, not here.
func (b *Buffer) DisplayName() string {
	if b.Path != "" {
		return filepath.Base(b.Path)
	}
	return b.Title
}

// LineCount reports the number of lines in the buffer (always >= 1).
func (b *Buffer) LineCount() int { return len(b.Lines) }

// Line returns the text of line i, or "" if i is out of range.
func (b *Buffer) Line(i int) string {
	if i < 0 || i >= len(b.Lines) {
		return ""
	}
	return b.Lines[i]
}

// --- editing primitives ---------------------------------------------------
//
// All column arguments are RUNE offsets (0-based) into the given line, not
// byte offsets. Out-of-range indices are clamped rather than panicking, so the
// caller's cursor math can be forgiving. Mutating operations set Modified.

// clampLine returns line clamped into [0, len(Lines)-1].
func (b *Buffer) clampLine(line int) int {
	if line < 0 {
		return 0
	}
	if line >= len(b.Lines) {
		return len(b.Lines) - 1
	}
	return line
}

// runesOf returns the rune slice of line i (after clamping).
func (b *Buffer) runesOf(line int) []rune {
	return []rune(b.Lines[b.clampLine(line)])
}

// clampCol clamps col into [0, n].
func clampCol(col, n int) int {
	if col < 0 {
		return 0
	}
	if col > n {
		return n
	}
	return col
}

// InsertRune inserts r at the given rune column on line. The column is clamped
// to the line's length.
func (b *Buffer) InsertRune(line, col int, r rune) {
	line = b.clampLine(line)
	rs := []rune(b.Lines[line])
	col = clampCol(col, len(rs))
	out := make([]rune, 0, len(rs)+1)
	out = append(out, rs[:col]...)
	out = append(out, r)
	out = append(out, rs[col:]...)
	b.Lines[line] = string(out)
	b.Modified = true
}

// InsertString inserts s (which must not contain newlines) at the given rune
// column on line. For multi-line text use InsertText.
func (b *Buffer) InsertString(line, col int, s string) {
	if s == "" {
		return
	}
	line = b.clampLine(line)
	rs := []rune(b.Lines[line])
	col = clampCol(col, len(rs))
	b.Lines[line] = string(rs[:col]) + s + string(rs[col:])
	b.Modified = true
}

// SplitLine breaks line at the given rune column (Enter): text before col stays
// on line, text from col onward becomes a new following line.
func (b *Buffer) SplitLine(line, col int) {
	line = b.clampLine(line)
	rs := []rune(b.Lines[line])
	col = clampCol(col, len(rs))
	head := string(rs[:col])
	tail := string(rs[col:])
	b.Lines[line] = head
	// insert tail as a new line after `line`
	b.Lines = append(b.Lines, "")
	copy(b.Lines[line+2:], b.Lines[line+1:])
	b.Lines[line+1] = tail
	b.Modified = true
}

// DeleteRune deletes the rune at the given column on line (forward Delete). If
// col is at end of line and a following line exists, the following line is
// joined onto this one. No-op at end of the final line.
func (b *Buffer) DeleteRune(line, col int) {
	line = b.clampLine(line)
	rs := []rune(b.Lines[line])
	if col < 0 {
		col = 0
	}
	if col < len(rs) {
		out := append(rs[:col], rs[col+1:]...)
		b.Lines[line] = string(out)
		b.Modified = true
		return
	}
	// col >= len: delete the line break by joining the next line up.
	if line+1 < len(b.Lines) {
		b.Lines[line] = b.Lines[line] + b.Lines[line+1]
		b.Lines = append(b.Lines[:line+1], b.Lines[line+2:]...)
		b.Modified = true
	}
}

// JoinLine merges line into the previous line (Backspace at column 0). The
// returned column is the rune offset in the previous line at which the two
// lines were joined (i.e. the new cursor column). For line <= 0 it is a no-op
// returning 0.
func (b *Buffer) JoinLine(line int) int {
	if line <= 0 || line >= len(b.Lines) {
		return 0
	}
	prevLen := len([]rune(b.Lines[line-1]))
	b.Lines[line-1] = b.Lines[line-1] + b.Lines[line]
	b.Lines = append(b.Lines[:line], b.Lines[line+1:]...)
	b.Modified = true
	return prevLen
}

// orderPositions returns the two (line,col) positions in document order so
// callers may pass a selection in either direction.
func orderPositions(sl, sc, el, ec int) (int, int, int, int) {
	if el < sl || (el == sl && ec < sc) {
		return el, ec, sl, sc
	}
	return sl, sc, el, ec
}

// DeleteRange removes the text from (startLine,startCol) up to but not
// including (endLine,endCol) and returns the removed text (newlines joined with
// "\n"). The two endpoints may be given in either order. Indices are clamped to
// valid ranges. After the call the cursor belongs at (startLine,startCol).
func (b *Buffer) DeleteRange(startLine, startCol, endLine, endCol int) string {
	sl, sc, el, ec := orderPositions(startLine, startCol, endLine, endCol)
	sl = b.clampLine(sl)
	el = b.clampLine(el)
	sRunes := []rune(b.Lines[sl])
	eRunes := []rune(b.Lines[el])
	sc = clampCol(sc, len(sRunes))
	ec = clampCol(ec, len(eRunes))

	var removed string
	if sl == el {
		removed = string(sRunes[sc:ec])
		b.Lines[sl] = string(sRunes[:sc]) + string(eRunes[ec:])
	} else {
		// First line tail + full middle lines + last line head.
		var sb strings.Builder
		sb.WriteString(string(sRunes[sc:]))
		for l := sl + 1; l < el; l++ {
			sb.WriteByte('\n')
			sb.WriteString(b.Lines[l])
		}
		sb.WriteByte('\n')
		sb.WriteString(string(eRunes[:ec]))
		removed = sb.String()

		merged := string(sRunes[:sc]) + string(eRunes[ec:])
		b.Lines[sl] = merged
		b.Lines = append(b.Lines[:sl+1], b.Lines[el+1:]...)
	}
	b.Modified = true
	return removed
}

// InsertText inserts possibly-multi-line text at (line,col). Newlines may be
// "\n" or "\r\n"; both are recognised as line breaks. It returns the cursor
// position (endLine,endCol) immediately after the inserted text.
func (b *Buffer) InsertText(line, col int, text string) (endLine, endCol int) {
	line = b.clampLine(line)
	rs := []rune(b.Lines[line])
	col = clampCol(col, len(rs))

	if text == "" {
		return line, col
	}

	norm := strings.ReplaceAll(text, "\r\n", "\n")
	parts := strings.Split(norm, "\n")

	head := string(rs[:col])
	tail := string(rs[col:])

	if len(parts) == 1 {
		b.Lines[line] = head + parts[0] + tail
		b.Modified = true
		return line, col + len([]rune(parts[0]))
	}

	// Multi-line: first part appends to head; last part prepends to tail; the
	// middle parts become whole new lines in between.
	first := head + parts[0]
	last := parts[len(parts)-1]
	middle := parts[1 : len(parts)-1]

	newLines := make([]string, 0, len(middle)+1)
	newLines = append(newLines, middle...)
	newLines = append(newLines, last+tail)

	b.Lines[line] = first
	// splice newLines in after `line`
	rest := append([]string(nil), b.Lines[line+1:]...)
	b.Lines = append(b.Lines[:line+1], newLines...)
	b.Lines = append(b.Lines, rest...)

	b.Modified = true
	endLine = line + len(parts) - 1
	endCol = len([]rune(last))
	return endLine, endCol
}
