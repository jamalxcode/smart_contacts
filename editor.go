package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// editor edits a copy of a contact; nothing touches the store until Save.
type editor struct {
	u        *ui
	orig     *Contact // nil for a new contact
	c        *Contact // the working copy
	start    string   // JSON of the copy when editing began, to detect changes
	photo    []byte   // new photo chosen in this session
	card     []byte   // a business card image to keep with the contact on Save
	cardNote string   // "added this contact" / "updated this contact"
	photoSet bool     // photo was changed (set or removed)

	first, last, nick, company, title, birthday *widget.Entry
	alt                                         *widget.Entry
	notes                                       *widget.Entry
	phones, emails, urls                        *fieldRows
	addrs                                       *addrRows
	groups                                      *widget.CheckGroup
	newGroup                                    *widget.Entry
	fav                                         *widget.Check
	av                                          *avatar
	bdayHint                                    *widget.Label
}

// fieldRows is a growable list of label + value rows (phones, emails, links).
type fieldRows struct {
	box         *fyne.Container
	labels      []string
	placeholder string
	rows        []*fieldRow
}

type fieldRow struct {
	label *widget.SelectEntry
	value *widget.Entry
	obj   fyne.CanvasObject
}

func newFieldRows(fs []Field, labels []string, placeholder string) *fieldRows {
	r := &fieldRows{box: container.NewVBox(), labels: labels, placeholder: placeholder}
	for _, f := range fs {
		r.add(f)
	}
	return r
}

func (r *fieldRows) add(f Field) *fieldRow {
	row := &fieldRow{label: widget.NewSelectEntry(r.labels), value: widget.NewEntry()}
	if f.Label == "" {
		f.Label = r.labels[0]
	}
	row.label.SetText(f.Label)
	row.value.SetText(f.Value)
	row.value.SetPlaceHolder(r.placeholder)
	del := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), nil)
	del.Importance = widget.LowImportance
	lbl := container.NewGridWrap(fyne.NewSize(130, row.label.MinSize().Height), row.label)
	row.obj = container.NewBorder(nil, nil, lbl, del, row.value)
	del.OnTapped = func() {
		for i, x := range r.rows {
			if x == row {
				r.rows = append(r.rows[:i], r.rows[i+1:]...)
				break
			}
		}
		r.box.Remove(row.obj)
	}
	r.rows = append(r.rows, row)
	r.box.Add(row.obj)
	return row
}

func (r *fieldRows) fields() []Field {
	var out []Field
	for _, x := range r.rows {
		out = append(out, Field{Label: strings.TrimSpace(x.label.Text), Value: strings.TrimSpace(x.value.Text)})
	}
	return out
}

func (r *fieldRows) section(addText string, w fyne.Window) fyne.CanvasObject {
	add := widget.NewButtonWithIcon(addText, theme.ContentAddIcon(), nil)
	add.Importance = widget.LowImportance
	add.Alignment = widget.ButtonAlignLeading
	add.OnTapped = func() {
		row := r.add(Field{})
		w.Canvas().Focus(row.value)
	}
	return container.NewVBox(r.box, container.NewHBox(add))
}

// addrRows is the same idea for postal addresses.
type addrRows struct {
	box  *fyne.Container
	rows []*addrRow
}

type addrRow struct {
	label                                   *widget.SelectEntry
	street, city, region, postcode, country *widget.Entry
	obj                                     fyne.CanvasObject
}

func (r *addrRows) add(a Address) *addrRow {
	ent := func(v, ph string) *widget.Entry {
		e := widget.NewEntry()
		e.SetText(v)
		e.SetPlaceHolder(ph)
		return e
	}
	row := &addrRow{label: widget.NewSelectEntry(AddressLabels)}
	if a.Label == "" {
		a.Label = "Home"
	}
	row.label.SetText(a.Label)
	row.street = widget.NewMultiLineEntry()
	row.street.SetText(a.Street)
	row.street.SetPlaceHolder("Street, block, building")
	row.street.SetMinRowsVisible(2)
	row.city, row.region = ent(a.City, "City / area"), ent(a.Region, "Governorate / state")
	row.postcode, row.country = ent(a.Postcode, "Postcode"), ent(a.Country, "Country")
	del := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), nil)
	del.Importance = widget.LowImportance
	lbl := container.NewGridWrap(fyne.NewSize(130, row.label.MinSize().Height), row.label)
	fields := container.NewVBox(row.street, container.NewGridWithColumns(2, row.city, row.region), container.NewGridWithColumns(2, row.postcode, row.country))
	row.obj = container.NewBorder(nil, nil, container.NewVBox(lbl), container.NewVBox(del), fields)
	del.OnTapped = func() {
		for i, x := range r.rows {
			if x == row {
				r.rows = append(r.rows[:i], r.rows[i+1:]...)
				break
			}
		}
		r.box.Remove(row.obj)
	}
	r.rows = append(r.rows, row)
	r.box.Add(row.obj)
	return row
}

