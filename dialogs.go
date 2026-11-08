package main

import (
	"context"
	"fmt"
	"image/color"
	"io"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
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
		item("Import…", cmd(fyne.KeyO), u.importHub),
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
			if u.ed != nil {
				u.ed.save()
			}
		}),
		item("Settings…", cmd(fyne.KeyComma), u.settings), // macOS moves this into the app menu
	)

	// Edit → Delete says what it will delete, as Finder's "Move 3 Items to Trash" does.
	delLabel := "Delete Card…"
	if u.selectMode && len(u.picked) > 0 {
		delLabel = fmt.Sprintf("Delete %d Clipped %s…", len(u.picked), pluralWords(len(u.picked), "Card", "Cards"))
	} else if c := current(); c != nil {
		delLabel = "Delete " + c.DisplayName() + "…"
	}

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
		item("Edit Card", cmd(fyne.KeyL), func() { // the card is editable: go to its first field
			if u.ed != nil {
				u.win.Canvas().Focus(u.ed.first)
			}
		}),
		item(delLabel, cmd(fyne.KeyBackspace), func() {
			// In the search box ⌘⌫ clears what was typed; while typing it never deletes a card.
			if u.win.Canvas().Focused() == u.search {
				u.search.SetText("")
				return
			}
			if _, typing := u.win.Canvas().Focused().(*widget.Entry); typing {
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

	// View — which list to show and how to sort it.
	byFirst := fyne.NewMenuItem("Sort by First Name", nil)
	byLast := fyne.NewMenuItem("Sort by Last Name", nil)
	setSort := func(last bool) {
		u.cfg.SortByLast = last
		_ = u.cfg.Save(u.store.Dir())
		byFirst.Checked, byLast.Checked = !last, last
		u.win.MainMenu().Refresh()
		u.refreshList()
	}
	byFirst.Action, byLast.Action = func() { setSort(false) }, func() { setSort(true) }
	byFirst.Checked, byLast.Checked = !u.cfg.SortByLast, u.cfg.SortByLast
	filofaxItem := fyne.NewMenuItem("Filofax Look", u.toggleLook)
	filofaxItem.Checked = !u.cfg.ClassicLook
	view := fyne.NewMenu("View",
		item("All Contacts", cmd(fyne.Key1), func() { u.showGroup(grpAll) }),
		item("Favorites", cmd(fyne.Key2), func() { u.showGroup(grpFavorites) }),
		item("Upcoming Birthdays", cmd(fyne.Key3), func() { u.showGroup(grpBirthdays) }),
		item("Recently Added", cmd(fyne.Key4), func() { u.showGroup(grpRecent) }),
		item("Reach Out", cmd(fyne.Key5), func() { u.showGroup(grpReachOut) }),
		item("Needs Attention", cmd(fyne.Key6), func() { u.showGroup(grpAttention) }),
		fyne.NewMenuItem("Save Search as Smart Group…", u.saveSmartGroup),
		sep(),
		byFirst, byLast,
		sep(),
		filofaxItem,
		item("Close Organiser", cmdShift(fyne.KeyW), u.showCover),
		sep(),
		item("Clip Cards Together", cmdShift(fyne.KeyS), u.toggleSelectMode),
		item("Calendar Log…", cmdShift(fyne.KeyL), u.calendarDialog),
		fyne.NewMenuItem("Trash", u.trash),
	)

	// Card — what you do with people.
	keepMenu := fyne.NewMenuItem("Keep in Touch", nil)
	var keepItems []*fyne.MenuItem
	for _, k := range append(keepChoices, struct {
		Days  int
		Label string
	}{0, "No Reminder"}) {
		k := k
		keepItems = append(keepItems, fyne.NewMenuItem(k.Label, func() {
			if c := current(); c != nil {
				u.setKeepInTouch(c, k.Days)
			}
		}))
	}
	keepMenu.ChildMenu = fyne.NewMenu("", keepItems...)
	// Card — in three sections, each headed by what it acts on (Apple: a command's title says what
	// it will act on, and commands that can't act are dimmed, not hidden):
	//   the card that's open · the cards ticked in Select mode · the whole address book.
	c := current()
	picked := []*Contact{}
	if u.selectMode {
		picked = u.pickedContacts()
	}
	header := func(s string) *fyne.MenuItem {
		h := fyne.NewMenuItem(s, nil)
		h.Disabled = true
		return h
	}
	onCard := func(label string, sc fyne.Shortcut, f func(*Contact)) *fyne.MenuItem {
		m := item(label, sc, func() {
			if c := current(); c != nil {
				f(c)
			}
		})
		m.Disabled = c == nil
		return m
	}
	cardName := "No Card Open"
	if c != nil {
		cardName = "This Card — " + c.DisplayName()
	}
	fav := "Add to Favorites"
	if c != nil && c.Favorite {
		fav = "Remove from Favorites"
	}
	keepMenu.Disabled = c == nil
	section1 := []*fyne.MenuItem{
		header(cardName),
		onCard("Add Note…", cmd(fyne.KeyJ), u.addNote),
		keepMenu,
		onCard(fav, cmdShift(fyne.KeyF), u.toggleFavorite),
		onCard("Connect to…", cmd(fyne.KeyK), u.connectDialog),
		onCard("How Are They Connected?", cmdShift(fyne.KeyK), func(c *Contact) { u.pathDialog(c, nil) }),
		onCard("Clean Up This Card…", nil, func(c *Contact) { u.cleanupFor([]*Contact{c}, "") }),
		onCard("Tidy This Note with AI…", nil, func(c *Contact) { u.tidyNotesAI([]*Contact{c}, true) }),
		onCard("Copy Card as Text", nil, u.copyCard),
		onCard("Save Card as vCard…", nil, func(c *Contact) { u.exportVCF([]*Contact{c}, c.DisplayName()) }),
	}

	n := len(picked)
	sel := fmt.Sprintf("%d Clipped %s", n, pluralWords(n, "Card", "Cards"))
	onPicked := func(label string, sc fyne.Shortcut, need int, f func([]*Contact)) *fyne.MenuItem {
		m := item(label, sc, func() {
			if cs := u.pickedContacts(); u.selectMode && len(cs) >= need {
				f(cs)
			}
		})
		m.Disabled = n < need
		return m
	}
	var section2 []*fyne.MenuItem
	if !u.selectMode || n == 0 {
		hint := "Clipped Cards — none clipped yet"
		if !u.selectMode {
			hint = "Clipped Cards — click a card's paper clip to start"
		}
		section2 = []*fyne.MenuItem{header(hint), item("Clip Cards Together", cmdShift(fyne.KeyS), func() {
			if !u.selectMode {
				u.toggleSelectMode()
			}
		})}
	} else {
		bulk := u.bulkMenu(picked) // the same submenus as right-clicking the selection
		var group, keepN *fyne.MenuItem
		for _, it := range bulk.Items {
			switch it.Label {
			case "Add to Group":
				group = it
			case "Keep in Touch":
				keepN = it
			}
		}
		section2 = []*fyne.MenuItem{header(sel)}
		if group != nil {
			section2 = append(section2, group)
		}
		if keepN != nil {
			section2 = append(section2, keepN)
		}
		section2 = append(section2,
			onPicked(fmt.Sprintf("Clean Up %s…", sel), nil, 1, func(cs []*Contact) { u.cleanupFor(cs, "") }),
			onPicked(fmt.Sprintf("Tidy Jumbled Notes in %s…", sel), nil, 1, func(cs []*Contact) { u.tidyNotesAI(cs, false) }),
			onPicked(fmt.Sprintf("Merge %s…", sel), cmdShift(fyne.KeyM), 2, u.bulkMerge),
			onPicked(fmt.Sprintf("Delete %s…", sel), nil, 1, u.bulkDelete),
		)
	}

	all := fmt.Sprintf("All %d Cards", len(u.store.Contacts))
	section3 := []*fyne.MenuItem{
		header("Whole Address Book — " + all),
		fyne.NewMenuItem("Clean Up "+all+"…", u.cleanup),
		fyne.NewMenuItem("Tidy Jumbled Notes in "+all+"…", func() { u.tidyNotesAI(u.store.Contacts, false) }),
		item("Look for Duplicates in "+all+"…", cmdShift(fyne.KeyD), u.duplicates),
		item("Update Last Talked from Messages, Calls and Mail", cmdShift(fyne.KeyU), func() { u.readActivity(true) }),
		fyne.NewMenuItem("Find People Not in Contacts…", u.unknownPeople),
	}
	var cardItems []*fyne.MenuItem
	cardItems = append(cardItems, section1...)
	cardItems = append(cardItems, sep())
	cardItems = append(cardItems, section2...)
	cardItems = append(cardItems, sep())
	cardItems = append(cardItems, section3...)
	card := fyne.NewMenu("Card", cardItems...)

	help := fyne.NewMenu("Help",
		item("Smart Contacts Help", cmdShift(fyne.KeySlash), u.help),
		sep(),
		fyne.NewMenuItem("Check for Updates…", u.checkForUpdates),
		fyne.NewMenuItem("About", u.about), // macOS moves this into the app menu
	)
	return fyne.NewMainMenu(file, edit, view, card, help)
}

// refreshMenu rebuilds the menu bar when what the commands act on changes (another card opened,
// cards ticked), so their titles stay true. It's skipped when nothing in the menus would change.
func (u *ui) refreshMenu() {
	if u.win == nil || u.list == nil {
		return
	}
	m := u.mainMenu()
	var sig strings.Builder
	var walk func(items []*fyne.MenuItem)
	walk = func(items []*fyne.MenuItem) {
		for _, it := range items {
			fmt.Fprintf(&sig, "%s|%v|%v;", it.Label, it.Disabled, it.Checked)
			if it.ChildMenu != nil {
				walk(it.ChildMenu.Items)
			}
		}
	}
	for _, mm := range m.Items {
		walk(mm.Items)
	}
	if sig.String() == u.menuSig {
		return
	}
	u.menuSig = sig.String()
	u.win.SetMainMenu(m)
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

func (u *ui) smartAdd() {
	in := widget.NewMultiLineEntry()
	in.Wrapping = fyne.TextWrapWord
	in.SetMinRowsVisible(10)
	in.SetPlaceHolder("Paste an email signature, a business card's text, a WhatsApp contact message, a LinkedIn blurb…")
	if clip := strings.TrimSpace(u.app.Clipboard().Content()); clip != "" && len(clip) < 4000 && (reEmail.MatchString(clip) || rePhone.MatchString(clip)) {
		in.SetText(clip) // looks like contact details: save a paste
	}
	mode := "Private: read on this Mac only. Nothing you paste leaves this computer."
	if u.cloudAI() {
		mode = "Cloud AI is ON: this text will be sent to " + u.cfg.AIModel + " via OpenRouter (turn it off in Settings to keep everything on this Mac)."
	}
	note := caption(mode)
	note.Wrapping = fyne.TextWrapWord
	progress := widget.NewProgressBarInfinite()
	progress.Hide()
	var d *dialog.ConfirmDialog
	cardBtn := widget.NewButtonWithIcon("Business Card Image…", theme.FileImageIcon(), func() {
		d.Hide()
		u.chooseCardImage()
	})
	cardNote := caption("…or read a photo or screenshot of a business card (the text is read on this Mac; the image is kept with the contact). You can also drag a card image onto the window.")
	cardNote.Wrapping = fyne.TextWrapWord
	content := container.NewBorder(nil, container.NewVBox(note, progress, widget.NewSeparator(), container.NewBorder(nil, nil, nil, cardBtn, cardNote)), nil, nil, in)
	d = dialog.NewCustomConfirm("Smart Add", "Read It", "Cancel", content, func(ok bool) {
		text := strings.TrimSpace(in.Text)
		if !ok || text == "" {
			return
		}
		u.runSmartAdd(text)
	}, u.win)
	d.Resize(fyne.NewSize(600, 420))
	d.Show()
	u.win.Canvas().Focus(in)
}

func (u *ui) runSmartAdd(text string) { u.runSmartAddCard(text, nil) }

// runSmartAddCard reads text (typed, pasted or recognised from a card image) into a contact.
// card is the business card image, kept with whichever contact is saved.
func (u *ui) runSmartAddCard(text string, card []byte) {
	cc := u.cfg.CountryCode
	if !u.cloudAI() {
		u.finishSmartAddCard(ParseLocal(text, cc), "Read privately on this Mac — check the details, then Save.", card)
		return
	}
	prog := dialog.NewCustomWithoutButtons("Reading…", container.NewVBox(widget.NewLabel("Asking "+u.cfg.AIModel+"…"), widget.NewProgressBarInfinite()), u.win)
	prog.Show()
	go func() {
		c, confident, err := ParseAI(context.Background(), u.cfg.OpenRouterKey, u.cfg.AIModel, text, cc)
		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				u.finishSmartAddCard(ParseLocal(text, cc), "The AI didn't answer ("+err.Error()+"), so this was read on this Mac — check it, then Save.", card)
				return
			}
			msg := "Filled in by AI — check the details, then Save."
			if !confident {
				msg = "The text may describe more than one person; this is the AI's best single contact. Check it, then Save."
			}
			u.finishSmartAddCard(c, msg, card)
		})
	}()
}

