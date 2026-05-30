package tui

import "github.com/gdamore/tcell/v2"

// App is the single authority for the event loop, focus and modality. It owns
// the tcell.Screen, the root Widget, a stack of modal overlays, the focused
// widget, and an optional global key hook.
type App struct {
	screen  tcell.Screen
	root    Widget
	modals  []Widget
	focused Widget
	keyHook func(ev *tcell.EventKey) bool
	dirty   bool
	running bool

	// prevButtons holds the primary-button mask from the previous mouse event,
	// used to derive press/release/move/drag transitions (tcell reports a bare
	// position on every motion, including with no buttons held).
	prevButtons tcell.ButtonMask
}

// mouseButtons masks the physical buttons (Button1..Button8), excluding the
// wheel bits, so a no-button motion is distinguishable from a press/release.
const mouseButtons = tcell.ButtonMask(0xff)

// NewApp returns an App bound to screen.
func NewApp(screen tcell.Screen) *App {
	return &App{screen: screen, dirty: true}
}

// Screen returns the underlying tcell.Screen.
func (a *App) Screen() tcell.Screen { return a.screen }

// SetRoot sets the root widget, gives it full-screen bounds and focuses its
// first focusable widget (when no modal is open).
func (a *App) SetRoot(w Widget) {
	a.root = w
	a.layout()
	if len(a.modals) == 0 {
		a.focusFirst()
	}
	a.dirty = true
}

// SetKeyHook installs a global accelerator hook, consulted ONLY when no modal is
// open and BEFORE the focused widget. Returning true consumes the event.
func (a *App) SetKeyHook(fn func(ev *tcell.EventKey) bool) { a.keyHook = fn }

// TopLayer returns the active layer: the top modal if any, else the root.
func (a *App) TopLayer() Widget {
	if len(a.modals) > 0 {
		return a.modals[len(a.modals)-1]
	}
	return a.root
}

// PushModal adds w as a modal overlay (full-screen bounds), trapping focus to
// its subtree and blocking input below.
func (a *App) PushModal(w Widget) {
	a.modals = append(a.modals, w)
	a.sizeFull(w)
	a.focused = nil
	a.focusFirst()
	a.dirty = true
}

// PopModal removes the topmost modal and restores focus to the new active layer.
func (a *App) PopModal() {
	if len(a.modals) == 0 {
		return
	}
	a.modals = a.modals[:len(a.modals)-1]
	a.focused = nil
	a.focusFirst()
	a.dirty = true
}

// Focus sets the focused widget, but only if it belongs to the active layer's
// focus ring. Clears the previous focus flag.
func (a *App) Focus(w Widget) {
	ring := a.focusRing()
	for _, c := range ring {
		if c == w {
			if a.focused != nil {
				a.focused.SetFocused(false)
			}
			a.focused = w
			w.SetFocused(true)
			a.dirty = true
			return
		}
	}
}

// Focused returns the currently focused widget (may be nil).
func (a *App) Focused() Widget { return a.focused }

// FocusNext advances focus to the next widget in the active layer's ring,
// wrapping around.
func (a *App) FocusNext() { a.focusStep(+1) }

// FocusPrev moves focus to the previous widget in the active layer's ring,
// wrapping around.
func (a *App) FocusPrev() { a.focusStep(-1) }

// Redraw marks the screen as needing a redraw on the next loop pass.
func (a *App) Redraw() { a.dirty = true }

// Stop exits Run cleanly after the current event is handled.
func (a *App) Stop() { a.running = false }

// Run polls tcell events, dispatches them, and redraws on demand until Stop is
// called. It draws an initial frame before entering the loop.
func (a *App) Run() error {
	a.running = true
	a.layout()
	a.draw()
	for a.running {
		ev := a.screen.PollEvent()
		if ev == nil {
			continue
		}
		a.dispatch(ev)
		if a.dirty {
			a.draw()
		}
	}
	return nil
}

// ModalDepth returns the number of open modal layers (0 when none). Useful for
// tests and for code that needs to know whether a dialog/menu owns input.
func (a *App) ModalDepth() int { return len(a.modals) }

// HandleEvent dispatches a single event synchronously, without the blocking
// PollEvent loop. It is intended for tests and embedding: callers feed events
// and call Sync to render, all on one goroutine, so reads of the screen never
// race the Run loop.
func (a *App) HandleEvent(ev tcell.Event) { a.dispatch(ev) }

