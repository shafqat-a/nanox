# `internal/tui` — VB-for-DOS widget toolkit (design contract)

This is the SHARED CONTRACT for a from-scratch, event-driven, retained-mode widget
toolkit built directly on **tcell** (no tview). Every agent building part of `tui`
MUST follow the type signatures and appearance rules here so the pieces compose.
Authoritative look = `VBDOS-Editor-Spec.pdf` §4 (palette/glyphs/shadows) and §6 (each
component). Reuse `internal/theme` (colours/styles/glyphs) and `internal/buffer`.

## Principles
- Pure tcell. The toolkit owns the screen, the event loop, focus, layout, drawing.
- Retained-mode tree: a root `Widget`; `Container`s hold children. Redraw on demand.
- Event-based: keyboard + mouse events dispatch through the tree; focus + modality
  are managed centrally by `App`.
- Widgets draw VBDOS-authentic chrome using `theme` styles/glyphs ONLY (no hardcoded
  colours).

## Geometry & drawing (package tui)
```go
type Rect struct{ X, Y, W, H int }
func (r Rect) Contains(x, y int) bool
func (r Rect) Inset(dx, dy int) Rect
func (r Rect) Empty() bool

// Surface is clipped drawing onto the screen within a widget's bounds. All coords
// are ABSOLUTE screen coords; writes outside the surface's clip are dropped.
type Surface interface {
    Set(x, y int, r rune, style tcell.Style)
    Fill(r Rect, ch rune, style tcell.Style)
    Text(x, y int, s string, style tcell.Style) // single line, clipped
    Clip(r Rect) Surface
    Bounds() Rect
}
```

## Events
```go
type MouseAction int
const ( MouseDown MouseAction = iota; MouseUp; MouseMove; MouseDrag; WheelUp; WheelDown )
type MouseEvent struct{ X, Y int; Action MouseAction; Button tcell.ButtonMask }
// Keyboard uses *tcell.EventKey directly.
```

## Widget / Container
```go
type Widget interface {
    Draw(s Surface)
    SetBounds(Rect)
    Bounds() Rect
    HandleKey(ev *tcell.EventKey) bool   // true = consumed
    HandleMouse(ev MouseEvent) bool      // true = consumed
    Focusable() bool
    SetFocused(bool)
    Focused() bool
}

// BaseWidget is embeddable: stores bounds + focused flag, Focusable()=false,
// no-op Draw/HandleKey/HandleMouse. Concrete widgets embed it and override.
type BaseWidget struct{ ... }

type Container interface {
    Widget
    Add(w Widget)
    Children() []Widget
}
// BaseContainer embeds BaseWidget, holds children, and provides default
// child mouse hit-testing (topmost child under the point) + a flat
// focus-order accessor for the App's focus manager. Layout is the concrete
// container's job (it sets child bounds in SetBounds/Draw).
```

## App: event loop + focus + modality (the single authority)
```go
type App struct{ ... }
func NewApp(screen tcell.Screen) *App
func (a *App) SetRoot(w Widget)
func (a *App) PushModal(w Widget)   // overlay; traps focus + blocks input below
func (a *App) PopModal()
func (a *App) TopLayer() Widget      // top modal or root
func (a *App) Focus(w Widget)        // set focused widget within the active layer
func (a *App) Focused() Widget
func (a *App) FocusNext()            // Tab within active layer's focus ring
func (a *App) FocusPrev()            // Shift+Tab
func (a *App) SetKeyHook(fn func(ev *tcell.EventKey) bool) // global accelerators; consulted
                                     // ONLY when no modal is open, BEFORE the focused widget
func (a *App) Redraw()
func (a *App) Run() error            // poll tcell events, dispatch, draw
func (a *App) Stop()
func (a *App) Screen() tcell.Screen
```
Routing rules (implemented in App):
- **Key**: if a modal is open → dispatch to the top modal (it traverses Tab/Esc/Enter
  internally via its container + focused child). Else → call key hook (global
  accelerators); if not consumed → Tab/Shift+Tab move focus; otherwise the focused
  widget's HandleKey, bubbling to ancestors if unconsumed.
- **Mouse**: hit-test the top layer from topmost child down; if a modal is open,
  events outside the modal are swallowed (true modality). A click on a focusable
  widget focuses it, then the widget's HandleMouse runs.
- **Focus ring** = the focusable widgets of the active layer in tree order; Tab wraps;
  modal layers trap focus.

