package main

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const grpFilter = "filter"

// The filter form, opened by the funnel in the search field: name, company, country, city,
// where the card came from (LinkedIn, a spreadsheet…) and anything else (a number, an email, a
// word from the notes). Only people matching every filled-in field are shown, as you type, in
// whichever list is open.
type contactFilter struct {
	name, company, country, city, other *widget.Entry
	from                                *widget.SelectEntry
	box                                 *fyne.Container
}

func (u *ui) buildFilter() fyne.CanvasObject {
	f := &contactFilter{}
	mk := func(ph string) *widget.Entry {
		e := widget.NewEntry()
		e.SetPlaceHolder(ph)
		e.OnChanged = func(string) { u.refreshList() }
		return e
	}
	f.name = mk("Part of a name (Arabic or English)")
	f.company = mk("Company or job title")
	f.country = mk("Country (Kuwait, UK…) — address or phone code")
	f.city = mk("City or area")
	f.other = mk("Anything else: number, email, note…")
	f.from = widget.NewSelectEntry(u.store.SourceNames())
	f.from.SetPlaceHolder("LinkedIn, Spreadsheet, Business card…")
	f.from.OnChanged = func(string) { u.refreshList() }
	clear := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), func() {
		for _, e := range []*widget.Entry{f.name, f.company, f.country, f.city, f.other} {
			e.OnChanged = nil
			e.SetText("")
			e.OnChanged = func(string) { u.refreshList() }
		}
		f.from.OnChanged = nil
		f.from.SetText("")
		f.from.OnChanged = func(string) { u.refreshList() }
		u.refreshList()
	})
	clear.Importance = widget.LowImportance
	save := widget.NewButtonWithIcon("Save as Smart Group…", theme.SearchIcon(), func() {
		u.search.SetText(f.query())
		u.saveSmartGroup()
	})
	save.Importance = widget.LowImportance
	row := func(label string, e fyne.CanvasObject) fyne.CanvasObject {
		l := widget.NewLabel(label)
		l.Importance = widget.LowImportance
		return container.NewBorder(nil, nil, container.New(&fixedWidth{78}, l), nil, e)
	}
	f.box = container.NewVBox(
		row("Name", f.name), row("Company", f.company), row("Country", f.country),
		row("City", f.city), row("From", f.from), row("Other", f.other),
		container.NewHBox(clear, save),
		widget.NewSeparator(),
	)
	f.box.Hide()
	u.filter = f
	return f.box
}

// empty reports whether nothing is filled in.
func (f *contactFilter) empty() bool {
	return f == nil || strings.TrimSpace(f.name.Text+f.company.Text+f.country.Text+f.city.Text+f.other.Text+f.from.Text) == ""
}

// matches applies every filled-in field.
func (f *contactFilter) matches(c *Contact) bool {
	if f == nil {
		return true
	}
	if v := strings.TrimSpace(f.name.Text); v != "" {
		hay := c.FullName() + " " + c.AltName + " " + c.Nickname + " " + c.DisplayName()
		if !foldContains(hay, v) && !c.matchesNameSkeleton(fold(v)) {
			return false
		}
	}
	if v := strings.TrimSpace(f.company.Text); v != "" && !foldContains(c.Company+" "+c.Title, v) {
		return false
	}
	if v := strings.TrimSpace(f.country.Text); v != "" && !(searchFilter{key: "country", val: v}).test(c, nil, nil) {
		return false
	}
	if v := strings.TrimSpace(f.city.Text); v != "" {
		ok := false
		for _, a := range c.Addresses {
			if foldContains(a.City+" "+a.Region+" "+a.Street, v) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	if v := strings.TrimSpace(f.from.Text); v != "" && !(searchFilter{key: "source", val: v}).test(c, nil, nil) {
		return false
	}
	if v := strings.TrimSpace(f.other.Text); v != "" && !c.Matches(v) {
		return false
	}
	return true
}

// query turns the form into a search ("company:boubyan country:kuwait …") for a smart group.
func (f *contactFilter) query() string {
	var parts []string
	q := func(k, v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if strings.Contains(v, " ") {
			v = `"` + v + `"`
		}
		if k == "" {
			parts = append(parts, v)
		} else {
			parts = append(parts, k+":"+v)
		}
	}
	q("", f.name.Text)
	q("company", f.company.Text)
	q("country", f.country.Text)
	q("city", f.city.Text)
	q("from", f.from.Text)
	q("", f.other.Text)
	return strings.Join(parts, " ")
}

// showFilterForm shows the form while the funnel in the search field has it open.
func (u *ui) showFilterForm() {
	if u.filter == nil {
		return
	}
	if u.filterOpen {
		u.filter.from.SetOptions(u.store.SourceNames())
		if !u.filter.box.Visible() {
			u.filter.box.Show()
			u.win.Canvas().Focus(u.filter.name)
		}
	} else {
		u.filter.box.Hide()
	}
}
