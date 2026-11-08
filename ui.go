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
	"fyne.io/fyne/v2/driver/desktop"
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

	ed          *editor // non-nil while a contact is being edited
	cal         *Calendar
	graph       *Graph               // cached connections graph (nil = rebuild)
	calCache    map[string][]Mention // cached calendar matches per contact
	openSec     map[string]bool      // which folding card sections are open
	selectMode  bool                 // list shows tick boxes for bulk actions
	picked      map[string]bool      // contacts ticked in Select mode
	bulkBar     *fyne.Container
	listPane    *fyne.Container
	flashGen    int
	photoCache  map[string]fyne.Resource
	sideBusy    bool
	act         *ActivityBook // when you were last in touch, from Messages, calls, Mail, WhatsApp
	actBusy     bool
	saveSearch  *widget.Button // "Save as Smart Group", shown while searching
	attention   map[string]int // clean-up suggestions per contact (worked out in the background)
	qf          *quickFind     // the menu bar's Quick Find window
	trayMenuSig string
	selAnchor   string // where a Shift-click range starts
	attnGen     int
}

// searchEntry passes arrow keys, Return and Escape to the list so you can search and pick without the mouse.
type searchEntry struct {
	widget.Entry
	onKey       func(*fyne.KeyEvent) bool
	onSelectAll func()
}

func newSearchEntry() *searchEntry {
	e := &searchEntry{}
	e.ExtendBaseWidget(e)
	return e
}

// TypedShortcut: ⌘A in the empty search field selects every card shown.
func (e *searchEntry) TypedShortcut(sc fyne.Shortcut) {
	if _, all := sc.(*fyne.ShortcutSelectAll); all && e.Text == "" && e.onSelectAll != nil {
		e.onSelectAll()
		return
	}
	e.Entry.TypedShortcut(sc)
}

func (e *searchEntry) TypedKey(k *fyne.KeyEvent) {
	if e.onKey != nil && e.onKey(k) {
		return
	}
	e.Entry.TypedKey(k)
}

