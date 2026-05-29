// Package dlg builds the DOSEdit application dialogs on top of the tcell-only
// internal/tui toolkit. Each constructor composes tui widgets into a *tui.Dialog
// with the primary button wired as the default (Enter) and Cancel/Esc invoking
// the supplied cancel callback. The caller (the app) is responsible for showing
// the dialog via App.PushModal and for closing it from the button callbacks; the
// onOK/onCancel functions passed here are the only side effects this package
// performs.
package dlg

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"dosedit/internal/tui"
)

// ----------------------------------------------------------------------------
// File-system helpers (shared by Open and Save As).
// ----------------------------------------------------------------------------

// dlgResolveDir returns an absolute, cleaned directory path, defaulting to the
// current working directory when start is empty or unusable.
func dlgResolveDir(start string) string {
	if start == "" {
		if cwd, err := os.Getwd(); err == nil {
			return cwd
		}
		return string(filepath.Separator)
	}
	if abs, err := filepath.Abs(start); err == nil {
		return abs
	}
	return start
}

// dlgMatchFilter reports whether name matches the glob filter (case-insensitive).
// An empty filter, "*", or "*.*" matches everything.
func dlgMatchFilter(name, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" || filter == "*" || filter == "*.*" {
		return true
	}
	ok, err := filepath.Match(strings.ToLower(filter), strings.ToLower(name))
	if err != nil {
		return true
	}
	return ok
}

// dlgListEntries reads dir and returns the sorted directory names (with a
// leading ".." entry unless at the filesystem root) and the sorted file names
// matching filter (a glob like "*.*"; empty/"*"/"*.*" means all files).
func dlgListEntries(dir, filter string) (dirs, files []string) {
	dirs = []string{}
	files = []string{}
	if parent := filepath.Dir(dir); parent != dir {
		dirs = append(dirs, "..")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dirs, files
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			dirs = append(dirs, name)
			continue
		}
		if dlgMatchFilter(name, filter) {
			files = append(files, name)
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i] == ".." {
			return true
		}
		if dirs[j] == ".." {
			return false
		}
		return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j])
	})
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i]) < strings.ToLower(files[j])
	})
	return dirs, files
}

// dlgFileState holds the mutable navigation state for a file dialog. It owns the
// name TextBox, the current-path Label, and the Directories and Files ListBoxes.
type dlgFileState struct {
	nameIn  *tui.TextBox
	pathLbl *tui.Label
	dirList *tui.ListBox
	fileLst *tui.ListBox
	dir     string
	filter  string
}

// nameField returns the trimmed "File Name" field value.
func (s *dlgFileState) nameField() string {
	return strings.TrimSpace(s.nameIn.Text())
}

// refresh re-reads the current directory and repopulates the lists and the
// current-path label. Directory names are bracketed (e.g. "[subdir]") so they
// are visually distinct from files.
func (s *dlgFileState) refresh() {
	s.pathLbl.SetText(s.dir)
	dirs, files := dlgListEntries(s.dir, s.filter)

	dirItems := make([]string, len(dirs))
	for i, d := range dirs {
		dirItems[i] = "[" + d + "]"
	}
	s.dirList.SetItems(dirItems)
	s.fileLst.SetItems(files)
}

// enterDir navigates into the directory list row at index i (relative to the
// raw dir names) and refreshes both lists.
func (s *dlgFileState) enterDir(i int) {
	dirs, _ := dlgListEntries(s.dir, s.filter)
	if i < 0 || i >= len(dirs) {
		return
	}
	name := dirs[i]
	var next string
	if name == ".." {
		next = filepath.Dir(s.dir)
	} else {
		next = filepath.Join(s.dir, name)
	}
	s.dir = dlgResolveDir(next)
	s.refresh()
}

