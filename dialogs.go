package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (u *ui) mainMenu() *fyne.MainMenu {
	cmd := func(k fyne.KeyName) fyne.Shortcut {
		return &desktop.CustomShortcut{KeyName: k, Modifier: fyne.KeyModifierShortcutDefault}
	}
	cmdShift := func(k fyne.KeyName) fyne.Shortcut {
		return &desktop.CustomShortcut{KeyName: k, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift}
	}
	item := func(label string, sc fyne.Shortcut, f func()) *fyne.MenuItem {
		m := fyne.NewMenuItem(label, f)
		m.Shortcut = sc
		return m
	}
	current := func() *Contact { return u.store.Get(u.selectedID) }

	file := fyne.NewMenu("File",
		item("New Contact", cmd(fyne.KeyN), func() { u.editContact(nil) }),
		item("Smart Add from Text…", cmdShift(fyne.KeyN), u.smartAdd),
		item("Save Contact", cmd(fyne.KeyS), func() {
			if u.ed != nil {
				u.ed.save()
			}
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Import from Mac Contacts…", u.importMac),
		item("Import vCard File…", cmd(fyne.KeyO), u.importVCF),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Export Shown Contacts as vCard…", func() { u.exportVCF(u.filtered, u.groupTitle()) }),
		fyne.NewMenuItem("Export Shown Contacts as CSV…", u.exportCSV),
		fyne.NewMenuItem("Copy Shown Contacts to Apple Contacts…", func() {
			if len(u.filtered) == 0 {
				return
			}
			u.sendToMacContacts(u.filtered)
		}),
	)
	edit := fyne.NewMenu("Contact",
		item("Find", cmd(fyne.KeyF), func() { u.win.Canvas().Focus(u.search) }),
		item("Edit Contact", cmd(fyne.KeyE), func() {
			if c := current(); c != nil && u.ed == nil {
				u.editContact(c)
			}
		}),
		item("Toggle Favorite", cmdShift(fyne.KeyF), func() {
			if c := current(); c != nil && u.ed == nil {
				c.Favorite = !c.Favorite
				u.store.Put(c)
				u.save()
			}
		}),
		item("Delete Contact", cmd(fyne.KeyBackspace), func() {
			// The menu catches ⌘⌫ before any text field does. In the search box it means
			// "clear what I typed", and while editing it must never delete the contact.
			if u.win.Canvas().Focused() == u.search {
				u.search.SetText("")
				return
			}
			if _, typing := u.win.Canvas().Focused().(*widget.Entry); typing {
				return
			}
			if c := current(); c != nil && u.ed == nil {
				u.deleteContact(c)
			}
		}),
	)
	tools := fyne.NewMenu("Tools",
		fyne.NewMenuItem("Find Duplicates…", u.duplicates),
		fyne.NewMenuItem("Clean Up Numbers and Names…", u.cleanup),
		fyne.NewMenuItem("Trash…", u.trash),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Show Data Folder", func() { RevealInFinder(u.store.Dir() + "/contacts.json") }),
		item("Settings…", cmd(fyne.KeyComma), u.settings),
	)
	help := fyne.NewMenu("Help", fyne.NewMenuItem("Smart Contacts Help", u.help))
	return fyne.NewMainMenu(file, edit, tools, help)
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
	content := container.NewBorder(nil, container.NewVBox(note, progress), nil, nil, in)
	var d *dialog.ConfirmDialog
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

func (u *ui) runSmartAdd(text string) {
	cc := u.cfg.CountryCode
	if !u.cloudAI() {
		u.finishSmartAdd(ParseLocal(text, cc), true, "Read privately on this Mac — check the details, then Save.")
		return
	}
	prog := dialog.NewCustomWithoutButtons("Reading…", container.NewVBox(widget.NewLabel("Asking "+u.cfg.AIModel+"…"), widget.NewProgressBarInfinite()), u.win)
	prog.Show()
	go func() {
		c, confident, err := ParseAI(context.Background(), u.cfg.OpenRouterKey, u.cfg.AIModel, text, cc)
		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				u.finishSmartAdd(ParseLocal(text, cc), true, "The AI didn't answer ("+err.Error()+"), so this was read on this Mac — check it, then Save.")
				return
			}
			msg := "Filled in by AI — check the details, then Save."
			if !confident {
				msg = "The text may describe more than one person; this is the AI's best single contact. Check it, then Save."
			}
			u.finishSmartAdd(c, confident, msg)
		})
	}()
}

