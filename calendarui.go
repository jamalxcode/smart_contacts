package main

import (
	"bytes"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// calendarDialog reads events from the Mac Calendar app or an .ics file, or forgets them.
func (u *ui) calendarDialog() {
	status := widget.NewLabel(u.calendarStatus())
	status.Wrapping = fyne.TextWrapWord
	period := widget.NewSelect([]string{"Last year", "Last 2 years", "Last 5 years", "Last 10 years"}, nil)
	period.SetSelected("Last 2 years")
	years := map[string]int{"Last year": 1, "Last 2 years": 2, "Last 5 years": 5, "Last 10 years": 10}
	var d *paperDialog
	readMac := widget.NewButtonWithIcon("Read Mac Calendar", theme.DownloadIcon(), func() {
		d.Hide()
		u.readMacCalendar(years[period.Selected] * 365)
	})
	readMac.Importance = widget.HighImportance
	ics := widget.NewButtonWithIcon("Import .ics File…", theme.FolderOpenIcon(), func() { d.Hide(); u.importFile() })
	forget := widget.NewButtonWithIcon("Forget Calendar Data", theme.DeleteIcon(), func() {
		ForgetCalendar(u.store.Dir())
		u.cal = LoadCalendar(u.store.Dir())
		u.calCache = nil
		status.SetText(u.calendarStatus())
		u.showDetail()
	})
	forget.Importance = widget.DangerImportance
	how := caption("Each contact's card then lists the meetings they were invited to (by their email or full name) or mentioned in " +
		"(their full name, in Arabic or English, in the title, place or notes). The Mac Calendar includes any Google, Outlook or iCloud " +
		"calendars added in System Settings → Internet Accounts. An .ics file can be exported from Google Calendar (Settings → Import & export). " +
		"Events are kept on this Mac only; repeating events appear once.")
	how.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(status, how, widget.NewSeparator(),
		container.NewBorder(nil, nil, widget.NewLabel("Period"), readMac, period),
		container.NewHBox(ics), widget.NewSeparator(), container.NewHBox(forget))
	d = newCustom("Calendar Log", "Close", content, u.win)
	d.Resize(fyne.NewSize(620, 420))
	d.Show()
}

func (u *ui) calendarStatus() string {
	if len(u.cal.Events) == 0 {
		return "No calendar events read yet."
	}
	return fmt.Sprintf("%d events from %s, last read %s.", len(u.cal.Events), u.cal.From, Ago(u.cal.Read, time.Now()))
}

func (u *ui) readMacCalendar(daysBack int) {
	prog := newCustomWithoutButtons("Reading the Calendar app…", container.NewVBox(
		widget.NewLabel("If macOS asks, allow Smart Contacts to use Calendar. Large calendars can take a minute."),
		widget.NewProgressBarInfinite()), u.win)
	prog.Show()
	go func() {
		evs, err := ReadMacCalendar(daysBack, 180)
		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				showErr(err, u.win)
				return
			}
			u.saveEvents(evs, "Mac Calendar")
		})
	}()
}

func (u *ui) importICS(name string, data []byte) {
	evs, err := ParseICS(bytes.NewReader(data))
	if err != nil {
		showErr(err, u.win)
		return
	}
	u.saveEvents(evs, name)
}

func (u *ui) saveEvents(evs []CalEvent, from string) {
	u.cal.Merge(evs, from)
	u.calCache = nil
	if err := u.cal.Save(u.store.Dir()); err != nil {
		showErr(err, u.win)
		return
	}
	linked := 0
	for _, c := range u.store.Contacts {
		if len(u.cal.EventsFor(c)) > 0 {
			linked++
		}
	}
	u.showDetail()
	showInfo("Calendar read", fmt.Sprintf("%d events from %s. %d of your contacts appear in them — see the Calendar section on their cards.",
		len(evs), from, linked), u.win)
}

// calendarSection lists a contact's meetings for the card: upcoming first, then the latest past ones.
func (u *ui) calendarSection(c *Contact) []fyne.CanvasObject {
	ms := u.eventsFor(c)
	if len(ms) == 0 {
		return nil
	}
	now := time.Now()
	row := func(m Mention) fyne.CanvasObject {
		e := m.Event
		when := e.Start.Format("2 Jan 2006")
		if !e.AllDay {
			when += e.Start.Format(" · 15:04")
		}
		if e.Start.After(now) {
			when = "upcoming · " + when
		}
		title := e.Title
		if title == "" {
			title = "(no title)"
		}
		extra := []string{}
		if m.How == "mentioned" {
			extra = append(extra, "mentioned")
		}
		if e.Location != "" {
			extra = append(extra, e.Location)
		}
		if e.Calendar != "" {
			extra = append(extra, e.Calendar)
		}
		t := widget.NewLabel(title)
		t.Wrapping = fyne.TextWrapWord
		sub := caption(joinNonEmpty(extra, " · "))
		l := caption(when)
		l.Wrapping = fyne.TextWrapWord
		return container.NewBorder(nil, nil, container.NewGridWrap(fyne.NewSize(150, l.MinSize().Height), l), nil, container.NewVBox(t, sub))
	}
	var rows []fyne.CanvasObject
	var upcoming, past []Mention
	for _, m := range ms {
		if m.Event.Start.After(now) {
			upcoming = append([]Mention{m}, upcoming...) // soonest first
		} else {
			past = append(past, m)
		}
	}
	shown := 0
	for _, m := range append(upcoming, past...) {
		if shown == 6 {
			break
		}
		rows = append(rows, row(m))
		shown++
	}
	if len(ms) > shown {
		all := widget.NewButton(fmt.Sprintf("Show all %d", len(ms)), func() {
			box := container.NewVBox()
			for _, m := range append(upcoming, past...) {
				box.Add(row(m))
			}
			d := newCustom(c.DisplayName()+" — calendar", "Close", container.NewVScroll(box), u.win)
			d.Resize(fyne.NewSize(720, 560))
			d.Show()
		})
		all.Importance = widget.LowImportance
		rows = append(rows, container.NewHBox(all))
	}
	return rows
}

func joinNonEmpty(ss []string, sep string) string {
	out := ""
	for _, s := range ss {
		if s == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += s
	}
	return out
}
