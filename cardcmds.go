package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// The Card commands, shared by the Card menu and the organiser's toolbar. There are eleven, and
// one rule for what they act on: the clipped cards if any are clipped, otherwise the card that's
// open. (The toolbar says which, at its left.) Tidy Up and Find Duplicates ask in their window,
// because they can also take the whole address book.

// targets is what the card commands act on now: the clipped cards, or the open card, or nothing.
func (u *ui) targets() []*Contact {
	if u.selectMode && len(u.picked) > 0 {
		return u.pickedContacts()
	}
	if u.busy() {
		return nil
	}
	if c := u.store.Get(u.selectedID); c != nil {
		return []*Contact{c}
	}
	return nil
}

// targetText says what the commands will act on, for the toolbar.
func (u *ui) targetText(cs []*Contact) string {
	switch {
	case len(cs) == 0 && u.ed != nil && u.ed.orig != nil:
		return "Writing on " + u.ed.orig.DisplayName()
	case len(cs) == 0 && u.ed != nil:
		return "Writing a new card"
	case len(cs) == 0:
		return "No card open"
	case len(cs) == 1 && !(u.selectMode && len(u.picked) > 0):
		return cs[0].DisplayName()
	}
	return fmt.Sprintf("%d clipped %s", len(cs), pluralWords(len(cs), "card", "cards"))
}

// ---------- Add Note ----------

func (u *ui) cmdNote() {
	cs := u.targets()
	switch len(cs) {
	case 0:
	case 1:
		u.addNote(cs[0])
	default:
		u.addNoteTo(cs)
	}
}

// ---------- Favorite ----------

// allFavorite reports whether every card in cs is a favorite.
func allFavorite(cs []*Contact) bool {
	for _, c := range cs {
		if !c.Favorite {
			return false
		}
	}
	return len(cs) > 0
}

func (u *ui) cmdFavorite() {
	cs := u.targets()
	switch len(cs) {
	case 0:
	case 1:
		u.toggleFavorite(cs[0])
	default:
		fav := !allFavorite(cs)
		u.bulkFavorite(cs, fav)
		if fav {
			u.flash(fmt.Sprintf("Added %d clipped cards to Favorites", len(cs)))
		} else {
			u.flash(fmt.Sprintf("Removed %d clipped cards from Favorites", len(cs)))
		}
	}
}

// ---------- Keep in Touch ----------

// keepItems are the Keep in Touch choices, for whatever the commands act on.
func (u *ui) keepItems() []*fyne.MenuItem {
	var items []*fyne.MenuItem
	choices := append(append([]struct {
		Days  int
		Label string
	}{}, keepChoices...), struct {
		Days  int
		Label string
	}{0, "No Reminder"})
	for i, k := range choices {
		k := k
		if i == len(choices)-1 {
			items = append(items, fyne.NewMenuItemSeparator())
		}
		items = append(items, fyne.NewMenuItem(k.Label, func() { u.cmdKeep(k.Days) }))
	}
	return items
}

func (u *ui) cmdKeep(days int) {
	cs := u.targets()
	switch len(cs) {
	case 0:
	case 1:
		u.setKeepInTouch(cs[0], days)
	default:
		u.bulkKeep(cs, days)
	}
}

// ---------- Groups ----------

