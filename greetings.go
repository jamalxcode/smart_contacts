package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Greetings for occasions: Eid, Ramadan, National Day, New Year, a birthday. Choose who (the
// clipped cards, Favorites, a group, or everyone with a mobile) and the message, with their
// name put in for each person. Then the app goes through them one by one: it opens WhatsApp (or
// Messages) with the message ready, you press send there, and it moves to the next. Each card
// notes that the greeting went, so nobody gets it twice.

var iconGift = svgIcon("gift", "M20 6h-2.18c.11-.31.18-.65.18-1 0-1.66-1.34-3-3-3-1.05 0-1.96.54-2.5 1.35l-.5.67-.5-.68C10.96 2.54 10.05 2 9 2 7.34 2 6 3.34 6 5c0 .35.07.69.18 1H4c-1.11 0-1.99.89-1.99 2L2 19c0 1.11.89 2 2 2h16c1.11 0 2-.89 2-2V8c0-1.11-.89-2-2-2zm-5-2c.55 0 1 .45 1 1s-.45 1-1 1-1-.45-1-1 .45-1 1-1zM9 4c.55 0 1 .45 1 1s-.45 1-1 1-1-.45-1-1 .45-1 1-1zm11 15H4v-2h16v2zm0-5H4V8h5.08L7 10.83 8.62 12 11 8.76l1-1.36 1 1.36L15.38 12 17 10.83 14.92 8H20v6z")

type occasion struct {
	name, en, ar string
}

var occasions = []occasion{
	{"Eid al-Fitr", "Eid Mubarak, {name}! Wishing you and your family a blessed Eid.", "عيدكم مبارك يا {name}، وكل عام وأنتم بخير"},
	{"Eid al-Adha", "Eid al-Adha Mubarak, {name}! Wishing you and your family joy and blessings.", "عيد أضحى مبارك يا {name}، تقبّل الله منا ومنكم"},
	{"Ramadan", "Ramadan Kareem, {name}! Wishing you a blessed month.", "رمضان كريم يا {name}، مبارك عليكم الشهر"},
	{"National Day", "Happy National Day, {name}!", "عيد وطني سعيد يا {name}، وكل عام والكويت بخير"},
	{"New Year", "Happy New Year, {name}! Wishing you a wonderful year ahead.", "كل عام وأنت بخير يا {name}، سنة سعيدة"},
	{"Birthday", "Happy birthday, {name}! Wishing you a great year ahead.", "كل عام وأنت بخير يا {name}، عيد ميلاد سعيد"},
	{"Something else", "", ""},
}

// greetedNote is what a card's timeline says once the greeting went, e.g. "Eid al-Fitr greeting
// sent (2026)".
func greetedNote(occ string, at time.Time) string {
	return fmt.Sprintf("%s greeting sent (%d)", occ, at.Year())
}

func greeted(c *Contact, note string) bool {
	for _, e := range c.Timeline {
		if strings.EqualFold(e.Text, note) {
			return true
		}
	}
	return false
}

// cmdGreetings asks who and what, then starts sending.
func (u *ui) cmdGreetings() {
	type who struct {
		label string
		cs    func() []*Contact
	}
	reachable := func(cs []*Contact) []*Contact {
		var out []*Contact
		for _, c := range cs {
			if len(c.Phones) > 0 {
				out = append(out, c)
			}
		}
		return out
	}
	var whos []who
	if u.selectMode && len(u.picked) > 0 {
		cs := u.pickedContacts()
		whos = append(whos, who{fmt.Sprintf("The %d clipped cards", len(cs)), func() []*Contact { return cs }})
	}
	var favs []*Contact
	for _, c := range u.store.Contacts {
		if c.Favorite {
			favs = append(favs, c)
		}
	}
	whos = append(whos, who{fmt.Sprintf("Favorites (%d)", len(favs)), func() []*Contact { return favs }})
	groups := u.store.Groups()
	group := widget.NewSelect(groups, nil)
	group.PlaceHolder = "Choose a group"
	if len(groups) > 0 {
		whos = append(whos, who{"A group:", func() []*Contact {
			var out []*Contact
			for _, c := range u.store.Contacts {
				if group.Selected != "" && c.HasGroup(group.Selected) {
					out = append(out, c)
				}
			}
			return out
		}})
	}
	all := reachable(u.store.Contacts)
	whos = append(whos, who{fmt.Sprintf("Everyone with a phone number (%d)", len(all)), func() []*Contact { return all }})
	var wl []string
	for _, w := range whos {
		wl = append(wl, w.label)
	}
	whoR := widget.NewRadioGroup(wl, nil)
	whoR.Required = true
	whoR.SetSelected(wl[0])
	group.OnChanged = func(string) { whoR.SetSelected("A group:") }

	var on []string
	for _, o := range occasions {
		on = append(on, o.name)
	}
	occ := widget.NewSelect(on, nil)
	lang := widget.NewRadioGroup([]string{"English", "العربية"}, nil)
	lang.Horizontal = true
	lang.Required = true
	lang.SetSelected("English")
	msg := widget.NewMultiLineEntry()
	msg.Wrapping = fyne.TextWrapWord
	msg.SetMinRowsVisible(3)
	msg.SetPlaceHolder("Your message — {name} becomes each person's first name")
	fill := func() {
		for _, o := range occasions {
			if o.name == occ.Selected {
				if lang.Selected == "English" {
					msg.SetText(o.en)
				} else {
					msg.SetText(o.ar)
				}
			}
		}
	}
	occ.OnChanged = func(string) { fill() }
	lang.OnChanged = func(string) { fill() }
	occ.SetSelected(occasions[0].name)

	form := container.NewVBox(
		boldLabel("Who"), whoR, group,
		boldLabel("Occasion"), occ, lang,
		boldLabel("Message"), msg,
		caption("Each one opens in WhatsApp (or Messages) ready to send — you press send there."),
	)
	d := newConfirm("Send Greetings", "Start", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		var cs []*Contact
		for _, w := range whos {
			if w.label == whoR.Selected {
				cs = reachable(w.cs())
			}
		}
		text := strings.TrimSpace(msg.Text)
		if len(cs) == 0 || text == "" {
			showInfo("Nobody to greet", "Choose people with a phone number, and write a message.", u.win)
			return
		}
		name := occ.Selected
		if name == "Something else" || name == "" {
			name = "Greeting"
		}
		u.greetingRun(cs, name, text, 0, 0)
	}, u.win)
	d.Resize(fyne.NewSize(520, 0))
	d.Show()
}

