package main

import (
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func (u *ui) mainMenu() *fyne.MainMenu {
	mod := func(k fyne.KeyName, m fyne.KeyModifier) fyne.Shortcut {
		return &desktop.CustomShortcut{KeyName: k, Modifier: fyne.KeyModifierShortcutDefault | m}
	}
	cmd := func(k fyne.KeyName) fyne.Shortcut { return mod(k, 0) }
	cmdShift := func(k fyne.KeyName) fyne.Shortcut { return mod(k, fyne.KeyModifierShift) }
	cmdOpt := func(k fyne.KeyName) fyne.Shortcut { return mod(k, fyne.KeyModifierAlt) }
	item := func(label string, sc fyne.Shortcut, f func()) *fyne.MenuItem {
		m := fyne.NewMenuItem(label, f)
		m.Shortcut = sc
		return m
	}
	sep := fyne.NewMenuItemSeparator
	current := func() *Contact {
		if u.busy() {
			return nil
		}
		return u.store.Get(u.selectedID)
	}
	// Cut, Copy, Paste and Select All go to the text field being typed in. The menu catches the
	// keys first, so they're passed on here.
	pass := func(sc fyne.Shortcut) bool {
		if f, ok := u.win.Canvas().Focused().(fyne.Shortcutable); ok {
			f.TypedShortcut(sc)
			return true
		}
		return false
	}
	clip := u.app.Clipboard()

	// File — as in Contacts: new card, new group, import, export.
	export := fyne.NewMenuItem("Export", nil)
	export.ChildMenu = fyne.NewMenu("",
		fyne.NewMenuItem("vCard…", func() { u.exportVCF(u.filtered, u.groupTitle()) }),
		fyne.NewMenuItem("Spreadsheet (CSV)…", u.exportCSV),
		fyne.NewMenuItem("Copy to Apple Contacts…", func() {
			if len(u.filtered) > 0 {
				u.sendToMacContacts(u.filtered)
			}
		}),
	)
	file := fyne.NewMenu("File",
		item("New Card", cmd(fyne.KeyN), func() { u.editContact(nil) }),
		item("New Group", cmdShift(fyne.KeyN), u.newGroupDialog),
		item("Smart Add…", cmdOpt(fyne.KeyN), u.smartAdd),
		sep(),
		func() *fyne.MenuItem { // the winged cards flying into the funnel, as on the cover
			m := item("Import…", cmd(fyne.KeyO), u.importHub)
			m.Icon = resImportMenu
			return m
		}(),
		fyne.NewMenuItem("Bring Everything In…", u.bringEverythingIn),
		item("Import from Mac Contacts", cmdShift(fyne.KeyR), u.importMac),
		fyne.NewMenuItem("Paste People from Text…", u.pasteImport),
		sep(),
		export,
		fyne.NewMenuItem("Back Up To…", u.backupTo),
		fyne.NewMenuItem("Restore from Backup…", u.restoreBackup),
		fyne.NewMenuItem("Show Data Folder in Finder", func() { RevealInFinder(u.store.Dir() + "/contacts.json") }),
		sep(),
		item("Save", cmd(fyne.KeyS), func() {
			if u.ed != nil && u.ed.live {
				u.finishWriting()
			} else if u.ed != nil {
				u.ed.save()
			}
		}),
		item("Settings…", cmd(fyne.KeyComma), u.settings), // macOS moves this into the app menu
	)

	// Edit → Delete deletes the clipped cards when there are any, otherwise the open card; its
	// confirmation names exactly which. (Menu titles can't change once the menu bar is built: see
	// refreshMenu.)
	// Edit — the standard items first, then the card ones.
	edit := fyne.NewMenu("Edit",
		item("Undo", cmd(fyne.KeyZ), u.undo),
		item("Redo", cmdShift(fyne.KeyZ), u.redo),
		sep(),
		item("Cut", cmd(fyne.KeyX), func() { pass(&fyne.ShortcutCut{Clipboard: clip}) }),
		item("Copy", cmd(fyne.KeyC), func() {
			if !pass(&fyne.ShortcutCopy{Clipboard: clip}) {
				if c := current(); c != nil { // nothing being typed in: copy the card
					u.copyCard(c)
				}
			}
		}),
		item("Paste", cmd(fyne.KeyV), func() { pass(&fyne.ShortcutPaste{Clipboard: clip}) }),
		item("Select All", cmd(fyne.KeyA), func() {
			// in an empty search field (where the keyboard usually is) ⌘A means every card shown
			if f := u.win.Canvas().Focused(); f == u.search && u.search.Text == "" {
				u.selectAllCards()
				return
			}
			if !pass(&fyne.ShortcutSelectAll{}) {
				u.selectAllCards()
			}
		}),
		item("Deselect All", cmdShift(fyne.KeyA), func() {
			if u.selectMode {
				u.selectNone()
			}
		}),
		sep(),
		item("Write on Card", cmd(fyne.KeyL), func() { u.writeCard(nil) }), // pick up the pen
		item("Delete…", cmd(fyne.KeyBackspace), func() {
			// In the search box ⌘⌫ clears what was typed; while typing it never deletes a card.
			if u.win.Canvas().Focused() == u.search {
				u.search.SetText("")
				return
			}
			if typing(u.win.Canvas().Focused()) {
				return
			}
			if u.selectMode && len(u.picked) > 0 {
				u.bulkDelete(u.pickedContacts())
				return
			}
			if c := current(); c != nil {
				u.deleteContact(c)
			}
		}),
		sep(),
		item("Find", cmd(fyne.KeyF), func() { u.win.Canvas().Focus(u.search) }),
		item("Quick Find…", cmdShift(fyne.KeyO), u.showQuickFind),
		fyne.NewMenuItem("History…", u.historyDialog),
	)

	// View — the same sections, names and order as the tabs down the left of the organiser
	// (and the sidebar in the plain look): the lists, To Tidy, Saved Searches, Groups, Sort, and
	// what else is in the book. Groups and saved searches change, and the menu can't (see
	// refreshMenu), so the menu goes to them through a chooser.
	header := func(s string) *fyne.MenuItem {
		h := fyne.NewMenuItem(s, nil)
		h.Disabled = true
		return h
	}
	list := func(label string, sc fyne.Shortcut, key string) *fyne.MenuItem {
		m := item(label, sc, func() { u.showGroup(key) })
		m.Disabled = !u.groupExists(key)
		m.Checked = u.group == key
		return m
	}
	byFirst := item("Sort by First Name", nil, func() { u.setSort(false) })
	byLast := item("Sort by Last Name", nil, func() { u.setSort(true) })
	byFirst.Checked, byLast.Checked = !u.cfg.SortByLast, u.cfg.SortByLast
	goSaved := fyne.NewMenuItem("Go to Saved Search…", func() { u.goToDialog("q:") })
	goSaved.Disabled = len(u.cfg.SmartGroups) == 0
	goGroup := item("Go to Group…", cmd(fyne.KeyG), func() { u.goToDialog("g:") })
	goGroup.Disabled = len(u.store.Groups()) == 0
	filofaxItem := fyne.NewMenuItem("Filofax Look", u.toggleLook)
	filofaxItem.Checked = !u.cfg.ClassicLook
	view := fyne.NewMenu("View",
		list("Today", cmd(fyne.Key0), grpToday),
		list("All Contacts", cmd(fyne.Key1), grpAll),
		list("Favorites", cmd(fyne.Key2), grpFavorites),
		list("Birthdays", cmd(fyne.Key3), grpBirthdays),
		list("Recently Added", cmd(fyne.Key4), grpRecent),
		list("Reach Out", cmd(fyne.Key5), grpReachOut),
		list("Follow-ups", cmd(fyne.Key8), grpFollowUps),
		sep(),
		header("To Tidy"),
		list("Needs Attention", cmd(fyne.Key6), grpAttention),
		list("Missing Phone or Email", cmd(fyne.Key7), grpIncomplete),
		sep(),
		header("Saved Searches"),
		goSaved,
		fyne.NewMenuItem("Save This Search…", u.saveSmartGroup),
		sep(),
		header("Groups"),
		goGroup,
		sep(),
		header("Sort"),
		byFirst, byLast,
		sep(),
		header("Also in the Book"),
		item("Calendar Log…", cmdShift(fyne.KeyL), u.calendarDialog),
		fyne.NewMenuItem("Trash…", u.trash),
		sep(),
		filofaxItem,
		item("Close Organiser", cmdShift(fyne.KeyW), u.showCover),
	)

	// Card — ten commands, all acting on the clipped cards if any are clipped, otherwise on the
	// open card (cardcmds.go). Tidy Up and Find Duplicates ask in their window, since they can
	// also take the whole book. Dimmed when there's nothing for them to act on.
	ts := u.targets()
	none := len(ts) == 0
	onTargets := func(label string, sc fyne.Shortcut, f func()) *fyne.MenuItem {
		m := item(label, sc, func() {
			if len(u.targets()) > 0 {
				f()
			}
		})
		m.Disabled = none
		return m
	}
	fav := onTargets("Favorite", cmdShift(fyne.KeyF), u.cmdFavorite)
	fav.Checked = allFavorite(ts)
	keepMenu := fyne.NewMenuItem("Keep in Touch", nil)
	keepMenu.ChildMenu = fyne.NewMenu("", u.keepItems()...)
	keepMenu.Disabled = none
	conn := item("Connections…", cmd(fyne.KeyK), u.cmdConnections)
	conn.Disabled = !u.connectionsEnabled(ts)
	share := fyne.NewMenuItem("Share", nil)
	share.ChildMenu = fyne.NewMenu("", u.shareItems()...)
	share.Disabled = none
	clipToggle := item("Clip Cards", cmdShift(fyne.KeyS), u.toggleSelectMode)
	clipToggle.Checked = u.selectMode
	dupes := item("Find Duplicates…", cmdShift(fyne.KeyD), u.cmdDuplicates)
	dupes.Disabled = len(u.store.Contacts) < 2
	cardItems := []*fyne.MenuItem{
		item("Today", nil, func() { u.showGroup(grpToday) }), // the day's page: birthdays, who to call, who you met (⌘0 in View)
		sep(),
		onTargets("Add Note…", cmd(fyne.KeyJ), u.cmdNote),
		fav,
		keepMenu,
		onTargets("Groups…", nil, u.cmdGroups),
		conn,
		sep(),
		item("Tidy Up…", nil, u.cmdTidy),
		dupes,
		share,
		item("Send Greetings…", nil, u.cmdGreetings), // Eid, Ramadan, birthdays: one by one on WhatsApp
		sep(),
		clipToggle,
		sep(),
		item("Check Messages…", cmdShift(fyne.KeyU), u.cmdCheckMessages),
		sep(),
		onTargets("Move to Trash", nil, u.cmdTrash),
		item("Show Trash…", nil, u.trash),
		func() *fyne.MenuItem {
			m := item("Empty Trash…", nil, u.cmdEmptyTrash)
			m.Disabled = len(u.store.Trash) == 0
			return m
		}(),
	}
	card := fyne.NewMenu("Card", cardItems...)
	iconize(card, cardMenuIcons()) // the same pictures as the toolbar's tabs
	iconize(view, viewMenuIcons()) // and as the side tabs

	help := fyne.NewMenu("Help",
		item("Smart Contacts Help", cmdShift(fyne.KeySlash), u.help),
		sep(),
		fyne.NewMenuItem("Check for Updates…", u.checkForUpdates),
		// Not titled plain "About": Fyne moves an item called that into the app menu by taking
		// over macOS's own About item, and that hand-over doesn't always hold (About would do
		// nothing until the app was reopened). So the app menu keeps Apple's About panel (name,
		// version, icon), and this one, in Help, opens the app's own with Check for Updates.
		fyne.NewMenuItem("About Smart Contacts…", u.about),
	)
	return fyne.NewMainMenu(file, edit, view, card, help)
}

// refreshMenu keeps the menu bar true to what's on screen: which commands can act now (dimmed
// or not), which are ticked, and what they'll act on.
//
// The Mac menu bar is built once and never rebuilt: Fyne's Mac menu code releases each divider
// once too often, so throwing a built menu away and making another crashed the app (opening one
// card and then another did it, when the Card menu named the open card). So titles and layout
// stay fixed, and only each item's dimmed/ticked state and action are updated, which the Mac
// reads afresh every time a menu opens.
func (u *ui) refreshMenu() {
	if u.win == nil || u.list == nil {
		return
	}
	u.refreshToolbar() // the toolbar follows the same state as the menu
	defer u.trashTitles()
	m := u.mainMenu()
	if u.menu == nil {
		u.menu = m
		u.win.SetMainMenu(m)
		return
	}
	var sync func(old, now []*fyne.MenuItem)
	sync = func(old, now []*fyne.MenuItem) {
		for i := 0; i < len(old) && i < len(now); i++ {
			o, n := old[i], now[i]
			if o.Label != n.Label || o.IsSeparator != n.IsSeparator {
				fyne.LogError("menu layout changed: "+o.Label+" → "+n.Label, nil)
				continue
			}
			o.Action, o.Disabled, o.Checked = n.Action, n.Disabled, n.Checked
			if o.ChildMenu != nil && n.ChildMenu != nil {
				sync(o.ChildMenu.Items, n.ChildMenu.Items)
			}
		}
	}
	for i := 0; i < len(u.menu.Items) && i < len(m.Items); i++ {
		sync(u.menu.Items[i].Items, m.Items[i].Items)
	}
	if runtime.GOOS != "darwin" { // elsewhere the menu bar is drawn by Fyne and needs redrawing
		u.win.SetMainMenu(u.menu)
	}
}

// selectAllCards ticks everyone in the list (⌘A when not typing).
func (u *ui) selectAllCards() {
	if u.busy() {
		return
	}
	if !u.selectMode {
		u.toggleSelectMode()
	}
	for _, c := range u.filtered {
		u.picked[c.ID] = true
	}
	u.list.Refresh()
	u.updateBulkBar()
}

// ---------- Smart Add ----------
