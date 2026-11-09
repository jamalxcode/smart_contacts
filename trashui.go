package main

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// trash is the Trash window (Card → Show Trash, View → Trash, the Trash tab): every card in it,
// newest first, each with Put Back; and Put Back All and Empty Trash at the bottom.
func (u *ui) trash() {
	if len(u.store.Trash) == 0 {
		showInfo("The Trash is empty", "Cards you move to the Trash stay there until you empty it, so you can put them back.", u.win)
		return
	}
	box := container.NewVBox()
	var d *paperDialog
	var fill func()
	putBack := func(ids ...string) {
		for _, id := range ids {
			u.store.Restore(id)
		}
		if len(ids) == 1 {
			u.selectedID = ids[0]
		}
		u.save()
		u.flash(fmt.Sprintf("Put back %d %s from the Trash", len(ids), pluralWords(len(ids), "card", "cards")))
		if len(u.store.Trash) == 0 {
			d.Hide()
			return
		}
		fill()
	}
	fill = func() {
		box.Objects = nil
		for i := len(u.store.Trash) - 1; i >= 0; i-- {
			t := u.store.Trash[i]
			av := newAvatar(30)
			av.set(t.Contact, nil)
			lbl := widget.NewLabelWithStyle(t.Contact.DisplayName(), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			lbl.Truncation = fyne.TextTruncateEllipsis
			what := t.Contact.Subtitle()
			when := caption(strings.Trim("trashed "+Ago(t.Deleted, time.Now())+" · "+what, " ·"))
			when.Truncation = fyne.TextTruncateEllipsis
			id := t.Contact.ID
			back := widget.NewButtonWithIcon("Put Back", theme.ContentUndoIcon(), func() { putBack(id) })
			back.Importance = widget.LowImportance
			box.Add(container.NewBorder(nil, nil, container.NewCenter(av), back, container.New(&tightVBox{}, lbl, when)))
		}
		d.title = fmt.Sprintf("Trash — %d %s", len(u.store.Trash), pluralWords(len(u.store.Trash), "card", "cards"))
		if d.sheet != nil {
			d.sheet.title = d.title
			d.sheet.Refresh()
		}
		box.Refresh()
	}
	all := newPaperButton("Put Back All", btnPlain, func() {
		var ids []string
		for _, t := range u.store.Trash {
			ids = append(ids, t.Contact.ID)
		}
		putBack(ids...)
	})
	empty := newPaperButton("Empty Trash…", btnDanger, func() { d.Hide(); u.cmdEmptyTrash() })
	closeB := newPaperButton("Close", btnPlain, func() { d.Hide() })
	note := caption("Cards stay here until you empty the Trash.")
	d = newCustomWithoutButtons("Trash", container.NewBorder(nil, note, nil, nil, container.NewVScroll(box)), u.win)
	d.clipboard = true
	d.SetButtons([]fyne.CanvasObject{all, empty, closeB})
	fill()
	d.Resize(fyne.NewSize(560, 480))
	d.Show()
}

// cmdEmptyTrash deletes everything in the Trash for good, after asking.
func (u *ui) cmdEmptyTrash() {
	n := len(u.store.Trash)
	if n == 0 {
		showInfo("The Trash is empty", "There's nothing to delete.", u.win)
		return
	}
	what := fmt.Sprintf("%d %s", n, pluralWords(n, "card", "cards"))
	u.confirmCB("Empty the Trash?", "The "+what+" in the Trash will be deleted for good, with their photos and business cards. This can't be undone.",
		"Empty Trash", true, func(ok bool) {
			if ok {
				u.store.EmptyTrash()
				u.save()
				u.flash("Emptied the Trash — " + what + " deleted for good")
			}
		}, u.win)
}

// cmdTrash moves what the commands act on (the open card, or the clipped cards) to the Trash.
func (u *ui) cmdTrash() {
	cs := u.targets()
	switch len(cs) {
	case 0:
	case 1:
		u.deleteContact(cs[0])
	default:
		u.bulkDelete(cs)
	}
}

// trashTitles puts the number of cards in the Trash into the Card menu's Show Trash and Empty
// Trash (on a Mac the titles are changed in place; the menu is never rebuilt — see refreshMenu).
func (u *ui) trashTitles() {
	n := len(u.store.Trash)
	show, empty := "Show Trash…", "Empty Trash…"
	if n > 0 {
		show, empty = fmt.Sprintf("Show Trash (%d)…", n), fmt.Sprintf("Empty Trash (%d)…", n)
	}
	setMenuTitle("Show Trash", show)
	setMenuTitle("Empty Trash", empty)
}

// ---------- groups ----------
