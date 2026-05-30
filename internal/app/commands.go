// commands.go implements DOSEdit's menu tree and the File / Edit / Search /
// Window / Help command actions (spec §7). Menu actions and global keys both
// funnel through the cmd* methods here.
//
// Dialogs are shown by pushing a *tui.Dialog as a modal (a.pushDialog); the
// dialog's onOK/onCancel callbacks pop it back off (a.popDialog) and then do the
// work. The tui.App makes the dialog truly modal, so commands never touch focus
// directly. Edit-menu clipboard/undo commands dispatch synthetic key events to
// the active editor's HandleKey (the editor already implements them).
package app

import (
	"fmt"
	"os"
	"path/filepath"

	"dosedit/internal/buffer"
	"dosedit/internal/dlg"
	"dosedit/internal/tui"

	"github.com/gdamore/tcell/v2"
)

// BuildMenus constructs the full menu tree (closing over the App's commands).
func (a *App) BuildMenus() []*tui.Menu {
	windowMenu := &tui.Menu{Title: "Window", Mnemonic: 'W', Items: []tui.MenuItem{
		{Label: "Next", Mnemonic: 'N', Accel: "F6", Action: func() { a.cycleWindow(1) }},
		{Label: "Previous", Mnemonic: 'P', Accel: "Shift+F6", Action: func() { a.cycleWindow(-1) }},
		{Separator: true},
		{Label: "Cascade", Mnemonic: 'C', Action: a.cascadeWindows},
		{Label: "Tile", Mnemonic: 'T', Accel: "F5", Action: a.tileWindows},
		{Separator: true},
		{Label: "Move/Size", Mnemonic: 'M', Accel: "Ctrl+F5", Action: a.cmdMoveSize},
		{Label: "Maximize/Restore", Mnemonic: 'x', Accel: "Ctrl+F10", Action: a.cmdToggleMax},
	}}

	return []*tui.Menu{
		{Title: "File", Mnemonic: 'F', Items: []tui.MenuItem{
			{Label: "New", Mnemonic: 'N', Action: a.cmdNew},
			{Label: "Open...", Mnemonic: 'O', Accel: "F3", Action: a.cmdOpen},
			{Separator: true},
			{Label: "Save", Mnemonic: 'S', Accel: "F2", Action: a.cmdSave},
			{Label: "Save As...", Mnemonic: 'A', Action: a.cmdSaveAs},
			{Label: "Save All", Mnemonic: 'l', Action: a.cmdSaveAll},
			{Separator: true},
			{Label: "Close", Mnemonic: 'C', Accel: "Ctrl+F4", Action: a.cmdCloseActive},
			{Separator: true},
			{Label: "Exit", Mnemonic: 'x', Accel: "Alt+X", Action: a.cmdExit},
		}},
		{Title: "Edit", Mnemonic: 'E', Items: []tui.MenuItem{
			{Label: "Undo", Mnemonic: 'U', Accel: "Ctrl+Z", Action: func() { a.editKey(tcell.NewEventKey(tcell.KeyCtrlZ, 0, tcell.ModNone)) }},
			{Label: "Redo", Mnemonic: 'R', Accel: "Ctrl+Y", Action: func() { a.editKey(tcell.NewEventKey(tcell.KeyCtrlY, 0, tcell.ModNone)) }},
			{Separator: true},
			{Label: "Cut", Mnemonic: 't', Accel: "Ctrl+X", Action: func() { a.editKey(tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone)) }},
			{Label: "Copy", Mnemonic: 'C', Accel: "Ctrl+C", Action: func() { a.editKey(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModNone)) }},
			{Label: "Paste", Mnemonic: 'P', Accel: "Ctrl+V", Action: func() { a.editKey(tcell.NewEventKey(tcell.KeyCtrlV, 0, tcell.ModNone)) }},
			{Label: "Delete", Mnemonic: 'D', Accel: "Del", Action: func() { a.editKey(tcell.NewEventKey(tcell.KeyDelete, 0, tcell.ModNone)) }},
			{Separator: true},
			{Label: "Select All", Mnemonic: 'A', Action: a.cmdSelectAll},
			{Separator: true},
			{Label: "Options...", Mnemonic: 'O', Action: a.cmdOptions},
		}},
		{Title: "Search", Mnemonic: 'S', Items: []tui.MenuItem{
			{Label: "Find...", Mnemonic: 'F', Accel: "Ctrl+F", Action: a.cmdFind},
			{Label: "Find Next", Mnemonic: 'N', Accel: "Ctrl+L", Action: a.cmdFindNext},
			{Label: "Replace...", Mnemonic: 'R', Accel: "Ctrl+H", Action: a.cmdReplace},
			{Separator: true},
			{Label: "Go to Line...", Mnemonic: 'G', Accel: "Ctrl+G", Action: a.cmdGotoLine},
		}},
		windowMenu,
		{Title: "Help", Mnemonic: 'H', Items: []tui.MenuItem{
			{Label: "Keys", Mnemonic: 'K', Action: a.cmdKeys},
			{Separator: true},
			{Label: "About...", Mnemonic: 'A', Action: a.cmdAbout},
		}},
	}
}