## Widget catalogue & VBDOS appearance
Use `theme` styles/glyphs throughout. Focus indication per VBDOS (see spec §6).

Containers:
- **Desktop** — fills its rect with the grey hatch (`theme.Texture` '▒', `theme.Desktop()`);
  hosts windows (z-order), the menu bar (top row), status bar (bottom row).
- **Window** (MDI) — active: double-line frame (`theme.TLDouble`…), `theme.ActiveFrame()`
  (white-on-blue) + magenta title (`theme.ActiveTitle()`); inactive: single-line +
  `theme.InactiveFrame()`/`InactiveTitle()`. Title shows `[n] name` / `*name`. Buttons:
  close `theme.BtnClose` top-left, max/restore `theme.BtnMax`/`BtnRestore` top-right.
  Draggable (title) + resizable (bottom-right corner).
- **Dialog** (modal) — double-line frame, centred magenta/white title, light-gray body
  (`theme.DialogBody()`), one-cell solid-black drop shadow right+bottom (`theme.Shadow()`).
- **Frame/GroupBox** — single-line border with an embedded caption; groups OptionButtons.
- **Panel** — plain layout container, no chrome.

Controls (focus = reverse/outline as noted):
- **Label** — static text (`theme.DialogBody()`); optional mnemonic char brighter. Not focusable.
- **PushButton** — ` Label ` on `theme.ButtonFace()` (black-on-lightgray); one-cell solid
  black shadow on right + bottom (L-shape). DEFAULT button: black single-line outline
  around the face. FOCUSED: label row reverse + outline. Fires on Enter/Space/click.
- **CheckBox** — `[ ] Label` / `[X] Label` (`theme.DialogBody()`); focused → the `[ ]`
  cell reverse. Toggle on Space/Enter/click. `IsChecked() bool`.
- **OptionButton** + **OptionGroup** — `( ) Label` / `(•) Label`; exactly one selected
  per group; Up/Down + click move selection; focused marker reverse. `Selected() int`.
- **TextBox** (single-line input) — white-on-black (`theme.InputField()`), recessed
  single-line frame (dark-gray edges), caret cell `theme.Yellow` when focused. `Text() string`.
- **ListBox** — black-on-white (`theme.ListBox()`), selected row reverse (`theme.ListSelected()`),
  vertical scrollbar on right edge (track `theme.SbTrack` '▒', thumb `theme.SbThumb` '█',
  arrows `theme.SbUp`/`SbDown`). `Selected() int`, items navigable by arrows/PgUp/Dn/click/wheel.
- **ComboBox / DropDown** — collapsed: an input-like field showing the selection + a
  `[▼]` button on the right; expanded: a popup ListBox overlay below it (Enter/click toggles).
- **ScrollBar** (H/V) — arrows (`SbUp`/`SbDown` or `SbLeft`/`SbRight`), track `SbTrack`,
  draggable thumb `SbThumb`; emits value changes.
- **MenuBar** + **Menu** + **MenuItem** — light-gray bar (`theme.MenuNormal()`), mnemonic
  bright (`theme.MenuMnemonic()`), open/active item reverse (`theme.MenuSelect()`); dropdown
  single-line light-gray box (`theme.DropdownBody()`), highlighted row `theme.DropdownHi()`
  with mnemonic `theme.DropdownHiMnemonic()`, disabled `theme.DropdownDisabled()`, separators
  full-width `theme.HSingle` joined with `TeeLeft/TeeRight`, solid-black drop shadow.
- **StatusBar** — full-width `theme.StatusBar()` (black-on-cyan), context hint sets +
  right-aligned `Ln n  Col n` + INS/OVR + `*` when modified.
- **TextArea / Editor** — multi-line editor widget (built in the app-port wave, reusing
  `internal/buffer` + the existing editor logic): white-on-blue text, blue-on-cyan selection,
  black-on-white cursor, both scrollbars, optional line-number gutter.

## Conventions for builders
- One package `tui`; each agent owns DISTINCT files; prefix unexported helpers per widget
  (e.g. `btn*`, `lb*`, `cb*`) to avoid collisions; do NOT redeclare `min`/`max` (builtins).
- No tview imports anywhere in `tui`. No `go get`/`go mod`/`git` (coordinator handles).
- Each widget: a headless render smoke test (tcell SimulationScreen) + behaviour tests where
  it makes sense (toggle, focus, selection, scroll).
