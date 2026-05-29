// This file implements DOSEdit's modal dialogs (spec §6.6): Open, Save As,
// MessageBox, Find, Replace, Go To Line, Options and About.
//
// Every dialog is built on ONE unified Dialog primitive. A Dialog draws the DOS
// chrome itself — a double-line frame, a centred magenta/white title bar, a
// light-gray body and a one-cell solid-black drop shadow on the right and bottom
// edges — and owns a flat ordered ring of focusable controls. Non-button
// controls (input fields, checkboxes, lists, caption lines) stack vertically in
// the body; command buttons (OK / Cancel / Help …) stack vertically down the
// right-hand side, matching the VBDOS "New Form" template (spec §6.6).
//
// The Dialog keeps the tview Application focus on ITSELF and forwards keys to the
// currently focused control. tview delivers key events only to the focused leaf
// primitive (not through ancestor containers), so a container input-capture never
// sees Tab. By holding focus at the Dialog level we guarantee uniform, trapped
// Tab/Shift+Tab cycling, Enter→default-button and Esc→cancel for every dialog.
//
// Nothing in this file references symbols from sibling files in package ui, and
// every unexported identifier is prefixed dlg to avoid collisions.
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

// ----------------------------------------------------------------------------
// Small drawing helpers
// ----------------------------------------------------------------------------

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

// ----------------------------------------------------------------------------
// Dialog: the unified modal primitive
// ----------------------------------------------------------------------------

// Dialog is the single modal primitive every DOSEdit dialog is built on. It
// implements tview.Primitive and owns a flat ordered focus ring of controls,
// drawing the DOS chrome (double frame, magenta title bar, light-gray body,
// drop shadow) itself.
type Dialog struct {
	*tview.Box

	title string

	// body holds the non-button controls (inputs, checkboxes, lists, captions),
	// stacked vertically in the left/main body area. buttons holds the command
	// buttons, stacked vertically down the right-hand side.
	body    []tview.Primitive
	buttons []*tview.Button

	// ring is the flat ordered focus ring: focusable body controls followed by
	// the buttons. index is the currently focused entry.
	ring  []tview.Primitive
	index int

	def    *tview.Button // Enter target; first button if unset
	cancel func()        // Esc / Cancel action

	width, height int
}

// NewDialog creates an empty Dialog with the given title and a sane default
// size. Callers add controls via the builder methods, then SetSize as needed.
func NewDialog(title string) *Dialog {
	d := &Dialog{
		Box:    tview.NewBox(),
		title:  title,
		width:  40,
		height: 10,
	}
	d.Box.SetBackgroundColor(theme.LGray)
	return d
}

// SetSize sets the dialog's outer size (including frame and shadow). The
// application centres the dialog using Size().
func (d *Dialog) SetSize(w, h int) {
	d.width, d.height = w, h
}

// Size returns the dialog's outer size so the application can centre it.
func (d *Dialog) Size() (w, h int) {
	return d.width, d.height
}

// SetCancel sets the action invoked by Esc and by a Cancel button.
func (d *Dialog) SetCancel(fn func()) {
	d.cancel = fn
}

// SetDefault sets the button activated by Enter when focus is not on a button.
func (d *Dialog) SetDefault(b *tview.Button) {
	d.def = b
}

// dlgRegister adds a control to the focus ring.
func (d *Dialog) dlgRegister(p tview.Primitive) {
	d.ring = append(d.ring, p)
}

// rebuildRing recomputes the focus ring: focusable body controls first, then
// the command buttons. (Non-focusable captions/path lines are excluded.)
func (d *Dialog) rebuildRing() {
	d.ring = d.ring[:0]
	for _, p := range d.body {
		if _, ok := p.(*tview.TextView); ok {
			continue // captions / path lines are not focusable
		}
		d.ring = append(d.ring, p)
	}
	for _, b := range d.buttons {
		d.ring = append(d.ring, b)
	}
	if d.index >= len(d.ring) {
		d.index = 0
	}
}

// ----------------------------------------------------------------------------
// Builder API
// ----------------------------------------------------------------------------

// AddField adds a labelled white-on-black input field to the body and returns
// it so callers can read the value.
func (d *Dialog) AddField(label, value string, fieldWidth int) *tview.InputField {
	in := tview.NewInputField()
	in.SetLabel(label)
	in.SetText(value)
	in.SetFieldWidth(fieldWidth)
	in.SetLabelColor(theme.Black)
	in.SetFieldStyle(theme.InputField())
	in.SetBackgroundColor(theme.LGray)
	d.body = append(d.body, in)
	d.rebuildRing()
	return in
}

