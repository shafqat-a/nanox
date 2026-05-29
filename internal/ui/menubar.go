package ui

import (
	"unicode"

	"dosedit/internal/theme"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// MenuItem is a single entry in a dropdown menu. APP populates these.
type MenuItem struct {
	Label     string
	Mnemonic  rune
	Accel     string
	Action    func()
	Separator bool
	Disabled  bool
}

// Menu is one top-level menu (a title plus its dropdown items).
type Menu struct {
	Title    string
	Mnemonic rune
	Items    []MenuItem
}

// MenuBar is the custom row-0 menu-bar primitive. It draws the top bar and,
// when a menu is open, the dropdown (anywhere on screen, below its title).
type MenuBar struct {
	*tview.Box

	mbMenus      []*Menu
	mbActive     bool // bar has focus
	mbOpen       bool // a dropdown is open
	mbSel        int  // index into mbMenus of the highlighted/open top-level menu
	mbItem       int  // index into the open menu's Items of the highlighted row
	mbOnClose    func()
	mbOnActivate func()
}

// NewMenuBar builds a menu bar over the supplied menus.
func NewMenuBar(menus []*Menu) *MenuBar {
	m := &MenuBar{
		Box:     tview.NewBox(),
		mbMenus: menus,
	}
	return m
}

// --- exported integration API (called by APP) ---

// Activate focuses the bar and highlights the first top-level menu.
func (m *MenuBar) Activate() {
	if len(m.mbMenus) == 0 {
		return
	}
	m.mbActive = true
	m.mbOpen = false
	m.mbSel = 0
	m.mbItem = 0
	m.mbFireActivate()
}

// OpenByMnemonic activates the bar and opens the menu whose mnemonic matches r.
// Returns true if a menu was opened.
func (m *MenuBar) OpenByMnemonic(r rune) bool {
	r = unicode.ToLower(r)
	for i, mn := range m.mbMenus {
		if unicode.ToLower(mn.Mnemonic) == r {
			m.mbActive = true
			m.mbSel = i
			m.mbFireActivate()
			m.mbOpenMenu()
			return true
		}
	}
	return false
}

// SetOnClose registers the callback fired when the bar fully closes.
func (m *MenuBar) SetOnClose(fn func()) { m.mbOnClose = fn }

// SetOnActivate registers the callback fired when the bar becomes active (so the
// app can switch the status bar to the menu context). It fires on both keyboard
// and mouse activation.
func (m *MenuBar) SetOnActivate(fn func()) { m.mbOnActivate = fn }

// mbFireActivate invokes the activation callback if one is registered.
func (m *MenuBar) mbFireActivate() {
	if m.mbOnActivate != nil {
		m.mbOnActivate()
	}
}

// IsActive reports whether the bar currently holds focus.
func (m *MenuBar) IsActive() bool { return m.mbActive }

// Menus returns the configured menus (for APP's accelerator table).
func (m *MenuBar) Menus() []*Menu { return m.mbMenus }

// --- internal state transitions ---

func (m *MenuBar) mbClose() {
	m.mbActive = false
	m.mbOpen = false
	if m.mbOnClose != nil {
		m.mbOnClose()
	}
}

func (m *MenuBar) mbOpenMenu() {
	m.mbOpen = true
	m.mbItem = m.mbFirstSelectable(m.mbSel)
}

// mbFirstSelectable returns the first selectable item index in menu i, or 0.
func (m *MenuBar) mbFirstSelectable(i int) int {
	if i < 0 || i >= len(m.mbMenus) {
		return 0
	}
	items := m.mbMenus[i].Items
	for j := range items {
		if !items[j].Separator && !items[j].Disabled {
			return j
		}
	}
	return 0
}

// mbStep advances the dropdown selection by dir, skipping separators/disabled.
func (m *MenuBar) mbStep(dir int) {
	items := m.mbMenus[m.mbSel].Items
	n := len(items)
	if n == 0 {
		return
	}
	i := m.mbItem
	for k := 0; k < n; k++ {
		i = (i + dir + n) % n
		if !items[i].Separator && !items[i].Disabled {
			m.mbItem = i
			return
		}
	}
}

// mbMoveTop moves the highlighted top-level menu by dir, wrapping.
func (m *MenuBar) mbMoveTop(dir int) {
	n := len(m.mbMenus)
	if n == 0 {
		return
	}
	m.mbSel = (m.mbSel + dir + n) % n
	if m.mbOpen {
		m.mbOpenMenu()
	}
}

// mbActivateItem runs the highlighted item's action and closes the bar.
func (m *MenuBar) mbActivateItem() {
	items := m.mbMenus[m.mbSel].Items
	if m.mbItem < 0 || m.mbItem >= len(items) {
		return
	}
	it := items[m.mbItem]
	if it.Separator || it.Disabled {
		return
	}
	m.mbClose()
	if it.Action != nil {
		it.Action()
	}
}

// mbHelpIndex returns the index of the last menu titled "Help", or -1.
func (m *MenuBar) mbHelpIndex() int {
	idx := -1
	for i, mn := range m.mbMenus {
		if mn.Title == "Help" {
			idx = i
		}
	}
	return idx
}

// --- geometry ---

// mbTitleX returns the screen column where menu i's title text begins, and the
// number of cells the title occupies (the title text only, not the pad space).
func (m *MenuBar) mbTitleX(i int) (int, int) {
	x, _, width, _ := m.GetRect()
	helpIdx := m.mbHelpIndex()
	// Left-aligned group: 2 leading spaces, one space between titles.
	cur := x + 2
	for j, mn := range m.mbMenus {
		w := len([]rune(mn.Title))
		if j == helpIdx {
			continue
		}
		if j == i {
			return cur, w
		}
		cur += w + 1
	}
	// Help is pushed to the right edge.
	if i == helpIdx && helpIdx >= 0 {
		w := len([]rune(m.mbMenus[helpIdx].Title))
		return x + width - w - 2, w
	}
	return cur, 0
}

// mbDropBounds returns the open dropdown's box geometry for menu i, in absolute
// screen coordinates: the box's top-left corner (bx,by), the inner content width
// and the box's outer width/height (including borders). Draw and hit-testing
// share this so the visible box and the clickable region stay identical.
func (m *MenuBar) mbDropBounds(i int) (bx, by, inner, boxW, boxH int) {
	x, y, _, _ := m.GetRect()
	tx, _ := m.mbTitleX(i)
	// Anchor the box so its left border sits one cell left of the title text,
	// keeping the title visually attached. Clamp to the bar's left edge.
	bx = tx - 1
	if bx < x {
		bx = x
	}
	by = y + 1
	inner = m.mbDropWidth(i)
	boxW = inner + 2 // borders
	boxH = len(m.mbMenus[i].Items) + 2
	return bx, by, inner, boxW, boxH
}

// mbDropWidth returns the inner content width of menu i's dropdown.
func (m *MenuBar) mbDropWidth(i int) int {
	maxLabel := 0
	maxAccel := 0
	hasAccel := false
	for _, it := range m.mbMenus[i].Items {
		if it.Separator {
			continue
		}
		l := len([]rune(it.Label))
		if l > maxLabel {
			maxLabel = l
		}
		a := len([]rune(it.Accel))
		if a > maxAccel {
			maxAccel = a
		}
		if a > 0 {
			hasAccel = true
		}
	}
	// 2 left pad + label + gap + accel + 2 right pad. Gap >= 2 when accels.
	gap := 0
	if hasAccel {
		gap = 2
	}
	return 2 + maxLabel + gap + maxAccel + 2
}

// --- drawing ---

// Draw renders the bar and, if open, the dropdown.
func (m *MenuBar) Draw(screen tcell.Screen) {
	m.DrawForSubclass(screen, m.Box)
	x, y, width, _ := m.GetRect()

	// Fill the whole bar with the normal menu style.
	normal := theme.MenuNormal()
	for col := x; col < x+width; col++ {
		screen.SetContent(col, y, ' ', nil, normal)
	}

	for i := range m.mbMenus {
		m.mbDrawTitle(screen, i, y)
	}

	if m.mbOpen {
		m.mbDrawDropdown(screen)
	}
}

// mbDrawTitle paints one top-level title with its mnemonic and selection state.
func (m *MenuBar) mbDrawTitle(screen tcell.Screen, i, y int) {
	tx, _ := m.mbTitleX(i)
	title := []rune(m.mbMenus[i].Title)
	selected := m.mbActive && i == m.mbSel
	base := theme.MenuNormal()
	mnem := theme.MenuMnemonic()
	if selected {
		base = theme.MenuSelect()
		mnem = theme.MenuSelect()
	}

	mnLower := unicode.ToLower(m.mbMenus[i].Mnemonic)
	mnemDone := false
	col := tx
	for _, r := range title {
		st := base
		if !mnemDone && mnLower != 0 && unicode.ToLower(r) == mnLower {
			st = mnem
			mnemDone = true
		}
		screen.SetContent(col, y, r, nil, st)
		col++
	}
}

// mbDrawDropdown renders the open menu's dropdown box with shadow.
func (m *MenuBar) mbDrawDropdown(screen tcell.Screen) {
	bx, by, inner, boxW, boxH := m.mbDropBounds(m.mbSel)
	items := m.mbMenus[m.mbSel].Items

	body := theme.DropdownBody()

	// Top border.
	m.mbHLine(screen, bx, by, boxW, theme.TLSingle, theme.TRSingle, theme.HSingle, body)
	// Rows.
	for r, it := range items {
		ry := by + 1 + r
		m.mbDrawRow(screen, bx, ry, inner, it, r == m.mbItem)
	}
	// Bottom border.
	m.mbHLine(screen, bx, by+boxH-1, boxW, theme.BLSingle, theme.BRSingle, theme.HSingle, body)

	// Drop shadow: right column and bottom row, one cell offset.
	shadow := theme.Shadow()
	for r := 0; r < boxH; r++ {
		screen.SetContent(bx+boxW, by+1+r, ' ', nil, shadow)
	}
	for c := 1; c <= boxW; c++ {
		screen.SetContent(bx+c, by+boxH, ' ', nil, shadow)
	}
}

// mbHLine draws a horizontal border line with left/right corner runes.
func (m *MenuBar) mbHLine(screen tcell.Screen, bx, by, boxW int, left, right, mid rune, st tcell.Style) {
	screen.SetContent(bx, by, left, nil, st)
	for c := 1; c < boxW-1; c++ {
		screen.SetContent(bx+c, by, mid, nil, st)
	}
	screen.SetContent(bx+boxW-1, by, right, nil, st)
}

// mbDrawRow renders one dropdown row (separator, normal, disabled, highlighted).
func (m *MenuBar) mbDrawRow(screen tcell.Screen, bx, ry, inner int, it MenuItem, hi bool) {
	body := theme.DropdownBody()

	if it.Separator {
		// Full-width rule joined to the side borders with tees.
		screen.SetContent(bx, ry, theme.TeeLeft, nil, body)
		for c := 0; c < inner; c++ {
			screen.SetContent(bx+1+c, ry, theme.HSingle, nil, body)
		}
		screen.SetContent(bx+inner+1, ry, theme.TeeRight, nil, body)
		return
	}

	rowStyle := body
	mnemStyle := body
	switch {
	case it.Disabled:
		rowStyle = theme.DropdownDisabled()
		mnemStyle = rowStyle
	case hi:
		rowStyle = theme.DropdownHi()
		mnemStyle = theme.DropdownHiMnemonic()
	}

	// Side borders.
	screen.SetContent(bx, ry, theme.VSingle, nil, body)
	screen.SetContent(bx+inner+1, ry, theme.VSingle, nil, body)

	// Blank the inner span.
	for c := 0; c < inner; c++ {
		screen.SetContent(bx+1+c, ry, ' ', nil, rowStyle)
	}

	// Label with 2-space left pad; highlight first mnemonic rune.
	label := []rune(it.Label)
	mnLower := unicode.ToLower(it.Mnemonic)
	mnemDone := it.Disabled // disabled rows get no cyan mnemonic
	col := bx + 1 + 2
	for _, r := range label {
		st := rowStyle
		if !mnemDone && mnLower != 0 && unicode.ToLower(r) == mnLower {
			st = mnemStyle
			mnemDone = true
		}
		screen.SetContent(col, ry, r, nil, st)
		col++
	}

	// Accelerator, right-aligned with 2-space right pad.
	if it.Accel != "" {
		acc := []rune(it.Accel)
		start := bx + 1 + inner - 2 - len(acc)
		for k, r := range acc {
			screen.SetContent(start+k, ry, r, nil, rowStyle)
		}
	}
}

// --- input ---

// InputHandler routes keys when the bar is focused.
func (m *MenuBar) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return m.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if !m.mbActive {
			return
		}
		switch event.Key() {
		case tcell.KeyLeft:
			m.mbMoveTop(-1)
		case tcell.KeyRight:
			m.mbMoveTop(1)
		case tcell.KeyDown:
			if m.mbOpen {
				m.mbStep(1)
			} else {
				m.mbOpenMenu()
			}
		case tcell.KeyUp:
			if m.mbOpen {
				m.mbStep(-1)
			}
		case tcell.KeyEnter:
			if m.mbOpen {
				m.mbActivateItem()
			} else {
				m.mbOpenMenu()
			}
		case tcell.KeyEscape:
			if m.mbOpen {
				m.mbOpen = false
			} else {
				m.mbClose()
			}
		case tcell.KeyRune:
			m.mbHandleRune(event.Rune())
		}
	})
}

