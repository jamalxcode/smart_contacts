package main

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// importSources are the places contacts come from, with how to get the file out of each.
var importSources = []struct {
	title, how string
	mac        bool
}{
	{"Mac & iPhone contacts", "Everything in the Mac's Contacts app, including iPhone contacts synced through iCloud and any Google or Exchange accounts added in System Settings → Internet Accounts.", true},
	{"Another phone", "Android: Contacts app → Settings → Export → .vcf file. Old iPhone: icloud.com → Contacts → select all → Export vCard. Then AirDrop or email the file to this Mac — or use Receive from a Phone below.", false},
	{"Gmail / Google", "contacts.google.com → Export → Google CSV or vCard.", false},
	{"Outlook / Hotmail", "outlook.com → People → Manage contacts → Export contacts (CSV).", false},
	{"LinkedIn", "linkedin.com → Me → Settings & Privacy → Data privacy → Get a copy of your data → Connections → Request archive. Open Connections.csv from the email LinkedIn sends.", false},
	{"Yahoo, iCloud or another email", "Use the account's Export contacts option and choose vCard or CSV.", false},
	{"Spreadsheet", "Any .csv or .xlsx file. Columns are matched automatically (English or Arabic headers) and you can change the matching before importing.", false},
	{"Notes or text", "A .txt file or text from Notes with several people in it — one per line (\"Ali 9999 8888\") or in blocks separated by an empty line.", false},
	{"Telegram", "Telegram Desktop → Settings → Advanced → Export Telegram data → tick Contacts list, format JSON. Import result.json — contacts come with their phone numbers.", false},
	{"Facebook / Snapchat friends", "Facebook: Settings → Download your information → Friends, format JSON (your_friends.json). Snapchat: Settings → My Data → friends.json. Names only, so they're matched to people you already have.", false},
}

// importHub is the one place to bring contacts in from anywhere.
func (u *ui) importHub() {
	box := container.NewVBox()
	var d *paperDialog
	for _, src := range importSources {
		src := src
		title := widget.NewLabelWithStyle(src.title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		how := caption(src.how)
		how.Wrapping = fyne.TextWrapWord
		label, icon := "Choose File…", theme.FolderOpenIcon()
		if src.mac {
			label, icon = "Import Now", theme.DownloadIcon()
		}
		btn := widget.NewButtonWithIcon(label, icon, func() {
			d.Hide()
			if src.mac {
				u.importMac()
			} else {
				u.importFile()
			}
		})
		box.Add(container.NewBorder(nil, nil, nil, container.NewVBox(btn), container.NewVBox(title, how)))
		box.Add(widget.NewSeparator())
	}
	phoneBtn := widget.NewButtonWithIcon("Receive from a Phone…", theme.DownloadIcon(), func() { d.Hide(); u.receiveFromPhone() })
	phoneBtn.Importance = widget.HighImportance
	phoneHow := caption("Over Bluetooth or AirDrop: the app waits for the contacts file your phone sends and opens it here.")
	phoneHow.Wrapping = fyne.TextWrapWord
	box.Objects = append([]fyne.CanvasObject{
		container.NewBorder(nil, nil, nil, container.NewVBox(phoneBtn), container.NewVBox(
			widget.NewLabelWithStyle("Receive from a phone (Bluetooth / AirDrop)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), phoneHow)),
		widget.NewSeparator()}, box.Objects...)
	paste := widget.NewButtonWithIcon("Paste Text…", theme.ContentPasteIcon(), func() { d.Hide(); u.pasteImport() })
	social := caption("Instagram and X exports list only usernames or account numbers — nothing to match a person by — so add those profile links by hand. You can also drag files onto the main window.")
	social.Wrapping = fyne.TextWrapWord
	privacy := caption("Everything is read on this Mac. People you already have are recognised (by number, email, profile link or name — in Arabic or English) and their cards are updated instead of duplicated. A backup is saved before each import.")
	privacy.Wrapping = fyne.TextWrapWord
	content := container.NewBorder(privacy, container.NewVBox(widget.NewSeparator(), container.NewBorder(nil, nil, nil, paste, social)), nil, nil, container.NewVScroll(box))
	d = newCustom("Import Contacts", "Close", content, u.win)
	d.Resize(fyne.NewSize(780, 640))
	d.Show()
}

// importFile picks any supported file.
func (u *ui) importFile() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil || r == nil {
			return
		}
		defer r.Close()
		b, err := io.ReadAll(io.LimitReader(r, 512<<20))
		if err != nil {
			showErr(err, u.win)
			return
		}
		u.importData(r.URI().Name(), b)
	}, u.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".vcf", ".vcard", ".csv", ".tsv", ".xlsx", ".txt", ".json", ".ics", ".VCF", ".CSV", ".XLSX", ".TXT"}))
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

