package app

import (
	"strings"
	"testing"
	"time"

	"dosedit/internal/ui"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// newTestApp builds a fully wired App backed by an 80x25 simulation screen and
// returns it together with the tview application and the screen. It mirrors the
// wiring in main.run so the test exercises the real assembly path.
func newTestApp(t *testing.T) (*App, *tview.Application, tcell.SimulationScreen) {
	t.Helper()

	// Do not call sim.Init here: tview.Application.Run initialises the screen
	// itself, and initialising a SimulationScreen twice deadlocks. SetSize
	// before SetScreen establishes the 80x25 reference geometry.
	sim := tcell.NewSimulationScreen("")
	sim.SetSize(80, 25)

	tapp := tview.NewApplication()
	tapp.SetScreen(sim)

	desktop := ui.NewDesktop()
	statusbar := ui.NewStatusBar()
	a := New(tapp, desktop, statusbar)
	a.OpenInitialWindow()

	tapp.SetInputCapture(a.RouteGlobalKeys)
	tapp.SetRoot(a.Root(), true)
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

// screenText flattens the whole simulation screen into a single string so tests
// can assert on rendered regions without caring about exact columns.
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

// TestM1ShellBuildsAndDraws is the M1 acceptance proof: the app assembles the
// master layout, draws without panicking, and the menu / desktop / status
// regions render.
func TestM1ShellBuildsAndDraws(t *testing.T) {
	_, tapp, sim := newTestApp(t)
	done := runApp(tapp)

	// The loop performs an initial draw on start; poll the simulation screen
	// until the menu bar has rendered.
	waitFor(t, func() bool {
		return contains(rowText(sim, 0), "File")
	}, "menu bar to render", done)

	// Row 0 is the menu bar.
	menu := rowText(sim, 0)
	for _, title := range []string{"File", "Edit", "Search", "Window", "Help"} {
		if !contains(menu, title) {
			t.Errorf("menu bar row missing %q; got %q", title, menu)
		}
	}

	// Bottom row is the status bar (cyan context hints).
	status := rowText(sim, 24)
	if !contains(status, "F1=Help") {
		t.Errorf("status bar row missing hints; got %q", status)
	}

	// The desktop region between bars should have drawn the editor window; its
	// title appears once the loop has performed its first layout and placement.
	waitFor(t, func() bool {
		return contains(screenText(sim), "Untitled1")
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

// TestNewWindowCommand verifies that the File>New command path creates an
// additional window and tracks it.
func TestNewWindowCommand(t *testing.T) {
	a, tapp, _ := newTestApp(t)
	done := runApp(tapp)
	defer func() {
		tapp.Stop()
		<-done
	}()

	tapp.QueueUpdateDraw(func() { a.cmdNew() })
	waitFor(t, func() bool {
		got := make(chan int, 1)
		tapp.QueueUpdate(func() { got <- len(a.windows) })
		return <-got == 2
	}, "second window to be created", done)
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
