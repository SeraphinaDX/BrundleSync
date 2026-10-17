// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"image/color"
)

// Keep stock Fyne widgets, fonts and metrics; only the accent matches our icon.
type applicationTheme struct{ fyne.Theme }

func (t applicationTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNamePrimary {
		if variant == theme.VariantDark {
			return color.NRGBA{196, 161, 255, 255}
		}
		return color.NRGBA{116, 84, 184, 255}
	}
	return t.Theme.Color(name, variant)
}