func (r *addrRows) addresses() []Address {
	var out []Address
	for _, x := range r.rows {
		out = append(out, Address{Label: x.label.Text, Street: x.street.Text, City: x.city.Text,
			Region: x.region.Text, Postcode: x.postcode.Text, Country: x.country.Text})
	}
	return out
}

// editContact opens the editor for c, or for a new contact when c is nil.
func (u *ui) editContact(c *Contact) {
	u.leaveEditor(func() { u.openEditor(c, nil, "") }, nil)
}

// openEditor shows the editor. prefill (from Smart Add) starts a new contact with values filled in;
// banner is an optional note shown at the top.
func (u *ui) openEditor(c *Contact, prefill *Contact, banner string) {
	u.openEditorCard(c, prefill, banner, nil, "")
}

// openEditorCard is openEditor with a business card image to keep on Save. With both c and
// prefill set, it edits c starting from prefill (c with new details merged in).
func (u *ui) openEditorCard(c *Contact, prefill *Contact, banner string, card []byte, cardNote string) {
	e := &editor{u: u, orig: c, card: card, cardNote: cardNote}
	switch {
	case c != nil && prefill != nil:
		e.c = prefill
	case c != nil:
		e.c = c.Clone()
	case prefill != nil:
		e.c = prefill
	default:
		e.c = NewContact()
		if strings.HasPrefix(u.group, "g:") { // a new contact made inside a group joins it
			e.c.Groups = []string{u.group[2:]}
		}
		if u.group == grpFavorites {
			e.c.Favorite = true
		}
	}
	u.ed = e
	u.right.Objects = []fyne.CanvasObject{e.build(banner)}
	u.right.Refresh()
	e.start = e.snapshot()
	u.win.Canvas().Focus(e.first)
	if c == nil { // a new contact isn't any of the ones in the list
		u.list.UnselectAll()
	}
}

