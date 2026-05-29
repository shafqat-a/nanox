# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status

Greenfield. The only artifact present is `VBDOS-Editor-Spec.pdf` — the authoritative build
specification for **DOSEdit** (spec v1.1). No Go code exists yet; build it from the spec, in the
milestone order below. When the spec's ASCII mockups (§5–§6) and prose ever disagree, the **ASCII
mockups win** — they are cell-exact. The PNGs referenced as `images/*.png` define target colours.

## What this is

A single static terminal binary (Linux/macOS/Windows) that recreates the early-1990s **Visual Basic
for DOS 1.0 / QuickBASIC 4.5** editor: grey hatched desktop, CUA menu bar, multiple
movable/resizable/overlapping MDI child windows with **magenta** active title bars, modal dialogs
with drop shadows, and a **cyan** status bar.

It is a **text editor, not an IDE** — no interpreter, compiler, debugger, or "Run". `.bas` is only
the default file filter. Keyboard-first; mouse supported but never required.

## Tech stack & the framework gotchas that shape the whole design

- **Go 1.22+**, module name `dosedit`.
- `github.com/gdamore/tcell/v2` — cell/colour/key/mouse engine.
- `github.com/rivo/tview` — app loop and base widgets.
- `github.com/epiclabs-io/winman` — the MDI window manager. **This is the key dependency.** If
  `go get` on it is stale, the API-compatible fork `github.com/r3ap3r2004/winman` is an acceptable
  substitute — verify `go get` succeeds before committing to either.

Three facts drive most of the architecture (verified in spec §3; do not assume otherwise):

1. **tview has no menu bar and no MDI manager.** The menu bar, dropdown menus, status/function-key
   bar, and the editor surface must all be **custom primitives** — embed `tview.Box`, implement
   `Draw` and `InputHandler`. Do **not** build the editor on `tview.TextArea`; v1 needs cell-exact
   cursor/selection rendering, both scrollbars, and the DOS colour treatment, which only a custom
   primitive gives cleanly.
2. **MDI windows come from winman**, layered on top of tview. Each `winman` window's content is one
   `Editor` primitive (`SetRoot(ed)`), with `SetDraggable(true)`, `SetResizable(true)`, and title-bar
   buttons via `AddButton(...)`.
3. **The desktop must paint the grey hatch texture itself.** winman has no textured background —
   subclass winman's manager `Draw`, or layer a texture `Box` behind the manager via `tview.Pages`.

## Visual system — non-negotiable

- Define the **16 DOS colours once** via `tcell.NewRGBColor(r,g,b)` (exact RGB in spec §4.2 and
  Appendix C). **Never rely on the terminal's named ANSI colours** — set RGB explicitly so the look
  is identical everywhere. This means the app needs a **truecolor** terminal (modern Linux/macOS
  terminals; on Windows use **Windows Terminal**, not legacy conhost).
- Colour assignments are taken from real screenshots and are authoritative (spec §4.3). The common
  early mistake (blue desktop / grey status bar) is **wrong**. Correct: grey hatched desktop,
  **magenta** active title bar, **cyan** status bar; blue appears only *inside* editor windows.
- Active window frame = double-line + white-on-blue; inactive = single-line + grey. Menus, dialogs,
  and buttons cast a one-cell **solid-black drop shadow**; MDI windows do **not** cast shadows.
- Logical screen = full terminal; reference design 80×25 but layout must reflow to any size ≥ 60×15.
  Row 0 = menu bar (always), row N-1 = status bar (always), rows between = the MDI desktop.

## Intended project structure (spec §10)