// AddCheckbox adds a labelled checkbox to the body and returns it.
func (d *Dialog) AddCheckbox(label string, checked bool) *tview.Checkbox {
	cb := tview.NewCheckbox()
	cb.SetLabel(label + " ")
	cb.SetChecked(checked)
	cb.SetLabelColor(theme.Black)
	cb.SetFieldTextColor(theme.White)
	cb.SetFieldBackgroundColor(theme.Black)
	cb.SetBackgroundColor(theme.LGray)
	d.body = append(d.body, cb)
	d.rebuildRing()
	return cb
}

// AddList adds a black-on-white scrollable list box to the body and returns it.
func (d *Dialog) AddList() *tview.List {
	list := tview.NewList()
	list.ShowSecondaryText(false)
	list.SetBackgroundColor(theme.White)
	list.SetMainTextStyle(theme.ListBox())
	list.SetSelectedStyle(theme.ListSelected())
	list.SetWrapAround(true)
	d.body = append(d.body, list)
	d.rebuildRing()
	return list
}

// AddTextLine adds a non-focusable light-gray caption / path line to the body.
func (d *Dialog) AddTextLine(text string) *tview.TextView {
	tv := tview.NewTextView()
	tv.SetText(text)
	tv.SetWrap(false)
	tv.SetTextStyle(theme.DialogBody())
	tv.SetBackgroundColor(theme.LGray)
	d.body = append(d.body, tv)
	// captions are not focusable, but rebuildRing skips them anyway.
	d.rebuildRing()
	return tv
}

// AddButton adds a command button to the vertical button column and returns it.
// The first button added becomes the default unless SetDefault overrides it.
func (d *Dialog) AddButton(label string, action func()) *tview.Button {
	b := tview.NewButton(label)
	b.SetStyle(theme.ButtonFace())
	// Focused/default button: reverse video (white-on-black), matching the
	// VB-for-DOS default-button treatment.
	b.SetActivatedStyle(tcell.StyleDefault.Foreground(theme.White).Background(theme.Black))
	if action != nil {
		b.SetSelectedFunc(action)
	}
	d.buttons = append(d.buttons, b)
	if d.def == nil {
		d.def = b
	}
	d.rebuildRing()
	return b
}

// ----------------------------------------------------------------------------
// Layout + drawing
// ----------------------------------------------------------------------------

// frameRect returns the frame rectangle (excluding the one-cell shadow on the
// right/bottom) given the dialog's outer rect.
func (d *Dialog) frameRect() (x, y, w, h int) {
	x, y, ow, oh := d.GetRect()
	w = ow - 1 // shadow occupies the last column
	h = oh - 1 // shadow occupies the last row
	if w < 1 {
		w = ow
	}
	if h < 1 {
		h = oh
	}
	return x, y, w, h
}

// innerRect returns the body rectangle inside the frame: skip the left/right
// borders, the top border + title row, and the bottom border.
func (d *Dialog) innerRect() (int, int, int, int) {
	x, y, w, h := d.frameRect()
	if w < 4 || h < 4 {
		return x + 1, y + 1, dlgMaxInt(w-2, 0), dlgMaxInt(h-2, 0)
	}
	return x + 1, y + 2, w - 2, h - 3
}

// layout positions every body control and button within the inner rectangle.
// Body controls stack vertically on the left; buttons stack vertically down a
// fixed-width column on the right.
func (d *Dialog) layout() {
	ix, iy, iw, ih := d.innerRect()
	if iw <= 0 || ih <= 0 {
		return
	}

	// Right-hand button column width = widest button label + 4 (padding), if any.
	btnW := 0
	for _, b := range d.buttons {
		if w := len([]rune(b.GetLabel())) + 4; w > btnW {
			btnW = w
		}
	}
	gap := 0
	if btnW > 0 {
		gap = 2
	}
	// Clamp so the body keeps at least a few columns on tiny terminals.
	if btnW+gap >= iw {
		btnW = dlgMaxInt(iw/3, 0)
		gap = 0
	}

	bodyX := ix + 1
	bodyW := iw - 2 - btnW - gap
	if bodyW < 1 {
		bodyW = dlgMaxInt(iw-2, 1)
	}

	// Stack body controls vertically. Lists are greedy (share the remaining
	// height); single-line controls take one row.
	var lists []tview.Primitive
	fixedRows := 0
	for _, p := range d.body {
		if _, ok := p.(*tview.List); ok {
			lists = append(lists, p)
		} else {
			fixedRows++
		}
	}
	avail := ih - fixedRows
	if avail < 0 {
		avail = 0
	}
	listH := 0
	if len(lists) > 0 {
		listH = avail / len(lists)
		if listH < 1 {
			listH = 1
		}
	}

	row := iy
	bottom := iy + ih
	for _, p := range d.body {
		if row >= bottom {
			p.SetRect(bodyX, bottom-1, bodyW, 0)
			continue
		}
		h := 1
		if _, ok := p.(*tview.List); ok {
			h = listH
			if row+h > bottom {
				h = bottom - row
			}
		}
		p.SetRect(bodyX, row, bodyW, h)
		row += h
	}

	// Buttons: vertical column on the right, top-aligned.
	if btnW > 0 {
		bx := ix + iw - btnW
		by := iy
		for _, b := range d.buttons {
			if by >= bottom {
				b.SetRect(bx, bottom-1, btnW, 0)
				continue
			}
			b.SetRect(bx, by, btnW, 1)
			by += 2 // one blank row between buttons
		}
	}
}

