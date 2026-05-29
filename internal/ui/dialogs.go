// This file implements DOSEdit's modal dialogs (spec §6.6): Open, Save As,
// MessageBox, Find, Replace, Go To Line and About.
//
// The single-field dialogs (MessageBox, Find, Replace, Go To Line, About) are
// built from a tview.Form (inputs + checkboxes + stacked command buttons),
// which handles its own Tab navigation. The composite file dialogs (Open,
// Save As) instead use a flat ring of standalone controls — a File Name input,
// two tview.List boxes (directories and files) and standalone buttons — owned
// by a dlgFocusGroup that keeps the Application focus on itself so Tab/Shift+Tab
// cycle reliably between the controls. Each constructor returns a ready-to-host
// tview.Primitive plus a suggested width/height so the application can centre
// the dialog inside a winman modal.
//
// The visible chrome — the magenta title bar, light-grey body, double-line
// frame and solid-black drop shadow — is drawn by the self-contained dlgFrame
// primitive declared here, which wraps an inner primitive. Nothing in this
// file references symbols from sibling files in package ui, and every
// unexported identifier is prefixed dlg to avoid collisions.
package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// dlgFrame wraps an inner primitive with a centred double-line frame, a
// magenta title bar (white centred text), a light-grey body and a one-cell
// solid-black drop shadow on the right and bottom edges. It is fully
// self-contained: the inner primitive is laid out inside the frame and
// receives all focus/input/mouse delegation.
type dlgFrame struct {
	*tview.Box
	title string
	inner tview.Primitive
}

// dlgNewFrame builds a frame around inner with the given title.
func dlgNewFrame(title string, inner tview.Primitive) *dlgFrame {
	f := &dlgFrame{
		Box:   tview.NewBox(),
		title: title,
		inner: inner,
	}
	f.Box.SetBackgroundColor(theme.LGray)
	return f
}

// dlgInnerRect returns the body rectangle available to the inner primitive,
// i.e. the area inside the double frame, excluding the title row, the frame
// borders and the one-cell shadow on the right/bottom.
func (f *dlgFrame) dlgInnerRect() (int, int, int, int) {
	x, y, w, h := f.GetRect()
	// Reserve a one-cell shadow on the right and bottom.
	fw := w - 1
	fh := h - 1
	if fw < 4 || fh < 4 {
		// Degenerate; hand back whatever we have without crashing.
		return x + 1, y + 1, dlgMaxInt(fw-2, 0), dlgMaxInt(fh-2, 0)
	}
	// Inside: skip left/right border (1 each), top border + title (2) and
	// bottom border (1).
	ix := x + 1
	iy := y + 2
	iw := fw - 2
	ih := fh - 3
	return ix, iy, iw, ih
}

// Draw paints the shadow, frame, title bar and body, then lays out and draws
// the inner primitive.
func (f *dlgFrame) Draw(screen tcell.Screen) {
	f.Box.DrawForSubclass(screen, f)
	x, y, w, h := f.GetRect()
	if w < 4 || h < 3 {
		return
	}

	fw := w - 1 // frame width  (shadow occupies the last column)
	fh := h - 1 // frame height (shadow occupies the last row)
	right := x + fw - 1
	bottom := y + fh - 1

	body := theme.DialogBody()
	title := theme.DialogTitle()
	shadow := theme.Shadow()

	// Fill the body interior with the dialog background.
	for row := y; row <= bottom; row++ {
		for col := x; col <= right; col++ {
			screen.SetContent(col, row, ' ', nil, body)
		}
	}

	// Double-line frame.
	screen.SetContent(x, y, theme.TLDouble, nil, body)
	screen.SetContent(right, y, theme.TRDouble, nil, body)
	screen.SetContent(x, bottom, theme.BLDouble, nil, body)
	screen.SetContent(right, bottom, theme.BRDouble, nil, body)
	for col := x + 1; col < right; col++ {
		screen.SetContent(col, y, theme.HDouble, nil, body)
		screen.SetContent(col, bottom, theme.HDouble, nil, body)
	}
	for row := y + 1; row < bottom; row++ {
		screen.SetContent(x, row, theme.VDouble, nil, body)
		screen.SetContent(right, row, theme.VDouble, nil, body)
	}

	// Magenta title bar on the row just below the top border.
	titleRow := y + 1
	if titleRow < bottom {
		for col := x + 1; col < right; col++ {
			screen.SetContent(col, titleRow, ' ', nil, title)
		}
		label := f.title
		avail := fw - 2
		if avail > 0 {
			label = dlgClip(label, avail)
			start := x + 1 + (avail-len(label))/2
			for i, r := range []rune(label) {
				screen.SetContent(start+i, titleRow, r, nil, title)
			}
		}
	}

	// Solid-black drop shadow: right column and bottom row, offset by one.
	for row := y + 1; row <= bottom+1; row++ {
		screen.SetContent(right+1, row, ' ', nil, shadow)
	}
	for col := x + 1; col <= right+1; col++ {
		screen.SetContent(col, bottom+1, ' ', nil, shadow)
	}

	// Lay out and draw the inner primitive.
	if f.inner != nil {
		ix, iy, iw, ih := f.dlgInnerRect()
		if iw > 0 && ih > 0 {
			f.inner.SetRect(ix, iy, iw, ih)
			f.inner.Draw(screen)
		}
	}
}

