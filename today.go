package main

import (
	"fmt"
	"image/color"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The Today page: what a real organiser opens on. It's the first tab, and the left page shows
// the day like a diary page instead of a list:
//
//   - birthdays this week, with a WhatsApp or call to wish them well;
//   - people it's time to get in touch with (Keep in Touch), with call / message and "Done";
//   - people you met in the last two weeks (from the calendar log), with Add Note;
//   - cards waiting to be tidied, with Review.
//
// Clicking a person opens their card on the right page.

const grpToday = "today"

var iconToday = svgIcon("today", "M19 3h-1V1h-2v2H8V1H6v2H5c-1.11 0-1.99.9-1.99 2L3 19c0 1.1.89 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm0 16H5V8h14v11zM7 10h5v5H7z")

type metItem struct {
	c     *Contact
	when  time.Time
	title string
}

type todayData struct {
	bdays []*Contact
	due   []*Contact
	met   []metItem
	tidy  []*Contact
}

// count is what the Today tab shows: the people on today's page (as its heading does).
func (t todayData) count() int { return len(t.set()) }

// today works out what's on today's page.
func (u *ui) today(now time.Time) todayData {
	var t todayData
	for _, c := range u.store.Contacts {
		if d, _, ok := c.NextBirthday(now); ok && d <= 7 {
			t.bdays = append(t.bdays, c)
		}
		if c.KeepInTouch > 0 && u.dueToReachOut(c, now) {
			t.due = append(t.due, c)
		}
		if ms := u.eventsFor(c); len(ms) > 0 {
			for _, m := range ms { // newest first
				if m.Event.Start.After(now) {
					continue
				}
				if now.Sub(m.Event.Start) <= 14*24*time.Hour {
					t.met = append(t.met, metItem{c, m.Event.Start, m.Event.Title})
				}
				break
			}
		}
		if u.attention[c.ID] > 0 || (len(c.Phones) == 0 && len(c.Emails) == 0) {
			t.tidy = append(t.tidy, c)
		}
	}
	sort.SliceStable(t.bdays, func(i, j int) bool {
		a, _, _ := t.bdays[i].NextBirthday(now)
		b, _, _ := t.bdays[j].NextBirthday(now)
		return a < b
	})
	sort.SliceStable(t.due, func(i, j int) bool { return u.lastTalkedAt(t.due[i]).Before(u.lastTalkedAt(t.due[j])) })
	sort.SliceStable(t.met, func(i, j int) bool { return t.met[i].when.After(t.met[j].when) })
	return t
}

// todaySet is everyone on today's page (for the list's keyboard walking and selection).
func (t todayData) set() map[string]bool {
	m := map[string]bool{}
	for _, c := range t.bdays {
		m[c.ID] = true
	}
	for _, c := range t.due {
		m[c.ID] = true
	}
	for _, x := range t.met {
		m[x.c.ID] = true
	}
	return m
}

// buildToday fills the Today page.
func (u *ui) buildToday() {
	if u.todayBox == nil {
		return
	}
	now := time.Now()
	t := u.today(now)
	box := u.todayBox
	box.Objects = nil

	ink := theme.Color(theme.ColorNameForeground)
	date := u.printed(now.Format("Monday, 2 January"), 22, true, false, ink)
	year := u.printed(fmt.Sprint(now.Year()), 13, false, true, color.NRGBA{0x8A, 0x7E, 0x6C, 0xFF})
	box.Add(container.NewVBox(date, year))

	heading := func(s string, n int) {
		title := s
		if n > 0 {
			title = fmt.Sprintf("%s  ·  %d", s, n)
		}
		h := u.printed(title, 15, true, false, color.NRGBA{0x8A, 0x32, 0x1E, 0xFF})
		box.Add(container.New(&insetLayout{top: 12, bottom: 2}, h))
		r := canvas.NewRectangle(color.NRGBA{0xB0, 0x60, 0x50, 0x80})
		r.SetMinSize(fyne.NewSize(1, 1))
		box.Add(r)
	}
	none := func(s string) {
		l := newInkLine("", []string{s}, nil, nil, nil)
		l.faint = true
		box.Add(l)
	}
	cc := u.cfg.CountryCode
	reach := func(c *Contact) []fyne.CanvasObject {
		var out []fyne.CanvasObject
		seen := map[string]bool{}
		for _, a := range c.Actions(cc) {
			a := a
			if seen[a.Kind] {
				continue // one of each: call, message, WhatsApp
			}
			seen[a.Kind] = true
			switch a.Kind {
			case "call", "message", "whatsapp":
				b := widget.NewButtonWithIcon("", actionIcon(a.Kind), func() { u.open(c, a.Link) })
				b.Importance = widget.LowImportance
				out = append(out, b)
			}
		}
		return out
	}

	heading("Birthdays this week", len(t.bdays))
	if len(t.bdays) == 0 {
		none("No birthdays in the next seven days")
	}
	for _, c := range t.bdays {
		d, turns, _ := c.NextBirthday(now)
		when := map[int]string{0: "Today", 1: "Tomorrow"}[d]
		if when == "" {
			when = now.AddDate(0, 0, d).Format("Monday")
		}
		s := when
		if turns > 0 {
			s += fmt.Sprintf(" — turns %d", turns)
		}
		s += " · " + BirthdayText(c.Birthday)
		box.Add(u.todayRow(c, s, d == 0, reach(c)...))
	}

	heading("Time to get in touch", len(t.due))
	if len(t.due) == 0 {
		none("Nobody is due — Keep in Touch on a card sets a reminder")
	}
	for _, c := range t.due {
		c := c
		lt, how := u.lastTalked(c)
		s := "Never in touch here"
		if !lt.IsZero() {
			s = "Last talked " + Ago(lt, now)
			if how != "" {
				s += " · " + how
			}
		}
		s += " · " + keepText(c.KeepInTouch)
		done := widget.NewButtonWithIcon("Done", theme.ConfirmIcon(), func() { u.markTalked(c) })
		done.Importance = widget.LowImportance
		box.Add(u.todayRow(c, s, false, append(reach(c), done)...))
	}

	heading("Met recently", len(t.met))
	if len(t.met) == 0 {
		none("No meetings in the last two weeks in the calendar log")
	}
	for _, m := range t.met {
		c := m.c
		s := "Met " + Ago(m.when, now)
		if m.title != "" {
			s += " · " + m.title
		}
		note := widget.NewButtonWithIcon("Note", theme.DocumentCreateIcon(), func() { u.addNote(c) })
		note.Importance = widget.LowImportance
		box.Add(u.todayRow(c, s, false, note))
	}

	heading("Cards to tidy", len(t.tidy))
	if len(t.tidy) == 0 {
		none("Every card is in order")
	}
	for i, c := range t.tidy {
		if i == 5 {
			more := widget.NewButton(fmt.Sprintf("See all %d…", len(t.tidy)), func() {
				if u.groupExists(grpAttention) {
					u.showGroup(grpAttention)
				} else {
					u.showGroup(grpIncomplete)
				}
			})
			more.Importance = widget.LowImportance
			box.Add(container.NewHBox(layout.NewSpacer(), more))
			break
		}
		c := c
		s := "Missing a phone number and an email"
		if n := u.attention[c.ID]; n > 0 {
			s = plural(n, "suggestion") + " to tidy"
		}
		review := widget.NewButtonWithIcon("Review", iconBroom, func() { u.cleanupFor([]*Contact{c}, c.DisplayName()) })
		review.Importance = widget.LowImportance
		box.Add(u.todayRow(c, s, false, review))
	}
	box.Refresh()
}

// todayRow is one person on the Today page: picture, name, why they're here, and actions.
// Clicking the row opens their card.
func (u *ui) todayRow(c *Contact, why string, highlight bool, actions ...fyne.CanvasObject) fyne.CanvasObject {
	av := newAvatar(32)
	av.set(c, u.photo(c))
	name := widget.NewLabelWithStyle(c.DisplayName(), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	name.Truncation = fyne.TextTruncateEllipsis
	sub := widget.NewLabel(why)
	sub.Truncation = fyne.TextTruncateEllipsis
	sub.Importance = widget.LowImportance
	sub.SizeName = theme.SizeNameCaptionText
	if highlight {
		sub.Importance = widget.DangerImportance
	}
	texts := container.New(&tightVBox{}, name, sub)
	row := container.NewBorder(nil, nil, container.NewCenter(av), container.NewHBox(actions...), texts)
	id := c.ID
	return newTapRow(row, func() {
		u.selectedID = id
		u.backID = ""
		u.showDetail()
	})
}

// tapRow is a row that opens something when clicked, lit under the pointer.
type tapRow struct {
	widget.BaseWidget
	content fyne.CanvasObject
	onTap   func()
	hover   bool
	bg      *canvas.Rectangle
}

func newTapRow(content fyne.CanvasObject, onTap func()) *tapRow {
	r := &tapRow{content: content, onTap: onTap}
	r.ExtendBaseWidget(r)
	return r
}
func (r *tapRow) Tapped(*fyne.PointEvent)        { r.onTap() }
func (r *tapRow) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (r *tapRow) MouseIn(*desktop.MouseEvent)    { r.hover = true; r.Refresh() }
func (r *tapRow) MouseMoved(*desktop.MouseEvent) {}
func (r *tapRow) MouseOut()                      { r.hover = false; r.Refresh() }
func (r *tapRow) CreateRenderer() fyne.WidgetRenderer {
	r.bg = canvas.NewRectangle(color.Transparent)
	r.bg.CornerRadius = 4
	line := canvas.NewRectangle(ruleInk)
	return &tapRowRenderer{r: r, line: line}
}

type tapRowRenderer struct {
	r    *tapRow
	line *canvas.Rectangle
}

func (x *tapRowRenderer) Layout(s fyne.Size) {
	x.r.bg.Resize(s)
	x.r.content.Resize(s)
	x.line.Move(fyne.NewPos(4, s.Height-1))
	x.line.Resize(fyne.NewSize(s.Width-8, 1))
}
func (x *tapRowRenderer) MinSize() fyne.Size {
	return x.r.content.MinSize().Add(fyne.NewSize(0, 4))
}
func (x *tapRowRenderer) Refresh() {
	if x.r.hover {
		x.r.bg.FillColor = color.NRGBA{0x60, 0x50, 0x30, 0x14}
	} else {
		x.r.bg.FillColor = color.Transparent
	}
	x.r.bg.Refresh()
	x.Layout(x.r.Size())
}
func (x *tapRowRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{x.r.bg, x.line, x.r.content}
}
func (x *tapRowRenderer) Destroy() {}

// markTalked records that you got in touch today (from the Today page's Done).
func (u *ui) markTalked(c *Contact) {
	now := time.Now()
	c.LastContacted = now
	c.AddEvent(now, "Got in touch", "You")
	u.store.Put(c)
	u.save()
	u.flash("Marked " + c.DisplayName() + " as in touch today")
}
