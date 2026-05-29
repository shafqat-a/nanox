// Package gallery provides a standalone visual showcase of every DOSEdit tui
// widget. It builds a tcell screen and a tui.App, lays out a menu bar, a status
// bar and a desktop with two overlapping windows whose content exercises one of
// each control, plus a demo modal dialog. It exists so the look & feel of the
// toolkit can be eyeballed without the full editor. Run it via the main binary's
// --gallery flag.
package gallery

import (
	"dosedit/internal/theme"
	"dosedit/internal/tui"

	"github.com/gdamore/tcell/v2"
)

// Run sets up a tcell screen and a tui.App showing all widgets, then runs the
// event loop until the user quits (Esc / Alt+X / File>Exit). The terminal is
// always restored via Fini.
func Run() error {
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

	app := tui.NewApp(screen)

	// --- Desktop with two overlapping windows -------------------------------
	desktop := tui.NewDesktop()

	showcase := newShowcase(app)
	win1 := tui.NewWindow(showcase, "[1] Showcase")
	win1.SetBounds(tui.Rect{X: 1, Y: 2, W: 52, H: 20})

	notes := newNotes()
	win2 := tui.NewWindow(notes, "[2] Notes")
	win2.SetBounds(tui.Rect{X: 40, Y: 5, W: 34, H: 12})

	// Add #2 first then #1 so #1 ends on top; Activate #1 to make it the
	// magenta active window over the grey inactive #2.
	desktop.AddWindow(win2)
	desktop.AddWindow(win1)
	desktop.Activate(win1)

	// --- Status bar ---------------------------------------------------------
	status := tui.NewStatusBar()
	status.SetContext(tui.CtxEditing)
	status.SetCursor(1, 1, true)

	// --- Menu bar -----------------------------------------------------------
	var menubar *tui.MenuBar
	openAbout := func() { app.PushModal(buildDemoDialog(app, status)) }

	menus := []*tui.Menu{
		{Title: "File", Mnemonic: 'F', Items: []tui.MenuItem{
			{Label: "Open...", Mnemonic: 'O', Accel: "F3"},
			{Label: "Save", Mnemonic: 'S', Accel: "F2"},
			{Separator: true},
			{Label: "Exit", Mnemonic: 'x', Accel: "Alt+X", Action: func() { app.Stop() }},
		}},
		{Title: "Edit", Mnemonic: 'E', Items: []tui.MenuItem{
			{Label: "Undo", Mnemonic: 'U', Accel: "Ctrl+Z"},
			{Separator: true},
			{Label: "Cut", Mnemonic: 't', Accel: "Ctrl+X"},
			{Label: "Copy", Mnemonic: 'C', Accel: "Ctrl+C"},
			{Label: "Paste", Mnemonic: 'P', Accel: "Ctrl+V"},
			{Label: "Delete", Mnemonic: 'D', Disabled: true},
		}},
		{Title: "View", Mnemonic: 'V', Items: []tui.MenuItem{
			{Label: "Cascade", Mnemonic: 'C', Action: func() { desktop.Cascade() }},
			{Label: "Tile", Mnemonic: 'T', Action: func() { desktop.Tile() }},
			{Label: "Next Window", Mnemonic: 'N', Accel: "F6", Action: func() { desktop.Next() }},
		}},
		{Title: "Help", Mnemonic: 'H', Items: []tui.MenuItem{
			{Label: "About...", Mnemonic: 'A', Accel: "F2", Action: openAbout},
		}},
	}
	menubar = tui.NewMenuBar(menus)
	menubar.SetApp(app)
	menubar.SetOnActivate(func() { status.SetContext(tui.CtxMenu) })
	menubar.SetOnClose(func() { status.SetContext(tui.CtxEditing) })

	// --- Root layout --------------------------------------------------------
	root := &galRoot{menubar: menubar, desktop: desktop, status: status}
	root.Add(menubar)
	root.Add(desktop)
	root.Add(status)
	app.SetRoot(root)
	app.Focus(win1)

	// --- Global key hook ----------------------------------------------------
	app.SetKeyHook(func(ev *tcell.EventKey) bool {
		switch ev.Key() {
		case tcell.KeyEscape:
			// Esc quits (no modal is open when the hook runs).
			app.Stop()
			return true
		case tcell.KeyF10:
			menubar.Activate()
			return true
		case tcell.KeyF2:
			openAbout()
			return true
		case tcell.KeyF6:
			desktop.Next()
			app.Redraw()
			return true
		case tcell.KeyRune:
			if ev.Modifiers()&tcell.ModAlt != 0 {
				r := ev.Rune()
				if r == 'x' || r == 'X' {
					app.Stop()
					return true
				}
				if menubar.OpenByMnemonic(r) {
					return true
				}
			}
		}
		return false
	})

	return app.Run()
}

