package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	grpReachOut  = "reachout"
	grpAttention = "attention"
)

// ---------- reading activity ----------

// readActivity reads Messages, call history and Mail in the background. interactive explains a
// missing Full Disk Access permission; the automatic read at launch stays quiet.
func (u *ui) readActivity(interactive bool) {
	if u.actBusy {
		return
	}
	u.actBusy = true
	if interactive {
		u.flash("Reading Messages, calls and Mail…")
	}
	go func() {
		home, _ := os.UserHomeDir()
		var touches []Touch
		var problems []string
		denied := 0
		for _, src := range []struct {
			name string
			read func(string) ([]Touch, error)
		}{{"Messages", ReadMessages}, {"Calls", ReadCalls}, {"Mail", ReadMail}} {
			ts, err := src.read(home)
			switch {
			case errors.Is(err, ErrNoAccess):
				denied++
				problems = append(problems, src.name+": needs Full Disk Access")
			case err != nil:
				problems = append(problems, src.name+": "+err.Error())
			default:
				touches = append(touches, ts...)
			}
		}
		fyne.Do(func() {
			u.actBusy = false
			if denied == 3 {
				if interactive {
					u.askFullDiskAccess()
				}
				return
			}
			u.act.Touches, u.act.Problems, u.act.Read = touches, problems, time.Now()
			u.applyActivity()
			if !u.cfg.AutoActivity {
				u.cfg.AutoActivity = true // it works: keep it fresh each time the app opens
				_ = u.cfg.Save(u.store.Dir())
			}
			if interactive {
				msg := fmt.Sprintf("Found history for %d people", len(u.act.People))
				if n := len(u.act.Unknown); n > 0 {
					msg += fmt.Sprintf(" · %d people you talk to aren't in your contacts", n)
				}
				u.flash(msg)
				if len(problems) > 0 {
					showInfo("Some history couldn't be read", strings.Join(problems, "\n"), u.win)
				}
			}
			if then := u.actThen; then != nil {
				u.actThen = nil
				then()
			}
		})
	}()
}

// applyActivity matches the history to the contacts again (after a read or when people change).
func (u *ui) applyActivity() {
	ignored := map[string]bool{}
	for _, k := range u.cfg.IgnoredKeys {
		ignored[k] = true
	}
	u.act.Apply(u.store.Contacts, u.act.All(), ignored)
	_ = u.act.Save(u.store.Dir())
	u.refreshAll()
}

func (u *ui) askFullDiskAccess() {
	msg := widget.NewLabel("To see when you last talked to each person, Smart Contacts reads the Messages, call history and Mail " +
		"databases on this Mac. Nothing leaves the Mac.\n\nmacOS keeps these private until you allow it:\n" +
		"1. Open System Settings → Privacy & Security → Full Disk Access.\n" +
		"2. Turn on Smart Contacts (use + to add it from Applications if it isn't listed).\n" +
		"3. Quit and reopen Smart Contacts, then choose Card → Check Messages….\n\n" +
		"WhatsApp can't be read this way: export a chat (in the chat, ⋯ → Export Chat) and drop the file on the window.")
	msg.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustomConfirm("Allow reading your history", "Open Privacy Settings", "Not Now", msg, func(ok bool) {
		if ok {
			p, _ := url.Parse("x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles")
			_ = u.app.OpenURL(p)
		}
	}, u.win)
	d.Resize(fyne.NewSize(560, 0))
	trackDialog(d, nil)
	d.Show()
}

// importWhatsApp reads a chat export dropped on the window or chosen in Import.
func (u *ui) importWhatsApp(name string, data []byte) {
	ts, chat, err := ReadWhatsAppExport(name, data)
	if err != nil {
		showErr(err, u.win)
		return
	}
	// In a one-to-one chat the export is named after the other person; record that name too, so
	// the chat matches even if their messages show a number.
	if chat != "" && len(ts) <= 3 {
		var last time.Time
		n := 0
		for _, t := range ts {
			if !strings.EqualFold(t.Show, chat) && t.Last.After(last) {
				last = t.Last
			}
			n += t.Count
		}
		if last.IsZero() {
			for _, t := range ts {
				if t.Last.After(last) {
					last = t.Last
				}
			}
		}
		ts = append(ts, Touch{Key: "n:" + fold(chat), Show: chat, Channel: "WhatsApp", Last: last, Count: n})
	}
	u.act.AddWhatsApp(ts)
	u.applyActivity()
	matched := 0
	for _, t := range ts {
		if u.matchTouch(t) != nil {
			matched++
		}
	}
	what := "chat"
	if chat != "" {
		what = "chat with " + chat
	}
	u.flash(fmt.Sprintf("Read the WhatsApp %s: %d of %d people matched", what, matched, len(ts)))
}

