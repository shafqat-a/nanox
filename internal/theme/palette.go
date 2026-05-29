// Package theme defines the shared visual contract for DOSEdit: the 16
// authoritative DOS colours, the named style helpers used across every UI
// module, and the box-drawing/glyph runes for frames, scrollbars and menus.
//
// Colours are defined once via tcell.NewRGBColor so the look is identical on
// every truecolor terminal; never rely on the terminal's named ANSI colours.
// This package is deliberately free of UI imports so it stays unit-testable.
package theme

import "github.com/gdamore/tcell/v2"

// The 16 standard DOS colours with their authoritative RGB values
// (spec §4.2 / Appendix C).
var (
	Black    = tcell.NewRGBColor(0, 0, 0)
	Blue     = tcell.NewRGBColor(0, 0, 170)
	Green    = tcell.NewRGBColor(0, 170, 0)
	Cyan     = tcell.NewRGBColor(0, 170, 170)
	Red      = tcell.NewRGBColor(170, 0, 0)
	Magenta  = tcell.NewRGBColor(170, 0, 170)
	Brown    = tcell.NewRGBColor(170, 85, 0)
	LGray    = tcell.NewRGBColor(170, 170, 170)
	DGray    = tcell.NewRGBColor(85, 85, 85)
	LBlue    = tcell.NewRGBColor(85, 85, 255)
	LGreen   = tcell.NewRGBColor(85, 255, 85)
	LCyan    = tcell.NewRGBColor(85, 255, 255)
	LRed     = tcell.NewRGBColor(255, 85, 85)
	LMagenta = tcell.NewRGBColor(255, 85, 255)
	Yellow   = tcell.NewRGBColor(255, 255, 85)
	White    = tcell.NewRGBColor(255, 255, 255)
)

// Style helpers. Each returns the foreground/background pairing for a UI
// element, per the colour-assignment table (spec §4.3). These assignments are
// authoritative — they are taken from real VB-for-DOS screenshots.

// Desktop is the grey hatched desktop background.
func Desktop() tcell.Style { return tcell.StyleDefault.Foreground(Black).Background(LGray) }

// MenuNormal is an unselected menu-bar item.
func MenuNormal() tcell.Style { return tcell.StyleDefault.Foreground(Black).Background(LGray) }

// MenuMnemonic is the highlighted mnemonic letter on the menu bar.
func MenuMnemonic() tcell.Style { return tcell.StyleDefault.Foreground(White).Background(LGray) }

// MenuSelect is the open/active top-level menu item (reverse video).
func MenuSelect() tcell.Style { return tcell.StyleDefault.Foreground(White).Background(Black) }

// DropdownBody is a normal row inside an open dropdown menu.
func DropdownBody() tcell.Style { return tcell.StyleDefault.Foreground(Black).Background(LGray) }

// DropdownHi is the highlighted (current) dropdown row.
func DropdownHi() tcell.Style { return tcell.StyleDefault.Foreground(White).Background(Black) }

// DropdownHiMnemonic is the mnemonic letter on the highlighted dropdown row.
func DropdownHiMnemonic() tcell.Style {
	return tcell.StyleDefault.Foreground(LCyan).Background(Black)
}

// DropdownDisabled is a disabled (greyed) dropdown item.
func DropdownDisabled() tcell.Style { return tcell.StyleDefault.Foreground(DGray).Background(LGray) }

// EditorText is the editor's text surface.
func EditorText() tcell.Style { return tcell.StyleDefault.Foreground(White).Background(Blue) }

// Selection is highlighted/selected text in the editor.
func Selection() tcell.Style { return tcell.StyleDefault.Foreground(Blue).Background(Cyan) }

// Cursor is the text cursor cell.
func Cursor() tcell.Style { return tcell.StyleDefault.Foreground(Black).Background(White) }

// ActiveFrame is the frame of the active MDI window.
func ActiveFrame() tcell.Style { return tcell.StyleDefault.Foreground(White).Background(Blue) }

// ActiveTitle is the title bar of the active MDI window.
func ActiveTitle() tcell.Style { return tcell.StyleDefault.Foreground(White).Background(Magenta) }

// InactiveFrame is the frame of an inactive MDI window.
func InactiveFrame() tcell.Style { return tcell.StyleDefault.Foreground(LGray).Background(Blue) }

// InactiveTitle is the title bar of an inactive MDI window.
func InactiveTitle() tcell.Style { return tcell.StyleDefault.Foreground(Black).Background(LGray) }

// StatusBar is the bottom status / function-key bar.
func StatusBar() tcell.Style { return tcell.StyleDefault.Foreground(Black).Background(Cyan) }

// DialogBody is the body of a modal dialog.
func DialogBody() tcell.Style { return tcell.StyleDefault.Foreground(Black).Background(LGray) }

// DialogTitle is the title bar of a modal dialog.
func DialogTitle() tcell.Style { return tcell.StyleDefault.Foreground(White).Background(Magenta) }

// InputField is a dialog text-input field.
func InputField() tcell.Style { return tcell.StyleDefault.Foreground(White).Background(Black) }

// ListBox is the body of a dialog list box.
func ListBox() tcell.Style { return tcell.StyleDefault.Foreground(Black).Background(White) }

// ListSelected is the selected row in a dialog list box.
func ListSelected() tcell.Style { return tcell.StyleDefault.Foreground(White).Background(Black) }

// ButtonFace is the face of a dialog push button.
func ButtonFace() tcell.Style { return tcell.StyleDefault.Foreground(Black).Background(LGray) }

// Shadow is the solid-black drop shadow cast by menus, dialogs and buttons.
func Shadow() tcell.Style { return tcell.StyleDefault.Background(Black) }

// Box-drawing and glyph runes for shared use (spec §4.4).
const (
	// Double-line frame set: active windows and the dialog outer frame.
	TLDouble = '╔'
	TRDouble = '╗'
	BLDouble = '╚'
	BRDouble = '╝'
	HDouble  = '═'
	VDouble  = '║'

	// Single-line frame set: inactive windows, dropdowns and inner controls.
	TLSingle = '┌'
	TRSingle = '┐'
	BLSingle = '└'
	BRSingle = '┘'
	HSingle  = '─'
	VSingle  = '│'

	// Texture is the medium-shade desktop hatch (U+2592).
	Texture = '▒'

	// Title-bar buttons.
	BtnClose   = '■'
	BtnMax     = '▲'
	BtnRestore = '▼'

	// Scrollbar glyphs.
	SbTrack = '▒'
	SbThumb = '█'
	SbUp    = '▲'
	SbDown  = '▼'
	SbLeft  = '◄'
	SbRight = '►'

	// Menu separator tee glyphs, joined to a full-width '─' rule.
	TeeLeft  = '├'
	TeeRight = '┤'
)