// Focus delegates focus to the inner primitive so keyboard navigation works.
func (f *dlgFrame) Focus(delegate func(p tview.Primitive)) {
	if f.inner != nil {
		delegate(f.inner)
		return
	}
	f.Box.Focus(delegate)
}

// HasFocus reports whether the inner primitive holds focus.
func (f *dlgFrame) HasFocus() bool {
	if f.inner != nil {
		return f.inner.HasFocus()
	}
	return f.Box.HasFocus()
}

// InputHandler forwards keys to the inner primitive.
func (f *dlgFrame) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if f.inner == nil {
			return
		}
		if h := f.inner.InputHandler(); h != nil {
			h(event, setFocus)
		}
	}
}

// MouseHandler forwards mouse events to the inner primitive.
func (f *dlgFrame) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		if f.inner == nil {
			return false, nil
		}
		if h := f.inner.MouseHandler(); h != nil {
			return h(action, event, setFocus)
		}
		return false, nil
	}
}

// dlgStyleForm applies the shared DOS dialog styling to a Form: light-grey
// body, white-on-black input fields and a light-grey button face with a
// reverse-video treatment for the focused/default button.
func dlgStyleForm(form *tview.Form) {
	form.SetBackgroundColor(theme.LGray)
	form.SetFieldStyle(theme.InputField())
	form.SetLabelColor(theme.Black)
	form.SetButtonStyle(theme.ButtonFace())
	// Focused/default button: reverse video (white-on-black) so it reads as
	// the active command, matching the VB-for-DOS default-button treatment.
	form.SetButtonActivatedStyle(tcell.StyleDefault.Foreground(theme.White).Background(theme.Black))
	form.SetButtonsAlign(tview.AlignRight)
	form.SetItemPadding(0)
}

// dlgStyleList applies the black-on-white list-box styling with a reverse
// selected row.
func dlgStyleList(list *tview.List) {
	list.ShowSecondaryText(false)
	list.SetBackgroundColor(theme.White)
	list.SetMainTextStyle(theme.ListBox())
	list.SetSelectedStyle(theme.ListSelected())
	list.SetWrapAround(true)
}