// Draw paints the chrome (shadow, frame, title bar, body fill) then lays out and
// draws every control. It is robust on small terminals and never panics.
func (d *Dialog) Draw(screen tcell.Screen) {
	d.Box.DrawForSubclass(screen, d)

	ox, oy, ow, oh := d.GetRect()
	if ow < 4 || oh < 3 {
		return
	}

	fx, fy, fw, fh := d.frameRect()
	right := fx + fw - 1
	bottom := fy + fh - 1

	body := theme.DialogBody()
	title := theme.DialogTitle()
	shadow := theme.Shadow()

	// Fill the frame interior with the dialog background.
	for r := fy; r <= bottom; r++ {
		for c := fx; c <= right; c++ {
			screen.SetContent(c, r, ' ', nil, body)
		}
	}

	// Double-line frame.
	screen.SetContent(fx, fy, theme.TLDouble, nil, body)
	screen.SetContent(right, fy, theme.TRDouble, nil, body)
	screen.SetContent(fx, bottom, theme.BLDouble, nil, body)
	screen.SetContent(right, bottom, theme.BRDouble, nil, body)
	for c := fx + 1; c < right; c++ {
		screen.SetContent(c, fy, theme.HDouble, nil, body)
		screen.SetContent(c, bottom, theme.HDouble, nil, body)
	}
	for r := fy + 1; r < bottom; r++ {
		screen.SetContent(fx, r, theme.VDouble, nil, body)
		screen.SetContent(right, r, theme.VDouble, nil, body)
	}

	// Magenta title bar on the row just below the top border.
	titleRow := fy + 1
	if titleRow < bottom {
		for c := fx + 1; c < right; c++ {
			screen.SetContent(c, titleRow, ' ', nil, title)
		}
		avail := fw - 2
		if avail > 0 {
			label := dlgClip(d.title, avail)
			start := fx + 1 + (avail-len([]rune(label)))/2
			for i, r := range []rune(label) {
				screen.SetContent(start+i, titleRow, r, nil, title)
			}
		}
	}

	// Solid-black drop shadow: right column and bottom row, offset by one.
	maxX := ox + ow - 1
	maxY := oy + oh - 1
	for r := fy + 1; r <= bottom+1 && r <= maxY; r++ {
		screen.SetContent(right+1, r, ' ', nil, shadow)
	}
	for c := fx + 1; c <= right+1 && c <= maxX; c++ {
		screen.SetContent(c, bottom+1, ' ', nil, shadow)
	}

	// Lay out and draw all controls.
	d.layout()
	for _, p := range d.body {
		_, _, w, h := p.GetRect()
		if w > 0 && h > 0 {
			p.Draw(screen)
		}
	}
	for _, b := range d.buttons {
		_, _, w, h := b.GetRect()
		if w > 0 && h > 0 {
			b.Draw(screen)
		}
	}
}

// ----------------------------------------------------------------------------
// Focus + input
// ----------------------------------------------------------------------------

// focusRing gives internal focus to ring entry i and drops it from the others.
// The Application focus stays on the Dialog; only the child's internal focus
// flag flips (via a no-op delegate).
func (d *Dialog) focusRing(i int) {
	if len(d.ring) == 0 {
		return
	}
	d.index = (i%len(d.ring) + len(d.ring)) % len(d.ring)
	for j, c := range d.ring {
		if j == d.index {
			c.Focus(func(tview.Primitive) {})
		} else if c.HasFocus() {
			c.Blur()
		}
	}
}