// cmdGroups ticks or unticks groups for the open card or the clipped cards. A group only some of
// the clipped cards are in is shown unticked and left alone unless you tick it.
func (u *ui) cmdGroups() {
	cs := u.targets()
	if len(cs) == 0 {
		return
	}
	groups := u.store.Groups()
	count := map[string]int{}
	for _, c := range cs {
		for _, g := range groups {
			if c.HasGroup(g) {
				count[g]++
			}
		}
	}
	var had, some []string
	for _, g := range groups {
		switch count[g] {
		case len(cs):
			had = append(had, g)
		case 0:
		default:
			some = append(some, g)
		}
	}
	check := widget.NewCheckGroup(groups, nil)
	check.SetSelected(had)
	newG := widget.NewEntry()
	newG.SetPlaceHolder("New group name")
	newG.OnSubmitted = func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, g := range check.Options {
			if strings.EqualFold(g, s) {
				s = g
			}
		}
		if !contains(check.Options, s) {
			check.Append(s)
		}
		if !contains(check.Selected, s) {
			check.SetSelected(append(append([]string{}, check.Selected...), s))
		}
		newG.SetText("")
	}
	items := []fyne.CanvasObject{}
	if len(groups) == 0 {
		items = append(items, caption("No groups yet — type a name below and press Return."))
	}
	items = append(items, container.NewVScroll(check))
	if len(some) > 0 {
		l := caption("Some of them are already in " + strings.Join(some, ", ") + ". Those stay as they are unless you tick them.")
		l.Wrapping = fyne.TextWrapWord
		items = append(items, l)
	}
	items = append(items, newG)
	who := u.scopeName(cs)
	d := dialog.NewCustomConfirm("Groups — "+who, "Done", "Cancel", container.NewBorder(nil, container.NewVBox(items[1:]...), nil, nil, items[0]), func(ok bool) {
		if !ok {
			return
		}
		newG.OnSubmitted(newG.Text) // a name typed but not entered still counts
		want := map[string]bool{}
		for _, g := range check.Selected {
			want[g] = true
		}
		changed := 0
		for _, c := range cs {
			before := strings.Join(c.Groups, "|")
			for _, g := range check.Options {
				switch {
				case want[g] && !c.HasGroup(g):
					c.Groups = append(c.Groups, g)
				case !want[g] && contains(had, g): // untick a group they were all in: out of it
					keep := c.Groups[:0]
					for _, x := range c.Groups {
						if !strings.EqualFold(x, g) {
							keep = append(keep, x)
						}
					}
					c.Groups = keep
				}
			}
			if strings.Join(c.Groups, "|") != before {
				u.store.Put(c)
				changed++
			}
		}
		if changed > 0 {
			u.save()
			u.flash("Groups changed for " + who)
		}
	}, u.win)
	d.Resize(fyne.NewSize(440, 460))
	trackDialog(d, nil)
	d.Show()
}

// ---------- Connections ----------

// connectionsEnabled: one card open, or two clipped (then it shows how those two are linked).
func (u *ui) connectionsEnabled(cs []*Contact) bool {
	return len(cs) == 1 || len(cs) == 2
}

func (u *ui) cmdConnections() {
	cs := u.targets()
	switch len(cs) {
	case 1:
	case 2:
		u.pathDialog(cs[0], cs[1])
		return
	default:
		return
	}
	c := cs[0]
	rows := u.connectionsSection(c)
	var body fyne.CanvasObject
	if len(rows) == 0 {
		l := caption(c.DisplayName() + " isn't connected to anyone yet.")
		body = l
	} else {
		body = container.NewVScroll(container.NewVBox(rows...))
	}
	var d *dialog.CustomDialog
	connect := widget.NewButton("Connect to…", func() { d.Hide(); u.connectDialog(c) })
	how := widget.NewButton("How Are They Connected?…", func() { d.Hide(); u.pathDialog(c, nil) })
	closeB := widget.NewButton("Close", func() { d.Hide() })
	connect.Importance = widget.HighImportance
	d = dialog.NewCustomWithoutButtons("Connections — "+c.DisplayName(),
		container.NewBorder(nil, container.NewHBox(how, layoutSpacer(), closeB, connect), nil, nil, body), u.win)
	d.Resize(fyne.NewSize(520, 420))
	trackDialog(d, connect.OnTapped)
	d.Show()
}

// ---------- choosing what a whole-book command acts on ----------

type scopeChoice struct {
	label string
	cs    []*Contact
}

// scopeChoices: the open card, the clipped cards (if any), and the whole address book. min is
// the fewest cards a choice needs (2 for finding duplicates).
func (u *ui) scopeChoices(min int) (opts []scopeChoice, def int) {
	if u.selectMode && len(u.picked) >= min {
		cs := u.pickedContacts()
		opts = append(opts, scopeChoice{fmt.Sprintf("The %d clipped cards", len(cs)), cs})
	} else if c := u.store.Get(u.selectedID); c != nil && min <= 1 && !u.busy() {
		opts = append(opts, scopeChoice{c.DisplayName() + "’s card", []*Contact{c}})
	}
	opts = append(opts, scopeChoice{fmt.Sprintf("All %d cards", len(u.store.Contacts)), u.store.Contacts})
	return opts, 0
}

func radioOf(opts []scopeChoice, def int) *widget.RadioGroup {
	var labels []string
	for _, o := range opts {
		labels = append(labels, o.label)
	}
	r := widget.NewRadioGroup(labels, nil)
	r.Required = true
	r.SetSelected(labels[def])
	return r
}