// Sync lays out and draws one frame immediately on the calling goroutine. Pair
// it with HandleEvent for synchronous, race-free test drends.
func (a *App) Sync() {
	a.layout()
	a.draw()
}

// dispatch routes a raw tcell event to the appropriate handler. Factored out so
// tests can drive it without a blocking PollEvent.
func (a *App) dispatch(ev tcell.Event) {
	switch e := ev.(type) {
	case *tcell.EventKey:
		a.dispatchKey(e)
	case *tcell.EventMouse:
		a.dispatchMouse(e)
	case *tcell.EventResize:
		a.layout()
		a.screen.Sync()
		a.dirty = true
	}
}

// dispatchKey implements the keyboard routing rules. Returns true if consumed.
func (a *App) dispatchKey(ev *tcell.EventKey) bool {
	// Modal open: the top modal traverses Tab/Esc/Enter internally. We still
	// provide Tab focus traversal within the modal's trapped ring for free.
	if len(a.modals) > 0 {
		top := a.modals[len(a.modals)-1]
		// Tab/Shift+Tab move focus within the modal's trapped ring first.
		if a.handleTab(ev) {
			return true
		}
		// Enter/Esc are owned by the Dialog: Enter activates the default
		// button and Esc invokes the cancel handler, regardless of which
		// control is focused. This must run before the focused child so a
		// focused Checkbox/Option does not swallow Enter by toggling.
		if ev.Key() == tcell.KeyEnter || ev.Key() == tcell.KeyEscape {
			if top.HandleKey(ev) {
				a.dirty = true
				return true
			}
		}
		// All other keys go to the focused control (typing, Space toggling,
		// arrows in OptionGroup/ListBox, etc.).
		if a.focused != nil && a.focused.HandleKey(ev) {
			a.dirty = true
			return true
		}
		// Fall back to the modal itself for anything unhandled.
		consumed := top.HandleKey(ev)
		if consumed {
			a.dirty = true
		}
		return consumed
	}

	// No modal: global accelerators first.
	if a.keyHook != nil && a.keyHook(ev) {
		a.dirty = true
		return true
	}
	// Then Tab/Shift+Tab move focus.
	if a.handleTab(ev) {
		return true
	}
	// Then the focused widget, bubbling to ancestors.
	if a.focused != nil {
		w := a.focused
		for w != nil {
			if w.HandleKey(ev) {
				a.dirty = true
				return true
			}
			w = a.parentOf(w)
		}
	}
	return false
}

// handleTab consumes Tab / Shift+Tab to move focus within the active ring.
func (a *App) handleTab(ev *tcell.EventKey) bool {
	if ev.Key() != tcell.KeyTab && ev.Key() != tcell.KeyBacktab {
		return false
	}
	if ev.Key() == tcell.KeyBacktab || ev.Modifiers()&tcell.ModShift != 0 {
		a.FocusPrev()
	} else {
		a.FocusNext()
	}
	return true
}

// dispatchMouse implements the mouse routing rules. Returns true if consumed.
func (a *App) dispatchMouse(ev *tcell.EventMouse) bool {
	x, y := ev.Position()
	me := a.translateMouse(ev, x, y)

	layer := a.TopLayer()
	if layer == nil {
		return false
	}

	// True modality: swallow events outside the modal's bounds.
	if len(a.modals) > 0 && !layer.Bounds().Contains(x, y) {
		return true
	}

	// A click on a focusable widget focuses it first.
	if me.Action == MouseDown {
		if w := a.hitFocusable(layer, x, y); w != nil {
			a.Focus(w)
		}
	}

	consumed := layer.HandleMouse(me)
	if consumed {
		a.dirty = true
	}
	return consumed
}