// importDropped handles files dragged onto the window.
func (u *ui) importDropped(_ fyne.Position, uris []fyne.URI) {
	for _, uri := range uris {
		b, err := os.ReadFile(uri.Path())
		if err != nil {
			showErr(err, u.win)
			continue
		}
		if IsImageFile(uri.Name()) { // a dropped picture is read as a business card
			u.readCardImage(uri.Name(), b)
			continue
		}
		u.importData(uri.Name(), b)
	}
}

func (u *ui) importData(name string, data []byte) {
	if len(name) > 4 && (name[len(name)-4:] == ".ics" || name[len(name)-4:] == ".ICS") {
		u.importICS(name, data)
		return
	}
	if IsWhatsAppExport(name, data) { // a chat export: who you talk to and when, not new cards
		u.importWhatsApp(name, data)
		return
	}
	b, err := ReadImport(name, data, u.cfg.CountryCode)
	if err != nil {
		showErr(err, u.win)
		return
	}
	u.previewImport(b)
}

// pasteImport takes text with several people in it.
func (u *ui) pasteImport() {
	in := widget.NewMultiLineEntry()
	in.Wrapping = fyne.TextWrapWord
	in.SetMinRowsVisible(12)
	in.SetPlaceHolder("Ali Hassan 6555 1212\nSara Ali 9999 0000\n\nDr. Yousef Al-Kandari\nDentist, Dar Al Shifa\nT: 1802555")
	d := newConfirm("Paste People", "Read", "Cancel", in, func(ok bool) {
		if ok && strings.TrimSpace(in.Text) != "" {
			u.previewImport(NotesBatch(in.Text, u.cfg.CountryCode))
		}
	}, u.win)
	d.Resize(fyne.NewSize(620, 460))
	d.Show()
	u.win.Canvas().Focus(in)
}

