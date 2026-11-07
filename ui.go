package main

import (
	"fmt"
	"image/color"
	"net/url"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Sidebar keys for the built-in smart groups; user groups are "g:<name>".
const (
	grpAll        = "all"
	grpFavorites  = "fav"
	grpBirthdays  = "bday"
	grpRecent     = "recent"
	grpIncomplete = "incomplete"
	grpHeader     = "-"
)

type sideItem struct {
	key   string
	title string
	icon  fyne.Resource
	count int
}

type ui struct {
	app   fyne.App
	win   fyne.Window
	store *Store
	cfg   *Config

	group      string
	filtered   []*Contact
	selectedID string

	side      *widget.List
	sideItems []sideItem
	list      *widget.List
	search    *searchEntry
	listTitle *widget.Label
	right     *fyne.Container
	status    *widget.Label

	ed         *editor // non-nil while a contact is being edited
	flashGen   int
	photoCache map[string]fyne.Resource
	sideBusy   bool
}

// searchEntry passes arrow keys, Return and Escape to the list so you can search and pick without the mouse.
type searchEntry struct {
	widget.Entry
	onKey func(*fyne.KeyEvent) bool
}

func newSearchEntry() *searchEntry {
	e := &searchEntry{}
	e.ExtendBaseWidget(e)
	return e
}

func (e *searchEntry) TypedKey(k *fyne.KeyEvent) {
	if e.onKey != nil && e.onKey(k) {
		return
	}
	e.Entry.TypedKey(k)
}

func newUI(a fyne.App, s *Store, cfg *Config) *ui {
	u := &ui{app: a, store: s, cfg: cfg, group: grpAll, photoCache: map[string]fyne.Resource{}}
	if cfg.LastGroup != "" {
		u.group = cfg.LastGroup
	}
	u.win = a.NewWindow("Smart Contacts")
	u.build()
	w, h := cfg.WindowW, cfg.WindowH
	if w < 700 || h < 450 {
		w, h = 1120, 720
	}
	u.win.Resize(fyne.NewSize(w, h))
	// ⌘Q from the app menu quits without going through the window's close button.
	a.Lifecycle().SetOnStopped(u.saveWindow)
	u.win.SetCloseIntercept(func() {
		if u.ed != nil && u.ed.dirty() {
			dialog.ShowConfirm("Unsaved changes", "Quit without saving the contact you're editing?", func(ok bool) {
				if ok {
					u.quit()
				}
			}, u.win)
			return
		}
		u.quit()
	})
	return u
}

func (u *ui) quit() {
	u.saveWindow()
	u.app.Quit()
}

func (u *ui) saveWindow() {
	if sz := u.win.Canvas().Size(); sz.Width > 0 && sz.Height > 0 {
		u.cfg.WindowW, u.cfg.WindowH = sz.Width, sz.Height
	}
	_ = u.cfg.Save(u.store.Dir())
}

func (u *ui) build() {
	// --- sidebar ---
	u.side = widget.NewList(
		func() int { return len(u.sideItems) },
		func() fyne.CanvasObject {
			ic := widget.NewIcon(theme.AccountIcon())
			name := widget.NewLabel("Group name")
			name.Truncation = fyne.TextTruncateEllipsis
			n := widget.NewLabel("000")
			n.Importance = widget.LowImportance
			head := widget.NewLabelWithStyle("GROUPS", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			head.Importance = widget.LowImportance
			return container.NewStack(container.NewBorder(nil, nil, ic, n, name), head)
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			it := u.sideItems[id]
			st := o.(*fyne.Container)
			row, head := st.Objects[0].(*fyne.Container), st.Objects[1].(*widget.Label)
			if it.key == grpHeader {
				row.Hide()
				head.SetText(it.title)
				head.Show()
				return
			}
			head.Hide()
			row.Show()
			row.Objects[0].(*widget.Label).SetText(it.title)
			row.Objects[1].(*widget.Icon).SetResource(it.icon)
			cnt := ""
			if it.count > 0 {
				cnt = fmt.Sprint(it.count)
			}
			row.Objects[2].(*widget.Label).SetText(cnt)
		})
	u.side.OnSelected = func(id widget.ListItemID) {
		if u.sideBusy || id >= len(u.sideItems) {
			return
		}
		it := u.sideItems[id]
		if it.key == grpHeader {
			u.side.Unselect(id)
			u.selectSideKey(u.group)
			return
		}
		u.group = it.key
		u.cfg.LastGroup = it.key
		u.refreshList()
		u.win.Canvas().Focus(u.search)
	}
	addGroup := widget.NewButtonWithIcon("Group", theme.ContentAddIcon(), u.newGroupDialog)
	addGroup.Importance = widget.LowImportance
	groupMenu := widget.NewButtonWithIcon("", theme.MoreHorizontalIcon(), nil)
	groupMenu.Importance = widget.LowImportance
	groupMenu.OnTapped = func() { u.groupMenu(groupMenu) }
	sidebar := container.NewBorder(nil, container.NewHBox(addGroup, layout.NewSpacer(), groupMenu), nil, nil, u.side)

	// --- list ---
	u.search = newSearchEntry()
	u.search.SetPlaceHolder("Search names, numbers, companies, notes…")
	u.search.ActionItem = widget.NewIcon(theme.SearchIcon())
	u.search.OnChanged = func(string) { u.refreshList() }
	u.search.onKey = u.searchKey
	u.list = widget.NewList(
		func() int { return len(u.filtered) },
		func() fyne.CanvasObject {
			name := widget.NewLabelWithStyle("Contact name", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			name.Truncation = fyne.TextTruncateEllipsis
			sub := widget.NewLabel("subtitle")
			sub.Truncation = fyne.TextTruncateEllipsis
			sub.Importance = widget.LowImportance
			sub.SizeName = theme.SizeNameCaptionText
			star := widget.NewIcon(iconStar)
			texts := container.New(&tightVBox{}, name, sub)
			return container.NewBorder(nil, nil, newAvatar(38), star, texts)
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id >= len(u.filtered) {
				return
			}
			c := u.filtered[id]
			b := o.(*fyne.Container)
			texts := b.Objects[0].(*fyne.Container)
			texts.Objects[0].(*widget.Label).SetText(c.DisplayName())
			texts.Objects[1].(*widget.Label).SetText(u.subtitleFor(c))
			b.Objects[1].(*avatar).set(c, u.photo(c))
			star := b.Objects[2].(*widget.Icon)
			if c.Favorite {
				star.Show()
			} else {
				star.Hide()
			}
		})
	u.list.OnSelected = func(id widget.ListItemID) {
		if id >= len(u.filtered) {
			return
		}
		c := u.filtered[id]
		if c.ID == u.selectedID && u.ed == nil {
			return
		}
		u.leaveEditor(func() {
			u.selectedID = c.ID
			u.showDetail()
		}, func() { u.reselect() })
	}
	newBtn := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() { u.editContact(nil) })
	smartBtn := widget.NewButtonWithIcon("Smart Add", iconSparkle, u.smartAdd)
	u.listTitle = widget.NewLabelWithStyle("All Contacts", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	listTop := container.NewVBox(
		container.NewBorder(nil, nil, nil, container.NewHBox(smartBtn, newBtn), u.listTitle),
		u.search,
	)
	listPane := container.NewBorder(listTop, nil, nil, nil, u.list)

	// --- right ---
	u.right = container.NewStack()
	u.status = widget.NewLabel("")
	u.status.Importance = widget.LowImportance
	u.status.SizeName = theme.SizeNameCaptionText

	inner := container.NewHSplit(listPane, u.right)
	inner.Offset = 0.40
	outer := container.NewHSplit(sidebar, inner)
	outer.Offset = 0.19
	u.win.SetContent(container.NewBorder(nil, u.status, nil, nil, outer))
	u.win.SetMainMenu(u.mainMenu())

	u.refreshAll()
	u.win.Canvas().Focus(u.search)
}

// tightVBox stacks the name and subtitle with less padding than a VBox.
type tightVBox struct{}

func (t *tightVBox) MinSize(objs []fyne.CanvasObject) fyne.Size {
	w, h := float32(0), float32(0)
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		w = fyne.Max(w, m.Width)
		h += m.Height
	}
	return fyne.NewSize(w, h-2*theme.Padding()*float32(len(objs)-1))
}

func (t *tightVBox) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	total := t.MinSize(objs).Height
	y := (size.Height - total) / 2
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(size.Width, m.Height))
		y += m.Height - 2*theme.Padding()
	}
}