// HandleMouse processes a mouse action at the absolute screen position (x,y).
// It is driven from the application's mouse capture rather than tview's normal
// per-primitive routing, because the open dropdown is drawn below row 0 and so
// lies outside this primitive's rect. Returns true when the event was handled
// (and should be swallowed by the caller).
//
// Behaviour for a left button-down / click:
//   - on a top-level title: activate the bar and open that menu.
//   - inside the open dropdown on a selectable row: run its action and close.
//   - inside the open dropdown on a separator/disabled row: ignore (consume).
//   - anywhere else while active/open: close the menu and consume the click.
//
// A hover (MouseMove) over a dropdown row or top-level title updates the
// highlight while the bar is open, but is not itself consumed.
func (m *MenuBar) HandleMouse(action tview.MouseAction, x, y int) bool {
	if len(m.mbMenus) == 0 {
		return false
	}

	switch action {
	case tview.MouseMove:
		return m.mbHandleHover(x, y)
	case tview.MouseLeftDown, tview.MouseLeftClick:
		// fall through to handling below.
	default:
		return false
	}

	// Click on a top-level title row.
	if i, ok := m.mbHitTitle(x, y); ok {
		m.mbActive = true
		m.mbSel = i
		m.mbFireActivate()
		m.mbOpenMenu()
		return true
	}

	// Click inside the open dropdown.
	if m.mbOpen {
		if row, ok := m.mbHitDropRow(x, y); ok {
			items := m.mbMenus[m.mbSel].Items
			if row >= 0 && row < len(items) {
				it := items[row]
				if it.Separator || it.Disabled {
					return true // inside the box but not actionable: just consume.
				}
				m.mbItem = row
				m.mbActivateItem()
			}
			return true
		}
	}

	// Active or open, but the click landed outside both the bar and the
	// dropdown: dismiss (same as Esc-to-closed) and consume the click.
	if m.mbActive || m.mbOpen {
		m.mbClose()
		return true
	}

	return false
}