// dlgClip truncates s to at most n runes.
func dlgClip(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// dlgMaxInt returns the larger of a and b (local helper; avoids shadowing the
// builtin max so sibling files are unaffected).
func dlgMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

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

// dlgFileDialog holds the shared state for the Open and Save As dialogs.
type dlgFileDialog struct {
	frame   *dlgFrame
	nameIn  *tview.InputField
	pathTV  *tview.TextView
	dirList *tview.List
	fileLst *tview.List
	dir     string
	filter  string
}

// dlgNameField returns the current "File Name" input value.
func (d *dlgFileDialog) dlgNameField() string {
	return strings.TrimSpace(d.nameIn.GetText())
}

// dlgSetNameField sets the "File Name" input value.
func (d *dlgFileDialog) dlgSetNameField(v string) {
	d.nameIn.SetText(v)
}

// dlgRefresh re-reads the current directory and repopulates the lists and the
// current-path line.
func (d *dlgFileDialog) dlgRefresh() {
	d.pathTV.SetText(dlgClip(d.dir, 200))
	dirs, files := dlgListEntries(d.dir, d.filter)

	d.dirList.Clear()
	for _, name := range dirs {
		nm := name
		d.dirList.AddItem("["+nm+"]", "", 0, func() {
			var next string
			if nm == ".." {
				next = filepath.Dir(d.dir)
			} else {
				next = filepath.Join(d.dir, nm)
			}
			d.dir = dlgResolveDir(next)
			d.dlgRefresh()
		})
	}

	d.fileLst.Clear()
	for _, name := range files {
		nm := name
		d.fileLst.AddItem(nm, "", 0, func() {
			d.dlgSetNameField(nm)
		})
	}
}

// dlgChosenPath resolves the current "File Name" field against the current
// directory, returning the absolute path to hand to onOK. Returns "" when the
// field is empty.
func (d *dlgFileDialog) dlgChosenPath() string {
	name := d.dlgNameField()
	if name == "" {
		return ""
	}
	// A bare glob filter (e.g. "*.*") is not a real selection.
	if strings.ContainsAny(name, "*?") {
		return ""
	}
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	return filepath.Join(d.dir, name)
}

// dlgBuildFileDialog constructs the shared Open/Save As layout. okLabel is the
// confirm button text ("OK" / "Save"); nameDefault seeds the File Name field.
//
// Focus moves between the four (plus) standalone controls — File Name input,
// Directories list, Files list, and the OK / Cancel buttons — via Tab and
// Shift+Tab. To make that work reliably the controls are placed in a flat ring
// owned by a dlgFocusGroup, which keeps the tview Application focus on itself so
// it sees every key (see dlgFocusGroup for the rationale). Using standalone
// controls rather than a nested tview.Form avoids the Form's own Tab handling
// swallowing the key.
func dlgBuildFileDialog(title, startDir, filter, nameDefault, okLabel string, onOK func(path string), onCancel func()) (tview.Primitive, int, int) {
	d := &dlgFileDialog{
		dir:    dlgResolveDir(startDir),
		filter: filter,
	}

	cancel := func() {
		if onCancel != nil {
			onCancel()
		}
	}

	d.pathTV = tview.NewTextView()
	d.pathTV.SetDynamicColors(false)
	d.pathTV.SetTextStyle(theme.DialogBody())
	d.pathTV.SetBackgroundColor(theme.LGray)
	d.pathTV.SetWrap(false)

	d.dirList = tview.NewList()
	dlgStyleList(d.dirList)
	d.fileLst = tview.NewList()
	dlgStyleList(d.fileLst)

	d.nameIn = tview.NewInputField()
	d.nameIn.SetLabel("File Name ")
	d.nameIn.SetText(nameDefault)
	d.nameIn.SetFieldWidth(40)
	d.nameIn.SetLabelColor(theme.Black)
	d.nameIn.SetFieldStyle(theme.InputField())
	d.nameIn.SetBackgroundColor(theme.LGray)

	okBtn := dlgButton(okLabel, func() {
		if p := d.dlgChosenPath(); p != "" && onOK != nil {
			onOK(p)
		}
	})
	cancelBtn := dlgButton("Cancel", cancel)

	// Esc cancels from any control.
	escCapture := func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			cancel()
			return nil
		}
		return event
	}
	d.nameIn.SetInputCapture(escCapture)
	d.dirList.SetInputCapture(escCapture)
	d.fileLst.SetInputCapture(escCapture)
	okBtn.SetExitFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			cancel()
		}
	})
	cancelBtn.SetExitFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			cancel()
		}
	})

	d.dlgRefresh()

	// Compose: File Name input on top, current-path line, then the two lists
	// side by side under "Directories" / "Files" captions, and the buttons row.
	topRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	topRow.SetBackgroundColor(theme.LGray)
	topRow.AddItem(d.nameIn, 0, 1, true)

	dirsCol := tview.NewFlex().SetDirection(tview.FlexRow)
	dirsCol.AddItem(dlgCaption("Directories"), 1, 0, false)
	dirsCol.AddItem(d.dirList, 0, 1, true)

	filesCol := tview.NewFlex().SetDirection(tview.FlexRow)
	filesCol.AddItem(dlgCaption("Files"), 1, 0, false)
	filesCol.AddItem(d.fileLst, 0, 1, false)

	listsRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	listsRow.AddItem(dirsCol, 0, 1, true)
	listsRow.AddItem(dlgSpacer(), 1, 0, false)
	listsRow.AddItem(filesCol, 0, 1, false)

	btnRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	btnRow.SetBackgroundColor(theme.LGray)
	btnRow.AddItem(dlgSpacer(), 0, 1, false)
	btnRow.AddItem(okBtn, len([]rune(okLabel))+4, 0, false)
	btnRow.AddItem(dlgSpacer(), 1, 0, false)
	btnRow.AddItem(cancelBtn, len("Cancel")+4, 0, false)

	body := tview.NewFlex().SetDirection(tview.FlexRow)
	body.SetBackgroundColor(theme.LGray)
	body.AddItem(topRow, 1, 0, true)
	body.AddItem(d.pathTV, 1, 0, false)
	body.AddItem(dlgSpacer(), 1, 0, false)
	body.AddItem(listsRow, 0, 1, false)
	body.AddItem(dlgSpacer(), 1, 0, false)
	body.AddItem(btnRow, 1, 0, false)

	// Flat focus ring: File Name -> Directories -> Files -> OK -> Cancel.
	ring := []tview.Primitive{d.nameIn, d.dirList, d.fileLst, okBtn, cancelBtn}
	group := dlgNewFocusGroup(body, ring)

	d.frame = dlgNewFrame(title, group)
	return d.frame, 54, 20
}