// pickFile fills the name field from the files list row at index i.
func (s *dlgFileState) pickFile(i int) {
	_, files := dlgListEntries(s.dir, s.filter)
	if i < 0 || i >= len(files) {
		return
	}
	s.nameIn.SetText(files[i])
}

// chosenPath resolves the current "File Name" field against the current
// directory, returning the absolute path to hand to onOK. Returns "" when the
// field is empty or holds a bare glob ("*.*"/"*"/contains * or ?).
func (s *dlgFileState) chosenPath() string {
	name := s.nameField()
	if name == "" {
		return ""
	}
	if strings.ContainsAny(name, "*?") {
		return ""
	}
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	return filepath.Join(s.dir, name)
}

// dlgBuildFileDialog assembles the shared layout for Open and Save As: a
// "File Name" TextBox, a current-path Label, a Directories ListBox and a Files
// ListBox. Selecting a directory refreshes both lists; selecting a file fills
// the name field. OK resolves the path and calls onOK; Cancel/Esc/Help wire the
// cancel callback. The first (OK) button is the default.
func dlgBuildFileDialog(title, startDir, filter, nameDefault, okLabel string, onOK func(path string), onCancel func()) *tui.Dialog {
	st := &dlgFileState{
		dir:    dlgResolveDir(startDir),
		filter: filter,
	}

	d := tui.NewDialog(title)

	st.nameIn = tui.NewTextBox(nameDefault, 40)
	st.pathLbl = tui.NewLabel("")
	st.dirList = tui.NewListBox(nil)
	st.fileLst = tui.NewListBox(nil)

	// Activating a directory row navigates into it; selecting a file row fills
	// the name field.
	st.dirList.SetOnActivate(func(i int) { st.enterDir(i) })
	st.dirList.SetOnSelect(func(i int) {})
	st.fileLst.SetOnSelect(func(i int) { st.pickFile(i) })
	st.fileLst.SetOnActivate(func(i int) {
		st.pickFile(i)
		if p := st.chosenPath(); p != "" && onOK != nil {
			onOK(p)
		}
	})

	st.refresh()

	d.Add(tui.NewLabel("File Name:"))
	d.Add(st.nameIn)
	d.Add(st.pathLbl)
	d.Add(tui.NewLabel("Directories:"))
	d.Add(st.dirList)
	d.Add(tui.NewLabel("Files:"))
	d.Add(st.fileLst)

	ok := tui.NewButton(okLabel, func() {
		if p := st.chosenPath(); p != "" && onOK != nil {
			onOK(p)
		}
	})
	ok.SetDefault(true)
	cancel := tui.NewButton("Cancel", func() {
		if onCancel != nil {
			onCancel()
		}
	})
	help := tui.NewButton("Help", func() {})

	d.AddButton(ok)
	d.AddButton(cancel)
	d.AddButton(help)
	d.SetDefault(ok)
	d.SetCancel(func() {
		if onCancel != nil {
			onCancel()
		}
	})

	d.SetSize(56, 18)
	return d
}

// NewOpen builds the File Open dialog. The "File Name" field defaults to filter
// (e.g. "*.*"); the Directories and Files lists are populated from startDir via
// os.ReadDir. OK resolves the chosen path and calls onOK; Cancel/Esc call
// onCancel. The app pushes/pops the modal; onOK/onCancel close it.
func NewOpen(app *tui.App, startDir, filter string, onOK func(path string), onCancel func()) *tui.Dialog {
	def := filter
	if def == "" {
		def = "*.*"
	}
	return dlgBuildFileDialog("Open", startDir, filter, def, "OK", onOK, onCancel)
}

// NewSaveAs builds the File Save As dialog. The "File Name" field defaults to
// suggestedName; navigation mirrors NewOpen (filter "*.*" → match all). OK
// resolves the path and calls onOK; Cancel/Esc call onCancel.
func NewSaveAs(app *tui.App, startDir, suggestedName string, onOK func(path string), onCancel func()) *tui.Dialog {
	return dlgBuildFileDialog("Save As", startDir, "*.*", suggestedName, "OK", onOK, onCancel)
}