// cloudAI is true only when the user has explicitly allowed sending text off this Mac AND set a key.
func (u *ui) cloudAI() bool {
	return u.cfg.AllowCloudAI && strings.TrimSpace(u.cfg.OpenRouterKey) != ""
}

func (u *ui) finishSmartAdd(c *Contact, _ bool, banner string) {
	c.Tidy()
	if x := NewDupIndex(u.store.Contacts, u.cfg.CountryCode).Find(c); x != nil {
		banner += "\nHeads-up: this looks like " + x.DisplayName() + ", who is already in your contacts."
	}
	u.leaveEditor(func() { u.openEditor(nil, c, banner) }, nil)
}

// ---------- import ----------

func (u *ui) importVCF() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil || r == nil {
			return
		}
		defer r.Close()
		b, err := io.ReadAll(io.LimitReader(r, 512<<20))
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		cards, err := ParseVCards(bytes.NewReader(b))
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		u.importCards(cards, r.URI().Name())
	}, u.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".vcf", ".vcard", ".VCF"}))
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

func (u *ui) importMac() {
	prog := dialog.NewCustomWithoutButtons("Reading Mac Contacts…", container.NewVBox(
		widget.NewLabel("If macOS asks, allow Smart Contacts to use Contacts."), widget.NewProgressBarInfinite()), u.win)
	prog.Show()
	go func() {
		cards, err := ReadMacContacts()
		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				dialog.ShowError(err, u.win)
				return
			}
			u.importCards(cards, "Mac Contacts")
		})
	}()
}

// importCards shows what will happen, then adds the new cards. Cards that match an existing
// contact (same phone, email or name) are skipped unless the user asks for all of them.
func (u *ui) importCards(cards []Card, source string) {
	if len(cards) == 0 {
		dialog.ShowInformation("Nothing found", "No contacts were found in "+source+".", u.win)
		return
	}
	idx := NewDupIndex(u.store.Contacts, u.cfg.CountryCode)
	var fresh, known []Card
	for _, card := range cards {
		if idx.Find(card.Contact) != nil {
			known = append(known, card)
		} else {
			fresh = append(fresh, card)
		}
	}
	add := func(cs []Card) {
		for _, card := range cs {
			c := card.Contact
			if len(card.Photo) > 0 && CanDecodeImage(card.Photo) {
				_ = u.store.SetPhoto(c, ShrinkPhoto(card.Photo, 512))
			}
			u.store.Put(c)
		}
		u.save()
		u.flash(fmt.Sprintf("Imported %d contacts from %s", len(cs), source))
	}
	if len(known) == 0 {
		dialog.ShowConfirm("Import", fmt.Sprintf("Add %d contacts from %s?", len(fresh), source), func(ok bool) {
			if ok {
				add(fresh)
			}
		}, u.win)
		return
	}
	msg := widget.NewLabel(fmt.Sprintf("%s has %d contacts.\n\n%d are new.\n%d look like contacts you already have (same phone, email or name).",
		source, len(cards), len(fresh), len(known)))
	newOnly := widget.NewButton(fmt.Sprintf("Add the %d new ones", len(fresh)), nil)
	newOnly.Importance = widget.HighImportance
	all := widget.NewButton("Add all (merge later with Find Duplicates)", nil)
	cancel := widget.NewButton("Cancel", nil)
	d := dialog.NewCustomWithoutButtons("Import", container.NewVBox(msg, newOnly, all, cancel), u.win)
	newOnly.OnTapped = func() { d.Hide(); add(fresh) }
	all.OnTapped = func() { d.Hide(); add(append(fresh, known...)) }
	cancel.OnTapped = d.Hide
	if len(fresh) == 0 {
		newOnly.Disable()
	}
	d.Show()
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
				dialog.ShowError(err, u.win)
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
					dialog.ShowError(err, u.win)
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

func (u *ui) exportCSV() {
	cs := u.filtered
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
				dialog.ShowError(err, u.win)
				return
			}
			u.flash(fmt.Sprintf("Saved %d contacts to %s", len(cs), w.URI().Name()))
		})
	}, u.win)
	d.SetFileName(safeName(u.groupTitle()) + ".csv")
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