// dlgButton builds a standalone command button styled like the DOS dialog
// buttons (light-grey face, reverse-video when focused).
func dlgButton(label string, selected func()) *tview.Button {
	b := tview.NewButton(label)
	b.SetStyle(theme.ButtonFace())
	b.SetActivatedStyle(tcell.StyleDefault.Foreground(theme.White).Background(theme.Black))
	if selected != nil {
		b.SetSelectedFunc(selected)
	}
	return b
}

// dlgCaption builds a small light-grey caption label used above list boxes.
func dlgCaption(text string) *tview.TextView {
	tv := tview.NewTextView()
	tv.SetText(text)
	tv.SetTextStyle(theme.DialogBody())
	tv.SetBackgroundColor(theme.LGray)
	return tv
}

// dlgSpacer is a blank light-grey filler primitive.
func dlgSpacer() *tview.Box {
	b := tview.NewBox()
	b.SetBackgroundColor(theme.LGray)
	return b
}

// dlgFocusGroup is a focus-owning container, modelled on the trick tview.Form
// uses internally. It wraps a *tview.Flex purely for layout and drawing, but
// holds a flat ordered ring of focusable leaf controls and rotates focus among
// them on Tab / Shift+Tab.
//
// The crucial point: tview's Application delivers key events to the *focused
// leaf primitive*, not through ancestor containers, so an input capture on a
// plain container never sees Tab. dlgFocusGroup instead keeps the Application
// focus on ITSELF (its Focus does not delegate down), so its InputHandler
// receives every key. It then forwards non-Tab keys to the currently selected
// child using the REAL setFocus, and handles Tab/Shift+Tab by advancing the
// internal index and refocusing children internally.
type dlgFocusGroup struct {
	*tview.Box
	layout *tview.Flex
	ring   []tview.Primitive
	index  int
}

// dlgNewFocusGroup builds a focus group drawing layout and cycling focus
// through ring (which must be non-empty; entries should be leaf controls that
// also appear somewhere inside layout).
func dlgNewFocusGroup(layout *tview.Flex, ring []tview.Primitive) *dlgFocusGroup {
	g := &dlgFocusGroup{
		Box:    tview.NewBox(),
		layout: layout,
		ring:   ring,
	}
	g.Box.SetBackgroundColor(theme.LGray)
	return g
}

