package tui

import "dosedit/internal/theme"

// ScrollBar is a horizontal or vertical scroll bar with end arrows, a shaded
// track and a draggable thumb sized proportionally to the page/range. It is a
// reusable standalone widget; ListBox and ComboBox may draw their own inline
// bars instead. It is not focusable (driven by the mouse / its owner).
type ScrollBar struct {
	BaseWidget
	vertical bool
	min      int
	max      int
	page     int
	value    int
	dragging bool
	onChange func(int)
}

// NewScrollBar returns a ScrollBar. If vertical is true the bar runs top to
// bottom, otherwise left to right.
func NewScrollBar(vertical bool) *ScrollBar {
	return &ScrollBar{vertical: vertical, min: 0, max: 0, page: 1}
}

// SetRange sets the value range [min, max] and the page size (the span covered
// by one "page" jump and used to size the thumb). Negative or zero page is
// clamped to 1. The current value is re-clamped into range.
func (s *ScrollBar) SetRange(min, max, page int) {
	if max < min {
		max = min
	}
	if page < 1 {
		page = 1
	}
	s.min, s.max, s.page = min, max, page
	s.setValue(s.value, false)
}

// SetValue sets the current value, clamped to [min, max]. It does NOT fire the
// onChange callback (programmatic set).
func (s *ScrollBar) SetValue(v int) { s.setValue(v, false) }

// Value returns the current value.
func (s *ScrollBar) Value() int { return s.value }

// SetOnChange installs a callback invoked whenever the value changes due to
// user interaction (arrow, page, drag).
func (s *ScrollBar) SetOnChange(fn func(int)) { s.onChange = fn }

// setValue clamps v into range, stores it, and fires onChange when notify is
// true and the value actually changed.
func (s *ScrollBar) setValue(v int, notify bool) {
	if v < s.min {
		v = s.min
	}
	if v > s.max {
		v = s.max
	}
	if v == s.value {
		return
	}
	s.value = v
	if notify && s.onChange != nil {
		s.onChange(v)
	}
}

// sbTrackLen returns the number of cells available for the track (the bar
// length minus the two end-arrow cells). It may be zero or negative for tiny
// bars, in which case callers must guard.
func (s *ScrollBar) sbTrackLen() int {
	if s.vertical {
		return s.bounds.H - 2
	}
	return s.bounds.W - 2
}

// sbThumbSpan returns the thumb length and its offset (in cells from the start
// of the track). The thumb is at least one cell.
func (s *ScrollBar) sbThumbSpan() (length, offset int) {
	track := s.sbTrackLen()
	if track <= 0 {
		return 0, 0
	}
	span := s.max - s.min + s.page // total logical extent
	if span < 1 {
		span = 1
	}
	length = s.page * track / span
	if length < 1 {
		length = 1
	}
	if length > track {
		length = track
	}
	rng := s.max - s.min
	if rng <= 0 {
		return length, 0
	}
	offset = (s.value - s.min) * (track - length) / rng
	if offset < 0 {
		offset = 0
	}
	if offset > track-length {
		offset = track - length
	}
	return length, offset
}

// Draw renders the arrows, track and thumb.
func (s *ScrollBar) Draw(surf Surface) {
	b := s.bounds
	if b.Empty() {
		return
	}
	st := theme.ListBox()
	if s.vertical {
		if b.H < 2 {
			surf.Fill(b, theme.SbTrack, st)
			return
		}
		surf.Set(b.X, b.Y, theme.SbUp, st)
		surf.Set(b.X, b.Y+b.H-1, theme.SbDown, st)
		track := s.sbTrackLen()
		length, offset := s.sbThumbSpan()
		for i := 0; i < track; i++ {
			r := theme.SbTrack
			if i >= offset && i < offset+length {
				r = theme.SbThumb
			}
			surf.Set(b.X, b.Y+1+i, r, st)
		}
		return
	}
	if b.W < 2 {
		surf.Fill(b, theme.SbTrack, st)
		return
	}
	surf.Set(b.X, b.Y, theme.SbLeft, st)
	surf.Set(b.X+b.W-1, b.Y, theme.SbRight, st)
	track := s.sbTrackLen()
	length, offset := s.sbThumbSpan()
	for i := 0; i < track; i++ {
		r := theme.SbTrack
		if i >= offset && i < offset+length {
			r = theme.SbThumb
		}
		surf.Set(b.X+1+i, b.Y, r, st)
	}
}

// sbPos returns the position of ev along the bar's main axis relative to the
// bar's origin (0 = first arrow cell).
func (s *ScrollBar) sbPos(ev MouseEvent) int {
	if s.vertical {
		return ev.Y - s.bounds.Y
	}
	return ev.X - s.bounds.X
}

// sbMainLen returns the bar length along its main axis.
func (s *ScrollBar) sbMainLen() int {
	if s.vertical {
		return s.bounds.H
	}
	return s.bounds.W
}

// HandleMouse implements arrow stepping, track paging, thumb dragging. It
// consumes every mouse event whose position lies within the bar (or any drag
// in progress).
func (s *ScrollBar) HandleMouse(ev MouseEvent) bool {
	switch ev.Action {
	case MouseDrag:
		if !s.dragging {
			return false
		}
		s.sbDragTo(ev)
		return true
	case MouseUp:
		if s.dragging {
			s.dragging = false
			return true
		}
		return false
	case WheelUp:
		s.setValue(s.value-1, true)
		return true
	case WheelDown:
		s.setValue(s.value+1, true)
		return true
	case MouseDown:
		if !s.bounds.Contains(ev.X, ev.Y) {
			return false
		}
		pos := s.sbPos(ev)
		mainLen := s.sbMainLen()
		if pos <= 0 {
			s.setValue(s.value-1, true)
			return true
		}
		if pos >= mainLen-1 {
			s.setValue(s.value+1, true)
			return true
		}
		// Within the track: pos-1 is the track-relative index.
		ti := pos - 1
		length, offset := s.sbThumbSpan()
		switch {
		case ti < offset:
			s.setValue(s.value-s.page, true)
		case ti >= offset+length:
			s.setValue(s.value+s.page, true)
		default:
			s.dragging = true
		}
		return true
	}
	return false
}

// sbDragTo maps a drag position to a value across the usable thumb travel.
func (s *ScrollBar) sbDragTo(ev MouseEvent) {
	track := s.sbTrackLen()
	length, _ := s.sbThumbSpan()
	travel := track - length
	if travel <= 0 {
		return
	}
	ti := s.sbPos(ev) - 1
	if ti < 0 {
		ti = 0
	}
	if ti > travel {
		ti = travel
	}
	rng := s.max - s.min
	v := s.min + ti*rng/travel
	s.setValue(v, true)
}
