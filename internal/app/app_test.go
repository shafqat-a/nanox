package app

import (
	"strings"
	"testing"
	"time"

	"dosedit/internal/ui"
	"dosedit/internal/ui/wm"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// newTestApp builds a fully wired App backed by an 80x25 simulation screen and
// returns it together with the tview application and the screen. It mirrors the
// wiring in main.run so the test exercises the real assembly path.
func newTestApp(t *testing.T) (*App, *tview.Application, tcell.SimulationScreen) {
	t.Helper()

	// Do not call sim.Init here: tview.Application.Run initialises the screen
	// itself, and initialising a SimulationScreen twice deadlocks. SetSize before
	// SetScreen establishes the 80x25 reference geometry.
	sim := tcell.NewSimulationScreen("")
	sim.SetSize(80, 25)

	tapp := tview.NewApplication()
	tapp.SetScreen(sim)

	manager := wm.NewManager()
	statusbar := ui.NewStatusBar()
	a := New(tapp, manager, statusbar)
	a.OpenInitialWindow()

	tapp.SetRoot(a.Root(), true)
	tapp.SetFocus(a.Root())
	return a, tapp, sim
}

// runApp starts the tview loop in a goroutine and returns a channel that
// receives the Run error when the loop exits. The caller is responsible for
// stopping the app (e.g. by injecting Alt+X) so the goroutine terminates.
func runApp(tapp *tview.Application) <-chan error {
	done := make(chan error, 1)
	go func() { done <- tapp.Run() }()
	return done
}

// screenText flattens the whole simulation screen into a single string.
func screenText(sim tcell.SimulationScreen) string {
	cells, w, h := sim.GetContents()
	buf := make([]rune, 0, w*h+h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				buf = append(buf, c.Runes[0])
			} else {
				buf = append(buf, ' ')
			}
		}
		buf = append(buf, '\n')
	}
	return string(buf)
}

// rowText returns the rendered text of a single screen row.
func rowText(sim tcell.SimulationScreen, row int) string {
	cells, w, h := sim.GetContents()
	if row < 0 || row >= h {
		return ""
	}
	buf := make([]rune, 0, w)
	for x := 0; x < w; x++ {
		c := cells[row*w+x]
		if len(c.Runes) > 0 {
			buf = append(buf, c.Runes[0])
		} else {
			buf = append(buf, ' ')
		}
	}
	return string(buf)
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// query runs fn on the tview loop and returns its result, so tests can read App
// state without racing the loop goroutine.
func query[T any](tapp *tview.Application, fn func() T) T {
	got := make(chan T, 1)
	tapp.QueueUpdate(func() { got <- fn() })
	return <-got
}

// waitFor polls cond until it is true, failing the test on timeout. It also
// fails fast if the app loop has already exited.
func waitFor(t *testing.T, cond func() bool, what string, done <-chan error) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if cond() {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("app loop exited while waiting for %s: %v", what, err)
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		case <-tick.C:
		}
	}
}

// TestBootsAndExits proves the app boots to the three-region layout with one
// Untitled window and that Alt+X stops it cleanly.
func TestBootsAndExits(t *testing.T) {
	_, tapp, sim := newTestApp(t)
	done := runApp(tapp)

	waitFor(t, func() bool {
		return contains(query(tapp, func() string { return rowText(sim, 0) }), "File")
	}, "menu bar to render", done)

	menu := query(tapp, func() string { return rowText(sim, 0) })
	for _, title := range []string{"File", "Edit", "Search", "Window", "Help"} {
		if !contains(menu, title) {
			t.Errorf("menu bar row missing %q; got %q", title, menu)
		}
	}

	if status := query(tapp, func() string { return rowText(sim, 24) }); !contains(status, "F1=Help") {
		t.Errorf("status bar row missing hints; got %q", status)
	}

	waitFor(t, func() bool {
		return contains(query(tapp, func() string { return screenText(sim) }), "Untitled1")
	}, "initial editor window to render", done)

	// Alt+X must stop the app cleanly.
	tapp.QueueEvent(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModAlt))
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		tapp.Stop()
		t.Fatal("app did not stop within timeout after Alt+X")
	}
}

// TestNewWindowCommand verifies File>New creates an additional window.
func TestNewWindowCommand(t *testing.T) {
	a, tapp, _ := newTestApp(t)
	done := runApp(tapp)
	defer func() {
		tapp.Stop()
		<-done
	}()

	tapp.QueueUpdateDraw(func() { a.cmdNew() })
	waitFor(t, func() bool {
		return query(tapp, func() int { return len(a.windows) }) == 2
	}, "second window to be created", done)
}