// --- dialog push/pop wrappers ----------------------------------------------

// pushDialog shows d as a modal and sets the status bar to the dialog context.
func (a *App) pushDialog(d *tui.Dialog) {
	if d == nil {
		return
	}
	a.statusbar.SetContext(tui.CtxDialog)
	a.app.PushModal(d)
}

// popDialog dismisses the topmost modal dialog and restores the editing context,
// refocusing the active editor.
func (a *App) popDialog() {
	a.app.PopModal()
	if a.app.TopLayer() == a.root {
		a.statusbar.SetContext(tui.CtxEditing)
		a.focusActiveEditor()
	}
}

// editKey forwards a synthetic key event to the active editor's HandleKey (used
// for Edit-menu clipboard/undo commands the editor already implements).
func (a *App) editKey(ev *tcell.EventKey) {
	ed := a.activeEditor()
	if ed == nil {
		return
	}
	ed.HandleKey(ev)
	if w := a.activeWindow(); w != nil {
		a.updateTitle(w)
	}
	a.app.Redraw()
}

// startDir returns a sensible starting directory for file dialogs.
func (a *App) startDir() string {
	if ed := a.activeEditor(); ed != nil {
		if p := ed.Buffer().Path; p != "" {
			return filepath.Dir(p)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

// --- File commands ---------------------------------------------------------

func (a *App) cmdNew() { a.newEditorWindow(buffer.NewUntitled()) }

func (a *App) cmdOpen() {
	d := dlg.NewOpen(a.app, a.startDir(), "*.*",
		func(path string) {
			a.popDialog()
			buf, err := buffer.Load(path)
			if err != nil {
				a.showMessage("Open Failed", fmt.Sprintf("Cannot open\n%s\n\n%v", path, err), []string{"OK"}, nil)
				return
			}
			a.newEditorWindow(buf)
		},
		func() { a.popDialog() },
	)
	a.pushDialog(d)
}

// cmdSave saves the active buffer, running the Save As flow if it has no path.
func (a *App) cmdSave() { a.saveActive(nil) }

// saveActive saves the active buffer; after a successful save it invokes done
// (used by close/exit flows). If the buffer has no path it runs Save As.
func (a *App) saveActive(done func()) {
	w := a.activeWindow()
	if w == nil {
		if done != nil {
			done()
		}
		return
	}
	ed := a.editorOf[w]
	buf := ed.Buffer()
	if buf.Path == "" {
		a.saveAsFlow(done)
		return
	}
	if err := buf.Save(); err != nil {
		a.showMessage("Save Failed", fmt.Sprintf("%v", err), []string{"OK"}, nil)
		return
	}
	a.updateTitle(w)
	a.statusbar.SetModified(buf.Modified)
	if done != nil {
		done()
	}
}

func (a *App) cmdSaveAs() { a.saveAsFlow(nil) }

// saveAsFlow opens the Save As dialog, confirming overwrite of an existing file,
// and calls done after a successful save.
func (a *App) saveAsFlow(done func()) {
	ed := a.activeEditor()
	if ed == nil {
		return
	}
	suggested := ed.Buffer().DisplayName()
	d := dlg.NewSaveAs(a.app, a.startDir(), suggested,
		func(path string) {
			a.popDialog()
			a.doSaveAs(path, done)
		},
		func() { a.popDialog() },
	)
	a.pushDialog(d)
}

// doSaveAs writes the active buffer to path, prompting for overwrite if the
// target already exists.
func (a *App) doSaveAs(path string, done func()) {
	if _, err := os.Stat(path); err == nil {
		a.showMessage("Confirm Save As",
			fmt.Sprintf("%s already exists.\nReplace it?", filepath.Base(path)),
			[]string{"Yes", "No"},
			func(idx int) {
				a.popDialog()
				if idx == 0 {
					a.writeSaveAs(path, done)
				}
			})
		return
	}
	a.writeSaveAs(path, done)
}

func (a *App) writeSaveAs(path string, done func()) {
	w := a.activeWindow()
	if w == nil {
		return
	}
	ed := a.editorOf[w]
	if err := ed.Buffer().SaveAs(path); err != nil {
		a.showMessage("Save Failed", fmt.Sprintf("%v", err), []string{"OK"}, nil)
		return
	}
	a.updateTitle(w)
	a.statusbar.SetModified(ed.Buffer().Modified)
	if done != nil {
		done()
	}
}

// cmdSaveAll saves every modified buffer that already has a path; untitled
// modified buffers are left for an explicit Save As.
func (a *App) cmdSaveAll() {
	for _, w := range a.windows {
		buf := a.editorOf[w].Buffer()
		if buf.Modified && buf.Path != "" {
			_ = buf.Save()
			a.updateTitle(w)
		}
	}
	if ed := a.activeEditor(); ed != nil {
		a.statusbar.SetModified(ed.Buffer().Modified)
	}
}

// cmdCloseActive closes the active window, prompting to save if dirty.
func (a *App) cmdCloseActive() {
	w := a.activeWindow()
	if w == nil {
		return
	}
	a.closeWindowPrompt(w, func() { a.closeWindow(w) })
}

// closeWindowPrompt prompts to save a dirty buffer before running then. Yes
// saves and proceeds; No proceeds without saving; Cancel/Esc aborts.
func (a *App) closeWindowPrompt(w *tui.Window, then func()) {
	buf := a.editorOf[w].Buffer()
	if !buf.Modified {
		then()
		return
	}
	a.activate(w)
	a.showMessage("DOSEdit",
		fmt.Sprintf("Save changes to %s?", buf.DisplayName()),
		[]string{"Yes", "No", "Cancel"},
		func(idx int) {
			a.popDialog()
			switch idx {
			case 0: // Yes
				a.saveActive(then)
			case 1: // No
				then()
			default: // Cancel / Esc
			}
		})
}

// cmdExit prompts for each dirty buffer, then stops the application.
func (a *App) cmdExit() { a.exitNext(0) }

// exitNext walks the window list from index i, prompting for dirty buffers; when
// all are resolved it stops the app.
func (a *App) exitNext(i int) {
	for i < len(a.windows) {
		w := a.windows[i]
		buf := a.editorOf[w].Buffer()
		if buf.Modified {
			a.activate(w)
			a.showMessage("DOSEdit",
				fmt.Sprintf("Save changes to %s?", buf.DisplayName()),
				[]string{"Yes", "No", "Cancel"},
				func(idx int) {
					a.popDialog()
					switch idx {
					case 0: // Yes: save, then continue from the same index.
						a.saveActive(func() { a.exitNext(i + 1) })
					case 1: // No: skip this one.
						a.exitNext(i + 1)
					default: // Cancel: abort exit.
					}
				})
			return
		}
		i++
	}
	a.app.Stop()
}

// --- Edit commands ---------------------------------------------------------

// cmdSelectAll selects the entire active buffer by driving the editor's own
// movement keys: jump to document start, then extend to document end.
func (a *App) cmdSelectAll() {
	if a.activeEditor() == nil {
		return
	}
	a.editKey(tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModCtrl))
	a.editKey(tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModCtrl|tcell.ModShift))
	a.app.Redraw()
}

// cmdOptions opens the Options dialog, applying changes (the line-numbers
// preference) to all open editors on OK.
func (a *App) cmdOptions() {
	d := dlg.NewOptions(dlg.Options{LineNumbers: a.lineNumbers},
		func(opt dlg.Options) { a.popDialog(); a.setLineNumbers(opt.LineNumbers) },
		func() { a.popDialog() })
	a.pushDialog(d)
}

// --- Search commands -------------------------------------------------------

func (a *App) cmdFind() {
	ed := a.activeEditor()
	if ed == nil {
		return
	}
	d := dlg.NewFind("",
		func(query string, matchCase, wholeWord bool) {
			a.popDialog()
			if !ed.Find(query, matchCase, wholeWord, true) {
				a.showMessage("Find", fmt.Sprintf("%q not found.", query), []string{"OK"}, nil)
			}
		},
		func() { a.popDialog() },
	)
	a.pushDialog(d)
}

func (a *App) cmdFindNext() {
	ed := a.activeEditor()
	if ed == nil {
		return
	}
	if !ed.FindNext() {
		a.showMessage("Find", "No previous search, or not found.", []string{"OK"}, nil)
	} else {
		a.app.Redraw()
	}
}

func (a *App) cmdReplace() {
	ed := a.activeEditor()
	if ed == nil {
		return
	}
	d := dlg.NewReplace(
		func(find, repl string, matchCase, wholeWord, all bool) {
			if all {
				n := ed.ReplaceAll(find, repl, matchCase, wholeWord)
				a.popDialog()
				a.showMessage("Replace", fmt.Sprintf("%d replacement(s) made.", n), []string{"OK"}, nil)
				return
			}
			ed.Replace(find, repl, matchCase, wholeWord)
			a.app.Redraw()
		},
		func() { a.popDialog() },
	)
	a.pushDialog(d)
}

func (a *App) cmdGotoLine() {
	ed := a.activeEditor()
	if ed == nil {
		return
	}
	d := dlg.NewGotoLine(
		func(line int) {
			a.popDialog()
			ed.GotoLine(line)
			a.app.Redraw()
		},
		func() { a.popDialog() },
	)
	a.pushDialog(d)
}

// --- Window commands -------------------------------------------------------

func (a *App) cmdToggleMax() {
	w := a.activeWindow()
	if w == nil {
		return
	}
	w.ToggleMaximize()
	a.app.Redraw()
}

// cmdMoveSize puts the status bar into the move/size context and enables the
// keyboard arrow-key move/size mode (Ctrl+F5).
func (a *App) cmdMoveSize() {
	if a.activeWindow() == nil {
		return
	}
	a.statusbar.SetContext(tui.CtxMove)
	a.moveSize = true
	a.app.Redraw()
}

// --- Help commands ---------------------------------------------------------

func (a *App) cmdAbout() {
	d := dlg.NewAbout(func() { a.popDialog() })
	a.pushDialog(d)
}

// cmdKeys shows the key reference in a message box.
func (a *App) cmdKeys() {
	const keys = "F1 Help    F2 Save    F3 Open\n" +
		"F5 Tile    F6 Next Window\n" +
		"Ctrl+F4 Close   Ctrl+F5 Move/Size\n" +
		"Ctrl+F10 Maximize/Restore\n" +
		"Ctrl+F Find   Ctrl+L Find Next\n" +
		"Ctrl+H Replace   Ctrl+G Go to Line\n" +
		"Alt+1..9 Window N    Alt+X Exit"
	a.showMessage("Keyboard Reference", keys, []string{"OK"}, nil)
}

// --- shared message-box helper ---------------------------------------------

// showMessage displays a modal message box. onResult (may be nil) receives the
// pressed button index, or -1 on Esc; when onResult is nil the box simply pops
// itself off on any button or Esc.
func (a *App) showMessage(title, message string, buttons []string, onResult func(idx int)) {
	d := dlg.NewMessage(title, message, buttons, func(idx int) {
		if onResult != nil {
			onResult(idx)
			return
		}
		a.popDialog()
	})
	a.pushDialog(d)
}