func (u *ui) subtitleFor(c *Contact) string {
	if u.group == grpBirthdays {
		d, turns, _ := c.NextBirthday(time.Now())
		when := map[int]string{0: "today", 1: "tomorrow"}[d]
		if when == "" {
			when = fmt.Sprintf("in %d days", d)
		}
		if turns > 0 {
			return fmt.Sprintf("Turns %d %s · %s", turns, when, BirthdayText(c.Birthday))
		}
		return "Birthday " + when + " · " + BirthdayText(c.Birthday)
	}
	return c.Subtitle()
}

// ---------- data → screen ----------

func (u *ui) refreshAll() {
	u.refreshSidebar()
	u.refreshList()
}

func (u *ui) refreshSidebar() {
	now := time.Now()
	fav, bday, recent, incomplete := 0, 0, 0, 0
	for _, c := range u.store.Contacts {
		if c.Favorite {
			fav++
		}
		if d, _, ok := c.NextBirthday(now); ok && d <= 30 {
			bday++
		}
		if now.Sub(c.Created) < 30*24*time.Hour {
			recent++
		}
		if len(c.Phones) == 0 && len(c.Emails) == 0 {
			incomplete++
		}
	}
	items := []sideItem{
		{grpAll, "All Contacts", theme.AccountIcon(), len(u.store.Contacts)},
		{grpFavorites, "Favorites", iconStar, fav},
		{grpBirthdays, "Birthdays · 30 days", iconCake, bday},
		{grpRecent, "Recently Added", theme.HistoryIcon(), recent},
	}
	if incomplete > 0 {
		items = append(items, sideItem{grpIncomplete, "No phone or email", theme.WarningIcon(), incomplete})
	}
	groups := u.store.Groups()
	if len(groups) > 0 {
		items = append(items, sideItem{key: grpHeader, title: "GROUPS"})
	}
	for _, g := range groups {
		n := 0
		for _, c := range u.store.Contacts {
			if c.HasGroup(g) {
				n++
			}
		}
		items = append(items, sideItem{"g:" + g, g, iconLabel, n})
	}
	u.sideItems = items
	u.side.Refresh()
	if !u.groupExists(u.group) {
		u.group = grpAll
	}
	u.selectSideKey(u.group)
	u.status.SetText(u.statusText())
}

