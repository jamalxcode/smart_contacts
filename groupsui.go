package main

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

func (u *ui) newGroupDialog() {
	e := newCardEntry()
	e.SetPlaceHolder("Family, Work, Clients…")
	showForm("New Group", "Create", "Cancel", []*widget.FormItem{widget.NewFormItem("Name", e)}, func(ok bool) {
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
	items := []*fyne.MenuItem{fyne.NewMenuItem("New Group…", u.newGroupDialog), fyne.NewMenuItem("New Smart Group from Search…", u.saveSmartGroup)}
	if strings.HasPrefix(u.group, "q:") {
		g := u.group[2:]
		items = append(items, fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Edit “"+g+"”…", func() {
				for _, sg := range u.cfg.SmartGroups {
					if sg.Name == g {
						u.deleteSmartGroup(g)
						u.search.SetText(sg.Query)
						u.saveSmartGroup()
					}
				}
			}),
			fyne.NewMenuItem("Delete “"+g+"”", func() { u.deleteSmartGroup(g) }))
	}
	if strings.HasPrefix(u.group, "g:") {
		g := u.group[2:]
		items = append(items, fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Rename “"+g+"”…", func() {
				e := newCardEntry()
				e.SetText(g)
				showForm("Rename Group", "Rename", "Cancel", []*widget.FormItem{widget.NewFormItem("Name", e)}, func(ok bool) {
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
				u.confirmCB("Delete group “"+g+"”?", "The people in it stay in your contacts; only the group goes.", "Delete Group", true, func(ok bool) {
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