// galRoot is the gallery's top-level layout container. It places the menu bar on
// row 0 (full width), the status bar on the last row (full width) and the
// desktop in the rows between, clamping to small terminals without panicking.
type galRoot struct {
	tui.BaseContainer
	menubar *tui.MenuBar
	desktop *tui.Desktop
	status  *tui.StatusBar
}

// SetBounds lays the three regions out within r.
func (g *galRoot) SetBounds(r tui.Rect) {
	g.BaseContainer.SetBounds(r)
	if r.W <= 0 || r.H <= 0 {
		return
	}
	g.menubar.SetBounds(tui.Rect{X: r.X, Y: r.Y, W: r.W, H: 1})
	if r.H >= 2 {
		g.status.SetBounds(tui.Rect{X: r.X, Y: r.Y + r.H - 1, W: r.W, H: 1})
	}
	midY := r.Y + 1
	midH := r.H - 2
	if midH < 1 {
		midH = 1
		midY = r.Y
	}
	g.desktop.SetBounds(tui.Rect{X: r.X, Y: midY, W: r.W, H: midH})
}

// Draw paints the children in order (desktop under the bars).
func (g *galRoot) Draw(s tui.Surface) {
	g.desktop.Draw(s.Clip(g.desktop.Bounds()))
	g.menubar.Draw(s.Clip(g.menubar.Bounds()))
	g.status.Draw(s.Clip(g.status.Bounds()))
}

// showcase is the content of window #1: a container that lays out one of every
// control on a vertical flow so they are all visible at once.
type showcase struct {
	tui.BaseContainer
	rows []showRow
}

// showRow pairs a widget with the height it should occupy in the flow layout.
type showRow struct {
	w tui.Widget
	h int
}

// newShowcase builds the showcase content. app is needed to wire the ComboBox
// and is passed through to its SetApp.
func newShowcase(app *tui.App) *showcase {
	sc := &showcase{}
	add := func(w tui.Widget, h int) {
		sc.rows = append(sc.rows, showRow{w: w, h: h})
		sc.Add(w)
	}

	add(tui.NewLabel("Buttons (Tab cycles focus):"), 1)
	btnOK := tui.NewButton("OK", nil)
	btnOK.SetDefault(true)
	btnCancel := tui.NewButton("Cancel", nil)
	btnRow := newRowPanel(btnOK, btnCancel)
	add(btnRow, 2)

	chk1 := tui.NewCheckbox("Word wrap", true)
	chk2 := tui.NewCheckbox("Auto indent", false)
	add(chk1, 1)
	add(chk2, 1)

	// Option group inside a Frame group box.
	og := tui.NewOptionGroup([]string{"Spaces", "Tabs", "Smart"})
	ogFrame := newFramePanel("Indentation", og)
	add(ogFrame, og.PreferredHeight()+2)

	add(tui.NewLabel("Text box:"), 1)
	add(tui.NewTextBox("hello.bas", 22), 1)

	add(tui.NewLabel("List box:"), 1)
	lb := tui.NewListBox([]string{
		"alpha", "bravo", "charlie", "delta", "echo",
		"foxtrot", "golf", "hotel", "india", "juliet",
	})
	add(lb, 4)

	add(tui.NewLabel("Combo box:"), 1)
	cmb := tui.NewComboBox([]string{"ASCII", "UTF-8", "CP437", "Latin-1"})
	cmb.SetApp(app)
	add(cmb, 1)

	return sc
}