func (u *ui) groupExists(key string) bool {
	for _, it := range u.sideItems {
		if it.key == key {
			return true
		}
	}
	return false
}

func (u *ui) selectSideKey(key string) {
	for i, it := range u.sideItems {
		if it.key == key {
			u.sideBusy = true
			u.side.Select(i)
			u.sideBusy = false
			return
		}
	}
}

func (u *ui) groupTitle() string {
	for _, it := range u.sideItems {
		if it.key == u.group {
			return it.title
		}
	}
	return "All Contacts"
}

func (u *ui) inGroup(c *Contact, now time.Time) bool {
	switch {
	case u.group == grpAll:
		return true
	case u.group == grpFavorites:
		return c.Favorite
	case u.group == grpBirthdays:
		d, _, ok := c.NextBirthday(now)
		return ok && d <= 30
	case u.group == grpRecent:
		return now.Sub(c.Created) < 30*24*time.Hour
	case u.group == grpIncomplete:
		return len(c.Phones) == 0 && len(c.Emails) == 0
	case strings.HasPrefix(u.group, "g:"):
		return c.HasGroup(u.group[2:])
	}
	return true
}

func (u *ui) refreshList() {
	now := time.Now()
	q := u.search.Text
	out := make([]*Contact, 0, len(u.store.Contacts))
	for _, c := range u.store.Contacts {
		if u.inGroup(c, now) && c.Matches(q) {
			out = append(out, c)
		}
	}
	switch u.group {
	case grpBirthdays:
		sort.SliceStable(out, func(i, j int) bool {
			a, _, _ := out[i].NextBirthday(now)
			b, _, _ := out[j].NextBirthday(now)
			return a < b
		})
	case grpRecent:
		sort.SliceStable(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	default:
		SortContacts(out, u.cfg.SortByLast)
	}
	u.filtered = out
	title := u.groupTitle()
	if q != "" {
		title = fmt.Sprintf("%s — %d found", title, len(out))
	} else {
		title = fmt.Sprintf("%s (%d)", title, len(out))
	}
	u.listTitle.SetText(title)
	u.list.UnselectAll()
	u.list.Refresh()
	if u.ed != nil {
		return
	}
	// Keep the shown contact if it is still in the list, otherwise show the first one.
	if u.indexOf(u.selectedID) < 0 {
		u.selectedID = ""
		if len(out) > 0 {
			u.selectedID = out[0].ID
		}
	}
	u.reselect()
	u.showDetail()
}

func (u *ui) indexOf(id string) int {
	for i, c := range u.filtered {
		if c.ID == id {
			return i
		}
	}
	return -1
}

// reselect highlights the current contact in the list without re-triggering OnSelected work.
func (u *ui) reselect() {
	i := u.indexOf(u.selectedID)
	if i < 0 {
		u.list.UnselectAll()
		return
	}
	cb := u.list.OnSelected
	u.list.OnSelected = nil
	u.list.Select(i)
	u.list.OnSelected = cb
}

func (u *ui) searchKey(k *fyne.KeyEvent) bool {
	move := func(d int) {
		if len(u.filtered) == 0 {
			return
		}
		i := u.indexOf(u.selectedID) + d
		if i < 0 {
			i = 0
		}
		if i >= len(u.filtered) {
			i = len(u.filtered) - 1
		}
		u.list.Select(i)
		u.list.ScrollTo(i)
	}
	switch k.Name {
	case fyne.KeyDown:
		move(1)
	case fyne.KeyUp:
		move(-1)
	case fyne.KeyReturn, fyne.KeyEnter:
		if c := u.store.Get(u.selectedID); c != nil && u.ed == nil {
			u.editContact(c)
		}
	case fyne.KeyEscape:
		if u.search.Text != "" {
			u.search.SetText("")
		}
	default:
		return false
	}
	return true
}

// save writes the store and redraws everything, keeping the selection.
func (u *ui) save() {
	if err := u.store.Save(); err != nil {
		dialog.ShowError(fmt.Errorf("couldn't save your contacts: %w", err), u.win)
	}
	u.refreshAll()
}

func (u *ui) photo(c *Contact) fyne.Resource {
	if !c.HasPhoto {
		return nil
	}
	if r, ok := u.photoCache[c.ID]; ok {
		return r
	}
	b := u.store.Photo(c)
	var r fyne.Resource
	if len(b) > 0 {
		r = fyne.NewStaticResource(c.ID+".png", CirclePhoto(b, 256))
	}
	u.photoCache[c.ID] = r
	return r
}

// ---------- detail view ----------

func (u *ui) showDetail() {
	if u.ed != nil {
		return
	}
	c := u.store.Get(u.selectedID)
	if c == nil {
		u.right.Objects = []fyne.CanvasObject{u.emptyState()}
		u.right.Refresh()
		return
	}
	u.right.Objects = []fyne.CanvasObject{u.detailView(c)}
	u.right.Refresh()
}

func (u *ui) emptyState() fyne.CanvasObject {
	if len(u.store.Contacts) > 0 {
		msg := "No contact selected"
		if q := strings.TrimSpace(u.search.Text); q != "" && len(u.filtered) == 0 {
			msg = "Nobody matches “" + q + "”"
			if u.group != grpAll {
				msg += " in " + u.groupTitle()
			}
		}
		l := widget.NewLabel(msg)
		l.Importance = widget.LowImportance
		return container.NewCenter(l)
	}
	title := canvas.NewText("Welcome to Smart Contacts", theme.Color(theme.ColorNameForeground))
	title.TextSize = 22
	title.TextStyle.Bold = true
	msg := widget.NewLabel("Bring your contacts in, or add the first one.")
	msg.Alignment = fyne.TextAlignCenter
	imp := widget.NewButtonWithIcon("Import from Mac Contacts", theme.DownloadIcon(), u.importMac)
	imp.Importance = widget.HighImportance
	vcf := widget.NewButtonWithIcon("Import a .vcf file…", theme.FolderOpenIcon(), u.importVCF)
	smart := widget.NewButtonWithIcon("Smart Add from text…", iconSparkle, u.smartAdd)
	add := widget.NewButtonWithIcon("New contact", theme.ContentAddIcon(), func() { u.editContact(nil) })
	return container.NewCenter(container.NewVBox(container.NewCenter(title), msg, imp, vcf, smart, add))
}

// big name text that follows the theme colour
func headline(s string, size float32, bold bool) *canvas.Text {
	t := canvas.NewText(s, theme.Color(theme.ColorNameForeground))
	t.TextSize = size
	t.TextStyle.Bold = bold
	return t
}

func caption(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Importance = widget.LowImportance
	l.SizeName = theme.SizeNameCaptionText
	return l
}

func (u *ui) detailView(c *Contact) fyne.CanvasObject {
	cc := u.cfg.CountryCode
	av := newAvatar(84)
	av.set(c, u.photo(c))

	nameBox := container.NewVBox(headline(c.DisplayName(), 24, true))
	if c.Nickname != "" && c.FullName() != "" {
		nameBox.Add(caption("“" + c.Nickname + "”"))
	}
	if s := strings.Join(nonEmpty([]string{c.Title, c.Company}), " · "); s != "" && !c.IsCompany() {
		nameBox.Add(widget.NewLabel(s))
	}
	favIcon := iconStarOff
	if c.Favorite {
		favIcon = iconStar
	}
	fav := widget.NewButtonWithIcon("", favIcon, func() {
		c.Favorite = !c.Favorite
		u.store.Put(c)
		u.save()
	})
	fav.Importance = widget.LowImportance
	edit := widget.NewButtonWithIcon("Edit", theme.DocumentCreateIcon(), func() { u.editContact(c) })
	head := container.NewBorder(nil, nil, av, container.NewVBox(container.NewHBox(fav, edit)), container.NewVBox(layout.NewSpacer(), container.NewPadded(nameBox), layout.NewSpacer()))

	// Quick actions use the first mobile (else first) number and the first email.
	actions := container.NewHBox()
	var phone *Field
	for i := range c.Phones {
		if phone == nil || strings.EqualFold(c.Phones[i].Label, "mobile") && !strings.EqualFold(phone.Label, "mobile") {
			phone = &c.Phones[i]
		}
	}
	if phone != nil {
		p := *phone
		actions.Add(u.actionButton("Call", iconPhone, c, "tel:"+NormalizePhone(p.Value, cc)))
		actions.Add(u.actionButton("Message", iconMessage, c, "sms:"+NormalizePhone(p.Value, cc)))
		if CanWhatsApp(p.Value, cc) {
			actions.Add(u.actionButton("WhatsApp", iconChat, c, "whatsapp://send?phone="+WhatsAppNumber(p.Value, cc)))
		}
	}
	if len(c.Emails) > 0 {
		actions.Add(u.actionButton("Email", theme.MailComposeIcon(), c, "mailto:"+c.Emails[0].Value))
	}

	body := container.NewVBox(head)
	if len(actions.Objects) > 0 {
		body.Add(actions)
	}
	body.Add(widget.NewSeparator())

	section := func(title string, rows ...fyne.CanvasObject) {
		if len(rows) == 0 {
			return
		}
		h := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		h.Importance = widget.LowImportance
		h.SizeName = theme.SizeNameCaptionText
		body.Add(h)
		for _, r := range rows {
			body.Add(r)
		}
	}

	var phones []fyne.CanvasObject
	for _, p := range c.Phones {
		p := p
		n := NormalizePhone(p.Value, cc)
		btns := []fyne.CanvasObject{u.iconLink(iconPhone, "Call", c, "tel:"+n)}
		if CanWhatsApp(p.Value, cc) {
			btns = append(btns, u.iconLink(iconChat, "WhatsApp", c, "whatsapp://send?phone="+WhatsAppNumber(p.Value, cc)))
		}
		btns = append(btns, u.copyButton(p.Value))
		phones = append(phones, u.valueRow(p.Label, p.Value, btns...))
	}
	section("PHONE", phones...)

	var emails []fyne.CanvasObject
	for _, e := range c.Emails {
		emails = append(emails, u.valueRow(e.Label, e.Value,
			u.iconLink(theme.MailComposeIcon(), "Email", c, "mailto:"+e.Value), u.copyButton(e.Value)))
	}
	section("EMAIL", emails...)

	var addrs []fyne.CanvasObject
	for _, a := range c.Addresses {
		one := a.OneLine()
		addrs = append(addrs, u.valueRow(a.Label, strings.TrimSpace(strings.Join(nonEmpty([]string{a.Street, strings.Join(nonEmpty([]string{a.Postcode, a.City}), " "), a.Region, a.Country}), "\n")),
			u.iconLink(iconPlace, "Map", nil, "maps://?q="+url.QueryEscape(one)), u.copyButton(one)))
	}
	section("ADDRESS", addrs...)

	var links []fyne.CanvasObject
	for _, l := range c.URLs {
		v := l.Value
		if !strings.Contains(v, "://") {
			v = "https://" + v
		}
		links = append(links, u.valueRow(l.Label, l.Value, u.iconLink(iconLink, "Open", nil, v), u.copyButton(l.Value)))
	}
	section("WEB", links...)

	if c.Birthday != "" {
		txt := BirthdayText(c.Birthday)
		if d, turns, ok := c.NextBirthday(time.Now()); ok {
			switch {
			case d == 0 && turns > 0:
				txt += fmt.Sprintf("   ·   turns %d today!", turns)
			case d == 0:
				txt += "   ·   today!"
			case turns > 0:
				txt += fmt.Sprintf("   ·   turns %d in %d days", turns, d)
			default:
				txt += fmt.Sprintf("   ·   in %d days", d)
			}
		}
		section("BIRTHDAY", container.NewBorder(nil, nil, widget.NewIcon(iconCake), nil, widget.NewLabel(txt)))
	}

	if len(c.Groups) > 0 {
		chips := container.NewHBox()
		for _, g := range c.Groups {
			g := g
			b := widget.NewButtonWithIcon(g, iconLabel, func() {
				u.group = "g:" + g
				u.cfg.LastGroup = u.group
				u.selectSideKey(u.group)
				u.search.SetText("") // refreshes the list
				u.refreshList()
			})
			b.Importance = widget.LowImportance
			chips.Add(b)
		}
		section("GROUPS", container.NewHScroll(chips))
	}

	section("CONNECTIONS", u.connectionsSection(c)...)

	if strings.TrimSpace(c.Notes) != "" {
		n := widget.NewLabel(c.Notes)
		n.Wrapping = fyne.TextWrapWord
		n.Selectable = true
		section("NOTES", n)
	}

	meta := []string{"Added " + c.Created.Format("2 Jan 2006")}
	if !c.LastContacted.IsZero() {
		meta = append(meta, "last contacted "+Ago(c.LastContacted, time.Now()))
	}
	body.Add(widget.NewSeparator())
	body.Add(caption(strings.Join(meta, " · ")))
	del := widget.NewButtonWithIcon("Delete Contact", theme.DeleteIcon(), func() { u.deleteContact(c) })
	del.Importance = widget.DangerImportance
	share := widget.NewButtonWithIcon("Copy to Apple Contacts", theme.UploadIcon(), func() { u.sendToMacContacts([]*Contact{c}) })
	share.Importance = widget.LowImportance
	exp := widget.NewButtonWithIcon("Save .vcf", theme.DocumentSaveIcon(), func() { u.exportVCF([]*Contact{c}, c.DisplayName()) })
	exp.Importance = widget.LowImportance
	body.Add(container.NewHBox(share, exp, layout.NewSpacer(), del))
	return container.NewVScroll(container.NewPadded(body))
}

func (u *ui) valueRow(label, value string, buttons ...fyne.CanvasObject) fyne.CanvasObject {
	l := widget.NewLabel(label)
	l.Importance = widget.LowImportance
	l.SizeName = theme.SizeNameCaptionText
	l.Truncation = fyne.TextTruncateEllipsis
	v := widget.NewLabel(value)
	v.Selectable = true
	v.Wrapping = fyne.TextWrapWord
	lbl := container.NewGridWrap(fyne.NewSize(90, l.MinSize().Height), l)
	return container.NewBorder(nil, nil, lbl, container.NewHBox(buttons...), v)
}

func (u *ui) copyButton(v string) fyne.CanvasObject {
	b := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), nil)
	b.Importance = widget.LowImportance
	b.OnTapped = func() {
		u.app.Clipboard().SetContent(v)
		u.flash("Copied " + v)
	}
	return b
}

