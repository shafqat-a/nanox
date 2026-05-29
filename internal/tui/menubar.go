package tui

import (
	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
)

// MenuBar is the top-row VBDOS menu bar. It draws each menu title left-aligned
// with a 2-cell lead and a space between titles, with a "Help" title pushed to
// the right edge. The active (open) title is painted reversed. Dropdowns are
// rendered through the App as a full-screen modal overlay (mbPopup), so the bar
// itself only owns the top row.
type MenuBar struct {
	BaseWidget
	menus      []*Menu
	helpIndex  int // index of the "Help" menu pushed to the right, or -1
	app        *App
	popup      *mbPopup
	active     int // index of the open menu, or -1 when closed
	onClose    func()
	onActivate func()
}

// NewMenuBar returns a MenuBar over the given menus. A menu titled "Help"
// (case-insensitive) is pushed to the right edge of the bar.
func NewMenuBar(menus []*Menu) *MenuBar {
	mb := &MenuBar{menus: menus, helpIndex: -1, active: -1}
	for i, m := range menus {
		if mbEqualFold(m.Title, "Help") {
			mb.helpIndex = i
		}
	}
	return mb
}

// SetApp wires the App used to push/pop the dropdown overlay. Without it,
// Activate is a no-op.
func (mb *MenuBar) SetApp(a *App) { mb.app = a }

// Menus returns the bar's menus.
func (mb *MenuBar) Menus() []*Menu { return mb.menus }

// SetOnClose registers a callback fired when the bar deactivates (dropdown and
// bar fully closed).
func (mb *MenuBar) SetOnClose(fn func()) { mb.onClose = fn }

// SetOnActivate registers a callback fired when the bar is first activated.
func (mb *MenuBar) SetOnActivate(fn func()) { mb.onActivate = fn }

// IsActive reports whether a menu is currently open.
func (mb *MenuBar) IsActive() bool { return mb.active >= 0 }

// Focusable reports true so the bar can receive keyboard focus on the root
// layer; activation itself is via Activate/OpenByMnemonic/click.
func (mb *MenuBar) Focusable() bool { return true }

// Activate opens the bar at the first menu. Requires SetApp; otherwise a no-op.
func (mb *MenuBar) Activate() {
	if mb.app == nil || len(mb.menus) == 0 {
		return
	}
	mb.openMenu(0)
	if mb.onActivate != nil {
		mb.onActivate()
	}
}

// OpenByMnemonic opens the menu whose top-level mnemonic matches r. Returns
// true if a menu was opened. Requires SetApp.
func (mb *MenuBar) OpenByMnemonic(r rune) bool {
	if mb.app == nil {
		return false
	}
	for i, m := range mb.menus {
		if mnuRuneEq(m.Mnemonic, r) {
			mb.openMenu(i)
			if mb.onActivate != nil {
				mb.onActivate()
			}
			return true
		}
	}
	return false
}

// openMenu opens (or switches to) the menu at index i, pushing the overlay on
// first open and just repositioning on subsequent switches.
func (mb *MenuBar) openMenu(i int) {
	if i < 0 || i >= len(mb.menus) {
		return
	}
	first := mb.active < 0
	mb.active = i
	if mb.popup == nil {
		mb.popup = newMBPopup(mb)
	}
	mb.popup.menu = mb.menus[i]
	mb.popup.hi = mnuFirstSelectable(mb.menus[i].Items)
	mb.popup.anchorX = mb.titleX(i)
	if first {
		mb.app.PushModal(mb.popup)
	} else {
		mb.app.Redraw()
	}
}

// close shuts the dropdown and deactivates the bar.
func (mb *MenuBar) close() {
	if mb.active < 0 {
		return
	}
	mb.active = -1
	if mb.app != nil && mb.popup != nil {
		mb.app.PopModal()
	}
	if mb.onClose != nil {
		mb.onClose()
	}
	if mb.app != nil {
		mb.app.Redraw()
	}
}