// SetBounds flows the rows top-to-bottom. A standalone vertical scrollbar is
// drawn in the right margin by Draw; the rows take the remaining width.
func (sc *showcase) SetBounds(r tui.Rect) {
	sc.BaseContainer.SetBounds(r)
	if r.W <= 0 || r.H <= 0 {
		return
	}
	contentW := r.W - 2 // leave a column for the standalone scrollbar
	if contentW < 1 {
		contentW = r.W
	}
	y := r.Y
	bottom := r.Y + r.H
	for _, row := range sc.rows {
		h := row.h
		if y >= bottom {
			h = 0
		} else if y+h > bottom {
			h = bottom - y
		}
		row.w.SetBounds(tui.Rect{X: r.X, Y: y, W: contentW, H: h})
		y += row.h
	}
}

// Draw renders the rows plus a standalone vertical scrollbar in the right
// margin so a bare ScrollBar widget is visible on its own.
func (sc *showcase) Draw(s tui.Surface) {
	b := sc.Bounds()
	for _, row := range sc.rows {
		rb := row.w.Bounds()
		if rb.H > 0 {
			row.w.Draw(s.Clip(rb))
		}
	}
	if b.W >= 2 && b.H >= 2 {
		bar := tui.NewScrollBar(true)
		bar.SetBounds(tui.Rect{X: b.X + b.W - 1, Y: b.Y, W: 1, H: b.H})
		bar.SetRange(0, 10, 4)
		bar.SetValue(3)
		bar.Draw(s.Clip(bar.Bounds()))
	}
}

// rowPanel lays a horizontal row of widgets side by side using their preferred
// widths. Used for the button row in the showcase.
type rowPanel struct {
	tui.BaseContainer
}

// newRowPanel builds a horizontal row from the given widgets.
func newRowPanel(ws ...tui.Widget) *rowPanel {
	p := &rowPanel{}
	for _, w := range ws {
		p.Add(w)
	}
	return p
}

// SetBounds places children left-to-right by preferred width, two cells apart.
func (p *rowPanel) SetBounds(r tui.Rect) {
	p.BaseContainer.SetBounds(r)
	x := r.X
	for _, w := range p.Children() {
		cw := 10
		if pw, ok := w.(interface{ PreferredWidth() int }); ok {
			cw = pw.PreferredWidth()
		}
		h := r.H
		if h < 1 {
			h = 1
		}
		w.SetBounds(tui.Rect{X: x, Y: r.Y, W: cw, H: h})
		x += cw + 2
	}
}

// Draw renders the row's children.
func (p *rowPanel) Draw(s tui.Surface) {
	for _, w := range p.Children() {
		w.Draw(s.Clip(w.Bounds()))
	}
}

// framePanel wraps a single widget in a Frame group box, laying the inner widget
// into the frame's inner rect on SetBounds.
type framePanel struct {
	tui.BaseContainer
	frame *tui.Frame
	inner tui.Widget
}

// newFramePanel builds a Frame group box around inner.
func newFramePanel(caption string, inner tui.Widget) *framePanel {
	fp := &framePanel{frame: tui.NewFrame(caption), inner: inner}
	fp.frame.Add(inner)
	fp.Add(fp.frame)
	return fp
}