// iconLink opens a URL; when c is set it also records "last contacted".
func (u *ui) iconLink(icon fyne.Resource, _ string, c *Contact, link string) fyne.CanvasObject {
	b := widget.NewButtonWithIcon("", icon, func() { u.open(c, link) })
	b.Importance = widget.LowImportance
	return b
}

func (u *ui) actionButton(title string, icon fyne.Resource, c *Contact, link string) fyne.CanvasObject {
	b := widget.NewButtonWithIcon(title, icon, func() { u.open(c, link) })
	return b
}

func (u *ui) open(c *Contact, link string) {
	p, err := url.Parse(link)
	if err != nil {
		dialog.ShowError(err, u.win)
		return
	}
	if err := u.app.OpenURL(p); err != nil {
		if p.Scheme == "whatsapp" {
			err = fmt.Errorf("the WhatsApp app isn't installed on this Mac (get it from the App Store)")
		}
		dialog.ShowError(err, u.win)
		return
	}
	if c != nil {
		c.LastContacted = time.Now()
		if err := u.store.Save(); err != nil {
			dialog.ShowError(err, u.win)
		}
		if u.selectedID == c.ID && u.ed == nil {
			u.showDetail()
		}
	}
}

// flash shows a short message in the status line, then restores it.
func (u *ui) flash(msg string) {
	u.flashGen++
	gen := u.flashGen
	u.status.SetText(msg)
	go func() {
		time.Sleep(3 * time.Second)
		fyne.Do(func() {
			if gen == u.flashGen { // a newer message is showing; let it finish
				u.status.SetText(u.statusText())
			}
		})
	}()
}