// previewImport shows what an import will do and lets the user adjust it before anything changes.
func (u *ui) previewImport(b *Batch) {
	if len(b.Cards) == 0 && b.Table == nil {
		showInfo("Nothing to import", "No contacts with a name, number or email were found in this file.", u.win)
		return
	}
	cc := u.cfg.CountryCode
	source := widget.NewEntry()
	source.SetText(b.Source)
	summary := widget.NewLabel("")
	summary.Wrapping = fyne.TextWrapWord
	lists := container.NewVBox()
	var items []ImportItem
	choice := map[int]string{}
	ticks := map[int]*widget.Check{}

	var replan func()
	render := func() {
		lists.Objects = nil
		count := map[int]int{}
		for _, it := range items {
			count[it.Kind]++
		}
		newWord := "new"
		if b.MatchOnly {
			newWord = "not matched"
		}
		summary.SetText(fmt.Sprintf("%s · %d people: %d %s, %d will update cards you have, %d already up to date, %d matched by name only (check), %d repeated in the file.%s",
			b.Format, len(items), count[piNew], newWord, count[piMerge], count[piUpToDate], count[piAsk], count[piInFile], skippedText(b.Skipped)))
		acc := widget.NewAccordion()
		section := func(kind int, title string, build func(i int, it ImportItem) fyne.CanvasObject) {
			box := container.NewVBox()
			n := 0
			for i, it := range items {
				if it.Kind == kind {
					if n < 400 {
						box.Add(build(i, it))
					}
					n++
				}
			}
			if n == 0 {
				return
			}
			if n > 400 {
				box.Add(caption(fmt.Sprintf("…and %d more", n-400)))
			}
			acc.Append(widget.NewAccordionItem(fmt.Sprintf("%s (%d)", title, n), box))
		}
		tick := func(i int, text string) fyne.CanvasObject {
			c := widget.NewCheck(text, nil)
			if old, ok := ticks[i]; ok {
				c.SetChecked(old.Checked)
			} else {
				c.SetChecked(!(b.MatchOnly && items[i].Kind == piNew)) // name-only strangers: off unless ticked
			}
			ticks[i] = c
			return c
		}
		section(piAsk, "Matched by name only — check these (the suggestion is pre-selected)", func(i int, it ImportItem) fyne.CanvasObject {
			opts := []string{"Add as a new person", "Skip"}
			keys := []string{"new", "skip"}
			for _, c := range it.Candidates {
				label := "Same as " + c.DisplayName()
				if s := c.Subtitle(); s != "" {
					label += " (" + s + ")"
				}
				opts = append(opts, label)
				keys = append(keys, "merge:"+c.ID)
			}
			sel := widget.NewSelect(opts, nil)
			cur, picked := choice[i]
			if !picked {
				cur = it.Default
			}
			idx := 0
			for k, key := range keys {
				if key == cur {
					idx = k
				}
			}
			sel.SetSelectedIndex(idx)
			choice[i] = keys[idx]
			sel.OnChanged = func(string) { choice[i] = keys[sel.SelectedIndex()] }
			who := widget.NewLabelWithStyle(incomingLabel(it.Card.Contact), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			conf := it.Confidence
			if conf != "" {
				conf = " (" + conf + ")"
			}
			hint := it.Why + conf
			if len(it.Adds) > 0 && strings.HasPrefix(it.Default, "merge:") {
				hint += " · would add " + strings.Join(it.Adds, ", ")
			}
			h := caption(hint)
			h.Wrapping = fyne.TextWrapWord
			return container.NewVBox(who, sel, h)
		})
		section(piMerge, "Will update cards you have", func(i int, it ImportItem) fyne.CanvasObject {
			c := tick(i, it.Match.DisplayName()) // separate labels: mixed Arabic/English in one line gets reordered
			return container.NewVBox(container.NewHBox(c, widget.NewLabel("←"), widget.NewLabel(incomingLabel(it.Card.Contact))),
				caption("adds "+strings.Join(it.Adds, ", ")+"   ·   "+it.Why))
		})
		newTitle := "New people"
		if b.MatchOnly {
			newTitle = "Not matched to anyone (names only — tick to add as new cards)"
		}
		section(piNew, newTitle, func(i int, it ImportItem) fyne.CanvasObject {
			return tick(i, incomingLabel(it.Card.Contact))
		})
		section(piUpToDate, "Already up to date", func(_ int, it ImportItem) fyne.CanvasObject {
			return widget.NewLabel(it.Match.DisplayName())
		})
		section(piInFile, "Repeated in this file (folded together)", func(_ int, it ImportItem) fyne.CanvasObject {
			return widget.NewLabel(incomingLabel(it.Card.Contact) + "  →  " + it.Match.DisplayName())
		})
		if len(acc.Items) > 0 {
			acc.Open(0)
		}
		lists.Add(acc)
		lists.Refresh()
	}
	replan = func() {
		for _, cd := range b.Cards {
			cd.Contact.Sources = nil
		}
		items = PlanImport(u.store, b.Cards, strings.TrimSpace(source.Text), cc)
		ticks = map[int]*widget.Check{}
		choice = map[int]string{}
		render()
	}

	top := container.NewVBox(container.NewBorder(nil, nil, widget.NewLabel("Source name"), nil, source), summary)
	for _, n := range b.Notes {
		l := caption(n)
		l.Wrapping = fyne.TextWrapWord
		top.Add(l)
	}
	if b.Table != nil {
		top.Add(u.columnMatcher(b, replan))
	}
	autoClean := widget.NewCheck("Clean up automatically: numbers, emails, repeats and notes (anything doubtful is shown to you)", nil)
	autoClean.SetChecked(true)
	top.Add(autoClean)
	source.OnChanged = func(string) {
		for _, it := range items {
			it.Card.Contact.Sources = []string{strings.TrimSpace(source.Text)}
		}
	}
	replan()

	d := newConfirm("Import from "+b.Source, "Import", "Cancel",
		container.NewBorder(top, nil, nil, nil, container.NewVScroll(lists)), func(ok bool) {
			if !ok {
				return
			}
			backup, err := u.store.SnapshotBackup("before-import")
			if err != nil {
				showErr(fmt.Errorf("couldn't save a backup first, so nothing was imported: %w", err), u.win)
				return
			}
			src := strings.TrimSpace(source.Text)
			for _, it := range items {
				it.Card.Contact.Sources = uniqueFold([]string{src})
			}
			final := map[int]string{}
			for i, c := range choice {
				final[i] = c
			}
			for i, t := range ticks {
				if !t.Checked {
					final[i] = "skip"
				}
			}
			touched := ApplyImport(u.store, items, final, cc)
			for _, c := range touched {
				delete(u.photoCache, c.ID)
			}
			u.save()
			u.flash(fmt.Sprintf("Imported from %s: %d cards added or updated · backup %s", src, len(touched), shortName(backup)))
			if len(touched) == 0 {
				return
			}
			intro := fmt.Sprintf("Imported %d people from %s. Tidy their numbers and emails now?", len(touched), src)
			if autoClean.Checked {
				n, rest := u.autoCleanup(touched)
				if len(rest) == 0 {
					u.flash(fmt.Sprintf("Imported from %s: %d cards added or updated, %d fixes applied", src, len(touched), n))
					u.checkNewDuplicates(touched)
					return
				}
				intro = fmt.Sprintf("Imported %d people from %s and applied %d safe fixes. These need your decision:", len(touched), src, n)
			}
			u.cleanupFor(touched, intro)
			u.checkNewDuplicates(touched)
		}, u.win)
	d.Resize(fyne.NewSize(900, 680))
	d.Show()
}

func skippedText(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" %d rows had no name, number or email and are skipped.", n)
}