// SetBounds sizes the frame to the bounds and lays the inner widget into the
// frame interior.
func (fp *framePanel) SetBounds(r tui.Rect) {
	fp.BaseContainer.SetBounds(r)
	fp.frame.SetBounds(r)
	inner := fp.frame.GetInnerRect()
	if !inner.Empty() {
		fp.inner.SetBounds(inner)
	}
}

// Draw renders the frame (which draws its child).
func (fp *framePanel) Draw(s tui.Surface) {
	fp.frame.Draw(s.Clip(fp.frame.Bounds()))
}

// notes is the content of window #2: a Frame with a couple of labels.
type notes struct {
	tui.BaseContainer
	frame *tui.Frame
	lines []*tui.Label
}

// newNotes builds the notes content.
func newNotes() *notes {
	n := &notes{frame: tui.NewFrame("Notes")}
	n.lines = []*tui.Label{
		tui.NewLabel("Active window: magenta title."),
		tui.NewLabel("Inactive: grey single-line."),
		tui.NewLabel("F6 cycles windows."),
		tui.NewLabel("F2 opens the dialog."),
	}
	for _, l := range n.lines {
		n.frame.Add(l)
	}
	n.Add(n.frame)
	return n
}

// SetBounds sizes the frame and stacks the labels in its interior.
func (n *notes) SetBounds(r tui.Rect) {
	n.BaseContainer.SetBounds(r)
	n.frame.SetBounds(r)
	inner := n.frame.GetInnerRect()
	y := inner.Y
	for _, l := range n.lines {
		if y >= inner.Y+inner.H {
			l.SetBounds(tui.Rect{})
			continue
		}
		l.SetBounds(tui.Rect{X: inner.X, Y: y, W: inner.W, H: 1})
		y++
	}
}

// Draw renders the frame and its labels.
func (n *notes) Draw(s tui.Surface) {
	n.frame.Draw(s.Clip(n.frame.Bounds()))
}

// buildDemoDialog builds the modal "Options" dialog with a few body controls and
// the OK/Cancel/Help button column. OK and Cancel both pop the modal.
func buildDemoDialog(app *tui.App, status *tui.StatusBar) *tui.Dialog {
	dlg := tui.NewDialog("Options")

	name := tui.NewTextBox("untitled.bas", 24)
	name.SetBounds(tui.Rect{W: 26, H: 1})
	dlg.Add(name)

	og := tui.NewOptionGroup([]string{"DOS", "Unix", "Mac"})
	ogFrame := newFramePanel("Line endings", og)
	ogFrame.SetBounds(tui.Rect{W: 26, H: og.PreferredHeight() + 2})
	dlg.Add(ogFrame)

	chk1 := tui.NewCheckbox("Backup on save", true)
	chk1.SetBounds(tui.Rect{W: 26, H: 1})
	dlg.Add(chk1)

	chk2 := tui.NewCheckbox("Read only", false)
	chk2.SetBounds(tui.Rect{W: 26, H: 1})
	dlg.Add(chk2)

	cmb := tui.NewComboBox([]string{"Default", "Bright", "Mono"})
	cmb.SetApp(app)
	cmb.SetBounds(tui.Rect{W: 26, H: 1})
	dlg.Add(cmb)

	closeDlg := func() {
		app.PopModal()
		status.SetContext(tui.CtxEditing)
	}

	ok := tui.NewButton("OK", closeDlg)
	ok.SetDefault(true)
	ok.SetBounds(tui.Rect{W: 10, H: 2})
	cancel := tui.NewButton("Cancel", closeDlg)
	cancel.SetBounds(tui.Rect{W: 10, H: 2})
	help := tui.NewButton("Help", nil)
	help.SetBounds(tui.Rect{W: 10, H: 2})

	dlg.AddButton(ok)
	dlg.AddButton(cancel)
	dlg.AddButton(help)
	dlg.SetDefault(ok)
	dlg.SetCancel(closeDlg)
	dlg.AutoSize()

	status.SetContext(tui.CtxDialog)
	return dlg
}
