package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// appTheme follows the macOS look: Apple's system colours for light and dark appearance
// (system blue accent, semantic label and separator greys, red/green/orange), 13-point body text
// and the tighter spacing and rounder corners of Mac controls. It switches with the system
// appearance automatically.
type appTheme struct{ fyne.Theme }

func newAppTheme() fyne.Theme { return &appTheme{theme.DefaultTheme()} }

func rgb(r, g, b uint8) color.NRGBA     { return color.NRGBA{r, g, b, 0xFF} }
func rgba(r, g, b, a uint8) color.NRGBA { return color.NRGBA{r, g, b, a} }
func pick(v fyne.ThemeVariant, light, dark color.Color) color.Color {
	if v == theme.VariantDark {
		return dark
	}
	return light
}

// macOS system colours (NSColor), light / dark.
var (
	sysBlueL, sysBlueD   = rgb(0x00, 0x7A, 0xFF), rgb(0x0A, 0x84, 0xFF)
	sysRedL, sysRedD     = rgb(0xFF, 0x3B, 0x30), rgb(0xFF, 0x45, 0x3A)
	sysGreenL, sysGreenD = rgb(0x28, 0xCD, 0x41), rgb(0x32, 0xD7, 0x4B)
	sysOrangeL, sysOrgD  = rgb(0xFF, 0x95, 0x00), rgb(0xFF, 0x9F, 0x0A)
)

// sidebarColor is the tint of the source list on the left, like Finder's and Contacts'.
func sidebarColor(v fyne.ThemeVariant) color.Color {
	return pick(v, rgb(0xEC, 0xEB, 0xEE), rgb(0x2A, 0x2A, 0x2C))
}

func (t *appTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground: // window / content background
		return pick(v, rgb(0xFF, 0xFF, 0xFF), rgb(0x1E, 0x1E, 0x1E))
	case theme.ColorNameForeground: // labelColor
		return pick(v, rgba(0x1D, 0x1D, 0x1F, 0xFF), rgb(0xF2, 0xF2, 0xF7))
	case theme.ColorNameDisabled: // secondaryLabelColor (Fyne draws low-importance text with it)
		return pick(v, rgb(0x6E, 0x6E, 0x73), rgb(0x98, 0x98, 0x9D))
	case theme.ColorNamePlaceHolder: // tertiaryLabelColor
		return pick(v, rgb(0x8E, 0x8E, 0x93), rgb(0x7C, 0x7C, 0x80))
	case theme.ColorNamePrimary, theme.ColorNameHyperlink: // controlAccentColor
		return pick(v, sysBlueL, sysBlueD)
	case theme.ColorNameForegroundOnPrimary:
		return color.White
	case theme.ColorNameFocus:
		return pick(v, rgba(0x00, 0x7A, 0xFF, 0x80), rgba(0x0A, 0x84, 0xFF, 0x90))
	case theme.ColorNameSelection: // selected row in a list (accent, translucent so text stays readable)
		return pick(v, rgba(0x00, 0x7A, 0xFF, 0x2E), rgba(0x0A, 0x84, 0xFF, 0x55))
	case theme.ColorNameHover:
		return pick(v, rgba(0x00, 0x00, 0x00, 0x0D), rgba(0xFF, 0xFF, 0xFF, 0x12))
	case theme.ColorNamePressed:
		return pick(v, rgba(0x00, 0x00, 0x00, 0x1F), rgba(0xFF, 0xFF, 0xFF, 0x24))
	case theme.ColorNameButton: // bordered push button
		return pick(v, rgb(0xE8, 0xE8, 0xED), rgb(0x3A, 0x3A, 0x3C))
	case theme.ColorNameDisabledButton:
		return pick(v, rgb(0xF2, 0xF2, 0xF5), rgb(0x2C, 0x2C, 0x2E))
	case theme.ColorNameInputBackground: // search and text fields
		return pick(v, rgb(0xF2, 0xF2, 0xF5), rgb(0x2C, 0x2C, 0x2E))
	case theme.ColorNameInputBorder:
		return pick(v, rgb(0xD1, 0xD1, 0xD6), rgb(0x48, 0x48, 0x4A))
	case theme.ColorNameSeparator: // separatorColor
		return pick(v, rgb(0xE3, 0xE3, 0xE8), rgb(0x38, 0x38, 0x3A))
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground: // popovers, sheets
		return pick(v, rgb(0xF6, 0xF6, 0xF8), rgb(0x2C, 0x2C, 0x2E))
	case theme.ColorNameHeaderBackground:
		return pick(v, rgb(0xF2, 0xF2, 0xF7), rgb(0x2C, 0x2C, 0x2E))
	case theme.ColorNameScrollBar:
		return pick(v, rgba(0x00, 0x00, 0x00, 0x40), rgba(0xFF, 0xFF, 0xFF, 0x50))
	case theme.ColorNameShadow:
		return pick(v, rgba(0x00, 0x00, 0x00, 0x26), rgba(0x00, 0x00, 0x00, 0x66))
	case theme.ColorNameError:
		return pick(v, sysRedL, sysRedD)
	case theme.ColorNameSuccess:
		return pick(v, sysGreenL, sysGreenD)
	case theme.ColorNameWarning:
		return pick(v, sysOrangeL, sysOrgD)
	}
	return t.Theme.Color(n, v)
}

// Sizes: 13-point body and 11-point captions as in macOS, smaller paddings, rounded controls.
func (t *appTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameText:
		return 13
	case theme.SizeNameCaptionText:
		return 11
	case theme.SizeNameSubHeadingText:
		return 15
	case theme.SizeNameHeadingText:
		return 22
	case theme.SizeNamePadding:
		return 4
	case theme.SizeNameInnerPadding:
		return 7
	case theme.SizeNameLineSpacing:
		return 3
	case theme.SizeNameInlineIcon:
		return 17
	case theme.SizeNameInputRadius:
		return 6
	case theme.SizeNameSelectionRadius:
		return 6
	case theme.SizeNameScrollBar:
		return 10
	case theme.SizeNameScrollBarSmall:
		return 4
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return t.Theme.Size(n)
}

// paneTheme draws the dividers between the three panes as a hairline-coloured gutter instead of
// Fyne's dark shadow band. Fyne paints the divider with the shadow colour; nothing else inside
// the panes uses it (pop-ups and dialogs take the app theme).
type paneTheme struct{ fyne.Theme }

func (t *paneTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if n == theme.ColorNameShadow {
		return t.Theme.Color(theme.ColorNameSeparator, v)
	}
	return t.Theme.Color(n, v)
}
