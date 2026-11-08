package main

import (
	"fmt"
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

// Quick Find: a small window opened from the menu bar icon, for "get me X's number" without
// switching to the main window. Return copies the number; ⌘O (or a double click) opens the card.

type quickFind struct {
	u       *ui
	win     fyne.Window
	entry   *searchEntry
	list    *widget.List
	matches []*Contact
	sel     int
}

func (u *ui) setupMenuBar() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	desk.SetSystemTrayIcon(theme.AccountIcon())
	u.refreshMenuBar()
}

// refreshMenuBar rebuilds the menu bar menu (favorites and Reach Out change).
func (u *ui) refreshMenuBar() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	items := []*fyne.MenuItem{fyne.NewMenuItem("Quick Find…", u.showQuickFind), fyne.NewMenuItemSeparator()}
	cc := u.cfg.CountryCode
	n := 0
	for _, c := range u.store.Contacts {
		if !c.Favorite || n == 15 {
			continue
		}
		n++
		c := c
		var sub []*fyne.MenuItem
		for _, a := range c.Actions(cc) {
			a := a
			if a.More {
				continue
			}
			sub = append(sub, fyne.NewMenuItem(a.Title, func() { u.open(c, a.Link) }))
		}
		if p := bestPhone(c, cc); p != "" {
			sub = append(sub, fyne.NewMenuItem("Copy "+p, func() { u.app.Clipboard().SetContent(p) }))
		}
		sub = append(sub, fyne.NewMenuItemSeparator(), fyne.NewMenuItem("Open Card", func() { u.bringForward(); u.showContact(c.ID) }))
		it := fyne.NewMenuItem(c.DisplayName(), nil)
		it.ChildMenu = fyne.NewMenu("", sub...)
		items = append(items, it)
	}
	if n > 0 {
		items = append(items, fyne.NewMenuItemSeparator())
	}
	due := 0
	now := time.Now()
	for _, c := range u.store.Contacts {
		if u.dueToReachOut(c, now) {
			due++
		}
	}
	if due > 0 {
		items = append(items, fyne.NewMenuItem(fmt.Sprintf("Reach Out (%d)", due), func() { u.bringForward(); u.showGroup(grpReachOut) }))
	}
	items = append(items, fyne.NewMenuItem("Open Smart Contacts", u.bringForward))
	var sig strings.Builder
	for _, it := range items {
		sig.WriteString(it.Label + "|")
		if it.ChildMenu != nil {
			for _, x := range it.ChildMenu.Items {
				sig.WriteString(x.Label + ",")
			}
		}
	}
	if sig.String() == u.trayMenuSig { // nothing changed: don't rebuild the native menu
		return
	}
	u.trayMenuSig = sig.String()
	desk.SetSystemTrayMenu(fyne.NewMenu("Smart Contacts", items...))
}

func (u *ui) bringForward() {
	u.win.Show()
	u.win.RequestFocus()
}

// bestPhone is the number to copy: the first mobile, else the first number.
func bestPhone(c *Contact, cc string) string {
	for _, p := range c.Phones {
		if strings.EqualFold(p.Label, "Mobile") || strings.EqualFold(p.Label, "iPhone") {
			return FormatPhone(p.Value, cc)
		}
	}
	if len(c.Phones) > 0 {
		return FormatPhone(c.Phones[0].Value, cc)
	}
	return ""
}

func (u *ui) showQuickFind() {
	if u.qf == nil {
		u.qf = u.newQuickFind()
	}
	u.qf.entry.SetText("")
	u.qf.filter("")
	u.qf.win.Show()
	u.qf.win.RequestFocus()
	u.qf.win.Canvas().Focus(u.qf.entry)
}