```
dosedit/
  main.go              # wire: screen, palette, menubar, statusbar, winman, first window
  internal/
    theme/palette.go   # the 16 tcell colours + named style helpers (§4.2–4.3, Appendix C)
    ui/menubar.go      # custom MenuBar primitive + dropdown overlay
    ui/statusbar.go    # custom context-aware StatusBar primitive
    ui/editor.go       # custom Editor primitive (Draw + InputHandler)
    ui/window.go       # winman window creation + title-bar buttons + active framing
    ui/dialogs.go      # Open / SaveAs / MessageBox / Find / Replace / About
    buffer/buffer.go   # Buffer model + load/save + line ops
    buffer/undo.go     # undo/redo stack
    app/app.go         # App struct: window manager, windows[], clipboard, key router
    app/commands.go    # newWindow/open/save/close/find/etc. (menu + key actions)
    app/keys.go        # global key routing & accelerator table
  testdata/HELLO.BAS
```

**Keep `internal/theme` and `internal/buffer` free of UI imports** so they stay unit-testable.

## Key routing model

Global keys (function keys, `Alt+letter` menu opens, accelerators like `F3`, `Ctrl+F4`, `Alt+1..9`,
`Alt+X`) are intercepted at app level via `tview.Application.SetInputCapture` and dispatched from
`app/keys.go`. Window-local keys fall through to the focused `Editor`. Primary scheme is **CUA**
(spec §8); WordStar chords are an optional alternate.

One conflict to resolve in code (spec §7): `F3` is both classic "Open" and "Find Next". **Keep
`F3 = Open`; bind `Find Next = Ctrl+L`.** State this in the status bar / help.

## Build, run, test

```bash
# first-time scaffold
go mod init dosedit
go get github.com/gdamore/tcell/v2
go get github.com/rivo/tview
go get github.com/epiclabs-io/winman      # or github.com/r3ap3r2004/winman

go run .                                   # run
go build -o dosedit .                      # build current OS
go test ./...                              # all tests
go test ./internal/buffer/ -run TestName   # a single test

# cross-compile (static binary per OS)
GOOS=windows GOARCH=amd64 go build -o dosedit.exe .
GOOS=darwin  GOARCH=arm64 go build -o dosedit-macos .
GOOS=linux   GOARCH=amd64 go build -o dosedit-linux .
```

## Build order — milestones (spec §11)

Build in this order; **each milestone must compile and produce a runnable binary.** Do not attempt
everything at once.

1. **M1 Shell** — boots; palette defined; textured grey desktop; static menu bar (no dropdowns) +
   static cyan status bar; `Alt+X` quits restoring the terminal.
2. **M2 One window + typing** — winman integrated; one centered editor window; insert/backspace/
   enter; cursor + scroll; Ln/Col in status bar; INS/OVR toggle.
3. **M3 Editor completeness** — selection (Shift+move), clipboard cut/copy/paste, undo/redo,
   Home/End/word/doc movement, Tab/indent, horizontal scroll + scrollbars.
4. **M4 Menus** — custom menu bar with dropdowns, mnemonics, accelerators, Esc semantics, drop
   shadows; wire File/Edit/Search/Window/Help actions.
5. **M5 MDI** — multiple windows: New, cascade offset, F6 cycling, active/inactive framing,
   move/size (mouse + Ctrl+F5), maximize (Ctrl+F10), close (Ctrl+F4) with dirty prompt, Tile (F5),
   window list / Alt+1..9.
6. **M6 Dialogs & files** — Open / Save As (dir navigation), Save / Save All, message box
   (save-changes), Find / Replace / Go to Line, About; real file I/O end-to-end.
7. **M7 Polish** — resize reflow & window clamping; mouse on menus; cross-compile; help/keys screen;
   optional `--highlight` BASIC syntax flag (off by default, pure display layer, never alters buffer).

## Buffer model (spec §9)

`Buffer{ Lines []string; Path; Title; Modified; EOL; TabWidth=4; UseSpaces }` — one entry per line,
no trailing newline stored. UTF-8 in memory; untitled buffers are `Untitled1`, `Untitled2`, …. The
`Modified` flag drives the `*` prefix in titles and status bar, and save prompts on close/exit.
Detect line endings on load, preserve on save; new files use LF.