// cloudAI is true only when the user has explicitly allowed sending text off this Mac AND set a key.
func (u *ui) cloudAI() bool {
	return u.cfg.AllowCloudAI && strings.TrimSpace(u.cfg.OpenRouterKey) != ""
}

// finishSmartAddCard opens the editor with what was read. If the person is already in the book,
// it asks whether to update that card (adding what's new) or add a new person.
func (u *ui) finishSmartAddCard(c *Contact, banner string, card []byte) {
	c.Tidy()
	if card != nil {
		banner += "\nThe business card image will be kept with this contact."
	}
	x := NewDupIndex(u.store.Contacts, u.cfg.CountryCode).Find(c)
	if x == nil {
		u.leaveEditor(func() { u.openEditorCard(nil, c, banner, card, "added this contact") }, nil)
		return
	}
	merged := x.Clone()
	Merge(merged, []*Contact{c}, u.cfg.CountryCode)
	adds := mergeAdds(x, c, u.cfg.CountryCode)
	what := "nothing new"
	if len(adds) > 0 {
		what = strings.Join(adds, ", ")
	}
	msg := widget.NewLabel(fmt.Sprintf("This looks like %s, who is already in your contacts.\n\nUpdating adds: %s.", x.DisplayName(), what))
	msg.Wrapping = fyne.TextWrapWord
	var d *dialog.CustomDialog
	update := widget.NewButton("Update "+x.DisplayName(), func() {
		d.Hide()
		u.leaveEditor(func() {
			u.selectedID = x.ID
			u.reselect()
			u.openEditorCard(x, merged, banner+"\nUpdated from the new details — check, then Save.", card, "updated this contact")
		}, nil)
	})
	update.Importance = widget.HighImportance
	add := widget.NewButton("Add as a new person", func() {
		d.Hide()
		u.leaveEditor(func() { u.openEditorCard(nil, c, banner, card, "added this contact") }, nil)
	})
	cancel := widget.NewButton("Cancel", func() { d.Hide() })
	d = dialog.NewCustomWithoutButtons("Already in your contacts?", container.NewVBox(msg, update, add, cancel), u.win)
	d.Resize(fyne.NewSize(520, 300))
	d.Show()
}

