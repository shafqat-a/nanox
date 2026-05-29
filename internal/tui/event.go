package tui

import "github.com/gdamore/tcell/v2"

// MouseAction identifies the kind of mouse event delivered to a widget.
type MouseAction int

const (
	// MouseDown is a button press.
	MouseDown MouseAction = iota
	// MouseUp is a button release.
	MouseUp
	// MouseMove is pointer motion with no button held.
	MouseMove
	// MouseDrag is pointer motion with a button held.
	MouseDrag
	// WheelUp is a scroll-wheel-up event.
	WheelUp
	// WheelDown is a scroll-wheel-down event.
	WheelDown
)

// MouseEvent is the toolkit's normalized mouse event. X, Y are absolute screen
// coordinates. Button is the button mask at the time of the event (wheel bits
// for wheel actions). Keyboard events use *tcell.EventKey directly.
type MouseEvent struct {
	X, Y   int
	Action MouseAction
	Button tcell.ButtonMask
}
