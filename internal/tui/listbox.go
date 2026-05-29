package tui

import (
	"github.com/gdamore/tcell/v2"

	"dosedit/internal/theme"
)

// ListBox is a scrollable single-selection list. Body is black-on-white
// (theme.ListBox); the selected row is reverse (theme.ListSelected). When the
// item count exceeds the visible rows a vertical scrollbar occupies the right
// inner column (arrows, shaded track, proportional thumb). Navigation: arrows,
// PageUp/Down, Home/End, mouse wheel, click to select, double-click to
// activate.
type ListBox struct {
	BaseWidget
	items      []string
	selected   int
	top        int // first visible row index
	bar        *ScrollBar
	onSelect   func(int)
	onActivate func(int)

	lastClickRow  int
	lastClickTick int
	clickTick     int
}

// NewListBox returns a ListBox over items, selecting the first row if any.
func NewListBox(items []string) *ListBox {
	lb := &ListBox{items: items, selected: 0, lastClickRow: -1}
	lb.bar = NewScrollBar(true)
	if len(items) == 0 {
		lb.selected = -1
	}
	return lb
}

// Focusable reports that a ListBox can hold keyboard focus.
func (lb *ListBox) Focusable() bool { return true }

// Items returns the current items.
func (lb *ListBox) Items() []string { return lb.items }

// SetItems replaces the items and clamps the selection/scroll.
func (lb *ListBox) SetItems(items []string) {
	lb.items = items
	if lb.selected >= len(items) {
		lb.selected = len(items) - 1
	}
	if lb.selected < 0 && len(items) > 0 {
		lb.selected = 0
	}
	lb.lbClamp()
}

// Selected returns the selected index, or -1 when the list is empty.
func (lb *ListBox) Selected() int { return lb.selected }