// chooseCardImage picks a business card photo or screenshot and reads it.
func (u *ui) chooseCardImage() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil || r == nil {
			return
		}
		defer r.Close()
		b, err := io.ReadAll(io.LimitReader(r, 60<<20))
		if err != nil {
			showErr(err, u.win)
			return
		}
		u.readCardImage(r.URI().Name(), b)
	}, u.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".jpg", ".jpeg", ".png", ".heic", ".heif", ".tiff", ".JPG", ".JPEG", ".PNG", ".HEIC"}))
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

// readCardImage recognises the text on a card image (on this Mac) and continues as Smart Add.
func (u *ui) readCardImage(name string, data []byte) {
	img, err := ToJPEG(data, name)
	if err != nil {
		showErr(err, u.win)
		return
	}
	prog := dialog.NewCustomWithoutButtons("Reading the business card…", container.NewVBox(
		widget.NewLabel("Recognising the text on this Mac."), widget.NewProgressBarInfinite()), u.win)
	prog.Show()
	go func() {
		text, err := ReadCardText(img)
		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				showErr(err, u.win)
				return
			}
			u.runSmartAddCard(text, img)
		})
	}()
}

// ---------- import ----------

func (u *ui) importMac() {
	prog := dialog.NewCustomWithoutButtons("Reading Mac Contacts…", container.NewVBox(
		widget.NewLabel("If macOS asks, allow Smart Contacts to use Contacts."), widget.NewProgressBarInfinite()), u.win)
	prog.Show()
	go func() {
		cards, err := ReadMacContacts()
		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				showErr(err, u.win)
				return
			}
			u.previewImport(&Batch{Source: "Mac Contacts", Format: "Mac Contacts app", Cards: cards})
		})
	}()
}

// sendToMacContacts copies cards into Apple's Contacts app, after warning that Apple's app
// may sync them to iCloud — the one way contacts could leave this Mac from here.
func (u *ui) sendToMacContacts(cs []*Contact) {
	if len(cs) == 0 {
		return
	}
	what := fmt.Sprintf("these %d contacts", len(cs))
	if len(cs) == 1 {
		what = cs[0].DisplayName()
	}
	msg := widget.NewLabel("This copies " + what + " into Apple's Contacts app.\n\n" +
		"If iCloud Contacts is turned on (System Settings → your name → iCloud → Contacts),\n" +
		"Apple will sync the copy to iCloud and your other devices — it will no longer be only on this Mac.\n\n" +
		"Smart Contacts itself keeps everything on this Mac either way.")
	dialog.ShowCustomConfirm("Copy to Apple Contacts?", "Copy Anyway", "Cancel", msg, func(ok bool) {
		if ok {
			if err := OpenInMacContacts(u.store, cs); err != nil {
				showErr(err, u.win)
			}
		}
	}, u.win)
}

// confirmExportPlace warns before writing contacts into a folder that syncs off this Mac.
func (u *ui) confirmExportPlace(w fyne.URIWriteCloser, write func()) {
	if !InCloudFolder(w.URI().Path()) {
		write()
		return
	}
	dialog.ShowCustomConfirm("This folder syncs to the cloud", "Save There Anyway", "Cancel",
		widget.NewLabel("“"+w.URI().Name()+"” is going into a cloud-synced folder (iCloud Drive, Dropbox, Google Drive…),\n"+
			"so the contacts would leave this Mac. Choose a folder like Documents or Desktop to keep them private."),
		func(ok bool) {
			if ok {
				write()
				return
			}
			w.Close()
			_ = storage.Delete(w.URI()) // don't leave an empty file in the cloud folder
		}, u.win)
}

// ---------- export ----------

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, s)
	if i := strings.Index(s, " ("); i > 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func (u *ui) exportVCF(cs []*Contact, name string) {
	if len(cs) == 0 {
		return
	}
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil || w == nil {
			return
		}
		u.confirmExportPlace(w, func() {
			defer w.Close()
			for _, c := range cs {
				if err := WriteVCard(w, c, u.store.Photo(c)); err != nil {
					showErr(err, u.win)
					return
				}
			}
			u.flash(fmt.Sprintf("Saved %d contacts to %s", len(cs), w.URI().Name()))
		})
	}, u.win)
	d.SetFileName(safeName(name) + ".vcf")
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