func (e *editor) build(banner string) fyne.CanvasObject {
	c := e.c
	ent := func(v, ph string) *widget.Entry {
		x := widget.NewEntry()
		x.SetText(v)
		x.SetPlaceHolder(ph)
		return x
	}
	e.first, e.last = ent(c.First, "First name"), ent(c.Last, "Last name")
	e.nick = ent(c.Nickname, "Nickname")
	e.alt = ent(c.AltName, "Name in other language (Arabic ↔ English)")
	e.company, e.title = ent(c.Company, "Company"), ent(c.Title, "Job title")
	e.birthday = ent(c.Birthday, "e.g. 1990-03-14, 14/03/1990 or 14/03")
	e.bdayHint = caption("")
	e.birthday.OnChanged = func(s string) {
		if strings.TrimSpace(s) == "" {
			e.bdayHint.SetText("")
			return
		}
		if _, ok := CanonicalBirthday(s); ok {
			e.bdayHint.SetText(BirthdayText(s))
		} else {
			e.bdayHint.SetText("Not a date I understand yet")
		}
	}
	e.birthday.OnChanged(c.Birthday)
	e.notes = widget.NewMultiLineEntry()
	e.notes.SetText(c.Notes)
	e.notes.Wrapping = fyne.TextWrapWord
	e.notes.SetMinRowsVisible(4)
	e.notes.SetPlaceHolder("Where you met, family names, anything worth remembering")

	e.phones = newFieldRows(c.Phones, PhoneLabels, "+965 9999 8888")
	e.emails = newFieldRows(c.Emails, EmailLabels, "name@example.com")
	e.urls = newFieldRows(c.URLs, URLLabels, "https://…")
	if len(c.Phones) == 0 {
		e.phones.add(Field{})
	}
	if len(c.Emails) == 0 {
		e.emails.add(Field{})
	}
	e.addrs = &addrRows{box: container.NewVBox()}
	for _, a := range c.Addresses {
		e.addrs.add(a)
	}

	allGroups := e.u.store.Groups()
	for _, g := range c.Groups { // groups that only exist on this card (from Smart Add)
		found := false
		for _, x := range allGroups {
			found = found || strings.EqualFold(x, g)
		}
		if !found {
			allGroups = append(allGroups, g)
		}
	}
	e.groups = widget.NewCheckGroup(allGroups, nil)
	e.groups.Horizontal = false // one per line: no sideways scroller to trap the mouse wheel
	e.groups.SetSelected(c.Groups)
	e.newGroup = widget.NewEntry()
	e.newGroup.SetPlaceHolder("New group name")
	addGroup := func() {
		g := strings.TrimSpace(e.newGroup.Text)
		if g == "" {
			return
		}
		for _, x := range e.groups.Options {
			if strings.EqualFold(x, g) {
				g = x
			}
		}
		if !contains(e.groups.Options, g) {
			e.groups.Append(g)
		}
		sel := append([]string(nil), e.groups.Selected...)
		if !contains(sel, g) {
			e.groups.SetSelected(append(sel, g))
		}
		e.newGroup.SetText("")
	}
	e.newGroup.OnSubmitted = func(string) { addGroup() }
	groupAdd := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), addGroup)

	e.fav = widget.NewCheck("Favorite", nil)
	e.fav.SetChecked(c.Favorite)

	// photo
	e.av = newAvatar(84)
	e.av.set(c, e.u.photo(c))
	choose := widget.NewButtonWithIcon("Photo", theme.FileImageIcon(), e.choosePhoto)
	choose.Importance = widget.LowImportance
	remove := widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
		e.photo, e.photoSet = nil, true
		e.av.set(e.c, nil)
	})
	remove.Importance = widget.LowImportance

	suggest := widget.NewButtonWithIcon("Suggest", iconSparkle, nil)
	suggest.Importance = widget.LowImportance
	suggest.OnTapped = func() {
		full := strings.TrimSpace(e.first.Text + " " + e.last.Text)
		if s := SuggestOtherName(full); s != "" {
			e.alt.SetText(s)
			return
		}
		e.u.flash("No suggestion for “" + full + "” — type the other spelling yourself")
	}
	names := container.NewVBox(
		container.NewGridWithColumns(2, e.first, e.last),
		container.NewBorder(nil, nil, nil, suggest, e.alt),
		container.NewGridWithColumns(2, e.company, e.title),
		e.nick,
	)
	head := container.NewBorder(nil, nil, container.NewVBox(e.av, container.NewHBox(choose, remove)), nil, names)

	label := func(s string) fyne.CanvasObject {
		l := widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		l.Importance = widget.LowImportance
		l.SizeName = theme.SizeNameCaptionText
		return l
	}
	addAddr := widget.NewButtonWithIcon("add address", theme.ContentAddIcon(), func() { e.addrs.add(Address{}) })
	addAddr.Importance = widget.LowImportance

	body := container.NewVBox()
	if banner != "" {
		b := widget.NewLabel(banner)
		b.Wrapping = fyne.TextWrapWord
		b.Importance = widget.HighImportance
		body.Add(container.NewBorder(nil, nil, widget.NewIcon(iconSparkle), nil, b))
	}
	body.Add(head)
	body.Add(label("PHONE"))
	body.Add(e.phones.section("add phone", e.u.win))
	body.Add(label("EMAIL"))
	body.Add(e.emails.section("add email", e.u.win))
	body.Add(label("ADDRESS"))
	body.Add(container.NewVBox(e.addrs.box, container.NewHBox(addAddr)))
	body.Add(label("WEB & SOCIAL"))
	body.Add(e.urls.section("add link", e.u.win))
	body.Add(label("BIRTHDAY"))
	body.Add(container.NewBorder(nil, nil, nil, e.bdayHint, e.birthday))
	body.Add(label("GROUPS"))
	if len(allGroups) > 0 {
		body.Add(e.groups)
	}
	body.Add(container.NewBorder(nil, nil, nil, groupAdd, e.newGroup))
	body.Add(e.fav)
	body.Add(label("NOTES"))
	body.Add(e.notes)

	save := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), e.save)
	save.Importance = widget.HighImportance
	cancel := widget.NewButton("Cancel", e.cancel)
	title := "New Contact"
	if e.orig != nil {
		title = "Edit " + e.orig.DisplayName()
	}
	hint := caption("⌘S to save")
	bar := container.NewBorder(nil, nil, headline(title, 18, true), container.NewHBox(hint, cancel, save))
	return container.NewBorder(container.NewVBox(container.NewPadded(bar), widget.NewSeparator()), nil, nil, nil,
		container.NewVScroll(container.NewPadded(body)))
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// collect copies the form into e.c.
func (e *editor) collect() {
	c := e.c
	c.First, c.Last, c.Nickname, c.AltName = e.first.Text, e.last.Text, e.nick.Text, e.alt.Text
	c.Company, c.Title = e.company.Text, e.title.Text
	c.Phones, c.Emails, c.URLs = e.phones.fields(), e.emails.fields(), e.urls.fields()
	c.Addresses = e.addrs.addresses()
	c.Birthday = strings.TrimSpace(e.birthday.Text)
	if b, ok := CanonicalBirthday(c.Birthday); ok {
		c.Birthday = b
	}
	c.Notes = e.notes.Text
	c.Groups = append([]string(nil), e.groups.Selected...)
	if g := strings.TrimSpace(e.newGroup.Text); g != "" { // typed but not "Add"ed
		c.Groups = append(c.Groups, g)
	}
	c.Favorite = e.fav.Checked
}