// Draw lays out the group's rectangle onto the inner Flex and draws it.
func (g *dlgFocusGroup) Draw(screen tcell.Screen) {
	g.Box.DrawForSubclass(screen, g)
	x, y, w, h := g.GetRect()
	g.layout.SetRect(x, y, w, h)
	g.layout.Draw(screen)
}

// dlgFocusChild gives internal focus to the ring entry at i and removes it from
// every other ring entry (via a no-op delegate, which only flips the child's
// internal focus flag — app focus stays on the group).
func (g *dlgFocusGroup) dlgFocusChild(i int) {
	if len(g.ring) == 0 {
		return
	}
	g.index = (i%len(g.ring) + len(g.ring)) % len(g.ring)
	for j, c := range g.ring {
		if j == g.index {
			c.Focus(func(tview.Primitive) {})
		} else if c.HasFocus() {
			// Drop internal focus from the previously selected child so it
			// stops drawing its cursor / selected styling.
			c.Blur()
		}
	}
}

// Focus keeps the Application focus on the group itself and sets internal focus
// on the current child. delegate is intentionally ignored.
func (g *dlgFocusGroup) Focus(delegate func(p tview.Primitive)) {
	g.Box.Focus(delegate)
	g.dlgFocusChild(g.index)
}

// HasFocus reports focus on the group or any of its children.
func (g *dlgFocusGroup) HasFocus() bool {
	if g.Box.HasFocus() {
		return true
	}
	for _, c := range g.ring {
		if c.HasFocus() {
			return true
		}
	}
	return false
}

// dlgKeepFocus returns a setFocus wrapper that prevents a child from stealing
// the Application focus away from the group: any request to focus a ring child
// is redirected to focusing the group itself (which re-asserts the child's
// internal focus). Requests to focus anything else (e.g. a future modal) are
// passed through unchanged.
func (g *dlgFocusGroup) dlgKeepFocus(setFocus func(p tview.Primitive)) func(p tview.Primitive) {
	return func(p tview.Primitive) {
		for _, c := range g.ring {
			if c == p {
				setFocus(g)
				return
			}
		}
		setFocus(p)
	}
}

// InputHandler rotates focus on Tab/Shift+Tab and forwards all other keys to
// the current child.
func (g *dlgFocusGroup) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return g.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if len(g.ring) == 0 {
			return
		}
		switch event.Key() {
		case tcell.KeyTab:
			g.dlgFocusChild(g.index + 1)
			return
		case tcell.KeyBacktab:
			g.dlgFocusChild(g.index - 1)
			return
		}
		child := g.ring[g.index]
		if h := child.InputHandler(); h != nil {
			h(event, g.dlgKeepFocus(setFocus))
		}
	})
}

// MouseHandler forwards mouse events to the children; a click on a control also
// selects it in the ring so subsequent keys go there. Children that request
// focus are redirected back to the group so it retains the Application focus.
func (g *dlgFocusGroup) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return g.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		keep := g.dlgKeepFocus(setFocus)
		for i, c := range g.ring {
			consumed, capture := c.MouseHandler()(action, event, keep)
			if consumed {
				if action == tview.MouseLeftDown || action == tview.MouseLeftClick {
					g.dlgFocusChild(i)
				}
				return consumed, capture
			}
		}
		return false, nil
	})
}

// NewOpenDialog builds the modal File Open dialog. The File Name field
// defaults to filter (e.g. "*.*"); selecting a file and confirming calls
// onOK with the full path. onCancel fires on Cancel/Esc.
func NewOpenDialog(startDir, filter string, onOK func(path string), onCancel func()) (p tview.Primitive, w, h int) {
	def := filter
	if def == "" {
		def = "*.*"
	}
	return dlgBuildFileDialog("Open", startDir, def, def, "OK", onOK, onCancel)
}

// NewSaveAsDialog builds the modal Save As dialog. The File Name field is
// seeded with suggestedName; confirming calls onOK with the full path (the
// application may follow up with a confirm-overwrite MessageBox). onCancel
// fires on Cancel/Esc.
func NewSaveAsDialog(startDir, suggestedName string, onOK func(path string), onCancel func()) (tview.Primitive, int, int) {
	filter := "*.*"
	return dlgBuildFileDialog("Save As", startDir, filter, suggestedName, "Save", onOK, onCancel)
}