func (u *ui) exportCSV() { u.exportCSVFor(u.filtered) }

// exportCSVFor saves some contacts as a spreadsheet.
func (u *ui) exportCSVFor(cs []*Contact) {
	if len(cs) == 0 {
		return
	}
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil || w == nil {
			return
		}
		u.confirmExportPlace(w, func() {
			defer w.Close()
			if err := WriteCSV(w, cs); err != nil {
				showErr(err, u.win)
				return
			}
			u.flash(fmt.Sprintf("Saved %d contacts to %s", len(cs), w.URI().Name()))
		})
	}, u.win)
	name := u.groupTitle()
	if len(cs) != len(u.filtered) {
		name = fmt.Sprintf("%d contacts", len(cs))
	}
	d.SetFileName(safeName(name) + ".csv")
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

// ---------- duplicates ----------

// richness scores how much a contact holds, so merging keeps the fullest card.
func richness(c *Contact) int {
	n := len(c.Phones)*3 + len(c.Emails)*3 + len(c.Addresses)*2 + len(c.URLs) + len(c.Groups) + len(c.Notes)/40
	for _, s := range []string{c.First, c.Last, c.Company, c.Title, c.Birthday} {
		if s != "" {
			n += 2
		}
	}
	if c.HasPhoto {
		n += 3
	}
	return n
}

// duplicates is Deduplicate: the same person on several cards (merge), and the same number, email,
// link, address or note line repeated inside one card (remove).
func (u *ui) duplicates() {
	u.cfg.LastDedupe = time.Now()
	_ = u.cfg.Save(u.store.Dir())
	u.duplicatesFor(u.store.Contacts)
}