// TestModalBlocksBackgroundMouse proves true modality: while a dialog is open, a
// left-click on a background editor window (outside the dialog rect) is
// swallowed by the UIManager and must NOT change focus or the active window —
// and the dialog must stay open.
func TestModalBlocksBackgroundMouse(t *testing.T) {
	a, tapp, sim := newTestApp(t)
	tapp.EnableMouse(true)
	done := runApp(tapp)
	defer func() {
		tapp.Stop()
		<-done
	}()

	waitFor(t, func() bool {
		return query(tapp, func() bool { return len(a.windows) == 1 })
	}, "initial window to exist", done)

	beforeActive := query(tapp, func() *wm.Window { return a.wm.Active() })

	tapp.QueueUpdateDraw(func() { a.cmdAbout() })
	waitFor(t, func() bool {
		return query(tapp, func() bool { return a.ui.DialogOpen() })
	}, "dialog to open", done)

	// The UIManager must remain the sole tview focus throughout.
	if f := query(tapp, func() tview.Primitive { return tapp.GetFocus() }); f != tview.Primitive(a.ui) {
		t.Fatalf("UIManager should be the tview focus; got %T", f)
	}

	// Click the background window title bar area (row 2, col 3) — outside the
	// centred About dialog.
	sim.InjectMouse(3, 2, tcell.Button1, tcell.ModNone)
	sim.InjectMouse(3, 2, tcell.ButtonNone, tcell.ModNone)

	// Allow the loop to process, then assert state is unchanged.
	deadline := time.After(500 * time.Millisecond)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			type state struct {
				dialog  bool
				focus   tview.Primitive
				menuAct bool
				active  *wm.Window
			}
			st := query(tapp, func() state {
				return state{a.ui.DialogOpen(), tapp.GetFocus(), a.ui.MenuActive(), a.wm.Active()}
			})
			if !st.dialog {
				t.Fatal("dialog closed after background click; should stay modal")
			}
			if st.menuAct {
				t.Fatal("menu bar became active after background click; modality leaked")
			}
			if st.focus != tview.Primitive(a.ui) {
				t.Fatalf("focus leaked off the UIManager after background click: got %T", st.focus)
			}
			if st.active != beforeActive {
				t.Fatal("active window changed after background click; modality leaked")
			}
			return
		case err := <-done:
			t.Fatalf("app loop exited unexpectedly: %v", err)
		case <-tick.C:
		}
	}
}

// TestEscClosesDialog proves Esc cancels (pops) an open dialog.
func TestEscClosesDialog(t *testing.T) {
	a, tapp, _ := newTestApp(t)
	done := runApp(tapp)
	defer func() {
		tapp.Stop()
		<-done
	}()

	waitFor(t, func() bool {
		return query(tapp, func() bool { return len(a.windows) == 1 })
	}, "initial window", done)

	tapp.QueueUpdateDraw(func() { a.cmdAbout() })
	waitFor(t, func() bool {
		return query(tapp, func() bool { return a.ui.DialogOpen() })
	}, "dialog to open", done)

	tapp.QueueEvent(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	waitFor(t, func() bool {
		return query(tapp, func() bool { return !a.ui.DialogOpen() })
	}, "dialog to close on Esc", done)
}

// TestGlobalKeyDoesNotFireOverDialog proves a WINDOWS-scope accelerator does NOT
// fire while a dialog is open: with the About dialog up, F3 (Open) must not push
// a second dialog, and the keystroke is owned by the open dialog.
func TestGlobalKeyDoesNotFireOverDialog(t *testing.T) {
	a, tapp, _ := newTestApp(t)
	done := runApp(tapp)
	defer func() {
		tapp.Stop()
		<-done
	}()

	waitFor(t, func() bool {
		return query(tapp, func() bool { return len(a.windows) == 1 })
	}, "initial window", done)

	tapp.QueueUpdateDraw(func() { a.cmdAbout() })
	waitFor(t, func() bool {
		return query(tapp, func() bool { return a.ui.DialogOpen() })
	}, "dialog to open", done)

	depthBefore := query(tapp, func() int { return len(a.ui.dialogStack) })

	// F3 would open the File>Open dialog in WINDOWS scope. It must be ignored.
	tapp.QueueEvent(tcell.NewEventKey(tcell.KeyF3, 0, tcell.ModNone))

	// Give the loop time to (not) act, then assert the stack depth is unchanged
	// and a window count of 1 (Open never ran).
	time.Sleep(200 * time.Millisecond)
	depthAfter := query(tapp, func() int { return len(a.ui.dialogStack) })
	if depthAfter != depthBefore {
		t.Fatalf("dialog stack changed (%d -> %d); a global accelerator fired over a dialog", depthBefore, depthAfter)
	}
	if n := query(tapp, func() int { return len(a.windows) }); n != 1 {
		t.Fatalf("window count changed to %d; accelerator leaked over dialog", n)
	}
}
