package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
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
		u.flash("Click people to clip them together, then choose what to do on the yellow note")
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
	defer u.refreshMenu() // "3 Selected Cards" in the Card menu
	if !u.selectMode {
		return
	}
	cs := u.pickedContacts()
	n := len(cs)
	count := widget.NewLabelWithStyle(fmt.Sprintf("%d %s clipped", n, pluralWords(n, "card", "cards")), fyne.TextAlignLeading, serif("x", true, true))
	all := widget.NewButton("Clip All", func() {
		for _, c := range u.filtered {
			u.picked[c.ID] = true
		}
		u.list.Refresh()
		u.updateBulkBar()
	})
	none := widget.NewButton("Unclip All", func() {
		u.picked = map[string]bool{}
		u.list.Refresh()
		u.updateBulkBar()
	})
	done := widget.NewButton("Take Clips Off", u.toggleSelectMode)
	done.Importance = widget.HighImportance
	btn := func(label string, icon fyne.Resource, need int, f func([]*Contact)) *widget.Button {
		b := widget.NewButtonWithIcon(label, icon, func() { f(u.pickedContacts()) })
		if n < need {
			b.Disable()
		}
		return b
	}
	var more *widget.Button
	more = widget.NewButtonWithIcon("More…", theme.MoreHorizontalIcon(), func() {
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(more)
		m := u.bulkMenu(u.pickedContacts())
		widget.ShowPopUpMenuAtPosition(m, u.win.Canvas(), pos.Add(fyne.NewPos(0, -float32(len(m.Items))*30)))
	})
	if n == 0 {
		more.Disable()
	}
	actions := container.NewGridWrap(fyne.NewSize(160, 36),
		btn("Add to Group", iconLabel, 1, u.bulkAddGroup),
		btn("Favorite", iconStar, 1, func(cs []*Contact) { u.bulkFavorite(cs, true) }),
		btn("Clean Up", theme.ViewRefreshIcon(), 1, func(cs []*Contact) { u.cleanupFor(cs, "") }),
		btn("Merge", iconMerge, 2, u.bulkMerge),
		btn("Delete", theme.DeleteIcon(), 1, u.bulkDelete),
		more,
	)
	hint := caption("Click a card (or its clip) to clip or unclip it · Shift-click clips a range · right-click for every command")
	hint.Wrapping = fyne.TextWrapWord // don't widen the list
	for _, o := range []*widget.Button{all, none, done} {
		o.Importance = widget.LowImportance
	}
	u.bulkBar.Objects = []fyne.CanvasObject{stickyNote(container.NewVBox(
		count, container.NewHBox(all, none, layout.NewSpacer(), done), actions, hint))}
	u.bulkBar.Refresh()
}