func newUI(a fyne.App, s *Store, cfg *Config) *ui {
	u := &ui{app: a, store: s, cfg: cfg, group: grpAll, photoCache: map[string]fyne.Resource{}, cal: LoadCalendar(s.Dir())}
	if cfg.LastGroup != "" {
		u.group = cfg.LastGroup
	}
	u.act = LoadActivity(s.Dir())
	u.win = a.NewWindow("Smart Contacts")
	u.build()
	if cfg.AutoActivity {
		u.readActivity(false) // refresh "last talked" quietly in the background
	}
	u.remindLoop()
	u.checkAttention()
	u.setupMenuBar()
	w, h := cfg.WindowW, cfg.WindowH
	if w < 700 || h < 450 {
		w, h = 1120, 720
	}
	u.win.Resize(fyne.NewSize(w, h))
	u.win.SetOnDropped(u.importDropped) // drag .vcf / .csv / .xlsx / .txt files onto the window
	// ⌘Q from the app menu quits without going through the window's close button.
	u.win.SetCloseIntercept(func() {
		if u.ed != nil && u.ed.dirty() {
			u.leaveEditor(u.quit, nil) // Save / Don't Save / Cancel
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
			head := widget.NewLabelWithStyle("Groups", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			head.Importance = widget.LowImportance
			head.SizeName = theme.SizeNameCaptionText
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
	sidebar := container.NewStack(sidebarBackground(u.app),
		container.NewBorder(nil, container.NewHBox(addGroup, layout.NewSpacer(), groupMenu), nil, nil, u.side))

	// --- list ---
	u.search = newSearchEntry()
	u.search.SetPlaceHolder("Search")
	u.search.ActionItem = widget.NewIcon(theme.SearchIcon())
	u.search.OnChanged = func(string) { u.refreshList() }
	u.search.onKey = u.searchKey
	u.search.onSelectAll = u.selectAllCards
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
			tick := widget.NewIcon(theme.CheckButtonIcon()) // shown in Select mode
			tick.Hide()
			row := container.NewBorder(nil, nil, container.NewHBox(tick, newAvatar(34)), star, texts)
			var ca *contextArea
			ca = newContextArea(row, u.win, func() *fyne.Menu { return u.rowMenu(ca.data) })
			ca.onTap = func() {
				if u.clickWithKeys(ca.data, ca.mods) {
					return
				}
				if i := u.indexOf(ca.data); i >= 0 {
					if u.selectMode { // ticking: toggle even if the row is already highlighted
						u.list.Unselect(i)
					}
					u.list.Select(i)
				}
			}
			return ca
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id >= len(u.filtered) {
				return
			}
			c := u.filtered[id]
			ca := o.(*contextArea)
			ca.data = c.ID
			b := ca.content.(*fyne.Container)
			texts := b.Objects[0].(*fyne.Container)
			texts.Objects[0].(*widget.Label).SetText(c.DisplayName())
			texts.Objects[1].(*widget.Label).SetText(u.subtitleFor(c))
			left := b.Objects[1].(*fyne.Container)
			left.Objects[1].(*avatar).set(c, u.photo(c))
			tick := left.Objects[0].(*widget.Icon)
			if u.selectMode {
				tick.Show()
				if u.picked[c.ID] {
					tick.SetResource(theme.CheckButtonCheckedIcon())
				} else {
					tick.SetResource(theme.CheckButtonIcon())
				}
			} else {
				tick.Hide()
			}
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
		if u.selectMode { // in Select mode a click ticks or unticks
			u.picked[c.ID] = !u.picked[c.ID]
			if !u.picked[c.ID] {
				delete(u.picked, c.ID)
			}
			u.list.UnselectAll()
			u.list.RefreshItem(id)
			u.updateBulkBar()
			return
		}
		if c.ID == u.selectedID && (u.ed == nil || u.ed.live) {
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
	selBtn := widget.NewButtonWithIcon("", theme.CheckButtonCheckedIcon(), u.toggleSelectMode)
	u.saveSearch = widget.NewButtonWithIcon("Save as Smart Group…", theme.SearchIcon(), u.saveSmartGroup)
	u.saveSearch.Importance = widget.LowImportance
	u.saveSearch.Hide()
	listTop := container.NewVBox(
		container.NewBorder(nil, nil, nil, container.NewHBox(selBtn, smartBtn, newBtn), u.listTitle),
		u.search,
		u.saveSearch,
	)
	u.bulkBar = container.NewVBox()
	u.bulkBar.Hide()
	listPane := container.NewBorder(listTop, u.bulkBar, nil, nil, u.list)
	u.listPane = listPane

	// --- right ---
	u.right = container.NewStack()
	u.status = widget.NewLabel("")
	u.status.Importance = widget.LowImportance
	u.status.SizeName = theme.SizeNameCaptionText

	inner := container.NewHSplit(listPane, u.right)
	inner.Offset = 0.40
	outer := container.NewHSplit(sidebar, inner)
	outer.Offset = 0.19
	panes := container.NewThemeOverride(outer, &paneTheme{newAppTheme()})
	u.win.SetContent(container.NewBorder(nil, u.status, nil, nil, panes))
	u.win.SetMainMenu(u.mainMenu())
	u.win.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) { dialogKey(k) })

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
	if u.group == grpReachOut {
		lt, how := u.lastTalked(c)
		s := "Never in touch here"
		if !lt.IsZero() {
			s = "Last talked " + Ago(lt, time.Now())
			if how != "" {
				s += " · " + how
			}
		}
		return s + " · " + keepText(c.KeepInTouch)
	}
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

// invalidate drops cached lookups (connections graph, calendar matches) after data changes.
func (u *ui) invalidate() {
	u.graph = nil
	u.calCache = nil
}

// graphNow returns the connections graph, built once until the data changes.
func (u *ui) graphNow() *Graph {
	if u.graph == nil {
		u.graph = NewGraph(u.store, true)
	}
	return u.graph
}

// eventsFor returns a contact's calendar matches, worked out once until the data changes.
func (u *ui) eventsFor(c *Contact) []Mention {
	if u.calCache == nil {
		u.calCache = map[string][]Mention{}
	}
	if m, ok := u.calCache[c.ID]; ok {
		return m
	}
	m := u.cal.EventsFor(c)
	u.calCache[c.ID] = m
	return m
}

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
		{grpBirthdays, "Upcoming Birthdays", iconCake, bday},
		{grpRecent, "Recently Added", theme.HistoryIcon(), recent},
	}
	keeping, due := 0, 0
	for _, c := range u.store.Contacts {
		if c.KeepInTouch > 0 {
			keeping++
			if u.dueToReachOut(c, now) {
				due++
			}
		}
	}
	if keeping > 0 {
		items = append(items, sideItem{grpReachOut, "Reach Out", theme.MailReplyIcon(), due})
	}
	if len(u.attention) > 0 {
		items = append(items, sideItem{grpAttention, "Needs Attention", theme.WarningIcon(), len(u.attention)})
	}
	if incomplete > 0 {
		items = append(items, sideItem{grpIncomplete, "Missing Phone or Email", theme.QuestionIcon(), incomplete})
	}
	if len(u.cfg.SmartGroups) > 0 {
		items = append(items, sideItem{key: grpHeader, title: "Smart Groups"})
		for _, sg := range u.cfg.SmartGroups {
			sq := parseQuery(sg.Query)
			n := 0
			for _, c := range u.store.Contacts {
				if sq.matches(c, u.lastTalkedAt, u.dueNow) {
					n++
				}
			}
			items = append(items, sideItem{"q:" + sg.Name, sg.Name, theme.SearchIcon(), n})
		}
	}
	groups := u.store.Groups()
	if len(groups) > 0 {
		items = append(items, sideItem{key: grpHeader, title: "Groups"})
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
	if srcs := u.store.SourceNames(); len(srcs) > 0 {
		items = append(items, sideItem{key: grpHeader, title: "Sources"})
		for _, src := range srcs {
			n := 0
			for _, c := range u.store.Contacts {
				if c.HasSource(src) {
					n++
				}
			}
			items = append(items, sideItem{"s:" + src, src, theme.DownloadIcon(), n})
		}
	}
	u.sideItems = items
	u.side.Refresh()
	u.refreshMenuBar()
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
	case strings.HasPrefix(u.group, "s:"):
		return c.HasSource(u.group[2:])
	case u.group == grpReachOut:
		return u.dueToReachOut(c, now)
	case u.group == grpAttention:
		return u.attention[c.ID] > 0
	case strings.HasPrefix(u.group, "q:"):
		for _, sg := range u.cfg.SmartGroups {
			if "q:"+sg.Name == u.group {
				return parseQuery(sg.Query).matches(c, u.lastTalkedAt, u.dueNow)
			}
		}
		return false
	}
	return true
}

func (u *ui) lastTalkedAt(c *Contact) time.Time { t, _ := u.lastTalked(c); return t }
func (u *ui) dueNow(c *Contact) bool            { return u.dueToReachOut(c, time.Now()) }

func (u *ui) refreshList() {
	now := time.Now()
	q := u.search.Text
	sq := parseQuery(q)
	out := make([]*Contact, 0, len(u.store.Contacts))
	for _, c := range u.store.Contacts {
		if u.inGroup(c, now) && sq.matches(c, u.lastTalkedAt, u.dueNow) {
			out = append(out, c)
		}
	}
	if u.saveSearch != nil {
		if strings.TrimSpace(q) != "" {
			u.saveSearch.Show()
		} else {
			u.saveSearch.Hide()
		}
	}
	switch u.group {
	case grpReachOut: // longest without contact first
		sort.SliceStable(out, func(i, j int) bool { return u.lastTalkedAt(out[i]).Before(u.lastTalkedAt(out[j])) })
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
	if u.busy() {
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
	if u.selectMode { // ticks show the selection; no highlight
		u.list.UnselectAll()
		return
	}
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
		if u.ed != nil && u.ed.live { // Return starts typing in the card
			u.win.Canvas().Focus(u.ed.first)
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
	u.invalidate()
	defer u.checkAttention()
	if err := u.store.Save(); err != nil {
		showErr(fmt.Errorf("couldn't save your contacts: %w", err), u.win)
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

// busy reports an edit that mustn't be disturbed: a new contact being made, or a card with
// unsaved changes. A card that's merely shown (editable, but untouched) isn't busy.
func (u *ui) busy() bool {
	return u.ed != nil && (!u.ed.live || u.ed.dirty())
}

// showDetail shows the selected contact's card, editable in place.
func (u *ui) showDetail() {
	if u.busy() {
		return
	}
	u.ed = nil
	c := u.store.Get(u.selectedID)
	if c == nil {
		u.right.Objects = []fyne.CanvasObject{u.emptyState()}
		u.right.Refresh()
		return
	}
	u.openLive(c)
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
	imp := widget.NewButtonWithIcon("Bring Everything In — step by step", theme.DownloadIcon(), u.bringEverythingIn)
	imp.Importance = widget.HighImportance
	hub := widget.NewButtonWithIcon("Import from Gmail, LinkedIn, phones, spreadsheets…", theme.FolderOpenIcon(), u.importHub)
	vcf := hub
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

// rowLabel is the grey, right-aligned label column of a card row.
func rowLabel(s string) fyne.CanvasObject {
	l := widget.NewLabel(fieldLabel(s))
	l.Importance = widget.LowImportance
	l.Alignment = fyne.TextAlignTrailing
	l.Truncation = fyne.TextTruncateEllipsis
	return container.NewGridWrap(fyne.NewSize(96, l.MinSize().Height), l)
}

// valueRowFrom is valueRow with a line saying where the value came from.
func (u *ui) valueRowFrom(label, value, origin string, buttons ...fyne.CanvasObject) fyne.CanvasObject {
	if origin == "" {
		return u.valueRow(label, value, buttons...)
	}
	v := widget.NewLabel(value)
	v.Selectable = true
	v.Wrapping = fyne.TextWrapWord
	from := caption("from " + origin)
	from.Truncation = fyne.TextTruncateEllipsis
	return container.NewBorder(nil, nil, rowLabel(label), container.NewHBox(buttons...), container.New(&tightVBox{}, v, from))
}

func (u *ui) valueRow(label, value string, buttons ...fyne.CanvasObject) fyne.CanvasObject {
	v := widget.NewLabel(value)
	v.Selectable = true
	v.Wrapping = fyne.TextWrapWord
	return container.NewBorder(nil, nil, rowLabel(label), container.NewHBox(buttons...), v)
}

// toggleFavorite marks or unmarks a contact as a favorite.
func (u *ui) toggleFavorite(c *Contact) {
	c.Favorite = !c.Favorite
	u.store.Put(c)
	u.save()
}

// showGroup switches the list to a group or smart list.
func (u *ui) showGroup(key string) {
	u.group = key
	u.cfg.LastGroup = key
	u.selectSideKey(key)
	u.search.SetText("") // refreshes the list
	u.refreshList()
}

// cardText is the contact as plain text, for the clipboard.
func cardText(c *Contact) string {
	lines := []string{c.DisplayName()}
	if c.AltName != "" {
		lines = append(lines, c.AltName)
	}
	if s := strings.Join(nonEmpty([]string{c.Title, c.Company}), ", "); s != "" {
		lines = append(lines, s)
	}
	for _, p := range c.Phones {
		lines = append(lines, fieldLabel(p.Label)+": "+p.Value)
	}
	for _, e := range c.Emails {
		lines = append(lines, "email: "+e.Value)
	}
	for _, a := range c.Addresses {
		lines = append(lines, fieldLabel(a.Label)+": "+a.OneLine())
	}
	for _, l := range c.URLs {
		lines = append(lines, l.Label+": "+l.Value)
	}
	if c.Birthday != "" {
		lines = append(lines, "birthday: "+BirthdayText(c.Birthday))
	}
	return strings.Join(lines, "\n")
}

func (u *ui) copyCard(c *Contact) {
	u.app.Clipboard().SetContent(cardText(c))
	u.flash("Copied " + c.DisplayName() + "’s card")
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
		showErr(err, u.win)
		return
	}
	if err := u.app.OpenURL(p); err != nil {
		apps := map[string]string{"whatsapp": "WhatsApp", "tg": "Telegram", "sgnl": "Signal", "msteams": "Microsoft Teams", "skype": "Skype"}
		if name, ok := apps[p.Scheme]; ok {
			err = fmt.Errorf("the %s app isn't installed on this Mac", name)
		}
		showErr(err, u.win)
		return
	}
	if c != nil {
		c.LastContacted = time.Now()
		if err := u.store.Save(); err != nil {
			showErr(err, u.win)
		}
		if u.selectedID == c.ID && !u.busy() {
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
	u.confirm("Delete “"+c.DisplayName()+"”?", "The card moves to the Trash for 30 days, and you can undo this with ⌘Z.", "Delete", true, func() {
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
	})
}

// ---------- avatar ----------

// avatar is a circle with initials, or the contact's photo cut to a circle.
type avatar struct {
	widget.BaseWidget
	onTap  func() // when set, clicking the picture calls it (change photo)
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

// Tapped makes the picture clickable when onTap is set.
func (a *avatar) Tapped(*fyne.PointEvent) {
	if a.onTap != nil {
		a.onTap()
	}
}

// Cursor shows a pointing hand over a clickable picture.
func (a *avatar) Cursor() desktop.Cursor {
	if a.onTap != nil {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

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

func actionIcon(kind string) fyne.Resource {
	switch kind {
	case "call":
		return iconPhone
	case "message":
		return iconMessage
	case "whatsapp":
		return iconChat
	case "chat":
		return iconBubble
	case "video":
		return theme.MediaVideoIcon()
	case "email":
		return theme.MailComposeIcon()
	}
	return iconLink
}

// foldingSections holds the card's longer, occasional sections: calendar, timeline, connections
// and business cards. Each shows its count; opening one keeps it open for the next contact.
func (u *ui) foldingSections(c *Contact) fyne.CanvasObject {
	if u.openSec == nil {
		u.openSec = map[string]bool{"Calendar": true}
	}
	box := container.NewVBox()
	add := func(name string, count int, rows []fyne.CanvasObject) {
		if len(rows) == 0 {
			return
		}
		title := name
		if count > 0 {
			title = fmt.Sprintf("%s (%d)", title, count)
		}
		content := container.NewVBox(rows...)
		var head *widget.Button
		sync := func() {
			if u.openSec[name] {
				head.SetIcon(theme.MenuDropDownIcon())
				content.Show()
			} else {
				head.SetIcon(theme.MenuExpandIcon())
				content.Hide()
			}
		}
		head = widget.NewButtonWithIcon(title, theme.MenuExpandIcon(), func() {
			u.openSec[name] = !u.openSec[name]
			sync()
		})
		head.Alignment = widget.ButtonAlignLeading
		head.Importance = widget.LowImportance
		sync()
		box.Add(head)
		box.Add(content)
	}
	act := u.activitySection(c)
	add("Activity", 0, act)
	add("Calendar", len(u.eventsFor(c)), u.calendarSection(c))
	add("Timeline", len(c.Timeline), u.timelineSection(c))
	add("Connections", len(u.store.LinksOf(c.ID)), u.connectionsSection(c))
	add("Business cards", len(c.CardImages), u.cardsSection(c))
	return box
}

// timelineSection lists what happened with a contact, newest first.
func (u *ui) timelineSection(c *Contact) []fyne.CanvasObject {
	if len(c.Timeline) == 0 {
		return nil
	}
	var rows []fyne.CanvasObject
	for i, e := range c.Timeline {
		if i == 12 {
			rows = append(rows, caption(fmt.Sprintf("…and %d older entries", len(c.Timeline)-12)))
			break
		}
		when := "—"
		if !e.At.IsZero() {
			when = e.At.Format("2 Jan 2006")
		}
		l := caption(when)
		t := widget.NewLabel(e.Text)
		t.Wrapping = fyne.TextWrapWord
		var del fyne.CanvasObject
		if e.Source == "You" { // your own notes can be removed
			i := i
			b := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {
				u.confirmCB("Delete this note?", e.Text, "Delete", true, func(ok bool) {
					if ok && i < len(c.Timeline) {
						c.Timeline = append(c.Timeline[:i], c.Timeline[i+1:]...)
						u.store.Put(c)
						u.save()
					}
				}, u.win)
			})
			b.Importance = widget.LowImportance
			del = b
		}
		rows = append(rows, container.NewBorder(nil, nil, container.NewGridWrap(fyne.NewSize(110, l.MinSize().Height), l), del, t))
	}
	return rows
}