func (e *editor) snapshot() string {
	e.collect()
	x := e.c.Clone()
	x.Tidy()
	b, _ := json.Marshal(x)
	return string(b)
}

func (e *editor) dirty() bool { return e.photoSet || e.card != nil || e.snapshot() != e.start }

func (e *editor) save() {
	e.collect()
	c := e.c
	if c.Birthday != "" {
		if _, ok := CanonicalBirthday(c.Birthday); !ok {
			dialog.ShowError(errors.New("the birthday isn't a date I understand — try 1990-03-14 or 14/03"), e.u.win)
			return
		}
	}
	c.Tidy()
	if c.DisplayName() == "No Name" {
		dialog.ShowInformation("Nothing to save", "Give the contact a name, a company, a phone number or an email.", e.u.win)
		return
	}
	if e.photoSet {
		if err := e.u.store.SetPhoto(c, e.photo); err != nil {
			dialog.ShowError(err, e.u.win)
			return
		}
		delete(e.u.photoCache, c.ID)
	}
	if e.card != nil {
		if err := e.u.store.AddCardImage(c, e.card, e.cardNote); err != nil {
			dialog.ShowError(err, e.u.win)
		}
		e.card = nil
	}
	isNew := e.orig == nil
	e.u.store.Put(c)
	e.u.ed = nil
	e.u.selectedID = c.ID
	e.u.save()
	// A brand-new contact may not be in the current group or search; show it anyway.
	if e.u.indexOf(c.ID) < 0 {
		e.u.search.SetText("")
		e.u.group = grpAll
		e.u.selectSideKey(grpAll)
		e.u.refreshList()
	}
	e.u.selectedID = c.ID
	e.u.reselect()
	if i := e.u.indexOf(c.ID); i >= 0 {
		e.u.list.ScrollTo(i)
	}
	e.u.showDetail()
	if isNew {
		e.u.flash("Added " + c.DisplayName())
		if dupes := e.u.lookalikes(c); len(dupes) > 0 {
			e.u.flash(fmt.Sprintf("Added %s — looks like %s, already in your contacts (Tools → Find Duplicates)", c.DisplayName(), dupes[0].DisplayName()))
		}
	}
}

func (e *editor) cancel() {
	e.u.leaveEditor(func() {}, nil)
}

// leaveEditor runs next once any edit in progress is saved-or-discarded.
// If the user chooses to keep editing, stay runs instead.
func (u *ui) leaveEditor(next func(), stay func()) {
	if u.ed == nil {
		next()
		return
	}
	finish := func() {
		u.ed = nil
		u.showDetail()
		u.reselect()
		next()
	}
	if !u.ed.dirty() {
		finish()
		return
	}
	dialog.ShowCustomConfirm("Discard changes?", "Discard", "Keep Editing",
		widget.NewLabel("You have unsaved changes to this contact."), func(discard bool) {
			if discard {
				finish()
			} else if stay != nil {
				stay()
			}
		}, u.win)
}

func (e *editor) choosePhoto() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil || r == nil {
			return
		}
		defer r.Close()
		b, err := io.ReadAll(io.LimitReader(r, 40<<20))
		if err != nil {
			dialog.ShowError(err, e.u.win)
			return
		}
		if !CanDecodeImage(b) {
			dialog.ShowError(errors.New("that picture couldn't be read — use a JPEG or PNG (iPhone HEIC photos: open in Preview and export as JPEG)"), e.u.win)
			return
		}
		small := ShrinkPhoto(b, 512)
		e.photo, e.photoSet = small, true
		e.av.set(e.c, fyne.NewStaticResource("new-photo.png", CirclePhoto(small, 256)))
	}, e.u.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".jpg", ".jpeg", ".png", ".JPG", ".JPEG", ".PNG"}))
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

// lookalikes finds other contacts that share a phone, email or exact name with c.
func (u *ui) lookalikes(c *Contact) []*Contact {
	if x := NewDupIndex(u.store.Contacts, u.cfg.CountryCode).Find(c); x != nil {
		return []*Contact{x}
	}
	return nil
}

var _ = layout.NewSpacer