// SetSelected sets the selection (clamped to a valid index) and fires
// onSelect when it changes.
func (lb *ListBox) SetSelected(i int) {
	if len(lb.items) == 0 {
		lb.selected = -1
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(lb.items) {
		i = len(lb.items) - 1
	}
	if i == lb.selected {
		lb.lbClamp()
		return
	}
	lb.selected = i
	lb.lbClamp()
	if lb.onSelect != nil {
		lb.onSelect(i)
	}
}

// SetOnSelect installs a callback fired when the selection changes.
func (lb *ListBox) SetOnSelect(fn func(int)) { lb.onSelect = fn }

// SetOnActivate installs a callback fired on Enter or double-click.
func (lb *ListBox) SetOnActivate(fn func(int)) { lb.onActivate = fn }

// lbVisibleRows returns how many item rows fit in the current bounds.
func (lb *ListBox) lbVisibleRows() int {
	if lb.bounds.H < 0 {
		return 0
	}
	return lb.bounds.H
}

// lbNeedBar reports whether a scrollbar is required.
func (lb *ListBox) lbNeedBar() bool {
	return len(lb.items) > lb.lbVisibleRows()
}

// lbClamp keeps the top within range and the selection visible.
func (lb *ListBox) lbClamp() {
	rows := lb.lbVisibleRows()
	if rows <= 0 {
		lb.top = 0
		return
	}
	maxTop := len(lb.items) - rows
	if maxTop < 0 {
		maxTop = 0
	}
	if lb.selected >= 0 {
		if lb.selected < lb.top {
			lb.top = lb.selected
		}
		if lb.selected > lb.top+rows-1 {
			lb.top = lb.selected - rows + 1
		}
	}
	if lb.top > maxTop {
		lb.top = maxTop
	}
	if lb.top < 0 {
		lb.top = 0
	}
}

// Draw renders the body, selection highlight and (when needed) the scrollbar.
func (lb *ListBox) Draw(surf Surface) {
	b := lb.bounds
	if b.Empty() {
		return
	}
	body := theme.ListBox()
	sel := theme.ListSelected()
	surf.Fill(b, ' ', body)
	lb.lbClamp()

	textW := b.W
	if lb.lbNeedBar() {
		textW = b.W - 1
	}
	if textW < 0 {
		textW = 0
	}
	rows := lb.lbVisibleRows()
	for r := 0; r < rows; r++ {
		idx := lb.top + r
		if idx >= len(lb.items) {
			break
		}
		st := body
		if idx == lb.selected {
			st = sel
		}
		y := b.Y + r
		if textW > 0 {
			surf.Fill(Rect{X: b.X, Y: y, W: textW, H: 1}, ' ', st)
			surf.Text(b.X, y, lbFit(lb.items[idx], textW), st)
		}
	}

	if lb.lbNeedBar() {
		lb.bar.SetBounds(Rect{X: b.X + b.W - 1, Y: b.Y, W: 1, H: b.H})
		lb.bar.SetRange(0, len(lb.items)-rows, rows)
		lb.bar.SetValue(lb.top)
		lb.bar.Draw(surf)
	}
}

// lbFit truncates s to at most w runes.
func lbFit(s string, w int) string {
	rs := []rune(s)
	if len(rs) <= w {
		return s
	}
	return string(rs[:w])
}

// HandleKey implements list navigation. Returns true when consumed.
func (lb *ListBox) HandleKey(ev *tcell.EventKey) bool {
	if len(lb.items) == 0 {
		return false
	}
	rows := lb.lbVisibleRows()
	switch ev.Key() {
	case tcell.KeyUp:
		lb.SetSelected(lb.selected - 1)
		return true
	case tcell.KeyDown:
		lb.SetSelected(lb.selected + 1)
		return true
	case tcell.KeyPgUp:
		lb.SetSelected(lb.selected - rows)
		return true
	case tcell.KeyPgDn:
		lb.SetSelected(lb.selected + rows)
		return true
	case tcell.KeyHome:
		lb.SetSelected(0)
		return true
	case tcell.KeyEnd:
		lb.SetSelected(len(lb.items) - 1)
		return true
	case tcell.KeyEnter:
		if lb.selected >= 0 && lb.onActivate != nil {
			lb.onActivate(lb.selected)
		}
		return true
	}
	return false
}

// HandleMouse implements wheel scrolling, click-to-select, double-click
// activation, and forwards events on the scrollbar column to the bar.
func (lb *ListBox) HandleMouse(ev MouseEvent) bool {
	b := lb.bounds
	switch ev.Action {
	case WheelUp:
		lb.SetSelected(lb.selected - 1)
		return true
	case WheelDown:
		lb.SetSelected(lb.selected + 1)
		return true
	}

	// Forward to the scrollbar when present and the event is on its column,
	// or a drag is in flight.
	if lb.lbNeedBar() {
		barX := b.X + b.W - 1
		onBar := ev.X == barX && b.Contains(ev.X, ev.Y)
		if lb.bar.dragging || onBar {
			lb.bar.SetRange(0, len(lb.items)-lb.lbVisibleRows(), lb.lbVisibleRows())
			lb.bar.SetValue(lb.top)
			lb.bar.SetOnChange(func(v int) { lb.top = v })
			if lb.bar.HandleMouse(ev) {
				return true
			}
		}
	}

	if ev.Action != MouseDown {
		return false
	}
	if !b.Contains(ev.X, ev.Y) {
		return false
	}
	row := ev.Y - b.Y
	idx := lb.top + row
	if idx < 0 || idx >= len(lb.items) {
		return true
	}
	// Double-click detection: same row within two consecutive clicks.
	lb.clickTick++
	doubled := idx == lb.lastClickRow && lb.clickTick-lb.lastClickTick == 1
	lb.lastClickRow = idx
	lb.lastClickTick = lb.clickTick

	lb.SetSelected(idx)
	if doubled && lb.onActivate != nil {
		lb.onActivate(idx)
	}
	return true
}