// firstName is what {name} becomes: the first name, or the whole name for a company.
func firstName(c *Contact, arabic bool) string {
	if arabic && c.AltName != "" && hasArabic(c.AltName) {
		return strings.Fields(c.AltName)[0]
	}
	if c.First != "" && !c.IsCompany() {
		return c.First
	}
	return c.DisplayName()
}

// greetingRun shows person i of cs, with their message ready to send.
func (u *ui) greetingRun(cs []*Contact, occ, text string, i, sent int) {
	if i >= len(cs) {
		u.flash(fmt.Sprintf("%s greetings: %d sent", occ, sent))
		return
	}
	c := cs[i]
	now := time.Now()
	note := greetedNote(occ, now)
	personal := strings.ReplaceAll(text, "{name}", firstName(c, hasArabic(text)))
	cc := u.cfg.CountryCode
	phone := bestPhone(c, cc)
	if phone == "" && len(c.Phones) > 0 {
		phone = c.Phones[0].Value
	}

	var d *paperDialog
	box := container.NewVBox()
	box.Add(caption(fmt.Sprintf("%d of %d · %d sent so far", i+1, len(cs), sent)))
	box.Add(widget.NewLabelWithStyle(c.DisplayName(), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	box.Add(caption(phone))
	if greeted(c, note) {
		w := caption("Already sent this " + occ + " greeting this year.")
		w.Importance = widget.DangerImportance
		box.Add(w)
	}
	preview := widget.NewLabel(personal)
	preview.Wrapping = fyne.TextWrapWord
	box.Add(preview)

	next := func(didSend bool) {
		d.Hide()
		n := sent
		if didSend {
			n++
		}
		u.greetingRun(cs, occ, text, i+1, n)
	}
	markSent := func() {
		c.AddEvent(now, note, "You")
		c.LastContacted = now
		u.store.Put(c)
		if err := u.store.Save(); err != nil {
			showErr(err, u.win)
		}
	}
	var sendBtn *paperButton
	if CanWhatsApp(phone, cc) {
		link := "whatsapp://send?phone=" + WhatsAppNumber(phone, cc) + "&text=" + url.QueryEscape(personal)
		sendBtn = newPaperButton("Send on WhatsApp", btnPrimary, func() {
			u.openLink(link)
			markSent()
			next(true)
		})
	} else {
		link := "sms:" + NormalizePhone(phone, cc) + "?body=" + url.QueryEscape(personal)
		sendBtn = newPaperButton("Send as a Message", btnPrimary, func() {
			u.openLink(link)
			markSent()
			next(true)
		})
	}
	skip := newPaperButton("Skip", btnPlain, func() { next(false) })
	stop := newPaperButton("Stop", btnPlain, func() {
		d.Hide()
		u.refreshAll()
		u.flash(fmt.Sprintf("%s greetings: %d sent, stopped at %d of %d", occ, sent, i+1, len(cs)))
	})
	d = newCustomWithoutButtons(occ+" Greetings", box, u.win)
	d.clipboard = true
	d.SetButtons([]fyne.CanvasObject{stop, skip, sendBtn})
	d.Resize(fyne.NewSize(500, 0))
	d.Show()
}

// openLink opens a link (WhatsApp, Messages, a web page), saying so if it can't.
func (u *ui) openLink(link string) {
	p, err := url.Parse(link)
	if err != nil {
		showErr(err, u.win)
		return
	}
	if err := u.app.OpenURL(p); err != nil {
		showErr(err, u.win)
	}
}