func (u *ui) statusText() string {
	where := "private · stored only on this Mac"
	if u.cloudAI() {
		where = "stored only on this Mac · Cloud AI on for Smart Add"
	}
	return fmt.Sprintf("%d contacts   ·   %s", len(u.store.Contacts), where)
}

func (u *ui) deleteContact(c *Contact) {
	dialog.ShowConfirm("Delete "+c.DisplayName()+"?", "It goes to the Trash for 30 days (Tools → Trash).", func(ok bool) {
		if !ok {
			return
		}
		i := u.indexOf(c.ID)
		u.store.Delete(c.ID)
		// show the neighbour, not the top of the list
		u.selectedID = ""
		if i >= 0 && len(u.filtered) > 1 {
			if i+1 < len(u.filtered) {
				u.selectedID = u.filtered[i+1].ID
			} else {
				u.selectedID = u.filtered[i-1].ID
			}
		}
		u.save()
		u.flash("Moved " + c.DisplayName() + " to the Trash")
	}, u.win)
}

// ---------- avatar ----------

// avatar is a circle with initials, or the contact's photo cut to a circle.
type avatar struct {
	widget.BaseWidget
	size   float32
	circle *canvas.Circle
	text   *canvas.Text
	img    *canvas.Image
}

func newAvatar(size float32) *avatar {
	a := &avatar{size: size}
	a.circle = canvas.NewCircle(color.NRGBA{0x88, 0x88, 0x88, 0xFF})
	a.text = canvas.NewText("", color.White)
	a.text.TextStyle.Bold = true
	a.text.TextSize = size * 0.38
	a.text.Alignment = fyne.TextAlignCenter
	a.img = &canvas.Image{FillMode: canvas.ImageFillContain} // photos arrive already cut to a circle
	a.img.Hide()
	a.ExtendBaseWidget(a)
	return a
}