// switchTo moves to an adjacent top-level menu (dir +1/-1), wrapping, keeping
// the dropdown open.
func (mb *MenuBar) switchTo(dir int) {
	n := len(mb.menus)
	if n == 0 {
		return
	}
	i := (mb.active + dir) % n
	if i < 0 {
		i += n
	}
	mb.openMenu(i)
}

// titleX returns the absolute starting X column of the title at index i, using
// the same layout as Draw.
func (mb *MenuBar) titleX(i int) int {
	b := mb.Bounds()
	if i == mb.helpIndex {
		w := mnuRuneLen(mb.menus[i].Title)
		return b.X + b.W - w - 2
	}
	x := b.X + 2
	for j := 0; j < i; j++ {
		if j == mb.helpIndex {
			continue
		}
		x += mnuRuneLen(mb.menus[j].Title) + 1
	}
	return x
}

// titleHit returns the menu index whose title cell contains absolute x on the
// bar row, or -1.
func (mb *MenuBar) titleHit(x int) int {
	for i, m := range mb.menus {
		tx := mb.titleX(i)
		if x >= tx && x < tx+mnuRuneLen(m.Title) {
			return i
		}
	}
	return -1
}

// Draw paints the menu bar across its (top) row.
func (mb *MenuBar) Draw(s Surface) {
	b := mb.Bounds()
	if b.Empty() {
		return
	}
	row := b.Y
	normal := theme.MenuNormal()
	mnem := theme.MenuMnemonic()
	sel := theme.MenuSelect()

	s.Fill(Rect{X: b.X, Y: row, W: b.W, H: 1}, ' ', normal)

	for i, m := range mb.menus {
		tx := mb.titleX(i)
		style := normal
		mnemStyle := mnem
		if i == mb.active {
			style = sel
			mnemStyle = sel
		}
		mb.drawTitle(s, tx, row, m, style, mnemStyle)
	}
}

// drawTitle renders a single title with its mnemonic char highlighted.
func (mb *MenuBar) drawTitle(s Surface, x, y int, m *Menu, base, mnemStyle tcell.Style) {
	mnemDone := false
	for _, r := range m.Title {
		st := base
		if !mnemDone && mnuRuneEq(r, m.Mnemonic) {
			st = mnemStyle
			mnemDone = true
		}
		s.Set(x, y, r, st)
		x++
	}
}

// HandleMouse on the bar row: clicking a title opens (or toggles) that menu.
func (mb *MenuBar) HandleMouse(ev MouseEvent) bool {
	b := mb.Bounds()
	if ev.Action != MouseDown || ev.Y != b.Y {
		return false
	}
	i := mb.titleHit(ev.X)
	if i < 0 {
		return false
	}
	if mb.app == nil {
		return false
	}
	if mb.active == i {
		mb.close()
	} else {
		wasActive := mb.active >= 0
		mb.openMenu(i)
		if !wasActive && mb.onActivate != nil {
			mb.onActivate()
		}
	}
	return true
}

// mbEqualFold reports case-insensitive equality of ASCII-ish strings.
func mbEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) != len(rb) {
		return false
	}
	for i := range ra {
		if !mbFoldEq(ra[i], rb[i]) {
			return false
		}
	}
	return true
}

func mbFoldEq(a, b rune) bool {
	if a == b {
		return true
	}
	la, lb := a, b
	if la >= 'A' && la <= 'Z' {
		la += 'a' - 'A'
	}
	if lb >= 'A' && lb <= 'Z' {
		lb += 'a' - 'A'
	}
	return la == lb
}

// --- dropdown overlay -------------------------------------------------------

// mbPopup is the full-screen modal container that renders the open dropdown box
// just under the active title and routes all mouse/keyboard for the open menu.
type mbPopup struct {
	BaseWidget
	bar     *MenuBar
	menu    *Menu
	hi      int // highlighted item index, or -1
	anchorX int // absolute X of the active title (box left edge)
}