func (u *ui) matchTouch(t Touch) *Contact {
	for _, c := range u.store.Contacts {
		for _, p := range c.Phones {
			if phoneKey(p.Value) == t.Key {
				return c
			}
		}
		for _, e := range c.Emails {
			if "e:"+strings.ToLower(strings.TrimSpace(e.Value)) == t.Key {
				return c
			}
		}
		if t.Key == "n:"+fold(c.DisplayName()) || (c.AltName != "" && t.Key == "n:"+fold(c.AltName)) {
			return c
		}
	}
	return nil
}

// unknownPeople lists people you talk to who aren't in your contacts, to add or ignore.
func (u *ui) unknownPeople() {
	if u.act.Read.IsZero() && len(u.act.WhatsApp) == 0 {
		u.readActivity(true)
		return
	}
	list := container.NewVBox()
	var d *dialog.CustomDialog
	var fill func()
	fill = func() {
		list.Objects = nil
		if len(u.act.Unknown) == 0 {
			list.Add(widget.NewLabel("Everyone you write to or call often is in your contacts."))
		}
		for _, t := range u.act.Unknown {
			t := t
			who := widget.NewLabelWithStyle(t.Show, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			info := caption(fmt.Sprintf("%s · %d · last %s", t.Channel, t.Count, Ago(t.Last, time.Now())))
			add := widget.NewButton("Add…", func() {
				d.Hide()
				c := NewContact()
				if strings.HasPrefix(t.Key, "e:") {
					c.Emails = []Field{{Label: "Work", Value: t.Show}}
				} else {
					c.Phones = []Field{{Label: "Mobile", Value: t.Show}}
				}
				u.openEditorCard(nil, c, "Someone you talk to ("+t.Channel+") — add their name", nil, "")
			})
			ignore := widget.NewButton("Not a Contact", func() {
				u.cfg.IgnoredKeys = append(u.cfg.IgnoredKeys, t.Key)
				_ = u.cfg.Save(u.store.Dir())
				u.applyActivity()
				fill()
			})
			add.Importance = widget.HighImportance
			list.Add(container.NewBorder(nil, nil, nil, container.NewHBox(ignore, add), container.NewVBox(who, info)))
		}
		list.Refresh()
	}
	fill()
	intro := widget.NewLabel("People you've written to or called several times who aren't in your contacts (from Messages, calls and Mail on this Mac).")
	intro.Wrapping = fyne.TextWrapWord
	d = dialog.NewCustom("People Not in Contacts", "Close", container.NewBorder(intro, nil, nil, nil, container.NewVScroll(list)), u.win)
	d.Resize(fyne.NewSize(640, 560))
	trackDialog(d, nil)
	d.Show()
}

// ---------- last talked and keep in touch ----------

// lastTalked is when you were last in touch with c, and how.
func (u *ui) lastTalked(c *Contact) (time.Time, string) {
	return LastTalked(u.act.People[c.ID], LastMet(u.eventsFor(c), time.Now()), c.LastContacted)
}

// dueToReachOut reports whether it's time to be in touch with c again.
func (u *ui) dueToReachOut(c *Contact, now time.Time) bool {
	if c.KeepInTouch <= 0 {
		return false
	}
	last, _ := u.lastTalked(c)
	if last.IsZero() {
		last = c.Created
	}
	return now.Sub(last) > time.Duration(c.KeepInTouch)*24*time.Hour
}

var keepChoices = []struct {
	Days  int
	Label string
}{{7, "Every Week"}, {14, "Every 2 Weeks"}, {30, "Every Month"}, {90, "Every 3 Months"}, {180, "Every 6 Months"}, {365, "Every Year"}}

func keepText(days int) string {
	for _, k := range keepChoices {
		if k.Days == days {
			return strings.ToLower(k.Label)
		}
	}
	if days > 0 {
		return fmt.Sprintf("every %d days", days)
	}
	return ""
}

// keepInTouchMenu sets how often to be in touch with c.
func (u *ui) keepInTouchMenu(c *Contact) *fyne.Menu {
	var items []*fyne.MenuItem
	for _, k := range keepChoices {
		k := k
		it := fyne.NewMenuItem(k.Label, func() { u.setKeepInTouch(c, k.Days) })
		it.Checked = c.KeepInTouch == k.Days
		items = append(items, it)
	}
	off := fyne.NewMenuItem("No Reminder", func() { u.setKeepInTouch(c, 0) })
	off.Checked = c.KeepInTouch == 0
	items = append(items, fyne.NewMenuItemSeparator(), off)
	return fyne.NewMenu("", items...)
}

func (u *ui) setKeepInTouch(c *Contact, days int) {
	c.KeepInTouch = days
	u.store.Put(c)
	u.save()
	if days > 0 {
		u.flash("You'll be reminded to be in touch with " + c.DisplayName() + " " + keepText(days))
	}
}

// addNote writes a dated note on c's timeline ("Lunch at the Avenues, talked about the move").
func (u *ui) addNote(c *Contact) { u.addNoteTo([]*Contact{c}) }

// addNoteTo writes the same note on each card in cs (one card, or the clipped ones).
func (u *ui) addNoteTo(cs []*Contact) {
	e := widget.NewMultiLineEntry()
	e.SetPlaceHolder("What happened, what you talked about, what to follow up…")
	e.Wrapping = fyne.TextWrapWord
	e.SetMinRowsVisible(4)
	when := widget.NewEntry()
	when.SetText(time.Now().Format("2 Jan 2006"))
	form := container.NewBorder(nil, container.NewBorder(nil, nil, widget.NewLabel("Date"), nil, when), nil, nil, e)
	d := dialog.NewCustomConfirm("Add a Note — "+u.scopeName(cs), "Add", "Cancel", form, func(ok bool) {
		text := strings.TrimSpace(e.Text)
		if !ok || text == "" {
			return
		}
		at := time.Now()
		if t, ok := parseLooseDate(when.Text); ok {
			at = time.Date(t.Year(), t.Month(), t.Day(), at.Hour(), at.Minute(), 0, 0, time.Local)
		}
		for _, c := range cs {
			c.AddEvent(at, text, "You")
			if at.After(c.LastContacted) && at.Before(time.Now().Add(time.Minute)) {
				c.LastContacted = at // a note about meeting counts as being in touch
			}
			u.store.Put(c)
		}
		u.openSec["Timeline"] = true
		u.save()
	}, u.win)
	d.Resize(fyne.NewSize(520, 0))
	d.Show()
	u.win.Canvas().Focus(e)
}

// activitySection is the card's "Activity" fold: each channel with its last date and count.
func (u *ui) activitySection(c *Contact) []fyne.CanvasObject {
	a := u.act.People[c.ID]
	var rows []fyne.CanvasObject
	if a != nil {
		var chs []string
		for ch := range a.Channels {
			chs = append(chs, ch)
		}
		sort.Slice(chs, func(i, j int) bool { return a.Channels[chs[i]].After(a.Channels[chs[j]]) })
		for _, ch := range chs {
			unit := map[string]string{"Messages": "messages", "WhatsApp": "messages", "Mail": "emails", "Calls": "calls"}[ch]
			rows = append(rows, u.valueRow(ch, fmt.Sprintf("%s · %d %s", Ago(a.Channels[ch], time.Now()), a.Counts[ch], unit)))
		}
	}
	if lm := LastMet(u.eventsFor(c), time.Now()); !lm.IsZero() {
		rows = append(rows, u.valueRow("Calendar", "met "+Ago(lm, time.Now())))
	}
	return rows
}

// ---------- reminders ----------

// remindLoop checks once at launch and then hourly: birthdays today and tomorrow, and people
// due a call. One notification a day at most.
func (u *ui) remindLoop() {
	check := func() {
		today := time.Now().Format("2006-01-02")
		if u.cfg.NoNotify || u.cfg.LastNotified == today {
			return
		}
		now := time.Now()
		var bdays []string
		due := 0
		for _, c := range u.store.Contacts {
			if d, turns, ok := c.NextBirthday(now); ok && d <= 1 {
				w := map[int]string{0: "today", 1: "tomorrow"}[d]
				if turns > 0 {
					bdays = append(bdays, fmt.Sprintf("%s turns %d %s", c.DisplayName(), turns, w))
				} else {
					bdays = append(bdays, c.DisplayName()+"'s birthday is "+w)
				}
			}
			if u.dueToReachOut(c, now) {
				due++
			}
		}
		if len(bdays) == 0 && due == 0 {
			return
		}
		u.cfg.LastNotified = today
		_ = u.cfg.Save(u.store.Dir())
		if len(bdays) > 0 {
			u.app.SendNotification(fyne.NewNotification("Birthdays", strings.Join(bdays, "\n")))
		}
		if due > 0 {
			u.app.SendNotification(fyne.NewNotification("Reach out", fmt.Sprintf("%d %s due a call or message — see Reach Out in Smart Contacts.", due, pluralWords(due, "person is", "people are"))))
		}
	}
	go func() {
		time.Sleep(5 * time.Second)
		for {
			fyne.Do(check)
			time.Sleep(time.Hour)
		}
	}()
}

func pluralWords(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

var _ = theme.HistoryIcon

// ---------- smart groups ----------

// saveSmartGroup saves the current search as a sidebar list.
func (u *ui) saveSmartGroup() {
	q := strings.TrimSpace(u.search.Text)
	if q == "" {
		showInfo("Search first", "Type a search (for example company:boubyan has:email), then save it as a smart group.", u.win)
		return
	}
	name := widget.NewEntry()
	name.SetText(q)
	d := dialog.NewForm("Save as Smart Group", "Save", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Name", name),
		widget.NewFormItem("Search", widget.NewLabel(q)),
	}, func(ok bool) {
		n := strings.TrimSpace(name.Text)
		if !ok || n == "" {
			return
		}
		for i, sg := range u.cfg.SmartGroups {
			if strings.EqualFold(sg.Name, n) {
				u.cfg.SmartGroups = append(u.cfg.SmartGroups[:i], u.cfg.SmartGroups[i+1:]...)
				break
			}
		}
		u.cfg.SmartGroups = append(u.cfg.SmartGroups, SmartGroup{Name: n, Query: q})
		_ = u.cfg.Save(u.store.Dir())
		u.search.SetText("")
		u.showGroup("q:" + n)
		u.refreshAll()
	}, u.win)
	d.Resize(fyne.NewSize(480, 0))
	d.Show()
	u.win.Canvas().Focus(name)
}

func (u *ui) deleteSmartGroup(name string) {
	for i, sg := range u.cfg.SmartGroups {
		if sg.Name == name {
			u.cfg.SmartGroups = append(u.cfg.SmartGroups[:i], u.cfg.SmartGroups[i+1:]...)
			break
		}
	}
	_ = u.cfg.Save(u.store.Dir())
	u.group = grpAll
	u.refreshAll()
}

// ---------- needs attention ----------

// checkAttention works out, in the background, which cards have clean-up suggestions (numbers
// without a country code, typos, repeats…), for the Needs Attention list and the card banner.
func (u *ui) checkAttention() {
	u.attnGen++
	gen := u.attnGen
	snap := make([]*Contact, len(u.store.Contacts))
	for i, c := range u.store.Contacts {
		snap[i] = c.Clone()
	}
	cc := u.cfg.CountryCode
	go func() {
		n := map[string]int{}
		for _, ch := range PlanCleanup(snap, cc) {
			if ch.Contact != nil && (ch.Default || ch.Kind == ckUnsure) {
				n[ch.Contact.ID]++
			}
		}
		fyne.Do(func() {
			if gen != u.attnGen {
				return // a newer check is running
			}
			changed := len(n) != len(u.attention)
			for k, v := range n {
				if u.attention[k] != v {
					changed = true
				}
			}
			u.attention = n
			if changed {
				u.refreshSidebar()
				if u.group == grpAttention {
					u.refreshList()
				} else {
					u.showDetail()
				}
			}
		})
	}()
}