// Focus keeps the Application focus on the Dialog itself and asserts internal
// focus on the current control. delegate is intentionally ignored.
func (d *Dialog) Focus(delegate func(p tview.Primitive)) {
	d.Box.Focus(delegate)
	d.focusRing(d.index)
}

// HasFocus reports focus on the Dialog or any of its controls.
func (d *Dialog) HasFocus() bool {
	if d.Box.HasFocus() {
		return true
	}
	for _, c := range d.ring {
		if c.HasFocus() {
			return true
		}
	}
	return false
}

// keepFocus wraps setFocus so a control cannot steal the Application focus away
// from the Dialog: any request to focus a ring control is redirected to focusing
// the Dialog itself (which re-asserts the child's internal focus). Requests for
// anything else pass through unchanged.
func (d *Dialog) keepFocus(setFocus func(p tview.Primitive)) func(p tview.Primitive) {
	return func(p tview.Primitive) {
		for _, c := range d.ring {
			if c == p {
				setFocus(d)
				return
			}
		}
		setFocus(p)
	}
}

// currentIsButton reports whether the focused ring entry is a command button.
func (d *Dialog) currentIsButton() (*tview.Button, bool) {
	if d.index < 0 || d.index >= len(d.ring) {
		return nil, false
	}
	b, ok := d.ring[d.index].(*tview.Button)
	return b, ok
}

// fire activates a button's selected handler.
func dlgFire(b *tview.Button) {
	if b == nil || b.IsDisabled() {
		return
	}
	if h := b.InputHandler(); h != nil {
		h(tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone), func(tview.Primitive) {})
	}
}

// InputHandler implements the uniform key semantics: Tab/Shift+Tab cycle the
// (trapped) focus ring, Enter activates the focused button or the default
// button, Esc invokes the cancel action, and all other keys are forwarded to
// the focused control.
func (d *Dialog) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return d.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyTab:
			d.focusRing(d.index + 1)
			return
		case tcell.KeyBacktab:
			d.focusRing(d.index - 1)
			return
		case tcell.KeyEscape:
			if d.cancel != nil {
				d.cancel()
			}
			return
		case tcell.KeyEnter:
			if b, ok := d.currentIsButton(); ok {
				dlgFire(b)
				return
			}
			dlgFire(d.def)
			return
		}
		if len(d.ring) == 0 {
			return
		}
		child := d.ring[d.index]
		if h := child.InputHandler(); h != nil {
			h(event, d.keepFocus(setFocus))
		}
	})
}

// MouseHandler forwards mouse events to the controls; a left click on a control
// also selects it in the focus ring so subsequent keys go there.
func (d *Dialog) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return d.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		keep := d.keepFocus(setFocus)
		for i, c := range d.ring {
			consumed, capture := c.MouseHandler()(action, event, keep)
			if consumed {
				if action == tview.MouseLeftDown || action == tview.MouseLeftClick {
					d.focusRing(i)
				}
				return consumed, capture
			}
		}
		return false, nil
	})
}

// ----------------------------------------------------------------------------
// File dialog support (Open / Save As)
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
	nameIn  *tview.InputField
	pathTV  *tview.TextView
	dirList *tview.List
	fileLst *tview.List
	dir     string
	filter  string
}

// nameField returns the trimmed "File Name" input value.
func (s *dlgFileDialog) nameField() string {
	return strings.TrimSpace(s.nameIn.GetText())
}

// refresh re-reads the current directory and repopulates the lists and the
// current-path line.
func (s *dlgFileDialog) refresh() {
	s.pathTV.SetText(dlgClip(s.dir, 200))
	dirs, files := dlgListEntries(s.dir, s.filter)

	s.dirList.Clear()
	for _, name := range dirs {
		nm := name
		s.dirList.AddItem("["+nm+"]", "", 0, func() {
			var next string
			if nm == ".." {
				next = filepath.Dir(s.dir)
			} else {
				next = filepath.Join(s.dir, nm)
			}
			s.dir = dlgResolveDir(next)
			s.refresh()
		})
	}

	s.fileLst.Clear()
	for _, name := range files {
		nm := name
		s.fileLst.AddItem(nm, "", 0, func() {
			s.nameIn.SetText(nm)
		})
	}
}

