package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ---------- contact picker ----------

// contactPicker is a search box with a short list of matching contacts under it.
type contactPicker struct {
	filter  func(string)
	entry   *widget.Entry
	list    *widget.List
	matches []*Contact
	chosen  *Contact
	obj     fyne.CanvasObject
	onPick  func(*Contact)
}

func (u *ui) newContactPicker(placeholder string, exclude func(*Contact) bool) *contactPicker {
	p := &contactPicker{entry: widget.NewEntry()}
	p.entry.SetPlaceHolder(placeholder)
	p.list = widget.NewList(
		func() int { return len(p.matches) },
		func() fyne.CanvasObject {
			name := widget.NewLabel("name")
			name.Truncation = fyne.TextTruncateEllipsis
			sub := caption("subtitle") // separate label: an Arabic name and a +965 number in one string get reordered
			sub.Truncation = fyne.TextTruncateEllipsis
			return container.NewBorder(nil, nil, newAvatar(26), nil, container.NewGridWithColumns(2, name, sub))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i >= len(p.matches) {
				return
			}
			c := p.matches[i]
			b := o.(*fyne.Container)
			cols := b.Objects[0].(*fyne.Container)
			cols.Objects[0].(*widget.Label).SetText(c.DisplayName())
			cols.Objects[1].(*widget.Label).SetText(c.Subtitle())
			b.Objects[1].(*avatar).set(c, u.photo(c))
		})
	filter := func(q string) {
		p.matches = p.matches[:0]
		for _, c := range u.store.Contacts {
			if (exclude == nil || !exclude(c)) && c.Matches(q) {
				p.matches = append(p.matches, c)
			}
		}
		SortContacts(p.matches, u.cfg.SortByLast)
		p.list.UnselectAll()
		p.list.Refresh()
	}
	p.entry.OnChanged = func(q string) {
		if p.chosen != nil && q == p.chosen.DisplayName() {
			return
		}
		p.chosen = nil
		filter(q)
	}
	p.list.OnSelected = func(i widget.ListItemID) {
		if i >= len(p.matches) || p.matches[i] == p.chosen {
			return
		}
		p.set(p.matches[i])
	}
	p.filter = filter
	filter("")
	box := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
	box.SetMinSize(fyne.NewSize(420, 170))
	p.obj = container.NewBorder(p.entry, nil, nil, nil, container.NewStack(box, p.list))
	return p
}

func (p *contactPicker) set(c *Contact) {
	p.chosen = c
	if c != nil {
		p.entry.SetText(c.DisplayName())
		p.matches = []*Contact{c} // show just the chosen person; typing again searches afresh
		p.list.Refresh()
		p.list.Select(0)
	}
	if p.onPick != nil {
		p.onPick(c)
	}
}

// ---------- on the contact card ----------

// connectionsSection lists c's relationships and network size for the detail view.
func (u *ui) connectionsSection(c *Contact) []fyne.CanvasObject {
	g := NewGraph(u.store, true)
	var rows []fyne.CanvasObject
	for _, l := range u.store.LinksOf(c.ID) {
		o := u.store.Get(l.other(c.ID))
		jump := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() { u.showContact(o.ID) })
		jump.Importance = widget.LowImportance
		oid := o.ID
		remove := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {
			dialog.ShowConfirm("Remove connection?", fmt.Sprintf("%s and %s will no longer be linked as %s.", c.DisplayName(), o.DisplayName(), strings.ToLower(l.Label)), func(ok bool) {
				if ok {
					u.store.RemoveLink(c.ID, oid)
					u.save()
				}
			}, u.win)
		})
		remove.Importance = widget.LowImportance
		rows = append(rows, u.valueRow(l.Label, o.DisplayName(), jump, remove))
	}
	if col := g.Colleagues(c); len(col) > 0 {
		names := []string{}
		for i, x := range col {
			if i == 3 {
				names = append(names, fmt.Sprintf("and %d more", len(col)-3))
				break
			}
			names = append(names, x.DisplayName())
		}
		rows = append(rows, u.valueRow("Same company", strings.Join(names, ", ")))
	}
	d := g.Degrees(c.ID, 3)
	if d[0]+d[1]+d[2] > 0 {
		parts := []string{fmt.Sprintf("%d direct", d[0])}
		if d[1] > 0 {
			parts = append(parts, fmt.Sprintf("%d at 2 steps", d[1]))
		}
		if d[2] > 0 {
			parts = append(parts, fmt.Sprintf("%d at 3 steps", d[2]))
		}
		rows = append(rows, caption("Network: "+strings.Join(parts, " · ")))
	}
	add := widget.NewButtonWithIcon("Connect to…", iconGroup, func() { u.connectDialog(c) })
	add.Importance = widget.LowImportance
	path := widget.NewButtonWithIcon("How connected to…", iconLink, func() { u.pathDialog(c, nil) })
	path.Importance = widget.LowImportance
	rows = append(rows, container.NewHBox(add, path))
	return rows
}