func (u *ui) duplicates() {
	groups := FindDuplicates(u.store.Contacts, u.cfg.CountryCode)
	if len(groups) == 0 {
		dialog.ShowInformation("No duplicates", "Every contact looks unique — no shared phones, emails or names.", u.win)
		return
	}
	box := container.NewVBox()
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
		Merge(keep, others, u.cfg.CountryCode)
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
		btn := widget.NewButtonWithIcon("Merge", iconMerge, nil)
		btn.OnTapped = func() {
			if merge(g) {
				u.save()
				btn.SetText("Merged")
			} else {
				btn.SetText("Already merged")
			}
			btn.Disable()
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
		dialog.ShowConfirm("Merge all?", fmt.Sprintf("Merge %d sets? Each keeps the fullest card and adds the others' details; the extra cards go to the Trash.", len(groups)), func(ok bool) {
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
	note := caption("Merging keeps the card with the most details and copies in the rest. The extra cards go to the Trash.")
	note.Wrapping = fyne.TextWrapWord
	content := container.NewBorder(note, container.NewHBox(layout.NewSpacer(), mergeAll), nil, nil, container.NewVScroll(box))
	title := fmt.Sprintf("%d sets of possible duplicates", len(groups))
	if len(groups) == 1 {
		title = "1 possible duplicate"
	}
	d = dialog.NewCustom(title, "Close", content, u.win)
	d.Resize(fyne.NewSize(640, 520))
	d.Show()
}

// ---------- clean up ----------

func (u *ui) cleanup() {
	changes := PlanCleanup(u.store.Contacts, u.cfg.CountryCode)
	if len(changes) == 0 {
		dialog.ShowInformation("All tidy", "Numbers, names, emails and links already look consistent.", u.win)
		return
	}
	box := container.NewVBox()
	checks := make([]*widget.Check, len(changes))
	for i, ch := range changes {
		checks[i] = widget.NewCheck(ch.Contact.DisplayName()+":   "+ch.What, nil)
		checks[i].SetChecked(true)
		box.Add(checks[i])
	}
	note := caption(fmt.Sprintf("Phones are written as +%s… for local numbers (change the country in Settings). Names are only re-capitalised when typed ALL CAPS or all lower case.", u.cfg.CountryCode))
	note.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustomConfirm(fmt.Sprintf("Clean up — %d suggestions", len(changes)), "Apply Ticked", "Cancel",
		container.NewBorder(note, nil, nil, nil, container.NewVScroll(box)), func(ok bool) {
			if !ok {
				return
			}
			n := 0
			for i, ch := range changes {
				if checks[i].Checked {
					ch.Apply()
					ch.Contact.Updated = time.Now()
					n++
				}
			}
			u.save()
			u.flash(fmt.Sprintf("Applied %d changes", n))
		}, u.win)
	d.Resize(fyne.NewSize(680, 520))
	d.Show()
}

// ---------- trash ----------

func (u *ui) trash() {
	if len(u.store.Trash) == 0 {
		dialog.ShowInformation("Trash is empty", "Deleted contacts stay here for 30 days.", u.win)
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
		dialog.ShowConfirm("Empty the Trash?", "These contacts will be gone for good.", func(ok bool) {
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
	items := []*fyne.MenuItem{fyne.NewMenuItem("New Group…", u.newGroupDialog)}
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
				dialog.ShowConfirm("Delete group “"+g+"”?", "The people in it stay in your contacts; only the group goes.", func(ok bool) {
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
	form := []*widget.FormItem{
		widget.NewFormItem("Your country code", cc),
		widget.NewFormItem("Sort by", sortBy),
		widget.NewFormItem("Cloud AI", allow),
		widget.NewFormItem("OpenRouter key", key),
		widget.NewFormItem("AI model", model),
		widget.NewFormItem("", container.NewVBox(keyNote, container.NewHBox(test), result)),
		widget.NewFormItem("Privacy", container.NewVBox(caption(privacyText(u.store.Dir())), container.NewHBox(folder))),
	}
	cc.Validator = func(s string) error {
		if d := digitsOnly(s); d == "" || len(d) > 4 || d != strings.TrimPrefix(strings.TrimSpace(s), "+") {
			return errors.New("digits only, like 965")
		}
		return nil
	}
	d := dialog.NewForm("Settings", "Save", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		u.cfg.CountryCode = digitsOnly(cc.Text)
		u.cfg.SortByLast = sortBy.Selected == "Last name"
		u.cfg.OpenRouterKey = strings.TrimSpace(key.Text)
		u.cfg.AllowCloudAI = allow.Checked
		u.cfg.AIModel = strings.TrimSpace(model.Text)
		if u.cfg.AIModel == "" {
			u.cfg.AIModel = DefaultAIModel
		}
		if err := u.cfg.Save(u.store.Dir()); err != nil {
			dialog.ShowError(err, u.win)
		}
		u.refreshAll()
	}, u.win)
	d.Resize(fyne.NewSize(620, 520))
	d.Show()
}

func (u *ui) help() {
	text := `**Finding people.** Type in the search box: names (Arabic or English, with or without hamza and accents), part of a phone number, a company, a note. ↑ and ↓ move through the results, Return edits, Esc clears.

**Smart Add** (⌘⇧N). Paste an email signature, a business card's text or a WhatsApp contact message. By default it is read privately on this Mac. Only if you switch on Cloud AI in Settings (and add an OpenRouter key) is the pasted text sent to an AI model. You always check the result before it's saved.

**Quick actions.** Call opens FaceTime/iPhone calling, Message opens Messages, WhatsApp opens the chat, Email opens Mail. The app remembers when you last contacted someone.

**Groups.** Tick groups on a contact's Edit page, or make one with + Group. A new contact made while a group is open joins that group.

**Birthdays.** Type a birthday as 1990-03-14, 14/03/1990, or 14/03 when you don't know the year. The Birthdays list shows the next 30 days and the age they turn.

**Bringing contacts in.** File → Import from Mac Contacts reads the built-in Contacts app (macOS asks permission once). Import vCard reads .vcf files from Google, Outlook or a phone. Contacts that already exist are spotted and skipped.

**Tidying.** Tools → Find Duplicates merges cards that share a phone, email or name. Tools → Clean Up writes every number the same way (+965 9999 8888).

**Privacy.** Your contacts live only on this Mac, in a folder only your user account can open, kept out of Spotlight. The app never uses the network unless you switch on Cloud AI. Copying to Apple's Contacts app or saving into iCloud Drive/Dropbox warns you first, because those sync off the Mac.

**Safety.** Deleted contacts sit in the Trash for 30 days. A backup of the whole address book is kept for each of the last 30 days, in the data folder.

**Keys:** ⌘N new · ⌘⇧N Smart Add · ⌘F search · ⌘E edit · ⌘S save · ⌘⌫ delete · ⌘⇧F favorite · ⌘O import · ⌘, settings`
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
		"The app uses no network and no accounts unless Cloud AI is on.\n" +
		"FileVault (System Settings → Privacy & Security) encrypts it on disk."
}
