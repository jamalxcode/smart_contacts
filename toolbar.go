package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The organiser's toolbar: the Card menu's commands as icons on the desk above the book, in the
// same order and groups. At its left it says what they'll act on — the open card, or the
// clipped cards — so there's never a doubt.

var (
	deskInk    = color.NRGBA{0xEC, 0xE0, 0xC8, 0xFF} // cream on the dark desk
	deskInkDim = color.NRGBA{0xEC, 0xE0, 0xC8, 0x55}
	iconBroom  = svgIcon("broom", "M16 11h-1V3c0-1.1-.9-2-2-2h-2c-1.1 0-2 .9-2 2v8H8c-2.76 0-5 2.24-5 5v7h18v-7c0-2.76-2.24-5-5-5zm-5-8h2v8h-2V3zm8 18h-2v-3c0-.55-.45-1-1-1s-1 .45-1 1v3h-2v-3c0-.55-.45-1-1-1s-1 .45-1 1v3H9v-3c0-.55-.45-1-1-1s-1 .45-1 1v3H5v-5c0-1.65 1.35-3 3-3h8c1.65 0 3 1.35 3 3v5z")
	iconPeople = svgIcon("people", "M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z")
	iconClock  = svgIcon("clock", "M11.99 2C6.47 2 2 6.48 2 12s4.47 10 9.99 10C17.52 22 22 17.52 22 12S17.52 2 11.99 2zM12 20c-4.42 0-8-3.58-8-8s3.58-8 8-8 8 3.58 8 8-3.58 8-8 8zm.5-13H11v6l5.25 3.15.75-1.23-4.5-2.67z")
)

// deskTheme draws the toolbar's icons in cream on the dark desk.
type deskTheme struct{ fyne.Theme }

func (t *deskTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameForeground:
		return deskInk
	case theme.ColorNameDisabled:
		return deskInkDim
	}
	return t.Theme.Color(n, v)
}

// toolButton is an icon with its name under it.
type toolButton struct {
	widget.BaseWidget
	res      fyne.Resource
	text     string
	onTap    func(b *toolButton)
	disabled bool
	on       bool // a toggle that's on (lit, like a menu item's tick)
	hover    bool
	icon     *widget.Icon
	img      *canvas.Image // for a picture that isn't a themed icon (the paper clip)
	label    *canvas.Text
	bg       *canvas.Rectangle
}

func newToolButton(res fyne.Resource, text string, onTap func(*toolButton)) *toolButton {
	b := &toolButton{res: res, text: text, onTap: onTap}
	b.ExtendBaseWidget(b)
	return b
}

func (b *toolButton) setIcon(res fyne.Resource) {
	b.res = res
	b.Refresh()
}

func (b *toolButton) setDisabled(d bool) {
	if b.disabled != d {
		b.disabled = d
		b.Refresh()
	}
}

func (b *toolButton) Tapped(*fyne.PointEvent) {
	if !b.disabled && b.onTap != nil {
		b.onTap(b)
	}
}
func (b *toolButton) Cursor() desktop.Cursor {
	if b.disabled {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}
func (b *toolButton) MouseIn(*desktop.MouseEvent)    { b.hover = true; b.Refresh() }
func (b *toolButton) MouseMoved(*desktop.MouseEvent) {}
func (b *toolButton) MouseOut()                      { b.hover = false; b.Refresh() }

func (b *toolButton) CreateRenderer() fyne.WidgetRenderer {
	b.bg = canvas.NewRectangle(color.Transparent)
	b.bg.CornerRadius = 6
	b.icon = widget.NewIcon(nil)
	b.img = canvas.NewImageFromResource(nil)
	b.img.FillMode = canvas.ImageFillContain
	b.label = canvas.NewText(b.text, deskInk)
	b.label.TextSize = 10
	b.label.Alignment = fyne.TextAlignCenter
	r := &toolRenderer{b: b}
	r.Refresh()
	return r
}

type toolRenderer struct{ b *toolButton }

func (r *toolRenderer) Layout(s fyne.Size) {
	b := r.b
	b.bg.Resize(s)
	const is = 22
	at := fyne.NewPos((s.Width-is)/2, 5)
	b.icon.Move(at)
	b.icon.Resize(fyne.NewSize(is, is))
	b.img.Move(fyne.NewPos((s.Width-10)/2, 3))
	b.img.Resize(fyne.NewSize(10, 26))
	b.label.Move(fyne.NewPos(0, 31))
	b.label.Resize(fyne.NewSize(s.Width, 14))
}

func (r *toolRenderer) MinSize() fyne.Size {
	return fyne.NewSize(fyne.Max(58, r.b.label.MinSize().Width+12), 48)
}

func (r *toolRenderer) Refresh() {
	b := r.b
	_, themed := b.res.(*theme.ThemedResource)
	_, svg := b.res.(*fyne.StaticResource)
	if themed || (svg && isSVG(b.res)) {
		res := b.res
		if b.disabled {
			res = theme.NewDisabledResource(res)
		}
		b.icon.SetResource(res)
		b.icon.Show()
		b.img.Hide()
	} else {
		b.img.Resource = b.res
		b.img.Translucency = 0
		if b.disabled {
			b.img.Translucency = 0.6
		}
		b.img.Show()
		b.img.Refresh()
		b.icon.Hide()
	}
	b.label.Color = deskInk
	if b.disabled {
		b.label.Color = deskInkDim
	}
	b.label.Refresh()
	if b.on {
		b.bg.FillColor = color.NRGBA{0xFF, 0xF0, 0xD0, 0x40}
	} else if b.hover && !b.disabled {
		b.bg.FillColor = color.NRGBA{0xFF, 0xF0, 0xD0, 0x22}
	} else {
		b.bg.FillColor = color.Transparent
	}
	b.bg.Refresh()
	r.Layout(b.Size())
}

func (r *toolRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.b.bg, r.b.icon, r.b.img, r.b.label}
}
func (r *toolRenderer) Destroy() {}