// showContact selects a contact, switching to All Contacts if it isn't in the current list.
func (u *ui) showContact(id string) {
	u.leaveEditor(func() {
		u.selectedID = id
		if u.indexOf(id) < 0 {
			u.group = grpAll
			u.selectSideKey(grpAll)
			u.search.SetText("")
			u.refreshList()
		}
		u.selectedID = id
		u.reselect()
		if i := u.indexOf(id); i >= 0 {
			u.list.ScrollTo(i)
		}
		u.showDetail()
	}, nil)
}

// connectDialog links c to another contact with a relationship.
func (u *ui) connectDialog(c *Contact) {
	linked := map[string]bool{}
	for _, l := range u.store.LinksOf(c.ID) {
		linked[l.other(c.ID)] = true
	}
	pick := u.newContactPicker("Search for the person…", func(x *Contact) bool { return x.ID == c.ID || linked[x.ID] })
	rel := widget.NewSelectEntry(RelationLabels)
	rel.SetText("Friend")
	info := caption("Connections stay on this Mac. They're used to show how people are linked.")
	info.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(
		widget.NewLabel("Connect "+c.DisplayName()+" with:"),
		pick.obj,
		container.NewBorder(nil, nil, widget.NewLabel("as"), nil, rel),
		info,
	)
	var d *dialog.ConfirmDialog
	d = dialog.NewCustomConfirm("Connect", "Connect", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		if pick.chosen == nil {
			dialog.ShowInformation("Pick someone", "Choose a person from the list first.", u.win)
			return
		}
		u.store.AddLink(c.ID, pick.chosen.ID, rel.Text)
		u.save()
		u.flash(fmt.Sprintf("Connected %s and %s", c.DisplayName(), pick.chosen.DisplayName()))
	}, u.win)
	d.Resize(fyne.NewSize(520, 440))
	d.Show()
	u.win.Canvas().Focus(pick.entry)
}

// pathDialog finds the chain between two people, up to six steps.
func (u *ui) pathDialog(a, b *Contact) {
	from := u.newContactPicker("From…", nil)
	to := u.newContactPicker("To…", nil)
	useCo := widget.NewCheck("People at the same company count as connected", nil)
	useCo.SetChecked(true)
	result := container.NewVBox()
	run := func() {
		result.Objects = nil
		if from.chosen == nil || to.chosen == nil {
			result.Add(caption("Pick two people."))
			result.Refresh()
			return
		}
		g := NewGraph(u.store, useCo.Checked)
		chain := g.Path(from.chosen.ID, to.chosen.ID, MaxDegrees)
		switch {
		case from.chosen.ID == to.chosen.ID:
			result.Add(widget.NewLabel("That's the same person."))
		case chain == nil:
			msg := widget.NewLabel(fmt.Sprintf("No chain of %d steps or fewer links %s and %s yet.\nAdd connections on their cards (Connect to…) to build your network.",
				MaxDegrees, from.chosen.DisplayName(), to.chosen.DisplayName()))
			msg.Wrapping = fyne.TextWrapWord
			result.Add(msg)
		default:
			steps := len(chain) - 1
			word := "steps"
			if steps == 1 {
				word = "step"
			}
			result.Add(widget.NewLabelWithStyle(fmt.Sprintf("%d %s apart", steps, word), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
			for _, st := range chain {
				st := st
				if st.Via != "" {
					result.Add(caption("      ↓  " + st.Via))
				}
				btn := widget.NewButtonWithIcon(st.Contact.DisplayName(), theme.AccountIcon(), nil)
				btn.Alignment = widget.ButtonAlignLeading
				btn.Importance = widget.LowImportance
				result.Add(btn)
				btn.OnTapped = func() { u.showContact(st.Contact.ID) }
			}
		}
		result.Refresh()
	}
	from.onPick = func(*Contact) { run() }
	to.onPick = func(*Contact) { run() }
	useCo.OnChanged = func(bool) { run() }
	if a != nil {
		from.set(a)
	}
	if b != nil {
		to.set(b)
	}
	pickers := container.NewGridWithColumns(2, from.obj, to.obj)
	content := container.NewBorder(container.NewVBox(pickers, useCo, widget.NewSeparator()), nil, nil, nil, container.NewVScroll(result))
	d := dialog.NewCustom("How are they connected?", "Close", content, u.win)
	d.Resize(fyne.NewSize(900, 640))
	d.Show()
	if a == nil {
		u.win.Canvas().Focus(from.entry)
	} else {
		u.win.Canvas().Focus(to.entry)
	}
	run()
}
