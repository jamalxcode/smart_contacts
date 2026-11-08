package main

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The Card and View menus carry the same pictures as the toolbar's card tabs and the side
// tabs, so a command looks the same wherever you meet it. Fyne tints a themed menu icon with
// the menu's highlighted-text colour (white), which vanishes in a light menu, so each icon
// is drawn in its tab's colour instead, deep enough to read in light and dark menus alike.
// (Menu icons are set once, when the menu bar is built: see refreshMenu.)

var iconClipOutline = svgIcon("clip", "M16.5 6v11.5c0 2.21-1.79 4-4 4s-4-1.79-4-4V5c0-1.38 1.12-2.5 2.5-2.5s2.5 1.12 2.5 2.5v10.5c0 .55-.45 1-1 1s-1-.45-1-1V6H10v9.5c0 1.38 1.12 2.5 2.5 2.5s2.5-1.12 2.5-2.5V5c0-2.21-1.79-4-4-4S7 2.79 7 5v12.5c0 3.04 2.46 5.5 5.5 5.5s5.5-2.46 5.5-5.5V6h-1.5z")

var (
	reFill    = regexp.MustCompile(`fill="[^"]*"`)
	reFillCSS = regexp.MustCompile(`fill:\s*#[0-9a-fA-F]{3,8}`)
	menuIcons = map[string]fyne.Resource{}
)

// deepen darkens a tab colour so it reads as ink in a light menu and still shows in a dark one.
func deepen(c color.NRGBA, k float64) color.NRGBA {
	return color.NRGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), 0xFF}
}

// inkIcon is res redrawn in one colour, as a plain (untinted) picture.
func inkIcon(res fyne.Resource, col color.NRGBA) fyne.Resource {
	key := fmt.Sprintf("%s#%02x%02x%02x", res.Name(), col.R, col.G, col.B)
	if r, ok := menuIcons[key]; ok {
		return r
	}
	hex := fmt.Sprintf("#%02x%02x%02x", col.R, col.G, col.B)
	svg := string(res.Content())
	svg = reFill.ReplaceAllStringFunc(svg, func(f string) string {
		if f == `fill="none"` || f == `fill="transparent"` {
			return f // the drawing's invisible background box stays invisible
		}
		return `fill="` + hex + `"`
	})
	svg = reFillCSS.ReplaceAllString(svg, "fill:"+hex)
	if !strings.Contains(svg, "fill") {
		svg = strings.Replace(svg, "<svg ", `<svg fill="`+hex+`" `, 1)
	}
	r := fyne.NewStaticResource(strings.TrimSuffix(res.Name(), ".svg")+"-menu-"+hex[1:]+".svg", []byte(svg))
	menuIcons[key] = r
	return r
}

type menuIcon struct {
	res fyne.Resource
	col color.NRGBA
}

// cardMenuIcons: the toolbar's tab for each Card command, in its tab colour.
func cardMenuIcons() map[string]menuIcon {
	deep := func(i int) color.NRGBA { return deepen(tabCards[i%len(tabCards)], 0.62) }
	return map[string]menuIcon{
		"Today":            {iconToday, color.NRGBA{0x8A, 0x6A, 0x3A, 0xFF}},
		"Add Note…":        {theme.DocumentCreateIcon(), deep(0)},
		"Favorite":         {iconStar, deep(1)},
		"Keep in Touch":    {iconClock, deep(2)},
		"Groups…":          {iconLabel, deep(3)},
		"Connections…":     {iconPeople, deep(4)},
		"Tidy Up…":         {iconBroom, deep(5)},
		"Find Duplicates…": {theme.ContentCopyIcon(), deep(6)},
		"Share":            {theme.UploadIcon(), deep(7)},
		"Clip Cards":       {iconClipOutline, deep(8)},
		"Check Messages…":  {iconMessage, deep(9)},
	}
}

// viewMenuIcons: the side tab's picture for each View item, in the side tab's colour (the tabs
// take tabColors in order: Today, All Contacts, Favorites…).
func viewMenuIcons() map[string]menuIcon {
	tab := func(i int) color.NRGBA { return deepen(tabColors[i%len(tabColors)], 0.85) }
	stone := color.NRGBA{0x6E, 0x68, 0x5E, 0xFF}
	return map[string]menuIcon{
		"Today":                  {iconToday, tab(0)},
		"All Contacts":           {theme.AccountIcon(), tab(1)},
		"Favorites":              {iconStar, tab(2)},
		"Birthdays":              {iconCake, tab(3)},
		"Recently Added":         {theme.HistoryIcon(), tab(4)},
		"Reach Out":              {theme.MailReplyIcon(), tab(5)},
		"Needs Attention":        {theme.WarningIcon(), stone},
		"Missing Phone or Email": {theme.QuestionIcon(), stone},
		"Go to Saved Search…":    {theme.SearchIcon(), tab(6)},
		"Save This Search…":      {theme.SearchIcon(), tab(6)},
		"Go to Group…":           {iconLabel, tab(7)},
		"Calendar Log…":          {theme.HistoryIcon(), stone},
		"Trash…":                 {theme.DeleteIcon(), stone},
	}
}

// iconize puts the pictures on a menu's items.
func iconize(m *fyne.Menu, icons map[string]menuIcon) {
	for _, it := range m.Items {
		if ic, ok := icons[it.Label]; ok && !isHeader(it) {
			it.Icon = inkIcon(ic.res, ic.col)
		}
	}
}

// isHeader: a section heading (dimmed, with nothing to do).
func isHeader(it *fyne.MenuItem) bool { return it.Disabled && it.Action == nil && it.ChildMenu == nil }
