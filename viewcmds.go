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

// sortPill is one half of the First Name / Last Name switch on the organiser's cover.
type sortPill struct {
	widget.BaseWidget
	text  string
	on    bool
	hover bool
	onTap func()
}

func newSortPill(text string, onTap func()) *sortPill {
	p := &sortPill{text: text, onTap: onTap}
	p.ExtendBaseWidget(p)
	return p
}
func (p *sortPill) Tapped(*fyne.PointEvent)        { p.onTap() }
func (p *sortPill) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (p *sortPill) MouseIn(*desktop.MouseEvent)    { p.hover = true; p.Refresh() }
func (p *sortPill) MouseMoved(*desktop.MouseEvent) {}
func (p *sortPill) MouseOut()                      { p.hover = false; p.Refresh() }

func (p *sortPill) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = 9
	bg.StrokeWidth = 1
	bg.StrokeColor = color.NRGBA{0xF0, 0xD8, 0xB8, 0x70}
	t := canvas.NewText(p.text, color.White)
	t.TextSize = 10
	t.TextStyle = serif(p.text, true, false)
	t.Alignment = fyne.TextAlignCenter
	return &pillRenderer{p: p, bg: bg, t: t}
}

type pillRenderer struct {
	p  *sortPill
	bg *canvas.Rectangle
	t  *canvas.Text
}

func (r *pillRenderer) Layout(s fyne.Size) {
	r.bg.Resize(s)
	ts := r.t.MinSize()
	r.t.Move(fyne.NewPos(0, (s.Height-ts.Height)/2))
	r.t.Resize(fyne.NewSize(s.Width, ts.Height))
}
func (r *pillRenderer) MinSize() fyne.Size {
	return fyne.NewSize(r.t.MinSize().Width+8, 20)
}
func (r *pillRenderer) Refresh() {
	switch {
	case r.p.on:
		r.bg.FillColor = color.NRGBA{0xF0, 0xE2, 0xC6, 0xFF}
		r.t.Color = color.NRGBA{0x4A, 0x26, 0x10, 0xFF}
	case r.p.hover:
		r.bg.FillColor = color.NRGBA{0xFF, 0xF0, 0xD0, 0x26}
		r.t.Color = color.NRGBA{0xF6, 0xEA, 0xD4, 0xFF}
	default:
		r.bg.FillColor = color.Transparent
		r.t.Color = color.NRGBA{0xF0, 0xD8, 0xB8, 0xD0}
	}
	r.bg.Refresh()
	r.t.Refresh()
}
func (r *pillRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bg, r.t} }
func (r *pillRenderer) Destroy()                     {}

// organiserFooter is the bottom of the tab column: Sort, and Also in the Book.
func (u *ui) organiserFooter() fyne.CanvasObject {
	head := func(s string) fyne.CanvasObject {
		t := &indexTab{header: true, title: strings.ToUpper(s)}
		t.ExtendBaseWidget(t)
		return t
	}
	u.sortFirst = newSortPill("First Name", func() { u.setSort(false) })
	u.sortLast = newSortPill("Last Name", func() { u.setSort(true) })
	u.refreshSortFooter()
	// rows of two small pills, clear of the page edge the tabs tuck under
	row := func(a, b fyne.CanvasObject) fyne.CanvasObject {
		return container.New(&insetLayout{left: 8, right: 16, top: 2, bottom: 4}, container.NewGridWithColumns(2, a, b))
	}
	return container.NewVBox(
		head("Sort"),
		row(u.sortFirst, u.sortLast),
		head("Also in the Book"),
		row(newSortPill("Calendar Log", u.calendarDialog), newSortPill("Trash", u.trash)),
	)
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