// ----------------------------------------------------------------------------
// Message box.
// ----------------------------------------------------------------------------

// NewMessage builds a message box showing message under title with one button
// per entry in buttons. Activating button i calls onResult(i); Esc calls
// onResult(-1). The first button is the default.
func NewMessage(title, message string, buttons []string, onResult func(idx int)) *tui.Dialog {
	d := tui.NewDialog(title)

	maxLine := 0
	for _, line := range strings.Split(message, "\n") {
		lbl := tui.NewLabel(line)
		d.Add(lbl)
		if n := lblWidth(line); n > maxLine {
			maxLine = n
		}
	}

	var def *tui.Button
	for i, label := range buttons {
		idx := i
		b := tui.NewButton(label, func() {
			if onResult != nil {
				onResult(idx)
			}
		})
		if i == 0 {
			b.SetDefault(true)
			def = b
		}
		d.AddButton(b)
	}
	if def != nil {
		d.SetDefault(def)
	}
	d.SetCancel(func() {
		if onResult != nil {
			onResult(-1)
		}
	})

	w := maxLine + 8 + 14
	if w < 30 {
		w = 30
	}
	h := len(strings.Split(message, "\n")) + 6
	if h < 7 {
		h = 7
	}
	d.SetSize(w, h)
	return d
}

// ----------------------------------------------------------------------------
// Find / Replace.
// ----------------------------------------------------------------------------

// NewFind builds the Find dialog: a search TextBox plus "Match case" and
// "Whole word" check boxes. The Find button (default) calls onFind with the
// query and flags; Cancel/Esc call onCancel.
func NewFind(initial string, onFind func(q string, matchCase, wholeWord bool), onCancel func()) *tui.Dialog {
	d := tui.NewDialog("Find")

	findIn := tui.NewTextBox(initial, 30)
	matchCase := tui.NewCheckbox("Match case", false)
	wholeWord := tui.NewCheckbox("Whole word", false)

	d.Add(tui.NewLabel("Find What:"))
	d.Add(findIn)
	d.Add(matchCase)
	d.Add(wholeWord)

	find := tui.NewButton("Find", func() {
		if onFind != nil {
			onFind(findIn.Text(), matchCase.IsChecked(), wholeWord.IsChecked())
		}
	})
	find.SetDefault(true)
	cancel := tui.NewButton("Cancel", func() {
		if onCancel != nil {
			onCancel()
		}
	})

	d.AddButton(find)
	d.AddButton(cancel)
	d.SetDefault(find)
	d.SetCancel(func() {
		if onCancel != nil {
			onCancel()
		}
	})

	d.SetSize(48, 9)
	return d
}

// NewReplace builds the Replace dialog: "Find What" and "Replace With" text
// boxes, "Match case"/"Whole word" check boxes, and Replace/Replace All/Cancel
// buttons. Replace (default) calls onReplace with all=false; Replace All calls
// it with all=true; Cancel/Esc call onCancel.
func NewReplace(onReplace func(find, replace string, matchCase, wholeWord, all bool), onCancel func()) *tui.Dialog {
	d := tui.NewDialog("Replace")

	findIn := tui.NewTextBox("", 30)
	replIn := tui.NewTextBox("", 30)
	matchCase := tui.NewCheckbox("Match case", false)
	wholeWord := tui.NewCheckbox("Whole word", false)

	d.Add(tui.NewLabel("Find What:"))
	d.Add(findIn)
	d.Add(tui.NewLabel("Replace With:"))
	d.Add(replIn)
	d.Add(matchCase)
	d.Add(wholeWord)

	fire := func(all bool) {
		if onReplace != nil {
			onReplace(findIn.Text(), replIn.Text(), matchCase.IsChecked(), wholeWord.IsChecked(), all)
		}
	}

	replace := tui.NewButton("Replace", func() { fire(false) })
	replace.SetDefault(true)
	replaceAll := tui.NewButton("Replace All", func() { fire(true) })
	cancel := tui.NewButton("Cancel", func() {
		if onCancel != nil {
			onCancel()
		}
	})

	d.AddButton(replace)
	d.AddButton(replaceAll)
	d.AddButton(cancel)
	d.SetDefault(replace)
	d.SetCancel(func() {
		if onCancel != nil {
			onCancel()
		}
	})

	d.SetSize(52, 12)
	return d
}