// mbHandleHover updates the highlight under the cursor while the bar is open. It
// does not consume the event.
func (m *MenuBar) mbHandleHover(x, y int) bool {
	if !m.mbActive {
		return false
	}
	if i, ok := m.mbHitTitle(x, y); ok {
		if i != m.mbSel {
			m.mbSel = i
			if m.mbOpen {
				m.mbOpenMenu()
			}
		}
		return false
	}
	if m.mbOpen {
		if row, ok := m.mbHitDropRow(x, y); ok {
			items := m.mbMenus[m.mbSel].Items
			if row >= 0 && row < len(items) && !items[row].Separator && !items[row].Disabled {
				m.mbItem = row
			}
		}
	}
	return false
}

// mbHitTitle reports the index of the top-level title whose cells contain (x,y),
// when (x,y) is on the bar row.
func (m *MenuBar) mbHitTitle(x, y int) (int, bool) {
	_, by, _, _ := m.GetRect()
	if y != by {
		return 0, false
	}
	for i := range m.mbMenus {
		tx, tw := m.mbTitleX(i)
		if tw > 0 && x >= tx && x < tx+tw {
			return i, true
		}
	}
	return 0, false
}

// mbHitDropRow reports the item-row index of the open dropdown that contains
// (x,y), when (x,y) is inside the dropdown's inner area.
func (m *MenuBar) mbHitDropRow(x, y int) (int, bool) {
	if !m.mbOpen {
		return 0, false
	}
	bx, by, _, boxW, boxH := m.mbDropBounds(m.mbSel)
	if x < bx || x >= bx+boxW || y < by || y >= by+boxH {
		return 0, false
	}
	// Rows sit between the top border (by) and bottom border (by+boxH-1).
	row := y - (by + 1)
	if row < 0 || row >= len(m.mbMenus[m.mbSel].Items) {
		return 0, false
	}
	return row, true
}

// mbHandleRune handles a typed mnemonic letter.
func (m *MenuBar) mbHandleRune(r rune) {
	r = unicode.ToLower(r)
	if r == 0 {
		return
	}
	if m.mbOpen {
		// Match an item's mnemonic in the open menu.
		items := m.mbMenus[m.mbSel].Items
		for j := range items {
			it := items[j]
			if it.Separator || it.Disabled {
				continue
			}
			if unicode.ToLower(it.Mnemonic) == r {
				m.mbItem = j
				m.mbActivateItem()
				return
			}
		}
		return
	}
	// Bar active but no dropdown: open the matching top-level menu.
	for i, mn := range m.mbMenus {
		if unicode.ToLower(mn.Mnemonic) == r {
			m.mbSel = i
			m.mbOpenMenu()
			return
		}
	}
}