// NewMessageBox builds a modal message box with a centred message and the
// given buttons stacked down the right. onResult receives the pressed button
// index, or -1 when dismissed with Esc.
func NewMessageBox(title, message string, buttons []string, onResult func(idx int)) (tview.Primitive, int, int) {
	if len(buttons) == 0 {
		buttons = []string{"OK"}
	}

	msg := tview.NewTextView()
	msg.SetText(message)
	msg.SetWrap(true)
	msg.SetTextAlign(tview.AlignCenter)
	msg.SetTextStyle(theme.DialogBody())
	msg.SetBackgroundColor(theme.LGray)

	form := tview.NewForm()
	dlgStyleForm(form)
	for i, label := range buttons {
		idx := i
		form.AddButton(label, func() {
			if onResult != nil {
				onResult(idx)
			}
		})
	}
	form.SetCancelFunc(func() {
		if onResult != nil {
			onResult(-1)
		}
	})

	body := tview.NewFlex().SetDirection(tview.FlexRow)
	body.SetBackgroundColor(theme.LGray)
	body.AddItem(msg, 0, 1, false)
	body.AddItem(form, 1, 0, true)

	// Width: fit the message and buttons, clamped to a sane range.
	lines := strings.Split(message, "\n")
	maxLine := 0
	for _, ln := range lines {
		if l := len([]rune(ln)); l > maxLine {
			maxLine = l
		}
	}
	btnTotal := 0
	for _, b := range buttons {
		btnTotal += len([]rune(b)) + 4
	}
	w := maxLine
	if btnTotal > w {
		w = btnTotal
	}
	w += 8
	if w < 32 {
		w = 32
	}
	if w > 72 {
		w = 72
	}
	h := len(lines) + 7
	if h < 8 {
		h = 8
	}

	frame := dlgNewFrame(title, body)
	return frame, w, h
}

// dlgSearchOpts builds a Find/Replace form with the shared option checkboxes.
// It returns the form plus accessors for the entered values.
func dlgSearchForm() *tview.Form {
	form := tview.NewForm()
	dlgStyleForm(form)
	return form
}

// dlgCheckboxState returns whether the named checkbox in form is ticked.
func dlgCheckboxState(form *tview.Form, label string) bool {
	if item := form.GetFormItemByLabel(label); item != nil {
		if cb, ok := item.(*tview.Checkbox); ok {
			return cb.IsChecked()
		}
	}
	return false
}

// dlgInputValue returns the trimmed text of the named input in form.
func dlgInputValue(form *tview.Form, label string) string {
	if item := form.GetFormItemByLabel(label); item != nil {
		if in, ok := item.(*tview.InputField); ok {
			return in.GetText()
		}
	}
	return ""
}

// NewFindDialog builds the modal Find dialog: a "Find What" input, "Match
// case" and "Whole word" checkboxes, and Find Next / Cancel buttons. onFind
// receives the query and option flags; onCancel fires on Cancel/Esc.
func NewFindDialog(initial string, onFind func(query string, matchCase, wholeWord bool), onCancel func()) (tview.Primitive, int, int) {
	form := dlgSearchForm()
	form.AddInputField("Find What", initial, 40, nil, nil)
	form.AddCheckbox("Match case", false, nil)
	form.AddCheckbox("Whole word", false, nil)
	form.AddButton("Find Next", func() {
		if onFind != nil {
			onFind(dlgInputValue(form, "Find What"),
				dlgCheckboxState(form, "Match case"),
				dlgCheckboxState(form, "Whole word"))
		}
	})
	form.AddButton("Cancel", func() {
		if onCancel != nil {
			onCancel()
		}
	})
	form.SetCancelFunc(func() {
		if onCancel != nil {
			onCancel()
		}
	})

	body := tview.NewFlex().SetDirection(tview.FlexRow)
	body.SetBackgroundColor(theme.LGray)
	body.AddItem(form, 0, 1, true)

	frame := dlgNewFrame("Find", body)
	return frame, 50, 10
}

