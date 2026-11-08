package main

import (
	"image/color"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The View menu and the sidebar (the organiser's tabs, or the plain look's list) are the same
// thing twice: the lists, To Tidy, Saved Searches, Groups, then Sort and Also in the Book
// (Calendar Log, Trash), with the same names in the same order. The sidebar shows each group
// and saved search by name; the menu, whose titles can't change once it's built, reaches them
// through Go to Group… and Go to Saved Search….

// setSort orders the list by first or last name.
func (u *ui) setSort(last bool) {
	if u.cfg.SortByLast == last {
		return
	}
	u.cfg.SortByLast = last
	_ = u.cfg.Save(u.store.Dir())
	u.refreshList()
	u.refreshSortFooter()
	u.refreshMenu()
}

// goToDialog picks a group (prefix "g:") or a saved search ("q:") and opens it, as clicking
// its tab does.
func (u *ui) goToDialog(prefix string) {
	var names []string
	title := "Go to Group"
	if prefix == "q:" {
		title = "Go to Saved Search"
		for _, sg := range u.cfg.SmartGroups {
			names = append(names, sg.Name)
		}
	} else {
		names = u.store.Groups()
	}
	if len(names) == 0 {
		return
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	var d dialog.Dialog
	l := widget.NewList(func() int { return len(names) },
		func() fyne.CanvasObject { return widget.NewLabel("Group name") },
		func(i widget.ListItemID, o fyne.CanvasObject) { o.(*widget.Label).SetText(names[i]) })
	l.OnSelected = func(i widget.ListItemID) {
		d.Hide()
		u.showGroup(prefix + names[i])
	}
	d = newCustom(title, "Cancel", l, u.win)
	d.Resize(fyne.NewSize(360, fyne.Min(460, float32(len(names))*38+110)))
	d.Show()
}

// ---------- the sidebar's footer: Sort, and Also in the Book ----------

// organiserFooter is the bottom of the tab column: Sort, and Also in the Book, printed in
// black on a cream index card tucked into the cover, one choice to a line.
func (u *ui) organiserFooter() fyne.CanvasObject {
	u.sortFirst = newCardLine("First Name", nil, func() { u.setSort(false) })
	u.sortLast = newCardLine("Last Name", nil, func() { u.setSort(true) })
	u.sortFirst.radio, u.sortLast.radio = true, true
	u.refreshSortFooter()
	body := container.NewVBox(
		newCardHeading("Sort by"),
		u.sortFirst,
		u.sortLast,
		newCardGap(4),
		newCardHeading("Also in the Book"),
		newCardLine("Calendar Log", theme.HistoryIcon(), u.calendarDialog),
		newCardLine("Trash", theme.DeleteIcon(), u.trash),
	)
	return container.New(&insetLayout{left: 6, right: 18, top: 8, bottom: 22}, newIndexCard(body))
}

// classicFooter is the same for the plain look's sidebar.
func (u *ui) classicFooter() fyne.CanvasObject {
	head := func(s string) fyne.CanvasObject {
		l := widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		l.Importance = widget.LowImportance
		l.SizeName = theme.SizeNameCaptionText
		return l
	}
	u.sortRadio = widget.NewRadioGroup([]string{"First Name", "Last Name"}, func(s string) {
		if s != "" {
			u.setSort(s == "Last Name")
		}
	})
	u.sortRadio.Horizontal = true
	u.sortRadio.Required = true
	u.refreshSortFooter()
	cal := widget.NewButtonWithIcon("Calendar Log", theme.HistoryIcon(), u.calendarDialog)
	trash := widget.NewButtonWithIcon("Trash", theme.DeleteIcon(), u.trash)
	cal.Importance, trash.Importance = widget.LowImportance, widget.LowImportance
	cal.Alignment, trash.Alignment = widget.ButtonAlignLeading, widget.ButtonAlignLeading
	return container.NewVBox(widget.NewSeparator(), head("Sort"), u.sortRadio, head("Also in the Book"), cal, trash)
}

// refreshSortFooter shows which sort is on, in either look.
func (u *ui) refreshSortFooter() {
	if u.sortFirst != nil {
		u.sortFirst.on, u.sortLast.on = !u.cfg.SortByLast, u.cfg.SortByLast
		u.sortFirst.Refresh()
		u.sortLast.Refresh()
	}
	if u.sortRadio != nil {
		want := "First Name"
		if u.cfg.SortByLast {
			want = "Last Name"
		}
		if u.sortRadio.Selected != want {
			u.sortRadio.SetSelected(want)
		}
	}
}

// insetLayout leaves a margin round its one object.
type insetLayout struct{ left, right, top, bottom float32 }

func (l *insetLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return objs[0].MinSize().Add(fyne.NewSize(l.left+l.right, l.top+l.bottom))
}
func (l *insetLayout) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	objs[0].Move(fyne.NewPos(l.left, l.top))
	objs[0].Resize(fyne.NewSize(s.Width-l.left-l.right, s.Height-l.top-l.bottom))
}

// ---------- the index card ----------

var cardBlack = color.NRGBA{0x14, 0x10, 0x0C, 0xFF}

// indexCard is a cream card with a faint shadow, holding its lines.
type indexCard struct {
	widget.BaseWidget
	body fyne.CanvasObject
}

func newIndexCard(body fyne.CanvasObject) *indexCard {
	c := &indexCard{body: body}
	c.ExtendBaseWidget(c)
	return c
}

func (c *indexCard) CreateRenderer() fyne.WidgetRenderer {
	shadow := canvas.NewRectangle(color.NRGBA{0x10, 0x05, 0x00, 0x66})
	shadow.CornerRadius = 4
	paper := canvas.NewRectangle(color.NRGBA{0xFB, 0xF7, 0xEC, 0xFF})
	paper.CornerRadius = 4
	paper.StrokeColor = color.NRGBA{0xC8, 0xBC, 0xA4, 0xFF}
	paper.StrokeWidth = 1
	top := canvas.NewRectangle(color.NRGBA{0xC0, 0x50, 0x48, 0x90}) // an index card's red top rule
	return &indexCardRenderer{c: c, shadow: shadow, paper: paper, top: top}
}

type indexCardRenderer struct {
	c                  *indexCard
	shadow, paper, top *canvas.Rectangle
}

func (r *indexCardRenderer) Layout(s fyne.Size) {
	r.shadow.Move(fyne.NewPos(2, 3))
	r.shadow.Resize(s)
	r.paper.Resize(s)
	r.top.Move(fyne.NewPos(8, 25))
	r.top.Resize(fyne.NewSize(s.Width-16, 1))
	r.c.body.Move(fyne.NewPos(10, 6))
	r.c.body.Resize(fyne.NewSize(s.Width-20, s.Height-10))
}
func (r *indexCardRenderer) MinSize() fyne.Size {
	return r.c.body.MinSize().Add(fyne.NewSize(20, 12))
}
func (r *indexCardRenderer) Refresh() { r.Layout(r.c.Size()) }
func (r *indexCardRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.shadow, r.paper, r.top, r.c.body}
}
func (r *indexCardRenderer) Destroy() {}

