package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// A tidy session goes through the cards that need tidying one at a time: what's wrong with
// this card, and Fix, Move to Trash or Skip. A counter shows how far you've got. Fix opens the
// fix (or the card, to add a missing number) and the session picks up again where it was from
// Tidy Up → One card at a time, or the Today page; skipped cards stay skipped until you quit.

// tidyQueue is the cards still to go through: needing tidying, and not skipped.
func (u *ui) tidyQueue() []*Contact {
	var out []*Contact
	for _, c := range u.store.Contacts {
		if u.tidySkipped[c.ID] {
			continue
		}
		if u.attention[c.ID] > 0 || (len(c.Phones) == 0 && len(c.Emails) == 0) {
			out = append(out, c)
		}
	}
	return out
}

// tidyProblems says what's wrong with a card, in a line each.
func (u *ui) tidyProblems(c *Contact) []string {
	var out []string
	if len(c.Phones) == 0 && len(c.Emails) == 0 {
		out = append(out, "No phone number and no email")
	}
	for _, ch := range PlanCleanup([]*Contact{c.Clone()}, u.cfg.CountryCode) {
		if ch.Default || ch.Kind == ckUnsure {
			short := func(v string) bool { return v != "" && len(v) < 40 && !strings.ContainsAny(v, "\n") }
			line := ch.Reason
			switch {
			case ch.Field != "notes" && short(ch.Old) && short(ch.New):
				line = fmt.Sprintf("%s → %s: %s", ch.Old, ch.New, ch.Reason)
			case ch.Field != "notes" && short(ch.Old):
				line = ch.Old + ": " + ch.Reason
			case ch.Field != "":
				line = strings.ToUpper(ch.Field[:1]) + ch.Field[1:] + ": " + ch.Reason
			}
			out = append(out, line)
		}
		if len(out) == 6 {
			break
		}
	}
	return out
}

// tidySession shows the next card to tidy.
func (u *ui) tidySession() {
	if u.tidySkipped == nil {
		u.tidySkipped = map[string]bool{}
	}
	q := u.tidyQueue()
	if len(q) == 0 {
		msg := "Every card is in order."
		if len(u.tidySkipped) > 0 {
			msg = fmt.Sprintf("You've been through them all. %d skipped — they come back next time you open the app.", len(u.tidySkipped))
		}
		showInfo("Nothing left to tidy", msg, u.win)
		return
	}
	if u.tidyTotal < len(q)+len(u.tidySkipped) {
		u.tidyTotal = len(q) + len(u.tidySkipped)
	}
	c := q[0]
	u.openCard(c.ID) // the card is open on the right page while you decide
	var d *paperDialog
	box := container.NewVBox()
	pos := u.tidyTotal - len(q) + 1
	box.Add(caption(fmt.Sprintf("Card %d of %d · %d left", pos, u.tidyTotal, len(q))))
	name := widget.NewLabelWithStyle(c.DisplayName(), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	box.Add(name)
	if s := c.Subtitle(); s != "" {
		box.Add(caption(s))
	}
	for i, p := range u.tidyProblems(c) {
		box.Add(newNumberedStep(i+1, p))
	}
	next := func() { d.Hide(); u.tidySession() }
	missing := len(c.Phones) == 0 && len(c.Emails) == 0
	fixLabel := "Fix…"
	if missing && u.attention[c.ID] == 0 {
		fixLabel = "Add a Number…"
	}
	fix := newPaperButton(fixLabel, btnPrimary, func() {
		d.Hide()
		if u.attention[c.ID] > 0 {
			u.cleanupFor([]*Contact{c}, c.DisplayName())
		} else {
			u.writeCard(func(e *editor) fyne.Focusable { return e.phoneEntry(0) })
		}
	})
	trash := newPaperButton("Move to Trash", btnDanger, func() {
		u.deleteContact(c)
		next()
	})
	skip := newPaperButton("Skip", btnPlain, func() {
		u.tidySkipped[c.ID] = true
		next()
	})
	stop := newPaperButton("Stop", btnPlain, func() { d.Hide() })
	d = newCustomWithoutButtons("Tidy One by One", container.NewVScroll(box), u.win)
	d.clipboard = true
	d.SetButtons([]fyne.CanvasObject{stop, skip, trash, fix})
	d.Resize(fyne.NewSize(560, 340))
	d.Show()
}
