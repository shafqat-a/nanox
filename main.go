// Command dosedit is a single static terminal text editor recreating the
// early-1990s Visual Basic for DOS 1.0 / QuickBASIC 4.5 editor (see CLAUDE.md
// and VBDOS-Editor-Spec.pdf). main wires the screen, palette, custom UI
// primitives, MDI window manager and global key router, then runs the tview
// application loop (spec §6.1, Appendix A).
package main

import (
	"flag"
	"fmt"
	"os"

	"dosedit/internal/app"
	"dosedit/internal/gallery"
	"dosedit/internal/ui"
	"dosedit/internal/ui/wm"

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
	lineNumbersFlag := flag.Bool("line-numbers", false, "show line numbers in editor windows")
	galleryFlag := flag.Bool("gallery", false, "show the widget gallery demo and exit")
	flag.Parse()

	if *galleryFlag {
		return gallery.Run()
	}

	tapp := tview.NewApplication()
	tapp.EnableMouse(true)

	manager := wm.NewManager()
	statusbar := ui.NewStatusBar()

	// The menu bar needs the App's command actions, and the App needs the menu
	// bar. Construct the App first (it builds the bar and the root UIManager),
	// set the line-numbers default, then open the first window.
	a := app.New(tapp, manager, statusbar)
	a.SetLineNumbersDefault(*lineNumbersFlag)
	a.OpenInitialWindow()

	// The UIManager is the sole root primitive AND the sole tview focus: it
	// routes all keyboard and mouse input by scope. No SetInputCapture /
	// SetMouseCapture is installed.
	tapp.SetRoot(a.Root(), true)
	tapp.SetFocus(a.Root())

	return tapp.Run()
}