// NewReplaceDialog builds the modal Replace dialog: "Find What" and "Replace
// With" inputs, "Match case" and "Whole word" checkboxes, and Replace /
// Replace All / Cancel buttons. onReplace receives both strings, the option
// flags and whether "all" was requested; onCancel fires on Cancel/Esc.
func NewReplaceDialog(onReplace func(find, replace string, matchCase, wholeWord, all bool), onCancel func()) (tview.Primitive, int, int) {
	form := dlgSearchForm()
	form.AddInputField("Find What", "", 40, nil, nil)
	form.AddInputField("Replace With", "", 40, nil, nil)
	form.AddCheckbox("Match case", false, nil)
	form.AddCheckbox("Whole word", false, nil)
	fire := func(all bool) {
		if onReplace != nil {
			onReplace(dlgInputValue(form, "Find What"),
				dlgInputValue(form, "Replace With"),
				dlgCheckboxState(form, "Match case"),
				dlgCheckboxState(form, "Whole word"),
				all)
		}
	}
	form.AddButton("Replace", func() { fire(false) })
	form.AddButton("Replace All", func() { fire(true) })
	form.AddButton("Cancel", func() {
		if onCancel != nil {
			onCancel()
		}
	})
	form.SetCancelFunc(func() {
		if onCancel != nil {
			onCancel()
		}
	})

	body := tview.NewFlex().SetDirection(tview.FlexRow)
	body.SetBackgroundColor(theme.LGray)
	body.AddItem(form, 0, 1, true)

	frame := dlgNewFrame("Replace", body)
	return frame, 52, 12
}

// NewGotoLineDialog builds the modal Go To Line dialog: a numeric "Line
// Number" input with OK / Cancel buttons. onOK receives the parsed line
// number (only when a positive integer was entered); onCancel fires on
// Cancel/Esc.
func NewGotoLineDialog(onOK func(line int), onCancel func()) (tview.Primitive, int, int) {
	form := tview.NewForm()
	dlgStyleForm(form)
	form.AddInputField("Line Number", "", 12, func(textToCheck string, lastChar rune) bool {
		return lastChar >= '0' && lastChar <= '9'
	}, nil)
	form.AddButton("OK", func() {
		v := strings.TrimSpace(dlgInputValue(form, "Line Number"))
		if n, err := strconv.Atoi(v); err == nil && n > 0 && onOK != nil {
			onOK(n)
		}
	})
	form.AddButton("Cancel", func() {
		if onCancel != nil {
			onCancel()
		}
	})
	form.SetCancelFunc(func() {
		if onCancel != nil {
			onCancel()
		}
	})

	body := tview.NewFlex().SetDirection(tview.FlexRow)
	body.SetBackgroundColor(theme.LGray)
	body.AddItem(form, 0, 1, true)

	frame := dlgNewFrame("Go To Line", body)
	return frame, 40, 8
}

// NewAboutDialog builds the modal About box with product credits and a single
// OK button. onOK fires on OK or Esc.
func NewAboutDialog(onOK func()) (tview.Primitive, int, int) {
	const about = "DOSEdit\n\nA terminal text editor in the style of\nVisual Basic for DOS 1.0 / QuickBASIC 4.5\n\nBuilt with Go, tcell and tview."

	tv := tview.NewTextView()
	tv.SetText(about)
	tv.SetWrap(true)
	tv.SetTextAlign(tview.AlignCenter)
	tv.SetTextStyle(theme.DialogBody())
	tv.SetBackgroundColor(theme.LGray)

	form := tview.NewForm()
	dlgStyleForm(form)
	form.AddButton("OK", func() {
		if onOK != nil {
			onOK()
		}
	})
	form.SetCancelFunc(func() {
		if onOK != nil {
			onOK()
		}
	})

	body := tview.NewFlex().SetDirection(tview.FlexRow)
	body.SetBackgroundColor(theme.LGray)
	body.AddItem(tv, 0, 1, false)
	body.AddItem(form, 1, 0, true)

	frame := dlgNewFrame("About DOSEdit", body)
	return frame, 46, 14
}
