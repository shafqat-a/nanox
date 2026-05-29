package ui

import (
	"fmt"

	"dosedit/internal/theme"

	"github.com/epiclabs-io/winman"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Desktop is the textured grey hatch surface that hosts the MDI window
// manager (spec §6.1). winman has no textured background of its own, so we
// install a draw callback on the manager's embedded Box that paints the
// medium-shade hatch across the whole desktop area. The callback runs from
// within winman.Manager.Draw's initial wm.Box.Draw(screen) call, i.e. BEFORE
// any window is drawn, so windows always render on top of the texture and the
// hatch shows through wherever no window covers it.
type Desktop struct {
	wm *winman.Manager
}

// NewDesktop creates a textured desktop backed by a fresh window manager.
func NewDesktop() *Desktop {
	wm := winman.NewWindowManager()
	d := &Desktop{wm: wm}

	style := theme.Desktop()
	wm.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		for ry := y; ry < y+height; ry++ {
			for rx := x; rx < x+width; rx++ {
				screen.SetContent(rx, ry, theme.Texture, nil, style)
			}
		}
		return x, y, width, height
	})
	return d
}

// Manager returns the underlying window manager so the app can add, remove,
// cycle and query windows.
func (d *Desktop) Manager() *winman.Manager { return d.wm }

// Primitive returns the tview.Primitive to place in the root layout's middle
// slot (the rows between the menu bar and the status bar).
func (d *Desktop) Primitive() tview.Primitive { return d.wm }

// EditorWindow is an MDI child window wrapping a single *Editor (spec §6.3).
// It embeds winman.WindowBase and overrides Draw so the frame and title adopt
// the DOS active/inactive colour treatment based on focus:
//
//	active   -> double-line blue frame + magenta title (theme.Active*)
//	inactive -> single-line grey frame + grey title   (theme.Inactive*)
//
// tview itself switches the border glyphs between single-line (blurred) and
// double-line (focused), which matches the spec, so we only need to drive the
// colours. Modified state and the window number feed the title, refreshed via
// Update (the buffer's Modified flag changes over the window's lifetime).
type EditorWindow struct {
	*winman.WindowBase
	ed     *Editor
	number int
}

// NewEditorWindow creates an MDI editor window hosting ed, adds it to the
// desktop's manager, and returns the wrapper. The window is draggable and
// resizable, cascaded by `number` and sized to ~2/3 of the desktop. Title-bar
// buttons are wired to the supplied callbacks: close (theme.BtnClose) on the
// left, maximize/restore (theme.BtnMax / theme.BtnRestore) on the right.
//
// onClose and onToggleMax may be nil. The window is shown and its title set;
// the caller is responsible for giving it focus (e.g. app.SetFocus / wm focus)
// and for calling Update when the buffer's Modified flag changes.
func NewEditorWindow(d *Desktop, ed *Editor, number int, onClose func(), onToggleMax func()) *EditorWindow {
	w := &EditorWindow{
		WindowBase: winman.NewWindow(),
		ed:         ed,
		number:     number,
	}
	w.SetRoot(ed)
	w.SetBorder(true)
	w.SetDraggable(true)
	w.SetResizable(true)

	// Close button, top-left.
	w.AddButton(&winman.Button{
		Symbol:    theme.BtnClose,
		Alignment: winman.ButtonLeft,
		OnClick:   onClose,
	})
	// Maximize/restore button, top-right.
	w.AddButton(&winman.Button{
		Symbol:    theme.BtnMax,
		Alignment: winman.ButtonRight,
		OnClick:   onToggleMax,
	})

	d.wm.AddWindow(w)
	w.winApplyDefaultRect(d)
	w.Show()
	w.Update()
	return w
}

// winApplyDefaultRect positions the window cascaded by its number and sized to
// roughly two thirds of the desktop. The manager clamps anything that would
// fall outside its bounds on the next Draw, so this only needs to be sensible.
func (w *EditorWindow) winApplyDefaultRect(d *Desktop) {
	dx, dy, dw, dh := d.wm.GetInnerRect()
	if dw <= 0 || dh <= 0 {
		// Manager not laid out yet; use the 80x25 reference desktop.
		dx, dy, dw, dh = 0, 0, 80, 23
	}
	ww := dw * 2 / 3
	wh := dh * 2 / 3
	if ww < winman.MinWindowWidth {
		ww = winman.MinWindowWidth
	}
	if wh < winman.MinWindowHeight {
		wh = winman.MinWindowHeight
	}
	off := ((w.number - 1) % 6) * 2
	x := dx + 1 + off
	y := dy + 1 + off
	w.SetRect(x, y, ww, wh)
}