// duplicatesFor looks for duplicates among some contacts (everyone, or a selection).
func (u *ui) duplicatesFor(cs []*Contact) {
	groups := FindDuplicates(cs, u.cfg.CountryCode)
	var inside []CleanupChange
	for _, ch := range PlanCleanup(cs, u.cfg.CountryCode) {
		if ch.Kind == ckDuplicate {
			inside = append(inside, ch)
		}
	}
	if len(groups) == 0 && len(inside) == 0 {
		showInfo("No duplicates", "Every contact is on one card, and no card repeats a number, email, link, address or note line.", u.win)
		return
	}
	box := container.NewVBox()
	if len(inside) > 0 {
		cards := map[*Contact]bool{}
		var lines []string
		for _, ch := range inside {
			cards[ch.Contact] = true
			if len(lines) < 8 {
				what := ch.Old
				if ch.Field == "notes" {
					what = "note lines (" + ch.Reason + ")"
				}
				lines = append(lines, ch.Contact.DisplayName()+": "+what)
			}
		}
		if len(inside) > 8 {
			lines = append(lines, fmt.Sprintf("…and %d more", len(inside)-8))
		}
		head := widget.NewLabelWithStyle(fmt.Sprintf("Repeats inside cards (%d on %d cards)", len(inside), len(cards)), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		list := caption(strings.Join(lines, "\n"))
		var rm *widget.Button
		rm = widget.NewButtonWithIcon("Remove Repeats", theme.DeleteIcon(), func() {
			if _, err := u.store.SnapshotBackup("before-dedupe"); err != nil {
				showErr(err, u.win)
				return
			}
			touched := map[*Contact]bool{}
			for _, ch := range inside {
				ch.Apply("")
				touched[ch.Contact] = true
			}
			for c := range touched {
				u.store.Put(c)
			}
			u.save()
			rm.SetText("Removed")
			rm.Disable()
		})
		box.Add(container.NewBorder(nil, nil, nil, container.NewVBox(rm), container.NewVBox(head, list)))
		box.Add(widget.NewSeparator())
	}
	if len(groups) > 0 {
		box.Add(widget.NewLabelWithStyle(fmt.Sprintf("Same person on several cards (%d)", len(groups)), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	}
	var d *dialog.CustomDialog
	merge := func(g DupeGroup) bool {
		var cs []*Contact // only cards still in the address book (not merged away already)
		for _, c := range g.Contacts {
			if u.store.Get(c.ID) != nil {
				cs = append(cs, c)
			}
		}
		if len(cs) < 2 {
			return false
		}
		best := 0
		for i, c := range cs {
			if richness(c) > richness(cs[best]) {
				best = i
			}
		}
		keep := cs[best]
		others := append(cs[:best:best], cs[best+1:]...)
		if !keep.HasPhoto {
			for _, o := range others {
				if p := u.store.Photo(o); len(p) > 0 {
					_ = u.store.SetPhoto(keep, p)
					delete(u.photoCache, keep.ID)
					break
				}
			}
		}
		for _, o := range others {
			keep.AddEvent(time.Now(), "Merged with another card ("+o.DisplayName()+")", "Deduplicate")
		}
		Merge(keep, others, u.cfg.CountryCode)
		for _, o := range others {
			u.store.RepointLinks(o.ID, keep.ID)
		}
		u.store.Put(keep)
		ids := []string{}
		for _, o := range others {
			ids = append(ids, o.ID)
		}
		u.store.Delete(ids...) // into the Trash, so a wrong merge can be undone by hand
		return true
	}
	for _, g := range groups {
		g := g
		names := []string{}
		for _, c := range g.Contacts {
			s := c.DisplayName()
			if len(c.Phones) > 0 {
				s += " · " + c.Phones[0].Value
			} else if len(c.Emails) > 0 {
				s += " · " + c.Emails[0].Value
			}
			names = append(names, s)
		}
		lbl := widget.NewLabel(strings.Join(names, "\n"))
		why := caption(g.Reason)
		var row *fyne.Container
		btn := widget.NewButtonWithIcon("Merge…", iconMerge, nil)
		btn.OnTapped = func() {
			var cs []*Contact
			for _, c := range g.Contacts {
				if u.store.Get(c.ID) != nil {
					cs = append(cs, c)
				}
			}
			if len(cs) < 2 {
				btn.SetText("Already merged")
				btn.Disable()
				return
			}
			u.mergeChooser(cs, func(*Contact) {
				btn.SetText("Merged")
				btn.Disable()
			})
		}
		row = container.NewBorder(nil, nil, nil, container.NewVBox(btn), container.NewVBox(lbl, why))
		box.Add(row)
		box.Add(widget.NewSeparator())
	}
	mergeAll := widget.NewButtonWithIcon(fmt.Sprintf("Merge all %d", len(groups)), iconMerge, nil)
	if len(groups) == 1 {
		mergeAll.Hide()
	}
	mergeAll.Importance = widget.HighImportance
	mergeAll.OnTapped = func() {
		u.confirmCB("Merge all?", fmt.Sprintf("Merge %d sets? Each keeps the fullest card and adds the others' details; the extra cards go to the Trash.", len(groups)), "Merge All", false, func(ok bool) {
			if !ok {
				return
			}
			n := 0
			for _, g := range groups {
				if merge(g) {
					n++
				}
			}
			u.save()
			d.Hide()
			u.flash(fmt.Sprintf("Merged %d sets of duplicates", n))
		}, u.win)
	}
	note := caption("Merge… lets you choose the name, job and photo to keep; Merge All keeps the fullest card's. Every number, email and note is kept, and the extra cards go to the Trash.")
	note.Wrapping = fyne.TextWrapWord
	if len(groups) == 0 {
		mergeAll.Hide()
	}
	content := container.NewBorder(note, container.NewHBox(layout.NewSpacer(), mergeAll), nil, nil, container.NewVScroll(box))
	title := "Look for Duplicates"
	d = dialog.NewCustom(title, "Close", content, u.win)
	d.Resize(fyne.NewSize(640, 520))
	d.Show()
}

// ---------- clean up ----------

func (u *ui) cleanup() { u.cleanupFor(u.store.Contacts, "") }

// cleanupFor shows Clean Up for some contacts (all of them, or the ones just imported).
// Suggestions are grouped: confident fixes and duplicates are ticked; wrong-looking numbers are
// ticked only when clearly junk; numbers whose country is unclear get a country picker.
func (u *ui) cleanupFor(cs []*Contact, intro string) {
	changes := PlanCleanup(cs, u.cfg.CountryCode)
	scope := u.scopeName(cs)
	if len(changes) == 0 {
		if intro == "" {
			showInfo("Nothing to clean up in "+scope, "Numbers, emails, names and links already look right. Nothing was changed.", u.win)
		}
		return
	}
	type row struct {
		check  *widget.Check
		choose *widget.Select
	}
	rows := make([]row, len(changes))
	sections := []struct {
		kind  int
		title string
		help  string
	}{
		{ckFix, "Fixes", "00 → +, country codes, extra zeros, email typos, values in the wrong field"},
		{ckDuplicate, "Duplicates on the same card", "the same number or email listed twice"},
		{ckWrong, "Looks wrong — remove?", "clear junk is ticked; numbers that might be real are not"},
		{ckUnsure, "Which country?", "pick the country for each number, or leave it as it is"},
	}
	box := container.NewVBox()
	for _, sec := range sections {
		var idx []int
		for i, ch := range changes {
			if ch.Kind == sec.kind {
				idx = append(idx, i)
			}
		}
		if len(idx) == 0 {
			continue
		}
		head := widget.NewLabelWithStyle(fmt.Sprintf("%s (%d)", sec.title, len(idx)), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		var all *widget.Button
		if sec.kind != ckUnsure {
			idx := idx
			all = widget.NewButton("Tick all", nil)
			all.Importance = widget.LowImportance
			all.OnTapped = func() {
				tick := false
				for _, i := range idx {
					tick = tick || !rows[i].check.Checked
				}
				for _, i := range idx {
					rows[i].check.SetChecked(tick)
				}
			}
			box.Add(container.NewBorder(nil, nil, nil, all, head))
		} else {
			box.Add(head)
		}
		box.Add(caption(sec.help))
		for _, i := range idx {
			ch := changes[i]
			who := widget.NewLabel(ch.Contact.DisplayName())
			who.TextStyle.Bold = true
			who.Truncation = fyne.TextTruncateEllipsis
			why := caption(ch.Reason)
			if sec.kind == ckUnsure {
				opts := []string{"Leave as is"}
				for _, o := range ch.Options {
					opts = append(opts, countryName(o)+":  "+FormatPhone(o, u.cfg.CountryCode))
				}
				sel := widget.NewSelect(opts, nil)
				sel.SetSelectedIndex(0)
				rows[i].choose = sel
				box.Add(container.NewBorder(nil, nil, container.NewGridWrap(fyne.NewSize(200, 36), who), nil,
					container.NewVBox(container.NewBorder(nil, nil, widget.NewLabel(ch.Old), nil, sel), why)))
				continue
			}
			chk := widget.NewCheck(ch.What(), nil)
			chk.SetChecked(ch.Default)
			rows[i].check = chk
			box.Add(container.NewBorder(nil, nil, container.NewGridWrap(fyne.NewSize(200, 36), who), nil, container.NewVBox(chk, why)))
		}
		box.Add(widget.NewSeparator())
	}
	top := fmt.Sprintf("Local numbers are taken as +%s (your country in Settings) unless the card points elsewhere — its address, other numbers or email domain. A full backup is saved before anything changes.", u.cfg.CountryCode)
	if intro != "" {
		top = intro + "\n" + top
	}
	note := caption(top)
	note.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustomConfirm(fmt.Sprintf("Clean Up %s — %d %s", scope, len(changes), pluralWords(len(changes), "suggestion", "suggestions")), "Apply", "Skip",
		container.NewBorder(note, nil, nil, nil, container.NewVScroll(box)), func(ok bool) {
			if !ok {
				return
			}
			backup, err := u.store.SnapshotBackup("before-cleanup")
			if err != nil {
				showErr(fmt.Errorf("couldn't save a backup first, so nothing was changed: %w", err), u.win)
				return
			}
			n := 0
			touched := map[*Contact]bool{}
			for i, ch := range changes {
				switch {
				case rows[i].choose != nil:
					if k := rows[i].choose.SelectedIndex(); k > 0 {
						ch.Apply(ch.Options[k-1])
						touched[ch.Contact] = true
						n++
					}
				case rows[i].check != nil && rows[i].check.Checked:
					ch.Apply("")
					touched[ch.Contact] = true
					n++
				}
			}
			for c := range touched {
				u.store.Put(c)
			}
			u.cfg.LastCleanup = time.Now()
			_ = u.cfg.Save(u.store.Dir())
			u.save()
			u.flash(fmt.Sprintf("Clean Up applied to %s — %d %s (backup %s saved first)", scope, n, pluralWords(n, "fix", "fixes"), filepath.Base(backup)))
		}, u.win)
	d.Resize(fyne.NewSize(860, 620))
	d.Show()
}

// ---------- trash ----------

func (u *ui) trash() {
	if len(u.store.Trash) == 0 {
		showInfo("Trash is empty", "Deleted contacts stay here for 30 days.", u.win)
		return
	}
	box := container.NewVBox()
	var d *dialog.CustomDialog
	var fill func()
	fill = func() {
		box.Objects = nil
		for i := len(u.store.Trash) - 1; i >= 0; i-- {
			t := u.store.Trash[i]
			left := 30 - int(time.Since(t.Deleted).Hours()/24)
			lbl := widget.NewLabel(t.Contact.DisplayName())
			when := caption(fmt.Sprintf("deleted %s · gone in %d days", Ago(t.Deleted, time.Now()), left))
			restore := widget.NewButtonWithIcon("Restore", theme.ContentUndoIcon(), func() {
				u.store.Restore(t.Contact.ID)
				u.selectedID = t.Contact.ID
				u.save()
				if len(u.store.Trash) == 0 {
					d.Hide()
				}
				fill()
			})
			box.Add(container.NewBorder(nil, nil, nil, restore, container.NewVBox(lbl, when)))
		}
		box.Refresh()
	}
	fill()
	empty := widget.NewButtonWithIcon("Empty Trash", theme.DeleteIcon(), func() {
		u.confirmCB("Empty the Trash?", "These contacts will be gone for good.", "Empty Trash", true, func(ok bool) {
			if ok {
				u.store.EmptyTrash()
				u.save()
				d.Hide()
			}
		}, u.win)
	})
	empty.Importance = widget.DangerImportance
	d = dialog.NewCustom("Trash", "Close", container.NewBorder(nil, container.NewHBox(layout.NewSpacer(), empty), nil, nil, container.NewVScroll(box)), u.win)
	d.Resize(fyne.NewSize(520, 460))
	d.Show()
}

// ---------- groups ----------

func (u *ui) newGroupDialog() {
	e := widget.NewEntry()
	e.SetPlaceHolder("Family, Work, Clients…")
	dialog.ShowForm("New Group", "Create", "Cancel", []*widget.FormItem{widget.NewFormItem("Name", e)}, func(ok bool) {
		name := strings.TrimSpace(e.Text)
		if !ok || name == "" {
			return
		}
		u.store.AddGroup(name)
		for _, g := range u.store.Groups() { // "work" when "Work" exists means Work
			if strings.EqualFold(g, name) {
				name = g
			}
		}
		u.group = "g:" + name
		u.save()
		u.flash("Created " + name + " — add people to it from their Edit page")
	}, u.win)
	u.win.Canvas().Focus(e)
}

func (u *ui) groupMenu(anchor fyne.CanvasObject) {
	items := []*fyne.MenuItem{fyne.NewMenuItem("New Group…", u.newGroupDialog), fyne.NewMenuItem("New Smart Group from Search…", u.saveSmartGroup)}
	if strings.HasPrefix(u.group, "q:") {
		g := u.group[2:]
		items = append(items, fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Edit “"+g+"”…", func() {
				for _, sg := range u.cfg.SmartGroups {
					if sg.Name == g {
						u.deleteSmartGroup(g)
						u.search.SetText(sg.Query)
						u.saveSmartGroup()
					}
				}
			}),
			fyne.NewMenuItem("Delete “"+g+"”", func() { u.deleteSmartGroup(g) }))
	}
	if strings.HasPrefix(u.group, "g:") {
		g := u.group[2:]
		items = append(items, fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Rename “"+g+"”…", func() {
				e := widget.NewEntry()
				e.SetText(g)
				dialog.ShowForm("Rename Group", "Rename", "Cancel", []*widget.FormItem{widget.NewFormItem("Name", e)}, func(ok bool) {
					n := strings.TrimSpace(e.Text)
					if ok && n != "" && n != g {
						u.store.RenameGroup(g, n)
						u.group = "g:" + n
						u.save()
					}
				}, u.win)
			}),
			fyne.NewMenuItem("Export “"+g+"” as vCard…", func() { u.exportVCF(u.filtered, g) }),
			fyne.NewMenuItem("Delete “"+g+"”…", func() {
				u.confirmCB("Delete group “"+g+"”?", "The people in it stay in your contacts; only the group goes.", "Delete Group", true, func(ok bool) {
					if ok {
						u.store.RemoveGroup(g)
						u.group = grpAll
						u.save()
					}
				}, u.win)
			}))
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), u.win.Canvas(), pos.Add(fyne.NewPos(0, -float32(len(items))*34)))
}

