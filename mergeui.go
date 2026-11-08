package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// mergeField is one detail where the cards disagree; the user picks which card's value to keep.
type mergeField struct {
	key, label string
	get        func(*Contact) string
}

var mergeChoices = []mergeField{
	{"name", "Name", func(c *Contact) string { return c.FullName() }},
	{"company", "Company", func(c *Contact) string { return c.Company }},
	{"title", "Job title", func(c *Contact) string { return c.Title }},
	{"birthday", "Birthday", func(c *Contact) string { return BirthdayText(c.Birthday) }},
	{"met", "How we met", func(c *Contact) string { return c.HowWeMet }},
}

// richest is the card with the most details (it becomes the merged card).
func richest(cs []*Contact) int {
	best := 0
	for i, c := range cs {
		if richness(c) > richness(cs[best]) {
			best = i
		}
	}
	return best
}

// mergeChooser shows the cards side by side: where they disagree (name, company, job, birthday,
// photo) you pick which to keep; numbers, emails, links, groups and notes are all kept.
func (u *ui) mergeChooser(cs []*Contact, done func(keep *Contact)) {
	best := richest(cs)
	picks := map[string]int{}
	var rows []fyne.CanvasObject
	for _, f := range mergeChoices {
		f := f
		var opts []string
		var idx []int
		seen := map[string]bool{}
		for i, c := range cs {
			v := strings.TrimSpace(f.get(c))
			if v == "" || seen[fold(v)] {
				continue
			}
			seen[fold(v)] = true
			opts = append(opts, v)
			idx = append(idx, i)
		}
		if len(opts) < 2 {
			continue // all the same, or only one card has it
		}
		rg := widget.NewRadioGroup(opts, func(s string) {
			for k, o := range opts {
				if o == s {
					picks[f.key] = idx[k]
				}
			}
		})
		def := 0
		for k, i := range idx {
			if i == best {
				def = k
			}
		}
		rg.SetSelected(opts[def])
		rows = append(rows, container.NewBorder(nil, nil, rowLabel(f.label), nil, rg))
	}
	// photos
	var withPhoto []int
	for i, c := range cs {
		if c.HasPhoto {
			withPhoto = append(withPhoto, i)
		}
	}
	if len(withPhoto) > 1 {
		pick := container.NewHBox()
		var btns []*widget.Button
		for _, i := range withPhoto {
			i := i
			img := canvas.NewImageFromResource(u.photo(cs[i]))
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(56, 56))
			var b *widget.Button
			b = widget.NewButton(cs[i].DisplayName(), func() {
				picks["photo"] = i
				for _, x := range btns {
					x.Importance = widget.MediumImportance
					x.Refresh()
				}
				b.Importance = widget.HighImportance
				b.Refresh()
			})
			btns = append(btns, b)
			pick.Add(container.NewVBox(img, b))
		}
		picks["photo"] = withPhoto[0]
		btns[0].Importance = widget.HighImportance
		rows = append(rows, container.NewBorder(nil, nil, rowLabel("Photo"), nil, pick))
	}
	keepAll := caption("Every phone number, email, address, link, group, note and timeline entry from all the cards is kept.")
	keepAll.Wrapping = fyne.TextWrapWord
	var names []string
	for _, c := range cs {
		names = append(names, c.DisplayName())
	}
	head := widget.NewLabel(fmt.Sprintf("Merge %d cards: %s", len(cs), strings.Join(names, ", ")))
	head.Wrapping = fyne.TextWrapWord
	body := container.NewVBox(head)
	if len(rows) == 0 {
		body.Add(caption("The cards don't disagree on anything."))
	} else {
		body.Add(caption("Where they differ, choose what to keep:"))
		for _, r := range rows {
			body.Add(r)
		}
	}
	body.Add(keepAll)
	d := dialog.NewCustomConfirm("Merge Cards", "Merge", "Cancel", container.NewVScroll(body), func(ok bool) {
		if !ok {
			return
		}
		for _, c := range cs { // a card deleted meanwhile can't be merged
			if u.store.Get(c.ID) == nil {
				showInfo("Already merged", c.DisplayName()+" is no longer in your contacts.", u.win)
				return
			}
		}
		keep := u.mergeCards(cs, picks)
		u.save()
		if done != nil {
			done(keep)
		}
		u.flash(fmt.Sprintf("Merged %d cards into %s (⌘Z to undo)", len(cs), keep.DisplayName()))
	}, u.win)
	d.SetConfirmImportance(widget.HighImportance)
	d.Resize(fyne.NewSize(620, 480))
	trackDialog(d, d.Confirm)
	d.Show()
}

// mergeCards merges cs into the richest card, applying the user's picks (field → card index).
func (u *ui) mergeCards(cs []*Contact, picks map[string]int) *Contact {
	best := richest(cs)
	keep := cs[best]
	chosen := map[string]*Contact{}
	for k, i := range picks {
		if i >= 0 && i < len(cs) {
			chosen[k] = cs[i]
		}
	}
	// photo first (from the picked card, else any card if the kept one has none)
	if pc := chosen["photo"]; pc != nil && pc != keep {
		if p := u.store.Photo(pc); len(p) > 0 {
			_ = u.store.SetPhoto(keep, p)
		}
	} else if !keep.HasPhoto {
		for _, o := range cs {
			if p := u.store.Photo(o); len(p) > 0 {
				_ = u.store.SetPhoto(keep, p)
				break
			}
		}
	}
	delete(u.photoCache, keep.ID)
	var others []*Contact
	var ids []string
	oldName := keep.FullName()
	for i, c := range cs {
		if i != best {
			others = append(others, c)
			ids = append(ids, c.ID)
			keep.AddEvent(nowTime(), "Merged with another card ("+c.DisplayName()+")", "You")
		}
	}
	Merge(keep, others, u.cfg.CountryCode)
	if c := chosen["name"]; c != nil && c != keep {
		keep.First, keep.Last = c.First, c.Last
		if fold(keep.AltName) == fold(keep.FullName()) {
			keep.AltName = ""
		}
		if keep.AltName == "" && hasArabic(oldName) != hasArabic(keep.FullName()) {
			keep.AltName = oldName // the other script's name stays as the second name
		}
	}
	if c := chosen["company"]; c != nil {
		keep.Company = c.Company
	}
	if c := chosen["title"]; c != nil {
		keep.Title = c.Title
	}
	if c := chosen["birthday"]; c != nil {
		keep.Birthday = c.Birthday
	}
	if c := chosen["met"]; c != nil {
		keep.HowWeMet = c.HowWeMet
	}
	for _, id := range ids {
		u.store.RepointLinks(id, keep.ID)
	}
	u.store.Put(keep)
	u.store.Delete(ids...)
	return keep
}
