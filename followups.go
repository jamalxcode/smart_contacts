package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Follow-ups: something to do for a person on a date — "send Khalid the proposal, Tuesday",
// "call Fatima after Eid". Keep in Touch says how often; a follow-up says what and when.
//
// They're added from Keep in Touch → Follow Up on a Date… (Card menu, the toolbar tab,
// right-click, the back of the card). On the day they come up on the Today page with Done and
// Snooze; the Follow-ups tab lists everyone with one open, soonest first.

const grpFollowUps = "followups"

var iconFlag = svgIcon("flag", "M14.4 6L14 4H5v17h2v-7h5.6l.4 2h7V6z")

// FollowUp is one thing to do for a person on a date.
type FollowUp struct {
	ID      string    `json:"id"`
	Text    string    `json:"text"`
	Due     time.Time `json:"due"`            // the day (local midnight)
	Done    time.Time `json:"done,omitempty"` // zero while open
	Created time.Time `json:"created"`
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.In(time.Local).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// OpenFollowUps are the ones not done yet, soonest first.
func (c *Contact) OpenFollowUps() []FollowUp {
	var out []FollowUp
	for _, f := range c.FollowUps {
		if f.Done.IsZero() {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Due.Before(out[j].Due) })
	return out
}

// DueFollowUps are the open ones due today or before.
func (c *Contact) DueFollowUps(now time.Time) []FollowUp {
	var out []FollowUp
	end := dayOf(now)
	for _, f := range c.OpenFollowUps() {
		if !f.Due.After(end) {
			out = append(out, f)
		}
	}
	return out
}

// whenText says when a follow-up is due, in words: "today", "2 days late", "Tuesday", "12 Dec".
func whenText(due, now time.Time) string {
	d := int(dayOf(due).Sub(dayOf(now)).Hours() / 24)
	switch {
	case d < -1:
		return fmt.Sprintf("%d days late", -d)
	case d == -1:
		return "yesterday"
	case d == 0:
		return "today"
	case d == 1:
		return "tomorrow"
	case d < 7:
		return due.Format("Monday")
	}
	return due.Format("2 Jan")
}

// followUpDialog asks what to do and when, for the open card or the clipped cards.
func (u *ui) followUpDialog(cs []*Contact) {
	if len(cs) == 0 {
		return
	}
	what := newCardEntry()
	what.SetPlaceHolder("What to do — send the proposal, call after Eid…")
	now := time.Now()
	choices := []struct {
		label string
		days  int
	}{{"Tomorrow", 1}, {"In 3 days", 3}, {"Next week", 7}, {"In 2 weeks", 14}, {"Next month", 30}}
	var labels []string
	for _, c := range choices {
		labels = append(labels, fmt.Sprintf("%s  (%s)", c.label, now.AddDate(0, 0, c.days).Format("Mon 2 Jan")))
	}
	labels = append(labels, "On a date:")
	when := widget.NewRadioGroup(labels, nil)
	when.Required = true
	when.SetSelected(labels[2])
	date := newCardEntry()
	date.SetPlaceHolder("e.g. 20 Dec, 2026-12-20 or 20/12")
	date.OnChanged = func(string) { when.SetSelected(labels[len(labels)-1]) }
	form := container.NewVBox(
		boldLabel("Follow up"), what,
		boldLabel("When"), when, date,
	)
	d := newConfirm("Follow Up — "+u.scopeName(cs), "Add", "Cancel", form, func(ok bool) {
		text := strings.TrimSpace(what.Text)
		if !ok {
			return
		}
		if text == "" {
			text = "Follow up"
		}
		due := dayOf(now.AddDate(0, 0, 7))
		for i, l := range labels {
			if l == when.Selected {
				if i < len(choices) {
					due = dayOf(now.AddDate(0, 0, choices[i].days))
				} else if t, ok := parseLooseDate(date.Text); ok {
					due = dayOf(t)
				}
			}
		}
		for _, c := range cs {
			c.FollowUps = append(c.FollowUps, FollowUp{ID: newID(), Text: text, Due: due, Created: now})
			u.store.Put(c)
		}
		u.save()
		u.flash(fmt.Sprintf("Follow up with %s %s: %s", u.scopeName(cs), whenText(due, now), text))
	}, u.win)
	d.Resize(fyne.NewSize(480, 0))
	d.Show()
}

func (u *ui) cmdFollowUp() { u.followUpDialog(u.targets()) }

// finishFollowUp marks a follow-up done, and notes it on the card's timeline.
func (u *ui) finishFollowUp(c *Contact, id string) {
	now := time.Now()
	for i, f := range c.FollowUps {
		if f.ID == id {
			c.FollowUps[i].Done = now
			c.AddEvent(now, "Done: "+f.Text, "You")
		}
	}
	u.store.Put(c)
	u.save()
	u.flash("Done — " + c.DisplayName())
}

// snoozeFollowUp moves a follow-up on by days.
func (u *ui) snoozeFollowUp(c *Contact, id string, days int) {
	for i, f := range c.FollowUps {
		if f.ID == id {
			c.FollowUps[i].Due = dayOf(time.Now().AddDate(0, 0, days))
		}
	}
	u.store.Put(c)
	u.save()
	u.flash(fmt.Sprintf("Moved %s's follow-up to %s", c.DisplayName(), whenText(time.Now().AddDate(0, 0, days), time.Now())))
}

// snoozeMenu offers when to move a follow-up to.
func (u *ui) snoozeMenu(c *Contact, id string, at fyne.Position) {
	item := func(label string, days int) *fyne.MenuItem {
		return fyne.NewMenuItem(label, func() { u.snoozeFollowUp(c, id, days) })
	}
	u.lineMenu(at, item("Tomorrow", 1), item("In 3 days", 3), item("Next week", 7), item("Next month", 30))
}

// followUpRows are a card's open follow-ups, each with Done and Snooze.
func (u *ui) followUpRows(c *Contact) []fyne.CanvasObject {
	var rows []fyne.CanvasObject
	now := time.Now()
	for _, f := range c.OpenFollowUps() {
		f := f
		done := widget.NewButtonWithIcon("Done", theme.ConfirmIcon(), func() { u.finishFollowUp(c, f.ID) })
		done.Importance = widget.LowImportance
		var later *widget.Button
		later = widget.NewButton("Later", func() {
			u.snoozeMenu(c, f.ID, fyne.CurrentApp().Driver().AbsolutePositionForObject(later).Add(fyne.NewPos(0, later.Size().Height)))
		})
		later.Importance = widget.LowImportance
		w := whenText(f.Due, now)
		l := newInkLine(w, []string{f.Text}, nil, nil, nil)
		rows = append(rows, container.NewBorder(nil, nil, nil, container.NewHBox(later, done), l))
	}
	return rows
}
