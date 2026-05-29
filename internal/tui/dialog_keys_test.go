package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// buildKeyDialog constructs a Dialog with a TextBox + Checkbox body and
// OK(default)/Cancel buttons whose actions set the supplied flags and pop the
// modal. It returns the dialog and the focusable checkbox so tests can focus it.
func buildKeyDialog(a *App, okFired, cancelFired *bool) (*Dialog, *Checkbox) {
	dlg := NewDialog("Keys")

	tb := NewTextBox("hi", 20)
	tb.SetBounds(Rect{W: 22, H: 1})
	dlg.Add(tb)

	chk := NewCheckbox("Toggle me", false)
	chk.SetBounds(Rect{W: 22, H: 1})
	dlg.Add(chk)

	ok := NewButton("OK", func() {
		*okFired = true
		a.PopModal()
	})
	ok.SetDefault(true)
	ok.SetBounds(Rect{W: 10, H: 1})

	cancel := NewButton("Cancel", func() {
		*cancelFired = true
		a.PopModal()
	})
	cancel.SetBounds(Rect{W: 10, H: 1})

	dlg.AddButton(ok)
	dlg.AddButton(cancel)
	dlg.SetDefault(ok)
	dlg.SetCancel(func() {
		*cancelFired = true
		a.PopModal()
	})
	dlg.AutoSize()
	return dlg, chk
}

func keyEnter() *tcell.EventKey { return tcell.NewEventKey(tcell.KeyEnter, '\r', tcell.ModNone) }
func keyEsc() *tcell.EventKey   { return tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone) }
func keySpace() *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone) }

// TestDialogEnterActivatesDefaultFromFocusedCheckbox verifies that Enter fires
// the OK (default) action and pops the modal even when a Checkbox is focused,
// and that the checkbox is NOT toggled by that Enter.
func TestDialogEnterActivatesDefaultFromFocusedCheckbox(t *testing.T) {
	a := newApp(t)
	var okFired, cancelFired bool
	dlg, chk := buildKeyDialog(a, &okFired, &cancelFired)

	a.PushModal(dlg)
	a.Focus(chk)
	if a.Focused() != chk {
		t.Fatalf("expected checkbox focused, got %T", a.Focused())
	}
	before := chk.IsChecked()

	a.dispatchKey(keyEnter())

	if !okFired {
		t.Fatalf("Enter did not fire OK action")
	}
	if cancelFired {
		t.Fatalf("Enter wrongly fired Cancel")
	}
	if len(a.modals) != 0 {
		t.Fatalf("Enter did not pop the modal; %d modals remain", len(a.modals))
	}
	if chk.IsChecked() != before {
		t.Fatalf("Enter toggled the focused checkbox; want %v got %v", before, chk.IsChecked())
	}
}

// TestDialogEscInvokesCancelFromFocusedCheckbox verifies Esc invokes Cancel and
// pops the modal regardless of focus.
func TestDialogEscInvokesCancelFromFocusedCheckbox(t *testing.T) {
	a := newApp(t)
	var okFired, cancelFired bool
	dlg, chk := buildKeyDialog(a, &okFired, &cancelFired)

	a.PushModal(dlg)
	a.Focus(chk)

	a.dispatchKey(keyEsc())

	if !cancelFired {
		t.Fatalf("Esc did not invoke Cancel")
	}
	if okFired {
		t.Fatalf("Esc wrongly fired OK")
	}
	if len(a.modals) != 0 {
		t.Fatalf("Esc did not pop the modal; %d modals remain", len(a.modals))
	}
}

// TestDialogSpaceTogglesFocusedCheckbox verifies that non-Enter/Esc keys still
// reach the focused control: Space toggles the checkbox and does not fire OK.
func TestDialogSpaceTogglesFocusedCheckbox(t *testing.T) {
	a := newApp(t)
	var okFired, cancelFired bool
	dlg, chk := buildKeyDialog(a, &okFired, &cancelFired)

	a.PushModal(dlg)
	a.Focus(chk)
	before := chk.IsChecked()

	a.dispatchKey(keySpace())

	if chk.IsChecked() == before {
		t.Fatalf("Space did not toggle the focused checkbox")
	}
	if okFired || cancelFired {
		t.Fatalf("Space wrongly fired a button action (ok=%v cancel=%v)", okFired, cancelFired)
	}
	if len(a.modals) != 1 {
		t.Fatalf("Space changed modal stack; %d modals", len(a.modals))
	}
}