func chosen(opts []scopeChoice, r *widget.RadioGroup) []*Contact {
	for _, o := range opts {
		if o.label == r.Selected {
			return o.cs
		}
	}
	return nil
}

// ---------- Tidy Up ----------

// cmdTidy: one window for both kinds of tidying, on the card, the clipped cards or everyone.
func (u *ui) cmdTidy() {
	opts, def := u.scopeChoices(1)
	scope := radioOf(opts, def)
	const fix, notes = "Numbers, names, emails and links", "Jumbled notes — rewrite them with AI"
	what := widget.NewRadioGroup([]string{fix, notes}, nil)
	what.Required = true
	what.SetSelected(fix)
	var aiNote fyne.CanvasObject = layoutSpacer()
	if !u.cloudAI() {
		l := caption("Rewriting notes needs Cloud AI (Settings → Cloud AI).")
		l.Wrapping = fyne.TextWrapWord
		aiNote = l
	}
	form := container.NewVBox(boldLabel("Tidy"), scope, widget.NewSeparator(), boldLabel("What"), what, aiNote,
		caption("You'll see every change before it's made, and Undo puts it back."))
	d := dialog.NewCustomConfirm("Tidy Up", "Look", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		cs := chosen(opts, scope)
		if what.Selected == notes {
			u.tidyNotesAI(cs, len(cs) == 1)
			return
		}
		intro := ""
		if len(cs) == 1 {
			intro = cs[0].DisplayName()
		}
		u.cleanupFor(cs, intro)
	}, u.win)
	d.Resize(fyne.NewSize(460, 0))
	trackDialog(d, nil)
	d.Show()
}

// ---------- Find Duplicates ----------

func (u *ui) cmdDuplicates() {
	var opts []scopeChoice
	if u.selectMode && len(u.picked) >= 2 {
		cs := u.pickedContacts()
		opts = append(opts,
			scopeChoice{fmt.Sprintf("Merge the %d clipped cards into one", len(cs)), cs},
			scopeChoice{fmt.Sprintf("Look for duplicates among the %d clipped cards", len(cs)), cs})
	}
	all := scopeChoice{fmt.Sprintf("Look for duplicates in all %d cards", len(u.store.Contacts)), u.store.Contacts}
	opts = append(opts, all)
	if len(opts) == 1 { // nothing to choose: just look
		u.duplicates()
		return
	}
	r := radioOf(opts, 0)
	form := container.NewVBox(r, caption("Nothing is merged until you choose which details to keep."))
	d := dialog.NewCustomConfirm("Find Duplicates", "Go", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		switch r.Selected {
		case opts[0].label:
			u.bulkMerge(opts[0].cs)
		case opts[1].label:
			u.duplicatesFor(opts[1].cs)
		default:
			u.duplicates()
		}
	}, u.win)
	d.Resize(fyne.NewSize(460, 0))
	trackDialog(d, nil)
	d.Show()
}

// ---------- Share ----------

// shareItems: copy, vCard or Apple Contacts, for what the commands act on.
func (u *ui) shareItems() []*fyne.MenuItem {
	return []*fyne.MenuItem{
		fyne.NewMenuItem("Copy as Text", func() {
			cs := u.targets()
			switch len(cs) {
			case 0:
			case 1:
				u.copyCard(cs[0])
			default:
				u.copyCards(cs)
			}
		}),
		fyne.NewMenuItem("Save as vCard…", func() {
			if cs := u.targets(); len(cs) > 0 {
				u.exportVCF(cs, u.scopeName(cs))
			}
		}),
		fyne.NewMenuItem("Copy to Apple Contacts…", func() {
			if cs := u.targets(); len(cs) > 0 {
				u.sendToMacContacts(cs)
			}
		}),
	}
}

// ---------- Check Messages, Calls & Mail ----------

// cmdCheckMessages updates "last talked" from Messages, calls and Mail, then lists the people
// you talk to who aren't in your contacts.
func (u *ui) cmdCheckMessages() {
	u.actThen = func() {
		if len(u.act.Unknown) > 0 {
			u.unknownPeople()
		}
	}
	u.readActivity(true)
}

// ---------- small helpers ----------

func layoutSpacer() fyne.CanvasObject { return layout.NewSpacer() }

func boldLabel(s string) *widget.Label {
	return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}