func isSVG(res fyne.Resource) bool {
	c := res.Content()
	return len(c) > 4 && (string(c[:4]) == "<svg" || string(c[:5]) == "<?xml")
}

// toolbarParts are the buttons whose state follows what's open or clipped.
type toolbarParts struct {
	who                                           *canvas.Text
	note, fav, keep, groups, conn, share, clipBtn *toolButton
}

// buildToolbar makes the toolbar for the organiser.
func (u *ui) buildToolbar() fyne.CanvasObject {
	loadClips()
	t := &toolbarParts{}
	popup := func(b *toolButton, items []*fyne.MenuItem) {
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(b)
		widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), u.win.Canvas(), pos.Add(fyne.NewPos(0, b.Size().Height)))
	}
	t.note = newToolButton(theme.DocumentCreateIcon(), "Add Note", func(*toolButton) { u.cmdNote() })
	t.fav = newToolButton(iconStarOff, "Favorite", func(*toolButton) { u.cmdFavorite() })
	t.keep = newToolButton(iconClock, "Keep in Touch", func(b *toolButton) { popup(b, u.keepItems()) })
	t.groups = newToolButton(iconLabel, "Groups", func(*toolButton) { u.cmdGroups() })
	t.conn = newToolButton(iconPeople, "Connections", func(*toolButton) { u.cmdConnections() })
	tidy := newToolButton(iconBroom, "Tidy Up", func(*toolButton) { u.cmdTidy() })
	dupes := newToolButton(theme.ContentCopyIcon(), "Find Duplicates", func(*toolButton) { u.cmdDuplicates() })
	t.share = newToolButton(theme.UploadIcon(), "Share", func(b *toolButton) { popup(b, u.shareItems()) })
	t.clipBtn = newToolButton(clipOn, "Clip Cards", func(*toolButton) { u.toggleSelectMode() })
	check := newToolButton(iconMessage, "Check Messages", func(*toolButton) { u.cmdCheckMessages() })

	divider := func() fyne.CanvasObject {
		l := canvas.NewRectangle(color.NRGBA{0xEC, 0xE0, 0xC8, 0x30})
		l.SetMinSize(fyne.NewSize(1, 34))
		return container.NewCenter(l)
	}
	actingOn := canvas.NewText("ACTING ON", deskInkDim)
	actingOn.TextSize = 9
	actingOn.TextStyle.Bold = true
	t.who = canvas.NewText("", deskInk)
	t.who.TextSize = 14
	t.who.TextStyle = serif("", true, false)
	whoBox := container.New(&fixedWidth{200}, container.NewVBox(layout.NewSpacer(), actingOn, t.who, layout.NewSpacer()))

	row := container.NewHBox(
		whoBox,
		layout.NewSpacer(),
		t.note, t.fav, t.keep, t.groups, t.conn,
		divider(),
		tidy, dupes, t.share,
		divider(),
		t.clipBtn,
		divider(),
		check,
		layout.NewSpacer(),
	)
	u.tools = t
	bar := container.NewThemeOverride(container.New(&toolbarLayout{}, row), &deskTheme{fyne.CurrentApp().Settings().Theme()})
	u.refreshToolbar()
	return bar
}

// toolbarLayout lines the toolbar up with the book under it.
type toolbarLayout struct{}

func (toolbarLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return objs[0].MinSize().Add(fyne.NewSize(56, 10))
}
func (toolbarLayout) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	objs[0].Move(fyne.NewPos(28, 6))
	objs[0].Resize(fyne.NewSize(s.Width-56, s.Height-8))
}

// refreshToolbar dims what can't act now, ticks Favorite, and says what the buttons act on.
func (u *ui) refreshToolbar() {
	t := u.tools
	if t == nil {
		return
	}
	ts := u.targets()
	none := len(ts) == 0
	t.who.Text = u.targetText(ts)
	if u.cfg.ClassicLook {
		t.who.TextStyle = fyne.TextStyle{Bold: true}
	} else {
		t.who.TextStyle = serif(t.who.Text, true, false)
	}
	if none {
		t.who.Color = deskInkDim
	} else {
		t.who.Color = deskInk
	}
	t.who.Refresh()
	for _, b := range []*toolButton{t.note, t.fav, t.keep, t.groups, t.share} {
		b.setDisabled(none)
	}
	t.conn.setDisabled(!u.connectionsEnabled(ts))
	if allFavorite(ts) {
		t.fav.setIcon(iconStar)
	} else {
		t.fav.setIcon(iconStarOff)
	}
	// Clip Cards stays lit while cards are being clipped, as the menu item has its tick
	t.clipBtn.on = u.selectMode
	t.clipBtn.Refresh()
}