func (a *avatar) set(c *Contact, photo fyne.Resource) {
	a.circle.FillColor = avatarColor(c.ID)
	a.text.Text = c.Initials()
	if photo != nil {
		a.img.Resource = photo
		a.img.Show()
	} else {
		a.img.Resource = nil
		a.img.Hide()
	}
	a.Refresh()
}

func (a *avatar) MinSize() fyne.Size { return fyne.NewSize(a.size, a.size) }

func (a *avatar) CreateRenderer() fyne.WidgetRenderer {
	return &avatarRenderer{a}
}

type avatarRenderer struct{ a *avatar }

func (r *avatarRenderer) Layout(s fyne.Size) {
	d := r.a.size
	pos := fyne.NewPos((s.Width-d)/2, (s.Height-d)/2)
	r.a.circle.Move(pos)
	r.a.circle.Resize(fyne.NewSize(d, d))
	r.a.img.Move(pos)
	r.a.img.Resize(fyne.NewSize(d, d))
	ts := r.a.text.MinSize()
	r.a.text.Move(fyne.NewPos(pos.X, pos.Y+(d-ts.Height)/2))
	r.a.text.Resize(fyne.NewSize(d, ts.Height))
}
func (r *avatarRenderer) MinSize() fyne.Size { return r.a.MinSize() }
func (r *avatarRenderer) Refresh() {
	r.a.circle.Refresh()
	r.a.text.Refresh()
	r.a.img.Refresh()
	r.Layout(r.a.Size())
}
func (r *avatarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.a.circle, r.a.text, r.a.img}
}
func (r *avatarRenderer) Destroy() {}