// newMBPopup creates a popup bound to its bar.
func newMBPopup(bar *MenuBar) *mbPopup {
	return &mbPopup{bar: bar, hi: -1}
}

// Focusable lets the modal layer route keys to the popup.
func (p *mbPopup) Focusable() bool { return true }

// boxRect computes the dropdown box rectangle (bordered, single line) in
// absolute coordinates, clamped to the screen.
func (p *mbPopup) boxRect() Rect {
	screen := p.Bounds()
	items := p.menu.Items
	inner := len(items)
	w := p.contentWidth() + 2 // borders
	h := inner + 2            // borders
	x := p.anchorX
	y := screen.Y + 1 // just under the bar (bar at row 0)
	// Clamp horizontally so the box (and its shadow) stay on screen.
	if x+w+1 > screen.X+screen.W {
		x = screen.X + screen.W - w - 1
	}
	if x < screen.X {
		x = screen.X
	}
	if y+h+1 > screen.Y+screen.H {
		// pull up if it would overflow the bottom (rare)
		if screen.H > h+1 {
			y = screen.Y + screen.H - h - 1
		}
	}
	return Rect{X: x, Y: y, W: w, H: h}
}

// contentWidth returns the inner text width of the dropdown (label + accel).
func (p *mbPopup) contentWidth() int {
	maxLbl := 0
	maxAcc := 0
	for _, it := range p.menu.Items {
		if it.Separator {
			continue
		}
		if l := mnuRuneLen(it.Label); l > maxLbl {
			maxLbl = l
		}
		if a := mnuRuneLen(it.Accel); a > maxAcc {
			maxAcc = a
		}
	}
	// 1 lead space + label + (gap + accel) + 1 trailing space.
	w := 1 + maxLbl + 1
	if maxAcc > 0 {
		w += 2 + maxAcc
	}
	if w < 6 {
		w = 6
	}
	return w
}

// Draw renders the dropdown box, its rows and the drop shadow.
func (p *mbPopup) Draw(s Surface) {
	if p.menu == nil {
		return
	}
	box := p.boxRect()
	body := theme.DropdownBody()
	hi := theme.DropdownHi()
	hiMnem := theme.DropdownHiMnemonic()
	disabled := theme.DropdownDisabled()
	shadow := theme.Shadow()

	// Drop shadow: one cell to the right and one row below the box.
	s.Fill(Rect{X: box.X + box.W, Y: box.Y + 1, W: 1, H: box.H}, ' ', shadow)
	s.Fill(Rect{X: box.X + 1, Y: box.Y + box.H, W: box.W, H: 1}, ' ', shadow)

	// Box background + border.
	s.Fill(box, ' ', body)
	left := box.X
	right := box.X + box.W - 1
	top := box.Y
	bottom := box.Y + box.H - 1
	s.Set(left, top, theme.TLSingle, body)
	s.Set(right, top, theme.TRSingle, body)
	s.Set(left, bottom, theme.BLSingle, body)
	s.Set(right, bottom, theme.BRSingle, body)
	for x := left + 1; x < right; x++ {
		s.Set(x, top, theme.HSingle, body)
		s.Set(x, bottom, theme.HSingle, body)
	}
	for y := top + 1; y < bottom; y++ {
		s.Set(left, y, theme.VSingle, body)
		s.Set(right, y, theme.VSingle, body)
	}

	// Rows.
	innerW := box.W - 2
	for i, it := range p.menu.Items {
		ry := box.Y + 1 + i
		if it.Separator {
			s.Set(left, ry, theme.TeeLeft, body)
			s.Set(right, ry, theme.TeeRight, body)
			for x := left + 1; x < right; x++ {
				s.Set(x, ry, theme.HSingle, body)
			}
			continue
		}
		rowStyle := body
		mnemStyle := body.Foreground(theme.White).Bold(true)
		if it.Disabled {
			rowStyle = disabled
			mnemStyle = disabled
		}
		if i == p.hi {
			rowStyle = hi
			mnemStyle = hiMnem
		}
		// Fill the inner row with the row style.
		s.Fill(Rect{X: left + 1, Y: ry, W: innerW, H: 1}, ' ', rowStyle)
		// Label with mnemonic highlight, 1-cell lead.
		lx := left + 2
		mnemDone := false
		for _, r := range it.Label {
			st := rowStyle
			if !mnemDone && mnuRuneEq(r, it.Mnemonic) {
				st = mnemStyle
				mnemDone = true
			}
			s.Set(lx, ry, r, st)
			lx++
		}
		// Accelerator, right-aligned within the inner row.
		if it.Accel != "" {
			ax := right - 1 - mnuRuneLen(it.Accel)
			cx := ax
			for _, r := range it.Accel {
				s.Set(cx, ry, r, rowStyle)
				cx++
			}
		}
	}
}

