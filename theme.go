package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// appTheme is Fyne's default theme with secondary text made readable. Fyne draws
// "low importance" labels in the disabled colour, which is nearly invisible in dark mode;
// these values match macOS's own secondary label colours.
type appTheme struct{ fyne.Theme }

func newAppTheme() fyne.Theme { return &appTheme{theme.DefaultTheme()} }

func (t *appTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if n == theme.ColorNameDisabled {
		if v == theme.VariantDark {
			return color.NRGBA{0x98, 0x98, 0x9D, 0xFF}
		}
		return color.NRGBA{0x6E, 0x6E, 0x73, 0xFF}
	}
	return t.Theme.Color(n, v)
}

func (t *appTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNameCaptionText {
		return 12
	}
	return t.Theme.Size(n)
}
