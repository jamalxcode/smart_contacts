package main

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"strings"

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
	{"Another phone", "Android: Contacts app → Settings → Export → .vcf file. Old iPhone: icloud.com → Contacts → select all → Export vCard. Then AirDrop or email the file to this Mac.", false},
	{"Gmail / Google", "contacts.google.com → Export → Google CSV or vCard.", false},
	{"Outlook / Hotmail", "outlook.com → People → Manage contacts → Export contacts (CSV).", false},
	{"LinkedIn", "linkedin.com → Me → Settings & Privacy → Data privacy → Get a copy of your data → Connections → Request archive. Open Connections.csv from the email LinkedIn sends.", false},
	{"Yahoo, iCloud or another email", "Use the account's Export contacts option and choose vCard or CSV.", false},
	{"Spreadsheet", "Any .csv or .xlsx file. Columns are matched automatically (English or Arabic headers) and you can change the matching before importing.", false},
	{"Notes or text", "A .txt file or text from Notes with several people in it — one per line (\"Ali 9999 8888\") or in blocks separated by an empty line.", false},
}

// importHub is the one place to bring contacts in from anywhere.
func (u *ui) importHub() {
	box := container.NewVBox()
	var d *dialog.CustomDialog
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
	paste := widget.NewButtonWithIcon("Paste Text…", theme.ContentPasteIcon(), func() { d.Hide(); u.pasteImport() })
	social := caption("Facebook, Instagram and X exports list only names or usernames — no numbers or emails — so they can't be matched to people; add those profile links by hand. You can also drag files onto the main window.")
	social.Wrapping = fyne.TextWrapWord
	privacy := caption("Everything is read on this Mac. People you already have are recognised (by number, email, profile link or name — in Arabic or English) and their cards are updated instead of duplicated. A backup is saved before each import.")
	privacy.Wrapping = fyne.TextWrapWord
	content := container.NewBorder(privacy, container.NewVBox(widget.NewSeparator(), container.NewBorder(nil, nil, nil, paste, social)), nil, nil, container.NewVScroll(box))
	d = dialog.NewCustom("Import Contacts", "Close", content, u.win)
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
			dialog.ShowError(err, u.win)
			return
		}
		u.importData(r.URI().Name(), b)
	}, u.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".vcf", ".vcard", ".csv", ".tsv", ".xlsx", ".txt", ".json", ".VCF", ".CSV", ".XLSX", ".TXT"}))
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

// importDropped handles files dragged onto the window.
func (u *ui) importDropped(_ fyne.Position, uris []fyne.URI) {
	for _, uri := range uris {
		b, err := os.ReadFile(uri.Path())
		if err != nil {
			dialog.ShowError(err, u.win)
			continue
		}
		u.importData(uri.Name(), b)
	}
}

func (u *ui) importData(name string, data []byte) {
	b, err := ReadImport(name, data, u.cfg.CountryCode)
	if err != nil {
		dialog.ShowError(err, u.win)
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
	d := dialog.NewCustomConfirm("Paste People", "Read", "Cancel", in, func(ok bool) {
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
		dialog.ShowInformation("Nothing to import", "No contacts with a name, number or email were found in this file.", u.win)
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
		summary.SetText(fmt.Sprintf("%s · %d people: %d new, %d will update cards you have, %d already up to date, %d need your decision, %d repeated in the file.%s",
			b.Format, len(items), count[piNew], count[piMerge], count[piUpToDate], count[piAsk], count[piInFile], skippedText(b.Skipped)))
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
				c.SetChecked(true)
			}
			ticks[i] = c
			return c
		}
		section(piAsk, "Need your decision", func(i int, it ImportItem) fyne.CanvasObject {
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
			cur := choice[i]
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
			return container.NewVBox(who, container.NewBorder(nil, nil, caption(it.Why+" →"), nil, sel))
		})
		section(piMerge, "Will update cards you have", func(i int, it ImportItem) fyne.CanvasObject {
			c := tick(i, it.Match.DisplayName()) // separate labels: mixed Arabic/English in one line gets reordered
			return container.NewVBox(container.NewHBox(c, widget.NewLabel("←"), widget.NewLabel(incomingLabel(it.Card.Contact))),
				caption("adds "+strings.Join(it.Adds, ", ")+"   ·   "+it.Why))
		})
		section(piNew, "New people", func(i int, it ImportItem) fyne.CanvasObject {
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
	source.OnChanged = func(string) {
		for _, it := range items {
			it.Card.Contact.Sources = []string{strings.TrimSpace(source.Text)}
		}
	}
	replan()

	d := dialog.NewCustomConfirm("Import from "+b.Source, "Import", "Cancel",
		container.NewBorder(top, nil, nil, nil, container.NewVScroll(lists)), func(ok bool) {
			if !ok {
				return
			}
			backup, err := u.store.SnapshotBackup("before-import")
			if err != nil {
				dialog.ShowError(fmt.Errorf("couldn't save a backup first, so nothing was imported: %w", err), u.win)
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
			if len(touched) > 0 {
				u.cleanupFor(touched, fmt.Sprintf("Imported %d people from %s. Tidy their numbers and emails now?", len(touched), src))
			}
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