// HandleKey implements dropdown keyboard navigation.
func (p *mbPopup) HandleKey(ev *tcell.EventKey) bool {
	if p.menu == nil {
		return false
	}
	switch ev.Key() {
	case tcell.KeyEscape:
		p.bar.close()
		return true
	case tcell.KeyUp:
		if p.hi < 0 {
			p.hi = mnuFirstSelectable(p.menu.Items)
		} else {
			p.hi = mnuNextSelectable(p.menu.Items, p.hi, -1)
		}
		p.redraw()
		return true
	case tcell.KeyDown:
		if p.hi < 0 {
			p.hi = mnuFirstSelectable(p.menu.Items)
		} else {
			p.hi = mnuNextSelectable(p.menu.Items, p.hi, +1)
		}
		p.redraw()
		return true
	case tcell.KeyLeft:
		p.bar.switchTo(-1)
		return true
	case tcell.KeyRight:
		p.bar.switchTo(+1)
		return true
	case tcell.KeyEnter:
		p.activate(p.hi)
		return true
	case tcell.KeyRune:
		r := ev.Rune()
		for i, it := range p.menu.Items {
			if mnuSelectable(p.menu.Items, i) && mnuRuneEq(it.Mnemonic, r) {
				p.activate(i)
				return true
			}
		}
		return true // swallow other runes while open
	}
	return false
}

// activate runs the item's action (if selectable) and closes the bar.
func (p *mbPopup) activate(i int) {
	if !mnuSelectable(p.menu.Items, i) {
		return
	}
	action := p.menu.Items[i].Action
	p.bar.close()
	if action != nil {
		action()
	}
}

// HandleMouse routes all mouse input (modal). Clicks on the bar titles switch
// menus; clicks inside the box highlight/activate rows; clicks elsewhere close.
func (p *mbPopup) HandleMouse(ev MouseEvent) bool {
	if p.menu == nil {
		return true
	}
	// Click on the bar row: let the bar switch/toggle menus.
	barRow := p.bar.Bounds().Y
	if ev.Y == barRow {
		i := p.bar.titleHit(ev.X)
		if i >= 0 {
			if ev.Action == MouseDown {
				if i == p.bar.active {
					p.bar.close()
				} else {
					p.bar.openMenu(i)
				}
			}
			return true
		}
	}

	box := p.boxRect()
	inner := box.Inset(1, 1)
	if ev.X >= inner.X && ev.X < inner.X+inner.W && ev.Y >= inner.Y && ev.Y < inner.Y+inner.H {
		idx := ev.Y - inner.Y
		switch ev.Action {
		case MouseMove, MouseDrag:
			if mnuSelectable(p.menu.Items, idx) {
				p.hi = idx
				p.redraw()
			}
		case MouseDown, MouseUp:
			if mnuSelectable(p.menu.Items, idx) {
				p.activate(idx)
			}
		}
		return true
	}

	// Click anywhere else closes the menu.
	if ev.Action == MouseDown || ev.Action == MouseUp {
		p.bar.close()
	}
	return true
}

// redraw asks the app to repaint.
func (p *mbPopup) redraw() {
	if p.bar.app != nil {
		p.bar.app.Redraw()
	}
}