// ---------- settings & help ----------

func (u *ui) settings() {
	cc := widget.NewEntry()
	cc.SetText(u.cfg.CountryCode)
	sortBy := widget.NewRadioGroup([]string{"First name", "Last name"}, nil)
	sortBy.Horizontal = true
	sortBy.SetSelected(map[bool]string{false: "First name", true: "Last name"}[u.cfg.SortByLast])
	key := widget.NewPasswordEntry()
	key.SetText(u.cfg.OpenRouterKey)
	key.SetPlaceHolder("sk-or-… (optional)")
	model := widget.NewSelectEntry([]string{DefaultAIModel, "anthropic/claude-sonnet-4.5", "openai/gpt-5-mini", "google/gemini-2.5-flash"})
	model.SetText(u.cfg.AIModel)
	allow := widget.NewCheck("Allow Smart Add to send pasted text to OpenRouter (it leaves this Mac)", nil)
	allow.SetChecked(u.cfg.AllowCloudAI)
	test := widget.NewButton("Test AI", nil)
	syncAI := func(on bool) {
		for _, w := range []fyne.Disableable{key, model, test} {
			if on {
				w.Enable()
			} else {
				w.Disable()
			}
		}
	}
	allow.OnChanged = syncAI
	syncAI(u.cfg.AllowCloudAI)
	result := caption("")
	result.Wrapping = fyne.TextWrapWord
	test.OnTapped = func() {
		result.SetText("Testing…")
		k, m, c := key.Text, model.Text, strings.TrimSpace(cc.Text)
		go func() {
			got, _, err := ParseAI(context.Background(), k, m, "Jane Doe, Head of Design at Example Co, +1 415 555 0100", c)
			fyne.Do(func() {
				switch {
				case err != nil:
					result.SetText("Didn't work: " + err.Error())
				case got.First == "":
					result.SetText("The model answered but missed the name; try another model.")
				default:
					result.SetText("Works — it read “" + got.DisplayName() + ", " + got.Title + "”.")
				}
			})
		}()
	}
	keyNote := caption("Off (default): Smart Add reads text on this Mac only and the app never uses the network. " +
		"The key is stored in config.json in the data folder (not the Keychain), readable only by your user.")
	keyNote.Wrapping = fyne.TextWrapWord
	folder := widget.NewButtonWithIcon("Show Data Folder", theme.FolderOpenIcon(), func() { RevealInFinder(u.store.Dir() + "/contacts.json") })
	autoAct := widget.NewCheck("Update “last talked” from Messages, calls and Mail when the app opens", nil)
	autoAct.SetChecked(u.cfg.AutoActivity)
	notify := widget.NewCheck("Remind me of birthdays and people to reach out to", nil)
	notify.SetChecked(!u.cfg.NoNotify)
	coverChk := widget.NewCheck("Show the closed organiser when the app opens", nil)
	coverChk.SetChecked(!u.cfg.NoCover)
	openAt := widget.NewRadioGroup([]string{"The last page I was on", "The first page (All Contacts)"}, nil)
	openAt.SetSelected(map[bool]string{false: "The last page I was on", true: "The first page (All Contacts)"}[u.cfg.OpenAtFirst])
	// Laid out like a Mac settings window: titled groups on a light panel, the whole thing
	// scrolling, and Cancel / Save always in their own row at the bottom.
	section := func(title string, items ...*widget.FormItem) fyne.CanvasObject {
		head := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		bg := canvas.NewRectangle(color.NRGBA{0xFF, 0xFF, 0xFF, 0x80})
		bg.CornerRadius = 10
		bg.StrokeColor = color.NRGBA{0, 0, 0, 0x18}
		bg.StrokeWidth = 1
		return container.NewVBox(head, container.NewStack(bg, container.NewPadded(widget.NewForm(items...))))
	}
	privacy := caption(privacyText(u.store.Dir()))
	privacy.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(
		section("General",
			widget.NewFormItem("Your country code", cc),
			widget.NewFormItem("Sort by", sortBy),
			widget.NewFormItem("History", autoAct),
			widget.NewFormItem("Reminders", notify)),
		section("Organiser",
			widget.NewFormItem("When the app opens", coverChk),
			widget.NewFormItem("Open at", openAt)),
		section("Cloud AI",
			widget.NewFormItem("Cloud AI", allow),
			widget.NewFormItem("OpenRouter key", key),
			widget.NewFormItem("AI model", model),
			widget.NewFormItem("", container.NewVBox(keyNote, container.NewHBox(test, result)))),
		section("Privacy",
			widget.NewFormItem("", container.NewVBox(privacy, container.NewHBox(folder, layout.NewSpacer())))),
	)
	d := dialog.NewCustomConfirm("Settings", "Save", "Cancel", container.NewVScroll(container.NewPadded(content)), func(ok bool) {
		if !ok {
			return
		}
		if code := digitsOnly(cc.Text); code != "" && len(code) <= 4 {
			u.cfg.CountryCode = code
		} else {
			showInfo("Country code not changed", "The country code should be digits only, like 965. The other settings were saved.", u.win)
		}
		u.cfg.SortByLast = sortBy.Selected == "Last name"
		u.cfg.OpenRouterKey = strings.TrimSpace(key.Text)
		u.cfg.AllowCloudAI = allow.Checked
		u.cfg.AutoActivity = autoAct.Checked
		u.cfg.NoNotify = !notify.Checked
		u.cfg.NoCover = !coverChk.Checked
		u.cfg.OpenAtFirst = openAt.Selected == "The first page (All Contacts)"
		u.cfg.AIModel = strings.TrimSpace(model.Text)
		if u.cfg.AIModel == "" {
			u.cfg.AIModel = DefaultAIModel
		}
		if err := u.cfg.Save(u.store.Dir()); err != nil {
			showErr(err, u.win)
		}
		u.refreshAll()
		u.flash("Settings saved")
	}, u.win)
	d.SetConfirmImportance(widget.HighImportance)
	trackDialog(d, d.Confirm)
	d.Resize(fyne.NewSize(700, 640))
	d.Show()
}