func (u *ui) bulkAddGroup(cs []*Contact) {
	e := widget.NewSelectEntry(u.store.Groups())
	e.SetPlaceHolder("Group name")
	showForm(fmt.Sprintf("Add %d people to a group", len(cs)), "Add", "Cancel", []*widget.FormItem{widget.NewFormItem("Group", e)}, func(ok bool) {
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
		showInfo("No groups", "None of the selected people are in a group.", u.win)
		return
	}
	sel := widget.NewSelect(groups, nil)
	sel.SetSelectedIndex(0)
	showForm("Remove from a group", "Remove", "Cancel", []*widget.FormItem{widget.NewFormItem("Group", sel)}, func(ok bool) {
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
	u.mergeChooser(cs, func(keep *Contact) {
		u.picked = map[string]bool{keep.ID: true}
		u.selectedID = keep.ID
		u.updateBulkBar()
		u.refreshList()
	})
}

func (u *ui) bulkDelete(cs []*Contact) {
	if len(cs) == 0 {
		return
	}
	names := []string{}
	for i, c := range cs {
		if i == 8 {
			names = append(names, fmt.Sprintf("…and %d more", len(cs)-8))
			break
		}
		names = append(names, c.DisplayName())
	}
	what := fmt.Sprintf("%d %s", len(cs), pluralWords(len(cs), "card", "cards"))
	// first warning
	u.confirmCB("Delete "+what+"?", strings.Join(names, "\n"), "Delete…", true, func(ok bool) {
		if !ok {
			return
		}
		// second warning: say plainly what happens, and make the button name the number
		u.confirmCB("Are you sure?", fmt.Sprintf("You're about to delete %s. They move to the Trash for 30 days "+
			"(View → Trash), and ⌘Z undoes it right away.", what), "Yes, Delete "+what, true, func(ok bool) {
			if !ok {
				return
			}
			ids := make([]string, len(cs))
			for i, c := range cs {
				ids[i] = c.ID
				delete(u.picked, c.ID)
			}
			u.store.Delete(ids...)
			u.save()
			u.updateBulkBar()
			u.flash(fmt.Sprintf("Moved %s to the Trash (⌘Z to undo)", what))
		}, u.win)
	}, u.win)
}

// bulkMenu is every command that works on several people at once: the selection bar's More
// button and the right-click menu of a selection.
func (u *ui) bulkMenu(cs []*Contact) *fyne.Menu {
	n := len(cs)
	what := fmt.Sprintf("%d %s", n, pluralWords(n, "card", "cards"))
	head := fyne.NewMenuItem(what+" clipped", nil)
	head.Disabled = true

	addG := fyne.NewMenuItem("Add to Group", nil)
	var gItems []*fyne.MenuItem
	for _, g := range u.store.Groups() {
		g := g
		gItems = append(gItems, fyne.NewMenuItem(g, func() { u.bulkSetGroup(cs, g, true) }))
	}
	if len(gItems) > 0 {
		gItems = append(gItems, fyne.NewMenuItemSeparator())
	}
	gItems = append(gItems, fyne.NewMenuItem("New Group…", func() { u.bulkAddGroup(cs) }))
	addG.ChildMenu = fyne.NewMenu("", gItems...)

	var rItems []*fyne.MenuItem
	seen := map[string]bool{}
	for _, c := range cs {
		for _, g := range c.Groups {
			if !seen[strings.ToLower(g)] {
				seen[strings.ToLower(g)] = true
				g := g
				rItems = append(rItems, fyne.NewMenuItem(g, func() { u.bulkSetGroup(cs, g, false) }))
			}
		}
	}
	remG := fyne.NewMenuItem("Remove from Group", nil)
	if len(rItems) == 0 {
		remG.Disabled = true
	} else {
		remG.ChildMenu = fyne.NewMenu("", rItems...)
	}

	keep := fyne.NewMenuItem("Keep in Touch", nil)
	var kItems []*fyne.MenuItem
	for _, k := range keepChoices {
		k := k
		kItems = append(kItems, fyne.NewMenuItem(k.Label, func() { u.bulkKeep(cs, k.Days) }))
	}
	kItems = append(kItems, fyne.NewMenuItemSeparator(), fyne.NewMenuItem("No Reminder", func() { u.bulkKeep(cs, 0) }))
	keep.ChildMenu = fyne.NewMenu("", kItems...)

	export := fyne.NewMenuItem("Export", nil)
	export.ChildMenu = fyne.NewMenu("",
		fyne.NewMenuItem("vCard…", func() { u.exportVCF(cs, what) }),
		fyne.NewMenuItem("Spreadsheet (CSV)…", func() { u.exportCSVFor(cs) }),
		fyne.NewMenuItem("Copy to Apple Contacts…", func() { u.sendToMacContacts(cs) }),
	)
	merge := fyne.NewMenuItem("Merge…", func() { u.bulkMerge(cs) })
	merge.Disabled = n < 2
	dupes := fyne.NewMenuItem("Look for Duplicates Among These…", func() { u.duplicatesFor(cs) })
	dupes.Disabled = n < 2

	return fyne.NewMenu("",
		head,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Clean Up Cards…", func() { u.cleanupFor(cs, "") }),
		fyne.NewMenuItem("Tidy Notes with AI…", func() { u.tidyNotesAI(cs, false) }),
		dupes,
		merge,
		fyne.NewMenuItemSeparator(),
		addG, remG,
		fyne.NewMenuItem("Add to Favorites", func() { u.bulkFavorite(cs, true) }),
		fyne.NewMenuItem("Remove from Favorites", func() { u.bulkFavorite(cs, false) }),
		keep,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Copy Cards as Text", func() { u.copyCards(cs) }),
		export,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Clip All", u.selectAllCards),
		fyne.NewMenuItem("Select None", u.selectNone),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Delete "+what+"…", func() { u.bulkDelete(cs) }),
	)
}

func (u *ui) bulkSetGroup(cs []*Contact, g string, add bool) {
	for _, c := range cs {
		if add {
			if !c.HasGroup(g) {
				c.Groups = append(c.Groups, g)
			}
		} else {
			keep := c.Groups[:0]
			for _, x := range c.Groups {
				if !strings.EqualFold(x, g) {
					keep = append(keep, x)
				}
			}
			c.Groups = keep
		}
		u.store.Put(c)
	}
	u.save()
	if add {
		u.flash(fmt.Sprintf("Added %d people to %s", len(cs), g))
	} else {
		u.flash(fmt.Sprintf("Removed %d people from %s", len(cs), g))
	}
}

func (u *ui) bulkKeep(cs []*Contact, days int) {
	for _, c := range cs {
		c.KeepInTouch = days
		u.store.Put(c)
	}
	u.save()
	if days > 0 {
		u.flash(fmt.Sprintf("Keep in touch %s with %d people", keepText(days), len(cs)))
	} else {
		u.flash(fmt.Sprintf("Reminders turned off for %d people", len(cs)))
	}
}

func (u *ui) copyCards(cs []*Contact) {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = cardText(c)
	}
	u.app.Clipboard().SetContent(strings.Join(parts, "\n\n"))
	u.flash(fmt.Sprintf("Copied %d cards", len(cs)))
}

// selectNone takes the clips off every card (and leaves clipping on).
func (u *ui) selectNone() {
	u.picked = map[string]bool{}
	u.list.Refresh()
	u.updateBulkBar()
}

// clickWithKeys handles ⌘-click (add or remove one) and Shift-click (a range), as in Finder.
// It reports whether it handled the click.
func (u *ui) clickWithKeys(id string, mods fyne.KeyModifier) bool {
	cmd := mods&fyne.KeyModifierShortcutDefault != 0
	shift := mods&fyne.KeyModifierShift != 0
	if !cmd && !shift {
		if u.selectMode {
			u.selAnchor = id
		}
		return false
	}
	if u.busy() {
		return true
	}
	if !u.selectMode {
		prev := u.selectedID
		u.toggleSelectMode()
		if prev != "" && prev != id {
			u.picked[prev] = true // the card that was showing joins the selection
			if u.selAnchor == "" {
				u.selAnchor = prev
			}
		}
	}
	if shift && u.selAnchor != "" {
		a, b := u.indexOf(u.selAnchor), u.indexOf(id)
		if a >= 0 && b >= 0 {
			if a > b {
				a, b = b, a
			}
			for i := a; i <= b; i++ {
				u.picked[u.filtered[i].ID] = true
			}
		}
	} else {
		if u.picked[id] {
			delete(u.picked, id)
		} else {
			u.picked[id] = true
		}
		u.selAnchor = id
	}
	u.list.Refresh()
	u.updateBulkBar()
	return true
}