// translateMouse converts a tcell mouse event into the toolkit's MouseEvent,
// deriving press/release/move/drag from the transition since the previous event.
// tcell emits an event on every cursor MOVE (with no buttons held), so a bare
// position change must map to MouseMove — not MouseUp — otherwise hover would be
// read as a click release (which would, e.g., dismiss an open menu).
func (a *App) translateMouse(ev *tcell.EventMouse, x, y int) MouseEvent {
	btn := ev.Buttons()
	me := MouseEvent{X: x, Y: y, Button: btn}

	// Wheel events are momentary and carry no persistent button state.
	if btn&tcell.WheelUp != 0 {
		me.Action = WheelUp
		return me
	}
	if btn&tcell.WheelDown != 0 {
		me.Action = WheelDown
		return me
	}

	cur := btn & mouseButtons
	prev := a.prevButtons & mouseButtons
	a.prevButtons = btn
	switch {
	case cur != 0 && prev == 0:
		me.Action = MouseDown
	case cur == 0 && prev != 0:
		me.Action = MouseUp
	case cur != 0: // button held across events => drag
		me.Action = MouseDrag
	default: // no buttons, no transition => hover/move
		me.Action = MouseMove
	}
	return me
}

// hitFocusable returns the topmost focusable widget under (x, y) within the
// given layer's subtree, or nil.
func (a *App) hitFocusable(layer Widget, x, y int) Widget {
	if !layer.Bounds().Contains(x, y) {
		return nil
	}
	var found Widget
	if layer.Focusable() {
		found = layer
	}
	if ct, ok := layer.(Container); ok {
		children := ct.Children()
		for i := len(children) - 1; i >= 0; i-- {
			ch := children[i]
			if ch.Bounds().Contains(x, y) {
				if deeper := a.hitFocusable(ch, x, y); deeper != nil {
					return deeper
				}
			}
		}
	}
	return found
}

// focusRing returns the focusable widgets of the active layer, in tree order.
func (a *App) focusRing() []Widget {
	layer := a.TopLayer()
	if layer == nil {
		return nil
	}
	var out []Widget
	collectFocusable(layer, &out)
	return out
}

// focusStep moves focus by dir (+1 next, -1 prev) within the active ring,
// wrapping around.
func (a *App) focusStep(dir int) {
	ring := a.focusRing()
	if len(ring) == 0 {
		return
	}
	idx := -1
	for i, w := range ring {
		if w == a.focused {
			idx = i
			break
		}
	}
	var next Widget
	if idx == -1 {
		if dir >= 0 {
			next = ring[0]
		} else {
			next = ring[len(ring)-1]
		}
	} else {
		n := (idx + dir) % len(ring)
		if n < 0 {
			n += len(ring)
		}
		next = ring[n]
	}
	a.Focus(next)
}

// focusFirst focuses the first widget in the active layer's ring, if any.
func (a *App) focusFirst() {
	ring := a.focusRing()
	if len(ring) > 0 {
		a.Focus(ring[0])
	}
}

// parentOf returns the parent container of w within the active layer, or nil if
// w is the layer root or not found.
func (a *App) parentOf(w Widget) Widget {
	layer := a.TopLayer()
	if layer == nil || layer == w {
		return nil
	}
	return findParent(layer, w)
}

// findParent searches the subtree rooted at root for the parent of target.
func findParent(root, target Widget) Widget {
	ct, ok := root.(Container)
	if !ok {
		return nil
	}
	for _, ch := range ct.Children() {
		if ch == target {
			return root
		}
		if p := findParent(ch, target); p != nil {
			return p
		}
	}
	return nil
}

// layout assigns full-screen bounds to the root and every modal, then marks
// dirty.
func (a *App) layout() {
	if a.screen == nil {
		return
	}
	if a.root != nil {
		a.sizeFull(a.root)
	}
	for _, m := range a.modals {
		a.sizeFull(m)
	}
}

// sizeFull sets w's bounds to the full screen.
func (a *App) sizeFull(w Widget) {
	if w == nil || a.screen == nil {
		return
	}
	sw, sh := a.screen.Size()
	w.SetBounds(Rect{X: 0, Y: 0, W: sw, H: sh})
}

// draw clears the screen, draws the root then modals bottom-to-top, and shows.
func (a *App) draw() {
	if a.screen == nil {
		return
	}
	a.screen.Clear()
	sw, sh := a.screen.Size()
	full := Rect{X: 0, Y: 0, W: sw, H: sh}
	if a.root != nil {
		a.root.Draw(NewScreenSurface(a.screen, full))
	}
	for _, m := range a.modals {
		m.Draw(NewScreenSurface(a.screen, full))
	}
	a.screen.Show()
	a.dirty = false
}
