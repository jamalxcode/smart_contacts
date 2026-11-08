package main

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Company pages: click the company on a card ("CTO · Gulf Bank") and see everyone you know
// there — their roles, when you last talked — and open any of them.

// peopleAt is everyone whose company is name (ignoring case and spacing), most recently in touch
// first.
func (u *ui) peopleAt(name string) []*Contact {
	key := fold(strings.TrimSpace(name))
	var out []*Contact
	for _, c := range u.store.Contacts {
		if key != "" && fold(strings.TrimSpace(c.Company)) == key {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return u.lastTalkedAt(out[i]).After(u.lastTalkedAt(out[j])) })
	return out
}

func (u *ui) companyDialog(name string) {
	people := u.peopleAt(name)
	if len(people) == 0 {
		return
	}
	var d *paperDialog
	box := container.NewVBox()
	intro := caption(fmt.Sprintf("%d %s you know at %s, most recently in touch first.", len(people), pluralWords(len(people), "person", "people"), name))
	intro.Wrapping = fyne.TextWrapWord
	box.Add(intro)
	now := time.Now()
	for _, c := range people {
		c := c
		why := c.Title
		if lt, how := u.lastTalked(c); !lt.IsZero() {
			s := "talked " + Ago(lt, now)
			if how != "" {
				s += " · " + how
			}
			why = strings.Trim(why+" · "+s, " ·")
		}
		if why == "" {
			why = "no job title"
		}
		box.Add(u.personRow(c, why, func() {
			d.Hide()
			u.openCard(c.ID)
		}))
	}
	d = newCustom(name, "Close", container.NewVScroll(box), u.win)
	d.clipboard = true
	d.Resize(fyne.NewSize(520, fyne.Min(560, float32(len(people))*64+170)))
	d.Show()
}

// personRow is a person in a list window: picture, name, a line about them; click to open.
func (u *ui) personRow(c *Contact, why string, open func()) fyne.CanvasObject {
	av := newAvatar(30)
	av.set(c, u.photo(c))
	name := widget.NewLabelWithStyle(c.DisplayName(), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	name.Truncation = fyne.TextTruncateEllipsis
	sub := caption(why)
	sub.Wrapping = fyne.TextWrapWord
	return newTapRow(container.NewBorder(nil, nil, container.NewCenter(av), nil, container.New(&tightVBox{}, name, sub)), open)
}

// openCard shows a person's card, on the list they're in (or All Contacts if they're not in it).
func (u *ui) openCard(id string) {
	u.leaveEditor(func() {
		if u.indexOf(id) < 0 && u.group != grpToday {
			u.search.SetText("")
			u.group = grpAll
			u.letter = ""
			u.refreshTabs()
			u.refreshList()
		}
		u.selectedID = id
		u.backID = ""
		u.reselect()
		if i := u.indexOf(id); i >= 0 {
			u.list.ScrollTo(i)
		}
		u.showDetail()
	}, nil)
}

// companyLink is the job line on a card, with the company underlined like a link when you point
// at it: clicking opens the company page.
type companyLink struct {
	widget.BaseWidget
	text  *canvas.Text
	under *canvas.Rectangle
	onTap func()
	hover bool
}

func newCompanyLink(text *canvas.Text, onTap func()) *companyLink {
	l := &companyLink{text: text, onTap: onTap}
	l.under = canvas.NewRectangle(color.Transparent)
	l.ExtendBaseWidget(l)
	return l
}
func (l *companyLink) Tapped(*fyne.PointEvent)        { l.onTap() }
func (l *companyLink) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (l *companyLink) MouseIn(*desktop.MouseEvent)    { l.hover = true; l.Refresh() }
func (l *companyLink) MouseMoved(*desktop.MouseEvent) {}
func (l *companyLink) MouseOut()                      { l.hover = false; l.Refresh() }
func (l *companyLink) CreateRenderer() fyne.WidgetRenderer {
	return &companyLinkRenderer{l: l}
}

type companyLinkRenderer struct{ l *companyLink }

func (r *companyLinkRenderer) Layout(s fyne.Size) {
	m := r.l.text.MinSize()
	r.l.text.Move(fyne.NewPos(0, (s.Height-m.Height)/2))
	r.l.text.Resize(m)
	r.l.under.Move(fyne.NewPos(0, (s.Height+m.Height)/2-1))
	r.l.under.Resize(fyne.NewSize(m.Width, 1))
}
func (r *companyLinkRenderer) MinSize() fyne.Size { return r.l.text.MinSize() }
func (r *companyLinkRenderer) Refresh() {
	if r.l.hover {
		r.l.under.FillColor = r.l.text.Color
	} else {
		r.l.under.FillColor = color.Transparent
	}
	r.l.under.Refresh()
	r.Layout(r.l.Size())
}
func (r *companyLinkRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.l.text, r.l.under}
}
func (r *companyLinkRenderer) Destroy() {}
