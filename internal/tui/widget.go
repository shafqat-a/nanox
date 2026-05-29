package tui

import "github.com/gdamore/tcell/v2"

// Widget is the unit of the retained-mode tree. Concrete widgets embed
// BaseWidget and override the methods they need.
type Widget interface {
	// Draw renders the widget onto s (already clipped to the widget's area).
	Draw(s Surface)
	// SetBounds assigns the widget's absolute screen rectangle.
	SetBounds(Rect)
	// Bounds returns the widget's absolute screen rectangle.
	Bounds() Rect
	// HandleKey processes a key event; returns true if consumed.
	HandleKey(ev *tcell.EventKey) bool
	// HandleMouse processes a mouse event; returns true if consumed.
	HandleMouse(ev MouseEvent) bool
	// Focusable reports whether the widget can receive keyboard focus.
	Focusable() bool
	// SetFocused sets the widget's focused state.
	SetFocused(bool)
	// Focused reports whether the widget currently has focus.
	Focused() bool
}

// BaseWidget is an embeddable Widget implementation that stores bounds and a
// focused flag. It is not focusable and has no-op Draw/HandleKey/HandleMouse,
// so concrete widgets only override what they need.
type BaseWidget struct {
	bounds  Rect
	focused bool
}

// Draw is a no-op; concrete widgets override it.
func (b *BaseWidget) Draw(Surface) {}

// SetBounds stores the widget's absolute rectangle.
func (b *BaseWidget) SetBounds(r Rect) { b.bounds = r }

// Bounds returns the widget's absolute rectangle.
func (b *BaseWidget) Bounds() Rect { return b.bounds }

// HandleKey is a no-op returning false (unconsumed).
func (b *BaseWidget) HandleKey(*tcell.EventKey) bool { return false }

// HandleMouse is a no-op returning false (unconsumed).
func (b *BaseWidget) HandleMouse(MouseEvent) bool { return false }

// Focusable reports false by default.
func (b *BaseWidget) Focusable() bool { return false }

// SetFocused sets the focused flag.
func (b *BaseWidget) SetFocused(v bool) { b.focused = v }

// Focused returns the focused flag.
func (b *BaseWidget) Focused() bool { return b.focused }
