package tui

import "unicode"

// MenuItem is a single row in a dropdown menu. A Separator item draws a
// full-width horizontal rule and is never selectable. A Disabled item is shown
// greyed and cannot be activated. Mnemonic is the access key (case-insensitive)
// and Accel is the right-aligned accelerator hint (e.g. "Ctrl+X").
type MenuItem struct {
	Label     string
	Mnemonic  rune
	Accel     string
	Action    func()
	Separator bool
	Disabled  bool
}

// Menu is a single top-level menu: a bar title plus its dropdown items.
type Menu struct {
	Title    string
	Mnemonic rune
	Items    []MenuItem
}

// mnuSelectable reports whether the item at index i in items can be selected
// (not a separator and not disabled).
func mnuSelectable(items []MenuItem, i int) bool {
	if i < 0 || i >= len(items) {
		return false
	}
	it := items[i]
	return !it.Separator && !it.Disabled
}

// mnuRuneEq reports whether two runes match case-insensitively (used for
// mnemonic matching). A zero mnemonic never matches.
func mnuRuneEq(a, b rune) bool {
	if a == 0 || b == 0 {
		return false
	}
	return unicode.ToLower(a) == unicode.ToLower(b)
}

// mnuFirstSelectable returns the index of the first selectable item, or -1.
func mnuFirstSelectable(items []MenuItem) int {
	for i := range items {
		if mnuSelectable(items, i) {
			return i
		}
	}
	return -1
}

// mnuNextSelectable returns the next selectable index after from, moving by dir
// (+1/-1) and wrapping. Returns from if no other selectable item exists, or -1
// if none at all.
func mnuNextSelectable(items []MenuItem, from, dir int) int {
	n := len(items)
	if n == 0 {
		return -1
	}
	i := from
	for k := 0; k < n; k++ {
		i += dir
		if i < 0 {
			i = n - 1
		} else if i >= n {
			i = 0
		}
		if mnuSelectable(items, i) {
			return i
		}
	}
	if mnuSelectable(items, from) {
		return from
	}
	return -1
}

// mnuRuneLen returns the number of runes in s.
func mnuRuneLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