func (u *ui) newQuickFind() *quickFind {
	q := &quickFind{u: u, win: u.app.NewWindow("Quick Find")}
	q.entry = newSearchEntry()
	q.entry.SetPlaceHolder("Name, number, company…")
	q.list = widget.NewList(
		func() int { return len(q.matches) },
		func() fyne.CanvasObject {
			name := widget.NewLabelWithStyle("Name", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			name.Truncation = fyne.TextTruncateEllipsis
			num := widget.NewLabel("+965 0000 0000")
			num.Importance = widget.LowImportance
			return container.NewBorder(nil, nil, newAvatar(28), container.NewHBox(container.NewCenter(newFlag()), num), name)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i >= len(q.matches) {
				return
			}
			c := q.matches[i]
			b := o.(*fyne.Container)
			b.Objects[0].(*widget.Label).SetText(c.DisplayName())
			b.Objects[1].(*avatar).set(c, u.photo(c))
			n := bestPhone(c, u.cfg.CountryCode)
			if n == "" && len(c.Emails) > 0 {
				n = c.Emails[0].Value
			}
			right := b.Objects[2].(*fyne.Container)
			right.Objects[1].(*widget.Label).SetText(n)
			fl := right.Objects[0].(*fyne.Container).Objects[0].(*canvas.Image)
			if strings.Contains(n, "@") {
				fl.Hide()
			} else {
				setFlag(fl, n, u.cfg.CountryCode)
			}
		})
	q.list.OnSelected = func(i widget.ListItemID) { q.sel = i }
	q.entry.OnChanged = q.filter
	q.entry.onKey = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyDown:
			if q.sel < len(q.matches)-1 {
				q.list.Select(q.sel + 1)
			}
		case fyne.KeyUp:
			if q.sel > 0 {
				q.list.Select(q.sel - 1)
			}
		case fyne.KeyReturn, fyne.KeyEnter:
			q.copySelected()
		case fyne.KeyEscape:
			q.win.Hide()
		default:
			return false
		}
		return true
	}
	open := widget.NewButton("Open Card", q.openSelected)
	call := widget.NewButtonWithIcon("Call", iconPhone, func() { q.act("call") })
	wa := widget.NewButtonWithIcon("WhatsApp", iconChat, func() { q.act("whatsapp") })
	cp := widget.NewButtonWithIcon("Copy Number", theme.ContentCopyIcon(), q.copySelected)
	cp.Importance = widget.HighImportance
	hint := caption("↑↓ choose · Return copies the number · Esc closes")
	q.win.SetContent(container.NewBorder(container.NewPadded(q.entry), container.NewVBox(container.NewHBox(call, wa, open, layout.NewSpacer(), cp), hint), nil, nil, q.list))
	q.win.Resize(fyne.NewSize(460, 420))
	q.win.SetCloseIntercept(q.win.Hide)
	q.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyO, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) { q.openSelected() })
	return q
}

func (q *quickFind) filter(s string) {
	sq := parseQuery(s)
	q.matches = q.matches[:0]
	for _, c := range q.u.store.Contacts {
		if sq.matches(c, q.u.lastTalkedAt, q.u.dueNow) {
			q.matches = append(q.matches, c)
		}
	}
	SortContacts(q.matches, q.u.cfg.SortByLast)
	if s == "" { // favorites first when nothing is typed
		var fav, rest []*Contact
		for _, c := range q.matches {
			if c.Favorite {
				fav = append(fav, c)
			} else {
				rest = append(rest, c)
			}
		}
		q.matches = append(fav, rest...)
	}
	q.list.Refresh()
	q.sel = 0
	if len(q.matches) > 0 {
		q.list.Select(0)
	}
}

func (q *quickFind) selected() *Contact {
	if q.sel < 0 || q.sel >= len(q.matches) {
		return nil
	}
	return q.matches[q.sel]
}

func (q *quickFind) copySelected() {
	c := q.selected()
	if c == nil {
		return
	}
	n := bestPhone(c, q.u.cfg.CountryCode)
	if n == "" && len(c.Emails) > 0 {
		n = c.Emails[0].Value
	}
	if n == "" {
		return
	}
	q.u.app.Clipboard().SetContent(n)
	q.u.flash("Copied " + n + " (" + c.DisplayName() + ")")
	q.win.Hide()
}

func (q *quickFind) openSelected() {
	if c := q.selected(); c != nil {
		q.win.Hide()
		q.u.bringForward()
		q.u.showContact(c.ID)
	}
}

func (q *quickFind) act(kind string) {
	c := q.selected()
	if c == nil {
		return
	}
	for _, a := range c.Actions(q.u.cfg.CountryCode) {
		if a.Kind == kind {
			q.win.Hide()
			q.u.open(c, a.Link)
			return
		}
	}
	showInfo("Not on this card", c.DisplayName()+" has no number for that.", q.win)
}
