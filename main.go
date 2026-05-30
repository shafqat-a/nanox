// Command dosedit is a single static terminal text editor recreating the
// early-1990s Visual Basic for DOS 1.0 / QuickBASIC 4.5 editor (see CLAUDE.md
// and VBDOS-Editor-Spec.pdf). main wires a tcell screen, the tui toolkit App
// (menu bar, MDI desktop, status bar) and the global key router, then runs the
// event loop.
package main

import (
	"flag"
	"fmt"
	"os"

	"dosedit/internal/app"
	"dosedit/internal/gallery"
	"dosedit/internal/theme"
	"dosedit/internal/tui"

	"github.com/gdamore/tcell/v2"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "dosedit:", err)
		os.Exit(1)
	}
}

// run assembles and runs the application. The terminal is always restored via
// screen.Fini on return.
func run() error {
	lineNumbersFlag := flag.Bool("line-numbers", false, "show line numbers in editor windows")
	galleryFlag := flag.Bool("gallery", false, "show the widget gallery demo and exit")
	flag.Parse()

	if *galleryFlag {
		return gallery.Run()
	}

	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	screen.EnableMouse()
	screen.SetStyle(theme.Desktop())
	screen.Clear()

	tapp := tui.NewApp(screen)

	// New builds the menu bar (from its command tree), the root layout (menu bar /
	// desktop / status bar) and installs the key hook. Set the line-numbers default
	// before the first window, then open it focused.
	a := app.New(tapp)
	a.SetLineNumbersDefault(*lineNumbersFlag)
	a.OpenInitialWindow()

	return a.Run()
}