func shortName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

// incomingLabel names an incoming card with its main detail.
func incomingLabel(c *Contact) string {
	s := c.DisplayName()
	switch {
	case len(c.Phones) > 0:
		s += " · " + c.Phones[0].Value
	case len(c.Emails) > 0:
		s += " · " + c.Emails[0].Value
	case c.Company != "":
		s += " · " + c.Company
	}
	return s
}

// columnMatcher shows each spreadsheet column with a sample value and what it will be read as.
func (u *ui) columnMatcher(b *Batch, changed func()) fyne.CanvasObject {
	t := b.Table
	grid := container.NewGridWithColumns(3)
	grid.Add(widget.NewLabelWithStyle("Column", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	grid.Add(widget.NewLabelWithStyle("Example", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	grid.Add(widget.NewLabelWithStyle("Read as", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	for i := range t.Header {
		i := i
		sample := ""
		for _, r := range t.Rows {
			if i < len(r) && strings.TrimSpace(r[i]) != "" {
				sample = r[i]
				break
			}
		}
		h := widget.NewLabel(t.Header[i])
		h.Truncation = fyne.TextTruncateEllipsis
		ex := caption(sample)
		ex.Truncation = fyne.TextTruncateEllipsis
		sel := widget.NewSelect(colKinds, nil)
		sel.SetSelected(t.Cols[i].Kind)
		sel.OnChanged = func(k string) {
			t.Cols[i].Kind = k
			b.Cards, b.Skipped = t.Cards(u.cfg.CountryCode)
			changed()
		}
		grid.Add(h)
		grid.Add(ex)
		grid.Add(sel)
	}
	room := canvas.NewRectangle(color.Transparent)
	room.SetMinSize(fyne.NewSize(10, fyne.Min(float32(len(t.Header)+1)*40, 240)))
	acc := widget.NewAccordion(widget.NewAccordionItem(fmt.Sprintf("Column matching (%d columns)", len(t.Header)), container.NewStack(room, container.NewVScroll(grid))))
	if strings.HasPrefix(b.Format, "spreadsheet") || strings.HasPrefix(b.Format, "Excel") {
		acc.Open(0) // a hand-made sheet: show the matching straight away
	}
	return acc
}

// autoCleanup applies the confident fixes and repeat removals to the given contacts and returns
// how many it applied and the contacts that still have doubtful suggestions (wrong-looking
// numbers, unknown countries).
func (u *ui) autoCleanup(cs []*Contact) (int, []*Contact) {
	n := 0
	restSet := map[*Contact]bool{}
	for _, ch := range PlanCleanup(cs, u.cfg.CountryCode) {
		switch {
		case (ch.Kind == ckFix || ch.Kind == ckDuplicate) && ch.Default:
			ch.Apply("")
			n++
		case ch.Kind == ckWrong || ch.Kind == ckUnsure:
			restSet[ch.Contact] = true
		}
	}
	for _, c := range cs {
		u.store.Put(c)
	}
	u.save()
	var rest []*Contact
	for c := range restSet {
		rest = append(rest, c)
	}
	return n, rest
}

// checkNewDuplicates offers Deduplicate when people just imported look like someone else in the book.
func (u *ui) checkNewDuplicates(touched []*Contact) {
	in := map[string]bool{}
	for _, c := range touched {
		in[c.ID] = true
	}
	n := 0
	for _, g := range FindDuplicates(u.store.Contacts, u.cfg.CountryCode) {
		for _, c := range g.Contacts {
			if in[c.ID] {
				n++
				break
			}
		}
	}
	if n == 0 {
		return
	}
	u.confirmCB("Possible duplicates", fmt.Sprintf("%d of the people just imported look like someone already on another card. Review them now?", n),
		"Review", false, func(ok bool) {
			if ok {
				u.duplicates()
			}
		}, u.win)
}

// receiveFromPhone waits for a contacts file sent from a phone over Bluetooth or AirDrop
// (both land in Downloads) and opens the import preview when it arrives.
func (u *ui) receiveFromPhone() {
	home, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(home, "Downloads")}
	start := time.Now()
	how := widget.NewLabel("On this Mac: System Settings → General → Sharing → turn on Bluetooth Sharing (files go to Downloads). AirDrop also works.\n\n" +
		"Android: Contacts → select all (or ⋮ → Share) → Bluetooth → choose this Mac.\n" +
		"iPhone: Contacts → a contact → Share Contact → AirDrop → this Mac (or share many from icloud.com as a vCard).\n" +
		"Other phones: send the contacts as a .vcf file over Bluetooth.\n\nWaiting for the file…")
	how.Wrapping = fyne.TextWrapWord
	stop := make(chan struct{})
	d := newCustom("Receive from a Phone", "Cancel", container.NewVBox(how, widget.NewProgressBarInfinite()), u.win)
	d.SetOnClosed(func() {
		select {
		case <-stop:
		default:
			close(stop)
		}
	})
	d.Resize(fyne.NewSize(620, 380))
	d.Show()
	go func() {
		seenSize := map[string]int64{}
		tick := time.NewTicker(1500 * time.Millisecond)
		defer tick.Stop()
		deadline := time.After(15 * time.Minute)
		for {
			select {
			case <-stop:
				return
			case <-deadline:
				fyne.Do(func() { d.Hide() })
				return
			case <-tick.C:
			}
			for _, dir := range dirs {
				entries, err := os.ReadDir(dir)
				if err != nil {
					continue
				}
				for _, e := range entries {
					ext := strings.ToLower(filepath.Ext(e.Name()))
					if ext != ".vcf" && ext != ".vcard" {
						continue
					}
					fi, err := e.Info()
					if err != nil || fi.ModTime().Before(start) {
						continue
					}
					p := filepath.Join(dir, e.Name())
					if seenSize[p] != fi.Size() { // wait until the transfer has finished
						seenSize[p] = fi.Size()
						continue
					}
					data, err := os.ReadFile(p)
					if err != nil {
						continue
					}
					name := e.Name()
					fyne.Do(func() {
						d.Hide()
						u.importData(name, data)
					})
					return
				}
			}
		}
	}()
}
