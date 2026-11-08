package main

import (
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The card is editable in place: there's no Edit button. Fields look like plain text with a
// hairline outline until clicked. Save and Revert appear at the bottom once something changes,
// and leaving the card with unsaved changes asks "Are you sure you want to update…?".

// liveTheme makes text fields on the card look like text: no grey fill, a faint outline.
type liveTheme struct{ fyne.Theme }

func (t *liveTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameInputBackground:
		return color.Transparent // the page shows through
	case theme.ColorNameInputBorder:
		return t.Theme.Color(theme.ColorNameSeparator, v)
	}
	return t.Theme.Color(n, v)
}

func (u *ui) liveCard(e *editor, body *fyne.Container) fyne.CanvasObject {
	c := e.orig
	// what goes between the name block and the fields: ways to reach them, notices
	var extras []fyne.CanvasObject
	if a := u.actionsRow(c); a != nil {
		extras = append(extras, a)
	}
	if lt := u.lastTalkedLine(c); lt != nil {
		extras = append(extras, lt)
	}
	if n := u.attention[c.ID]; n > 0 {
		review := widget.NewButtonWithIcon(plural(n, "suggestion")+" to tidy this card — Review…", theme.WarningIcon(),
			func() { u.leaveEditor(func() { u.cleanupFor([]*Contact{c}, c.DisplayName()) }, nil) })
		review.Importance = widget.LowImportance
		review.Alignment = widget.ButtonAlignLeading
		extras = append(extras, review)
	}
	objs := append([]fyne.CanvasObject{}, body.Objects...)
	head := 1 // the photo and name fields come first
	all := append(append(append([]fyne.CanvasObject{}, objs[:head]...), extras...), objs[head:]...)
	all = append(all, widget.NewSeparator(), u.foldingSections(c))
	meta := []string{"Added " + c.Created.Format("2 Jan 2006")}
	if len(c.Sources) > 0 {
		meta = append(meta, "from "+strings.Join(c.Sources, ", "))
	}
	m := caption(strings.Join(meta, " · "))
	m.Wrapping = fyne.TextWrapWord
	all = append(all, m)
	content := container.NewThemeOverride(container.NewVBox(all...), &liveTheme{fyne.CurrentApp().Settings().Theme()})

	// bottom bar
	note := widget.NewButtonWithIcon("Note", theme.DocumentCreateIcon(), func() { u.addNote(c) })
	var keep *widget.Button
	keep = widget.NewButtonWithIcon("Keep in Touch", theme.MailReplyIcon(), func() {
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(keep)
		widget.ShowPopUpMenuAtPosition(u.keepInTouchMenu(c), u.win.Canvas(), pos.Add(fyne.NewPos(0, -270)))
	})
	note.Importance, keep.Importance = widget.LowImportance, widget.LowImportance
	var share *widget.Button
	share = widget.NewButtonWithIcon("", theme.UploadIcon(), func() {
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(share)
		menu := fyne.NewMenu("",
			fyne.NewMenuItem("Copy Card as Text", func() { u.copyCard(c) }),
			fyne.NewMenuItem("Save as vCard…", func() { u.exportVCF([]*Contact{c}, c.DisplayName()) }),
			fyne.NewMenuItem("Copy to Apple Contacts…", func() { u.sendToMacContacts([]*Contact{c}) }),
		)
		widget.ShowPopUpMenuAtPosition(menu, u.win.Canvas(), pos.Add(fyne.NewPos(0, -96)))
	})
	e.revertBtn = widget.NewButton("Revert", func() {
		u.ed = nil
		u.showDetail()
		u.flash("Changes discarded")
	})
	e.saveBtn = widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), e.save)
	e.saveBtn.Importance = widget.HighImportance
	e.revertBtn.Hide()
	e.saveBtn.Hide()
	bar := container.NewBorder(widget.NewSeparator(), nil, container.NewHBox(note, keep),
		container.NewHBox(share, e.revertBtn, e.saveBtn), layout.NewSpacer())
	return container.NewBorder(nil, container.NewPadded(bar), nil, nil, container.NewVScroll(container.NewPadded(content)))
}

// watch shows Save / Revert while the card has unsaved changes.
func (e *editor) watch() {
	go func() {
		for {
			time.Sleep(400 * time.Millisecond)
			stop := false
			fyne.Do(func() {
				if e.u.ed != e || e.saveBtn == nil {
					stop = true
					return
				}
				d := e.dirty()
				if d == e.dirtyShown {
					return
				}
				e.dirtyShown = d
				if d {
					e.saveBtn.Show()
					e.revertBtn.Show()
					e.u.status.SetText("Unsaved changes to " + e.c.DisplayName() + " — ⌘S saves")
				} else {
					e.saveBtn.Hide()
					e.revertBtn.Hide()
					e.u.status.SetText(e.u.statusText())
				}
			})
			if stop {
				return
			}
		}
	}()
}

// actionsRow is the round message / call / video / mail buttons for c.
func (u *ui) actionsRow(c *Contact) fyne.CanvasObject {
	actions := container.NewGridWrap(fyne.NewSize(62, roundSize+22))
	var more []*fyne.MenuItem
	shown := 0
	for _, a := range c.Actions(u.cfg.CountryCode) {
		a := a
		if a.More || shown == 6 {
			more = append(more, fyne.NewMenuItem(a.Title, func() { u.open(c, a.Link) }))
			continue
		}
		actions.Add(newRoundAction(a.Title, actionIcon(a.Kind), func() { u.open(c, a.Link) }))
		shown++
	}
	if len(more) > 0 {
		var mb *roundAction
		mb = newRoundAction("More", theme.MoreHorizontalIcon(), func() {
			pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(mb)
			widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", more...), u.win.Canvas(), pos.Add(fyne.NewPos(0, roundSize)))
		})
		actions.Add(mb)
	}
	if len(actions.Objects) == 0 {
		return nil
	}
	return actions
}

// lastTalkedLine is "Last talked 3 days ago · WhatsApp · Keep in touch every month".
func (u *ui) lastTalkedLine(c *Contact) fyne.CanvasObject {
	lt, how := u.lastTalked(c)
	if lt.IsZero() && c.KeepInTouch == 0 {
		return nil
	}
	var parts []string
	if !lt.IsZero() {
		s := "Last talked " + Ago(lt, time.Now())
		if how != "" {
			s += " · " + how
		}
		parts = append(parts, s)
	}
	if c.KeepInTouch > 0 {
		k := "Keep in touch " + keepText(c.KeepInTouch)
		if u.dueToReachOut(c, time.Now()) {
			k += " — due now"
		}
		parts = append(parts, k)
	}
	t := canvas.NewText(strings.Join(parts, "  ·  "), theme.Color(theme.ColorNamePrimary))
	t.TextSize = theme.CaptionTextSize()
	return container.NewPadded(t)
}