// newCardHeading is a small black heading on the card.
func newCardHeading(s string) fyne.CanvasObject {
	t := canvas.NewText(s, cardBlack)
	t.TextSize = 11
	t.TextStyle = serif(s, true, false)
	return container.New(&insetLayout{top: 2, bottom: 2}, t)
}

func newCardGap(h float32) fyne.CanvasObject {
	g := canvas.NewRectangle(color.Transparent)
	g.SetMinSize(fyne.NewSize(1, h))
	return g
}

// cardLine is one choice on the card: a radio mark (for Sort) or an icon, and its words.
type cardLine struct {
	widget.BaseWidget
	text  string
	icon  fyne.Resource
	radio bool
	on    bool
	hover bool
	onTap func()
}

func newCardLine(text string, icon fyne.Resource, onTap func()) *cardLine {
	l := &cardLine{text: text, icon: icon, onTap: onTap}
	l.ExtendBaseWidget(l)
	return l
}
func (l *cardLine) Tapped(*fyne.PointEvent)        { l.onTap() }
func (l *cardLine) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (l *cardLine) MouseIn(*desktop.MouseEvent)    { l.hover = true; l.Refresh() }
func (l *cardLine) MouseMoved(*desktop.MouseEvent) {}
func (l *cardLine) MouseOut()                      { l.hover = false; l.Refresh() }

func (l *cardLine) CreateRenderer() fyne.WidgetRenderer {
	r := &cardLineRenderer{l: l}
	r.bg = canvas.NewRectangle(color.Transparent)
	r.bg.CornerRadius = 4
	r.ring = canvas.NewCircle(color.Transparent)
	r.ring.StrokeColor = cardBlack
	r.ring.StrokeWidth = 1.4
	r.dot = canvas.NewCircle(cardBlack)
	if l.icon != nil {
		r.icon = canvas.NewImageFromResource(theme.NewColoredResource(l.icon, theme.ColorNameForeground))
		r.icon.FillMode = canvas.ImageFillContain
	}
	r.text = canvas.NewText(l.text, cardBlack)
	r.text.TextSize = 13
	r.Refresh()
	return r
}

type cardLineRenderer struct {
	l    *cardLine
	bg   *canvas.Rectangle
	ring *canvas.Circle
	dot  *canvas.Circle
	icon *canvas.Image
	text *canvas.Text
}

func (r *cardLineRenderer) Layout(s fyne.Size) {
	r.bg.Resize(s)
	cy := s.Height / 2
	r.ring.Move(fyne.NewPos(4, cy-6))
	r.ring.Resize(fyne.NewSize(12, 12))
	r.dot.Move(fyne.NewPos(7, cy-3))
	r.dot.Resize(fyne.NewSize(6, 6))
	if r.icon != nil {
		r.icon.Move(fyne.NewPos(2, cy-8))
		r.icon.Resize(fyne.NewSize(16, 16))
	}
	ts := r.text.MinSize()
	r.text.Move(fyne.NewPos(24, cy-ts.Height/2))
}
func (r *cardLineRenderer) MinSize() fyne.Size {
	return fyne.NewSize(r.text.MinSize().Width+30, 23)
}
func (r *cardLineRenderer) Refresh() {
	if r.l.hover {
		r.bg.FillColor = color.NRGBA{0x8A, 0x6A, 0x40, 0x22}
	} else {
		r.bg.FillColor = color.Transparent
	}
	r.text.TextStyle.Bold = r.l.radio && r.l.on
	r.ring.Hidden = !r.l.radio
	r.dot.Hidden = !(r.l.radio && r.l.on)
	r.bg.Refresh()
	r.ring.Refresh()
	r.dot.Refresh()
	r.text.Refresh()
	r.Layout(r.l.Size())
}
func (r *cardLineRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.bg, r.ring, r.dot}
	if r.icon != nil {
		objs = append(objs, r.icon)
	}
	return append(objs, r.text)
}
func (r *cardLineRenderer) Destroy() {}
