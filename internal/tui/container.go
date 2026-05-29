package tui

// Container is a Widget that holds child widgets. Layout (assigning child
// bounds) is the concrete container's responsibility, typically in SetBounds or
// Draw.
type Container interface {
	Widget
	// Add appends a child widget.
	Add(w Widget)
	// Children returns the container's children in z-order (last = topmost).
	Children() []Widget
}

// BaseContainer is an embeddable Container implementation. It embeds BaseWidget,
// holds a child slice, provides default topmost-child mouse hit-testing, and
// exposes focusable descendants in tree order for the App's focus manager.
type BaseContainer struct {
	BaseWidget
	children []Widget
}

// Add appends w to the container's children.
func (c *BaseContainer) Add(w Widget) { c.children = append(c.children, w) }

// Children returns the container's children (z-order; last is topmost).
func (c *BaseContainer) Children() []Widget { return c.children }

// HandleMouse routes the event to the topmost child whose bounds contain the
// point, forwarding the (absolute-coordinate) MouseEvent unchanged. Returns
// true if a child consumed it.
func (c *BaseContainer) HandleMouse(ev MouseEvent) bool {
	for i := len(c.children) - 1; i >= 0; i-- {
		ch := c.children[i]
		if ch.Bounds().Contains(ev.X, ev.Y) {
			return ch.HandleMouse(ev)
		}
	}
	return false
}

// childAt returns the topmost child whose bounds contain (x, y), or nil.
func (c *BaseContainer) childAt(x, y int) Widget {
	for i := len(c.children) - 1; i >= 0; i-- {
		if c.children[i].Bounds().Contains(x, y) {
			return c.children[i]
		}
	}
	return nil
}

// FocusableDescendants collects the focusable widgets in the subtree rooted at
// this container, in tree order (depth-first, child order). The container
// itself is not included.
func (c *BaseContainer) FocusableDescendants() []Widget {
	var out []Widget
	for _, ch := range c.children {
		collectFocusable(ch, &out)
	}
	return out
}

// collectFocusable appends w (if focusable) and its focusable descendants to
// out, in tree order.
func collectFocusable(w Widget, out *[]Widget) {
	if w.Focusable() {
		*out = append(*out, w)
	}
	if ct, ok := w.(Container); ok {
		for _, ch := range ct.Children() {
			collectFocusable(ch, out)
		}
	}
}
