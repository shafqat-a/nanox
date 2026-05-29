package tui

// Panel is a minimal grouping/layout container with no chrome of its own. It
// lays out children at the bounds assigned to them (children keep whatever
// bounds the caller sets) and draws each child clipped to the panel area.
// A Panel is useful as a generic grouping container, and offers an optional
// vertical-stack layout helper for the common case.
type Panel struct {
	BaseContainer
	// stack, when true, makes the panel lay its children out in a vertical
	// stack across SetBounds/Draw (full width, pnlRowH rows each, separated by
	// pnlGap blank rows).
	stack bool
	rowH  int
	gap   int
}

// NewPanel returns an empty chrome-less Panel.
func NewPanel() *Panel { return &Panel{} }

// NewVStack returns a Panel that arranges its children in a vertical stack:
// each child is laid out full-width, rowH rows tall, with gap blank rows
// between consecutive children. rowH < 1 is treated as 1; gap < 0 as 0.
func NewVStack(rowH, gap int) *Panel {
	if rowH < 1 {
		rowH = 1
	}
	if gap < 0 {
		gap = 0
	}
	return &Panel{stack: true, rowH: rowH, gap: gap}
}

// SetBounds stores the panel's rectangle and (re)lays out children.
func (p *Panel) SetBounds(r Rect) {
	p.BaseWidget.SetBounds(r)
	p.layout()
}

// layout positions children when the panel is in vertical-stack mode. In plain
// mode children keep the bounds the caller assigned.
func (p *Panel) layout() {
	if !p.stack {
		return
	}
	b := p.Bounds()
	y := b.Y
	for _, ch := range p.Children() {
		ch.SetBounds(Rect{X: b.X, Y: y, W: b.W, H: p.rowH})
		y += p.rowH + p.gap
	}
}

// Draw lays out (in stack mode) and renders each child clipped to the panel.
func (p *Panel) Draw(s Surface) {
	p.layout()
	for _, ch := range p.Children() {
		ch.Draw(s.Clip(ch.Bounds()))
	}
}