func (u *ui) help() {
	text := `**Search Filter.** The first tab opens a form on the page: fill in any of Name, Company, Country, City or Other, and only the people matching all of them are shown. Save as Smart Group keeps it in the sidebar.

**The organiser.** It opens closed: click the cover or press any key. The coloured tabs on the left are your lists and groups; the A–Z tabs down the right edge show only the people filed under those letters (All shows everyone, ع holds Arabic names, # the rest). Click the strap on the right (or press ⇧⌘W) to close it; next time it opens at the same page, unless Settings → Open at says the first page. View → Filofax Look switches to the plain Mac look and back. Each phone number shows its country's flag.

**Finding people.** Type in the search field: names (Arabic or English, with or without hamza and accents), part of a phone number, a company, a note. ↑ and ↓ move through the results, Return edits, Esc clears. View → All Contacts, Favorites, Upcoming Birthdays and Recently Added are ⌘1 to ⌘4.

**Reaching people.** The round buttons under the name follow what the card holds: message, call, WhatsApp, mail, FaceTime, and LinkedIn, Telegram, Instagram, Messenger, X, Signal, Snapchat, TikTok, GitHub or Skype when there's a link or handle. *More* has number-based options. The app remembers when you last contacted someone.

**Editing.** Every card is editable where it is: click any field and type (⌘L or Return jumps to the first field). Save and Revert appear at the bottom once something changes, ⌘S saves, and if you move to another person, close the window or quit with unsaved changes, you're asked whether to update the card (Save, Don't Save or Cancel). Right-click a person in the list for Favorites, Connect, Share and Delete. Share (the arrow button) copies the card as text, saves a vCard or copies it to Apple Contacts.

**Last talked.** Card → Update Last Talked (⇧⌘U) reads Messages, call history and Mail on this Mac (it needs Full Disk Access, which it asks for) and shows on each card when you were last in touch and how. For WhatsApp, export a chat (⋯ → Export Chat) and drop it on the window. Card → People Not in Contacts lists people you write to or call often who aren't in your contacts. Nothing leaves the Mac.

**Keeping in touch.** Keep in Touch at the bottom of a card sets a reminder (every week … every year). View → Reach Out (⌘5) lists who's due, longest first, and a notification reminds you once a day, along with birthdays. Note (⌘J) adds a dated note to the card's timeline; "How we met" is on the Edit page.

**Quick Find.** The person icon in the menu bar opens Quick Find (also ⇧⌘O): type a name, press Return to copy the number. Your favorites are in that menu too, with call and message.

**Search filters.** company:boubyan · title:engineer · group:family · source:linkedin · city:salmiya · country:kuwait · code:965 · has:email · no:phone · talked:<30d · talked:>1y · is:favorite · is:due — and a minus in front reverses one (-group:work). Save any search as a Smart Group in the sidebar.

**The organiser does what it looks like.** The pen in its loop writes a new card. The turned-up corner at the bottom of each page turns to the previous or next letter. The pocket inside the front cover takes business cards: click it, or drop a photo of a card anywhere on the window. The strap closes the book. Each lifts a little under the pointer and says what it does. Cards are printed pages: click a ruled line and write; the blue box shows where you're writing. When the open card changes it gets an UPDATED stamp and a pencil note of what changed. The To Tidy tabs (with a red flag) are cards waiting for you to fix something.

**What a command acts on.** The Card menu has three parts, each headed by its target: *This Card* (the person open on the right, by name), *Clipped Cards* (the people you've clipped together, with the count) and *Whole Address Book* (every card, with the count). Commands that can't act right now are dimmed. Right-click a person and the menu starts with their name.

**Seeing what changed.** After any change a banner at the bottom says what was done (“Clean Up applied to 3 Clipped Cards — 1 fix”), with *Show Changes* (each card, how it was in grey and how it is now in bold) and *Undo*. Cards that just changed have a blue dot in the list for a short while.

**Needs Attention** lists cards with something to tidy (a number without its country code, a typo in an email…); the card shows a Review button.

**Tidy notes with AI.** Card → Tidy Jumbled Notes with AI finds notes that are run together, full of export codes or repeats, and rewrites them readably, keeping every fact. It only runs with Cloud AI on (the note text goes to your OpenRouter model), asks first, and shows each rewrite before and after; one that seems to have lost a number, email or date isn't ticked.

**Merging.** Merge… in Look for Duplicates or on the clipped cards' note shows where the cards disagree (name, job, birthday, photo) so you pick; every number, email and note is kept.

**Smart Add** (⌥⌘N). Paste an email signature, a business card's text or a WhatsApp contact message, or choose a business card photo. It's read privately on this Mac unless you switch on Cloud AI in Settings. You check the result before it's saved.

**Bring Everything In.** File → Bring Everything In is a checklist of every source, then clean-up and duplicates; each step is ticked once done.

**Importing (⌘O).** File → Import brings people in from the Mac's Contacts app (and iPhone via iCloud), other phones (.vcf, Bluetooth or AirDrop), Gmail, Outlook, Yahoo or iCloud, LinkedIn, Telegram, Facebook, Snapchat, any spreadsheet (.csv or .xlsx, English or Arabic headers) and notes. You can also drag files onto the window. People you already have are recognised by number, email, profile link or name, in Arabic or English, and updated instead of duplicated. Anyone matched by name only is listed for you to check. Each phone, email, link and address shows where it came from, and the Timeline on each card records what happened without touching your notes.

**Undo (⌘Z, ⇧⌘Z).** Every change can be undone: edits, imports, clean-ups, merges, deletes. Edit → History lists this session's changes and the saved restore points.

**Many at once: paper clips.** Every card in the list has a paper clip at its left edge. Click it to clip that card; clip more and they gather under a yellow sticky note with what you can do to all of them. ⌘-click and Shift-click (a range) work too, as do ⌘A for everyone shown (⇧⌘A unclips all) and View → Clip Cards Together (⇧⌘S). Take Clips Off ends it. Right-click a clipped card — or use More… on the note — for Clean Up Cards, Tidy Notes with AI, Look for Duplicates Among These, Merge, Add to / Remove from Group, Favorites, Keep in Touch, Copy as Text, Export, and Delete. Deleting several cards asks twice, and Return never confirms a delete: the red button has to be clicked.

**Duplicates and clean-up.** Card → Look for Duplicates (⇧⌘D) merges the same person on several cards and removes repeats inside cards. Card → Clean Up writes every number the same way (+965 9999 8888), fixes emails and links, and tidies notes.

**Groups.** File → New Group (⇧⌘N), then tick groups on a card's Edit page. A new card made while a group is open joins it.

**Connections (six degrees).** Card → Connect to… (⌘K) links two people as friends, family, colleagues and so on. Card → How Are They Connected? (⇧⌘K) finds the shortest chain between two people, up to six steps.

**Calendar log (⇧⌘L).** Reads the Mac Calendar app or an .ics file; each card lists the meetings the person was in or mentioned in, and "last met".

**Photos and business cards.** Click the round picture to set the person's photo. A business card photo is read on this Mac and kept on the card.

**Names in two languages.** Each card can hold the name in the other language (محمد المطيري / Mohammed Al-Mutairi); Edit → Suggest fills common names.

**Privacy.** Your contacts live only on this Mac, in a folder only your user account can open, kept out of Spotlight. The app goes online only for Cloud AI (if you switch it on) and Help → Check for Updates.

**Safety.** Deleted cards stay in the Trash (View → Trash) for 30 days. A backup is kept for each of the last 30 days. File → Back Up To… saves everything to a drive you choose, encrypted if you give a password; File → Restore from Backup… brings one back. Undo history is kept when you quit.

**Keys:** ⌘N new card · ⇧⌘N new group · ⌥⌘N Smart Add · ⌘O import · ⌘F find · ⌘L type in the card · ⌘S save · ⌘⌫ delete · ⌘Z undo · ⇧⌘Z redo · ⌘A select all · ⌘1–4 lists · ⇧⌘F favorite · ⌘K connect · ⇧⌘K how connected · ⇧⌘D duplicates · ⇧⌘M merge · ⇧⌘L calendar · ⌘5 reach out · ⌘6 needs attention · ⌘J note · ⇧⌘U update last talked · ⇧⌘O quick find · ⌘, settings`
	r := widget.NewRichTextFromMarkdown(text)
	r.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustom("Smart Contacts Help", "Close", container.NewVScroll(r), u.win)
	d.Resize(fyne.NewSize(640, 560))
	d.Show()
}

// privacyText describes where the data is and what can leave the Mac.
func privacyText(dir string) string {
	return "Contacts, photos and 30 daily backups are stored only on this Mac, in\n" + dir +
		"\n(only your user can open it; kept out of Spotlight; not in iCloud Drive).\n" +
		"The app uses no network and no accounts unless Cloud AI is on, or you choose\n" +
		"Check for Updates (which asks GitHub for the newest build and sends no contacts).\n" +
		"FileVault (System Settings → Privacy & Security) encrypts it on disk."
}