// ----------------------------------------------------------------------------
// Go to Line.
// ----------------------------------------------------------------------------

// NewGotoLine builds the Go to Line dialog: a numeric TextBox. OK (default)
// parses the field to an int and calls onOK; a non-numeric or empty field is
// ignored. Cancel/Esc call onCancel.
func NewGotoLine(onOK func(line int), onCancel func()) *tui.Dialog {
	d := tui.NewDialog("Go to Line")

	lineIn := tui.NewTextBox("", 10)
	d.Add(tui.NewLabel("Line Number:"))
	d.Add(lineIn)

	ok := tui.NewButton("OK", func() {
		n, err := strconv.Atoi(strings.TrimSpace(lineIn.Text()))
		if err == nil && onOK != nil {
			onOK(n)
		}
	})
	ok.SetDefault(true)
	cancel := tui.NewButton("Cancel", func() {
		if onCancel != nil {
			onCancel()
		}
	})

	d.AddButton(ok)
	d.AddButton(cancel)
	d.SetDefault(ok)
	d.SetCancel(func() {
		if onCancel != nil {
			onCancel()
		}
	})

	d.SetSize(36, 8)
	return d
}

// ----------------------------------------------------------------------------
// About.
// ----------------------------------------------------------------------------

// NewAbout builds the About dialog: static product labels and an OK button
// (default) that calls onOK. Esc also calls onOK (the only dismissal).
func NewAbout(onOK func()) *tui.Dialog {
	d := tui.NewDialog("About DOSEdit")

	d.Add(tui.NewLabel("DOSEdit"))
	d.Add(tui.NewLabel("A Visual Basic for DOS-style text editor"))
	d.Add(tui.NewLabel("Version 1.1"))

	ok := tui.NewButton("OK", func() {
		if onOK != nil {
			onOK()
		}
	})
	ok.SetDefault(true)
	d.AddButton(ok)
	d.SetDefault(ok)
	d.SetCancel(func() {
		if onOK != nil {
			onOK()
		}
	})

	d.SetSize(46, 9)
	return d
}

// ----------------------------------------------------------------------------
// Options.
// ----------------------------------------------------------------------------

// Options holds the editor settings exposed by the Options dialog.
type Options struct {
	LineNumbers bool
}

// NewOptions builds the Options dialog seeded from cur: a "Line Numbers" check
// box plus OK (default)/Cancel. OK calls onOK with the new Options; Cancel/Esc
// call onCancel.
func NewOptions(cur Options, onOK func(Options), onCancel func()) *tui.Dialog {
	d := tui.NewDialog("Options")

	lineNums := tui.NewCheckbox("Line Numbers", cur.LineNumbers)
	d.Add(lineNums)

	ok := tui.NewButton("OK", func() {
		if onOK != nil {
			onOK(Options{LineNumbers: lineNums.IsChecked()})
		}
	})
	ok.SetDefault(true)
	cancel := tui.NewButton("Cancel", func() {
		if onCancel != nil {
			onCancel()
		}
	})

	d.AddButton(ok)
	d.AddButton(cancel)
	d.SetDefault(ok)
	d.SetCancel(func() {
		if onCancel != nil {
			onCancel()
		}
	})

	d.SetSize(40, 8)
	return d
}

// lblWidth returns the rune count of s (used for sizing message boxes).
func lblWidth(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
