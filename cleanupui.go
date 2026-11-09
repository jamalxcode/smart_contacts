package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

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
	var d *paperDialog
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
	d = newCustom(title, "Close", content, u.win)
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
	d := newConfirm(fmt.Sprintf("Clean Up %s — %d %s", scope, len(changes), pluralWords(len(changes), "suggestion", "suggestions")), "Apply", "Skip",
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
