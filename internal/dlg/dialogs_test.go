package dlg

import (
	"os"
	"path/filepath"
	"testing"

	"dosedit/internal/tui"

	"github.com/gdamore/tcell/v2"
)

// newTestApp returns a tui.App backed by an initialised SimulationScreen.
func newTestApp(t *testing.T) *tui.App {
	t.Helper()
	scr := tcell.NewSimulationScreen("")
	if err := scr.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	scr.SetSize(80, 25)
	return tui.NewApp(scr)
}

// sensibleSize asserts the dialog has a positive, non-trivial size.
func sensibleSize(t *testing.T, d *tui.Dialog, name string) {
	t.Helper()
	if d == nil {
		t.Fatalf("%s: dialog is nil", name)
	}
	w, h := d.Size()
	if w < 10 || h < 4 {
		t.Fatalf("%s: implausible size %dx%d", name, w, h)
	}
}

func TestNewMessageBuilds(t *testing.T) {
	got := -99
	d := NewMessage("Confirm", "Save changes?\nThis cannot be undone.",
		[]string{"Yes", "No", "Cancel"}, func(i int) { got = i })
	sensibleSize(t, d, "NewMessage")

	// Esc → onResult(-1).
	d.HandleKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if got != -1 {
		t.Fatalf("Esc: want onResult(-1), got %d", got)
	}
}

func TestNewFindReplaceGotoAboutBuild(t *testing.T) {
	sensibleSize(t, NewFind("foo", func(string, bool, bool) {}, func() {}), "NewFind")
	sensibleSize(t, NewReplace(func(string, string, bool, bool, bool) {}, func() {}), "NewReplace")
	sensibleSize(t, NewAbout(func() {}), "NewAbout")
}

func TestNewGotoLineParses(t *testing.T) {
	var got int
	called := false
	d := NewGotoLine(func(n int) { got = n; called = true }, func() {})
	sensibleSize(t, d, "NewGotoLine")

	// Type "42" into the focused field then activate the default button (Enter
	// routes through the dialog to the default button).
	app := newTestApp(t)
	app.PushModal(d)
	for _, r := range "42" {
		app.Focused().HandleKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
	d.HandleKey(tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
	if !called || got != 42 {
		t.Fatalf("GotoLine: want onOK(42), called=%v got=%d", called, got)
	}
}

func TestNewGotoLineRejectsNonNumeric(t *testing.T) {
	called := false
	d := NewGotoLine(func(int) { called = true }, func() {})
	app := newTestApp(t)
	app.PushModal(d)
	for _, r := range "abc" {
		app.Focused().HandleKey(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
	d.HandleKey(tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
	if called {
		t.Fatalf("GotoLine: non-numeric input should not call onOK")
	}
}

func TestNewOptionsToggleAndOK(t *testing.T) {
	var got Options
	called := false
	d := NewOptions(Options{LineNumbers: false},
		func(o Options) { got = o; called = true }, func() {})
	sensibleSize(t, d, "NewOptions")

	app := newTestApp(t)
	app.PushModal(d)
	// The Line Numbers checkbox is the first focusable widget.
	app.Focused().HandleKey(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	// Activate the default OK button via Enter on the dialog.
	d.HandleKey(tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
	if !called {
		t.Fatalf("Options: onOK not called")
	}
	if !got.LineNumbers {
		t.Fatalf("Options: want LineNumbers=true after toggle, got %+v", got)
	}
}

func TestNewOptionsSeedsFromCurrent(t *testing.T) {
	var got Options
	d := NewOptions(Options{LineNumbers: true}, func(o Options) { got = o }, func() {})
	app := newTestApp(t)
	app.PushModal(d)
	// Without toggling, OK should report the seeded value.
	d.HandleKey(tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone))
	if !got.LineNumbers {
		t.Fatalf("Options: want seeded LineNumbers=true, got %+v", got)
	}
}

func TestNewOpenPopulatesLists(t *testing.T) {
	dir := t.TempDir()
	// A subdirectory and a couple of files.
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"alpha.bas", "beta.bas", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	app := newTestApp(t)
	var chosen string
	d := NewOpen(app, dir, "*.bas", func(p string) { chosen = p }, func() {})
	sensibleSize(t, d, "NewOpen")

	// Reach into the dialog's lists by reconstructing the same state the
	// constructor builds. We instead verify behaviour through dlgListEntries
	// (the data source) and the chosenPath resolution, since the widgets are
	// not exported. Verify the directory listing includes ".." and "subdir".
	dirs, files := dlgListEntries(dlgResolveDir(dir), "*.bas")
	hasDotDot, hasSub := false, false
	for _, dd := range dirs {
		if dd == ".." {
			hasDotDot = true
		}
		if dd == "subdir" {
			hasSub = true
		}
	}
	if !hasDotDot || !hasSub {
		t.Fatalf("dir list missing entries: %v", dirs)
	}
	// Filter "*.bas" should drop notes.txt.
	if len(files) != 2 {
		t.Fatalf("want 2 .bas files, got %v", files)
	}
	for _, f := range files {
		if filepath.Ext(f) != ".bas" {
			t.Fatalf("unexpected file %q passed filter", f)
		}
	}

	// chosenPath: a state with a picked file resolves to an absolute path.
	st := &dlgFileState{dir: dlgResolveDir(dir), filter: "*.bas", nameIn: tui.NewTextBox("alpha.bas", 40)}
	if p := st.chosenPath(); p != filepath.Join(dlgResolveDir(dir), "alpha.bas") {
		t.Fatalf("chosenPath = %q", p)
	}
	_ = chosen
}

func TestNewSaveAsBuilds(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	d := NewSaveAs(app, dir, "Untitled1.bas", func(string) {}, func() {})
	sensibleSize(t, d, "NewSaveAs")
}

func TestMatchFilter(t *testing.T) {
	cases := []struct {
		name, filter string
		want         bool
	}{
		{"a.bas", "*.bas", true},
		{"a.txt", "*.bas", false},
		{"a.txt", "*.*", true},
		{"a.txt", "", true},
		{"a.txt", "*", true},
		{"README", "*.bas", false},
	}
	for _, c := range cases {
		if got := dlgMatchFilter(c.name, c.filter); got != c.want {
			t.Errorf("dlgMatchFilter(%q,%q)=%v want %v", c.name, c.filter, got, c.want)
		}
	}
}
