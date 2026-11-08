package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// toggleSelectMode switches the list between normal clicking and ticking several people.
func (u *ui) toggleSelectMode() {
	u.selectMode = !u.selectMode
	u.picked = map[string]bool{}
	u.list.UnselectAll()
	u.list.Refresh()
	if u.selectMode {
		u.bulkBar.Show()
		u.flash("Select mode: click people to tick them, then choose an action below")
	} else {
		u.bulkBar.Hide()
		u.reselect()
	}
	u.updateBulkBar()
	u.listPane.Refresh() // re-lay out so the list fills the space the bar used
}

func (u *ui) pickedContacts() []*Contact {
	var out []*Contact
	for _, c := range u.filtered {
		if u.picked[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

// updateBulkBar shows the actions for the ticked people.
func (u *ui) updateBulkBar() {
	if !u.selectMode {
		return
	}
	cs := u.pickedContacts()
	n := len(cs)
	count := widget.NewLabelWithStyle(fmt.Sprintf("%d selected", n), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	all := widget.NewButton("All", func() {
		for _, c := range u.filtered {
			u.picked[c.ID] = true
		}
		u.list.Refresh()
		u.updateBulkBar()
	})
	none := widget.NewButton("None", func() {
		u.picked = map[string]bool{}
		u.list.Refresh()
		u.updateBulkBar()
	})
	done := widget.NewButton("Done", u.toggleSelectMode)
	done.Importance = widget.HighImportance
	btn := func(label string, icon fyne.Resource, need int, f func([]*Contact)) *widget.Button {
		b := widget.NewButtonWithIcon(label, icon, func() { f(u.pickedContacts()) })
		if n < need {
			b.Disable()
		}
		return b
	}
	actions := container.NewGridWrap(fyne.NewSize(160, 36),
		btn("Add to Group", iconLabel, 1, u.bulkAddGroup),
		btn("Remove Group", iconLabel, 1, u.bulkRemoveGroup),
		btn("Favorite", iconStar, 1, func(cs []*Contact) { u.bulkFavorite(cs, true) }),
		btn("Unfavorite", iconStarOff, 1, func(cs []*Contact) { u.bulkFavorite(cs, false) }),
		btn("Merge", iconMerge, 2, u.bulkMerge),
		btn("Export vCard", theme.DocumentSaveIcon(), 1, func(cs []*Contact) { u.exportVCF(cs, fmt.Sprintf("%d contacts", len(cs))) }),
		btn("Delete", theme.DeleteIcon(), 1, u.bulkDelete),
	)
	u.bulkBar.Objects = []fyne.CanvasObject{widget.NewSeparator(), container.NewBorder(nil, nil, count, container.NewHBox(all, none, done)), actions}
	u.bulkBar.Refresh()
}

func (u *ui) bulkAddGroup(cs []*Contact) {
	e := widget.NewSelectEntry(u.store.Groups())
	e.SetPlaceHolder("Group name")
	dialog.ShowForm(fmt.Sprintf("Add %d people to a group", len(cs)), "Add", "Cancel", []*widget.FormItem{widget.NewFormItem("Group", e)}, func(ok bool) {
		g := strings.TrimSpace(e.Text)
		if !ok || g == "" {
			return
		}
		for _, x := range u.store.Groups() {
			if strings.EqualFold(x, g) {
				g = x
			}
		}
		for _, c := range cs {
			c.Groups = append(c.Groups, g)
			u.store.Put(c)
		}
		u.save()
		u.flash(fmt.Sprintf("Added %d people to %s", len(cs), g))
	}, u.win)
}

func (u *ui) bulkRemoveGroup(cs []*Contact) {
	seen := map[string]bool{}
	var groups []string
	for _, c := range cs {
		for _, g := range c.Groups {
			if !seen[strings.ToLower(g)] {
				seen[strings.ToLower(g)] = true
				groups = append(groups, g)
			}
		}
	}
	if len(groups) == 0 {
		dialog.ShowInformation("No groups", "None of the selected people are in a group.", u.win)
		return
	}
	sel := widget.NewSelect(groups, nil)
	sel.SetSelectedIndex(0)
	dialog.ShowForm("Remove from a group", "Remove", "Cancel", []*widget.FormItem{widget.NewFormItem("Group", sel)}, func(ok bool) {
		if !ok {
			return
		}
		for _, c := range cs {
			keep := c.Groups[:0]
			for _, g := range c.Groups {
				if !strings.EqualFold(g, sel.Selected) {
					keep = append(keep, g)
				}
			}
			c.Groups = keep
			u.store.Put(c)
		}
		u.save()
	}, u.win)
}

func (u *ui) bulkFavorite(cs []*Contact, fav bool) {
	for _, c := range cs {
		c.Favorite = fav
		u.store.Put(c)
	}
	u.save()
}

// bulkMerge merges the ticked cards into the one with the most details.
func (u *ui) bulkMerge(cs []*Contact) {
	names := []string{}
	for _, c := range cs {
		names = append(names, c.DisplayName())
	}
	dialog.ShowConfirm(fmt.Sprintf("Merge %d cards?", len(cs)), "They become one card (the fullest one keeps its details and gains the others'):\n"+strings.Join(names, "\n")+"\n\nYou can undo this (⌘Z).", func(ok bool) {
		if !ok {
			return
		}
		best := 0
		for i, c := range cs {
			if richness(c) > richness(cs[best]) {
				best = i
			}
		}
		keep := cs[best]
		var others []*Contact
		var ids []string
		for i, c := range cs {
			if i != best {
				others = append(others, c)
				ids = append(ids, c.ID)
				keep.AddEvent(nowTime(), "Merged with another card ("+c.DisplayName()+")", "You")
				if !keep.HasPhoto {
					if p := u.store.Photo(c); len(p) > 0 {
						_ = u.store.SetPhoto(keep, p)
					}
				}
			}
		}
		Merge(keep, others, u.cfg.CountryCode)
		for _, id := range ids {
			u.store.RepointLinks(id, keep.ID)
		}
		u.store.Put(keep)
		u.store.Delete(ids...)
		delete(u.photoCache, keep.ID)
		u.picked = map[string]bool{keep.ID: true}
		u.selectedID = keep.ID
		u.save()
		u.updateBulkBar()
		u.flash(fmt.Sprintf("Merged %d cards into %s (⌘Z to undo)", len(cs), keep.DisplayName()))
	}, u.win)
}

func (u *ui) bulkDelete(cs []*Contact) {
	dialog.ShowConfirm(fmt.Sprintf("Delete %d contacts?", len(cs)), "They go to the Trash for 30 days, and ⌘Z undoes it.", func(ok bool) {
		if !ok {
			return
		}
		var ids []string
		for _, c := range cs {
			ids = append(ids, c.ID)
		}
		u.store.Delete(ids...)
		u.picked = map[string]bool{}
		u.save()
		u.updateBulkBar()
	}, u.win)
}