// chosenPath resolves the current "File Name" field against the current
// directory, returning the absolute path to hand to onOK. Returns "" when the
// field is empty or holds a bare glob.
func (s *dlgFileDialog) chosenPath() string {
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

// dlgBuildFileDialog constructs the shared Open/Save As layout: a File Name
// field, a current-path line, side-by-side Directories and Files lists, and
// OK / Cancel buttons stacked down the right. It preserves the original
// navigation behaviour (".." and subdirs via os.ReadDir, dir-select refreshes,
// file-select fills the name field, OK resolves the full path).
func dlgBuildFileDialog(title, startDir, filter, nameDefault, okLabel string, onOK func(path string), onCancel func()) *Dialog {
	s := &dlgFileDialog{
		dir:    dlgResolveDir(startDir),
		filter: filter,
	}

	d := NewDialog(title)
	d.SetSize(54, 20)

	cancel := func() {
		if onCancel != nil {
			onCancel()
		}
	}
	d.SetCancel(cancel)

	// Body: File Name field, current-path line, Directories list, Files list.
	s.nameIn = d.AddField("File Name ", nameDefault, 40)
	s.pathTV = d.AddTextLine("")
	s.dirList = d.AddList()
	s.fileLst = d.AddList()

	s.refresh()

	okBtn := d.AddButton(okLabel, func() {
		if p := s.chosenPath(); p != "" && onOK != nil {
			onOK(p)
		}
	})
	d.AddButton("Cancel", cancel)
	d.SetDefault(okBtn)

	return d
}

// ----------------------------------------------------------------------------
// Public constructors
// ----------------------------------------------------------------------------

// NewOpenDialog builds the modal File Open dialog. The File Name field defaults
// to filter (e.g. "*.*"); selecting a file and confirming calls onOK with the
// full path. onCancel fires on Cancel/Esc.
func NewOpenDialog(startDir, filter string, onOK func(path string), onCancel func()) *Dialog {
	def := filter
	if def == "" {
		def = "*.*"
	}
	return dlgBuildFileDialog("Open", startDir, def, def, "OK", onOK, onCancel)
}

// NewSaveAsDialog builds the modal Save As dialog. The File Name field is seeded
// with suggestedName; confirming calls onOK with the full path. onCancel fires
// on Cancel/Esc.
func NewSaveAsDialog(startDir, suggestedName string, onOK func(path string), onCancel func()) *Dialog {
	return dlgBuildFileDialog("Save As", startDir, "*.*", suggestedName, "Save", onOK, onCancel)
}

// NewMessageBox builds a modal message box with a centred message and the given
// buttons stacked down the right. onResult receives the pressed button index,
// or -1 when dismissed with Esc.
func NewMessageBox(title, message string, buttons []string, onResult func(idx int)) *Dialog {
	if len(buttons) == 0 {
		buttons = []string{"OK"}
	}

	d := NewDialog(title)

	msg := d.AddTextLine(message)
	msg.SetWrap(true)
	msg.SetTextAlign(tview.AlignCenter)

	d.SetCancel(func() {
		if onResult != nil {
			onResult(-1)
		}
	})
	for i, label := range buttons {
		idx := i
		b := d.AddButton(label, func() {
			if onResult != nil {
				onResult(idx)
			}
		})
		if i == 0 {
			d.SetDefault(b)
		}
	}

	// Size: fit the message and the widest button.
	lines := strings.Split(message, "\n")
	maxLine := 0
	for _, ln := range lines {
		if l := len([]rune(ln)); l > maxLine {
			maxLine = l
		}
	}
	btnW := 0
	for _, b := range buttons {
		if w := len([]rune(b)) + 4; w > btnW {
			btnW = w
		}
	}
	w := maxLine + btnW + 10
	if w < 32 {
		w = 32
	}
	if w > 72 {
		w = 72
	}
	h := len(lines) + 6
	if n := len(buttons)*2 + 4; n > h {
		h = n
	}
	if h < 8 {
		h = 8
	}
	d.SetSize(w, h)
	return d
}

// NewFindDialog builds the modal Find dialog: a "Find What" input, "Match case"
// and "Whole word" checkboxes, and Find Next / Cancel buttons. onFind receives
// the query and option flags; onCancel fires on Cancel/Esc.
func NewFindDialog(initial string, onFind func(query string, matchCase, wholeWord bool), onCancel func()) *Dialog {
	d := NewDialog("Find")
	d.SetSize(50, 11)

	find := d.AddField("Find What ", initial, 30)
	matchCase := d.AddCheckbox("Match case", false)
	wholeWord := d.AddCheckbox("Whole word", false)

	cancel := func() {
		if onCancel != nil {
			onCancel()
		}
	}
	d.SetCancel(cancel)
	findBtn := d.AddButton("Find Next", func() {
		if onFind != nil {
			onFind(find.GetText(), matchCase.IsChecked(), wholeWord.IsChecked())
		}
	})
	d.AddButton("Cancel", cancel)
	d.SetDefault(findBtn)
	return d
}

// NewReplaceDialog builds the modal Replace dialog: "Find What" and "Replace
// With" inputs, "Match case" and "Whole word" checkboxes, and Replace /
// Replace All / Cancel buttons. onReplace receives both strings, the option
// flags and whether "all" was requested; onCancel fires on Cancel/Esc.
func NewReplaceDialog(onReplace func(find, replace string, matchCase, wholeWord, all bool), onCancel func()) *Dialog {
	d := NewDialog("Replace")
	d.SetSize(54, 13)

	find := d.AddField("Find What    ", "", 30)
	repl := d.AddField("Replace With ", "", 30)
	matchCase := d.AddCheckbox("Match case", false)
	wholeWord := d.AddCheckbox("Whole word", false)

	cancel := func() {
		if onCancel != nil {
			onCancel()
		}
	}
	d.SetCancel(cancel)

	fire := func(all bool) {
		if onReplace != nil {
			onReplace(find.GetText(), repl.GetText(),
				matchCase.IsChecked(), wholeWord.IsChecked(), all)
		}
	}
	replBtn := d.AddButton("Replace", func() { fire(false) })
	d.AddButton("Replace All", func() { fire(true) })
	d.AddButton("Cancel", cancel)
	d.SetDefault(replBtn)
	return d
}

// NewGotoLineDialog builds the modal Go To Line dialog: a numeric "Line Number"
// input with OK / Cancel buttons. onOK receives the parsed line number (only
// when a positive integer was entered); onCancel fires on Cancel/Esc.
func NewGotoLineDialog(onOK func(line int), onCancel func()) *Dialog {
	d := NewDialog("Go To Line")
	d.SetSize(40, 8)

	in := d.AddField("Line Number ", "", 12)
	in.SetAcceptanceFunc(func(textToCheck string, lastChar rune) bool {
		return lastChar >= '0' && lastChar <= '9'
	})

	cancel := func() {
		if onCancel != nil {
			onCancel()
		}
	}
	d.SetCancel(cancel)
	okBtn := d.AddButton("OK", func() {
		v := strings.TrimSpace(in.GetText())
		if n, err := strconv.Atoi(v); err == nil && n > 0 && onOK != nil {
			onOK(n)
		}
	})
	d.AddButton("Cancel", cancel)
	d.SetDefault(okBtn)
	return d
}

// Options holds the user-configurable editor preferences shown in the Options
// dialog. It is a plain value type so additional fields can be added later
// without changing the dialog's call signature.
type Options struct {
	LineNumbers bool
}

// NewOptionsDialog builds the modal Options dialog: a "Line Numbers" checkbox
// (seeded from cur) with OK / Cancel buttons. OK reads the checkbox state and
// calls onOK; onCancel fires on Cancel/Esc.
func NewOptionsDialog(cur Options, onOK func(Options), onCancel func()) *Dialog {
	d := NewDialog("Options")
	d.SetSize(40, 9)

	lineNums := d.AddCheckbox("Line Numbers", cur.LineNumbers)

	cancel := func() {
		if onCancel != nil {
			onCancel()
		}
	}
	d.SetCancel(cancel)
	okBtn := d.AddButton("OK", func() {
		if onOK != nil {
			onOK(Options{LineNumbers: lineNums.IsChecked()})
		}
	})
	d.AddButton("Cancel", cancel)
	d.SetDefault(okBtn)
	return d
}

// NewAboutDialog builds the modal About box with product credits and a single
// OK button. onOK fires on OK or Esc.
func NewAboutDialog(onOK func()) *Dialog {
	const about = "DOSEdit\n\nA terminal text editor in the style of\nVisual Basic for DOS 1.0 / QuickBASIC 4.5\n\nBuilt with Go, tcell and tview."

	d := NewDialog("About DOSEdit")
	d.SetSize(46, 14)

	tv := d.AddTextLine(about)
	tv.SetWrap(true)
	tv.SetTextAlign(tview.AlignCenter)

	ok := func() {
		if onOK != nil {
			onOK()
		}
	}
	d.SetCancel(ok)
	okBtn := d.AddButton("OK", ok)
	d.SetDefault(okBtn)
	return d
}
