// Command dosedit is a single static terminal text editor recreating the
// early-1990s Visual Basic for DOS 1.0 / QuickBASIC 4.5 editor (see CLAUDE.md
// and VBDOS-Editor-Spec.pdf). main wires the screen, palette, custom UI
// primitives, MDI window manager and global key router, then runs the tview
// application loop (spec §6.1, Appendix A).
package main

import (
	"fmt"
	"os"

	"dosedit/internal/app"
	"dosedit/internal/ui"

	"github.com/rivo/tview"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "dosedit:", err)
		os.Exit(1)
	}
}

// run assembles and runs the application. It is split out from main so the
// terminal is always restored (tview.Application.Run does this on return) and
// any error can be reported with a non-zero exit code.
func run() error {
	tapp := tview.NewApplication()
	tapp.EnableMouse(true)

	desktop := ui.NewDesktop()
	statusbar := ui.NewStatusBar()

	// The menu bar needs the App's command actions, and the App needs the menu
	// bar. Construct the App first (it builds the bar from its own command
	// tree and installs it into the root layout), then open the first window.
	a := app.New(tapp, desktop, statusbar)
	a.OpenInitialWindow()

	tapp.SetInputCapture(a.RouteGlobalKeys)
	tapp.SetRoot(a.Root(), true)

	return tapp.Run()
}
