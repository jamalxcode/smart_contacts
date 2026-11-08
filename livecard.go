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

// Writing on a card (readcard.go has reading it). Fields look like writing on ruled lines, with
// no box until clicked. Revert appears once something changes, and leaving the card with unsaved
// changes asks "Are you sure you want to update…?".

// liveTheme makes text fields on the card look like writing on a ruled page: no fill and no box
// until clicked (the blue focus box shows where you're writing).
type liveTheme struct{ fyne.Theme }

func (t *liveTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameInputBackground:
		return color.Transparent // the page shows through
	case theme.ColorNameInputBorder:
		return color.Transparent // a ruled line is drawn under each field instead (ruled.go)
	}
	return t.Theme.Color(n, v)
}

// liveCard is the card with the pen picked up: every field writable, the one clicked ready to
// type in. Done (or Esc) puts the pen down; Revert throws the changes away.
func (u *ui) liveCard(e *editor, body *fyne.Container) fyne.CanvasObject {
	c := e.orig
	all := append([]fyne.CanvasObject{}, body.Objects...)
	all = append(all, u.cardMeta(c))
	fields := container.NewVBox(all...)
	content := container.NewThemeOverride(container.NewStack(fields, newRuleOverlay(fields)), &liveTheme{fyne.CurrentApp().Settings().Theme()})

	done := widget.NewButtonWithIcon("Done", theme.ConfirmIcon(), u.finishWriting)
	done.Importance = widget.HighImportance
	e.revertBtn = widget.NewButton("Revert", func() {
		u.ed = nil
		u.showDetail()
		u.flash("Changes discarded")
	})
	e.saveBtn = done
	e.revertBtn.Hide()
	writing := caption("Writing on the card — Done or Esc when finished")
	if u.learned("write") {
		writing.SetText("Writing on the card")
	}
	cornerRoom := canvas.NewRectangle(color.Transparent) // the turned-up page corner sits here
	cornerRoom.SetMinSize(fyne.NewSize(26, 1))
	bar := container.NewBorder(widget.NewSeparator(), nil, writing,
		container.NewHBox(e.revertBtn, done, cornerRoom), layout.NewSpacer())
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
					e.revertBtn.Show()
					e.u.status.SetText("Unsaved changes to " + e.c.DisplayName() + " — Done saves them")
					e.u.fitStatus()
				} else {
					e.revertBtn.Hide()
					e.u.status.SetText(e.u.statusText())
					e.u.fitStatus()
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
	// the wastebasket sits right after message: for a card you don't need any more
	trash := newRoundAction("trash", theme.DeleteIcon(), func() { u.deleteContact(c) })
	trashed := false
	for _, a := range c.Actions(u.cfg.CountryCode) {
		a := a
		if a.More || shown == 6 {
			more = append(more, fyne.NewMenuItem(a.Title, func() { u.open(c, a.Link) }))
			continue
		}
		actions.Add(newRoundAction(a.Title, actionIcon(a.Kind), func() { u.open(c, a.Link) }))
		shown++
		if a.Kind == "message" {
			actions.Add(trash)
			trashed = true
		}
	}
	if !trashed { // no number to message: it goes first
		actions.Objects = append([]fyne.CanvasObject{trash}, actions.Objects...)
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