// Update refreshes the title from the editor's buffer. The title reads
// "[<number>] <name>" with a leading "*" on the name when the buffer is
// modified. Safe to call any time the Modified flag may have changed.
func (w *EditorWindow) Update() {
	name := w.ed.Buffer().DisplayName()
	if w.ed.Buffer().Modified {
		name = "*" + name
	}
	w.SetTitle(fmt.Sprintf("[%d] %s", w.number, name))
}

// Number returns the MDI window number assigned at creation.
func (w *EditorWindow) Number() int { return w.number }

// Editor returns the editor primitive hosted by this window.
func (w *EditorWindow) Editor() *Editor { return w.ed }

// ToggleMaximize maximizes the window if it is restored, or restores it if it
// is maximized. The manager honours IsMaximized() on its next Draw.
func (w *EditorWindow) ToggleMaximize() {
	if w.IsMaximized() {
		w.Restore()
	} else {
		w.Maximize()
	}
}

// Draw applies the DOS active/inactive frame + title colours.
//
// winman/tview can only colour the frame as a whole: tview's Box.Draw paints
// every border cell (including the top row) with one borderStyle and prints
// the title with a foreground colour only (Print never changes the
// background). That cannot produce a magenta title bar over a blue frame on
// its own, and winman draws its title-bar button glyphs in a hardcoded yellow.
//
// So we delegate to winman.WindowBase.Draw for the frame, editor content and
// drag/resize hit-areas, then repaint the title bar ourselves: the interior of
// the top row gets the title background, the title text is reprinted, and the
// close/maximize button glyphs are redrawn in the themed title colours. The
// double-line vs single-line border glyphs (driven by focus) come straight
// from tview and already match the active/inactive spec.
func (w *EditorWindow) Draw(screen tcell.Screen) {
	var frame, title tcell.Style
	if w.HasFocus() {
		frame = theme.ActiveFrame()
		title = theme.ActiveTitle()
	} else {
		frame = theme.InactiveFrame()
		title = theme.InactiveTitle()
	}

	frameFg, frameBg, _ := frame.Decompose()
	titleFg, _, _ := title.Decompose()

	w.SetBorderColor(frameFg)
	w.SetBorderStyle(frame)
	// The window background is the editor surface (blue): the 1-cell frame
	// interior reads blue and any inner cells the editor does not overpaint
	// match the editor surface. winman.WindowBase.Draw clears the inner rect
	// with this background before drawing the editor on top.
	w.SetBackgroundColor(frameBg)
	w.SetTitleColor(titleFg)

	w.WindowBase.Draw(screen)

	if !w.HasBorder() {
		return
	}
	x, y, width, _ := w.GetRect()
	if width < 4 {
		return
	}

	// Repaint the title-bar interior (between the corners) with the title
	// background, then reprint the title text and the button glyphs over it.
	for rx := x + 1; rx < x+width-1; rx++ {
		screen.SetContent(rx, y, ' ', nil, title)
	}
	tview.Print(screen, tview.Escape(w.GetTitle()), x+1, y, width-2, tview.AlignCenter, titleFg)
	w.winDrawButtons(screen, x, y, width, title, titleFg)
}

// winDrawButtons reprints the title-bar button glyphs in the themed title
// colours over the repainted title bar. It mirrors winman's own placement:
// AddButton assigns left buttons offsetX = 2, 5, 9 and right buttons
// offsetX = -3, -6, ... and the glyph is printed as "[symbol]" starting one
// column to the left of offsetX (see winman.WindowBase.Draw / AddButton).
func (w *EditorWindow) winDrawButtons(screen tcell.Screen, x, y, width int, bg tcell.Style, fg tcell.Color) {
	for i := 0; i < w.ButtonCount(); i++ {
		btn := w.GetButton(i)
		if btn == nil {
			continue
		}
		// Recompute offsetX exactly as winman.AddButton does; the field is
		// unexported, so we derive it from the button ordering.
		offsetLeft, offsetRight := 2, -3
		var offsetX int
		for j := 0; j <= i; j++ {
			b := w.GetButton(j)
			if b.Alignment == winman.ButtonRight {
				offsetX = offsetRight
				offsetRight -= 3
			} else {
				offsetX = offsetLeft
				offsetLeft += 3
			}
		}
		bx := x + offsetX
		if offsetX < 0 {
			bx = x + width + offsetX
		}
		label := tview.Escape(fmt.Sprintf("[%c]", btn.Symbol))
		for k := 0; k < 3; k++ {
			screen.SetContent(bx-1+k, y, ' ', nil, bg)
		}
		tview.Print(screen, label, bx-1, y, 3, tview.AlignLeft, fg)
	}
}

// RefreshWindowTitle is a convenience wrapper around (*EditorWindow).Update
// for call sites that hold the window only as the winman.Window interface or
// prefer a free function.
func RefreshWindowTitle(w *EditorWindow) { w.Update() }
