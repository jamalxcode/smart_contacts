package main

import (
	"image/color"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// A card is read like paper and written on deliberately, as in a real address book:
//
//   - Reading (the usual state) shows only what's written: the name, the ways to reach them, and
//     one ruled line per number, email, address or date. Nothing empty is shown; a faint
//     "+ add …" line lists what's missing.
//   - Clicking a line offers what it's for (call, message, email, open, copy). Double-clicking it,
//     or pressing Write, picks up the pen: the card becomes the writable form with every field,
//     the clicked one ready to type in. Done (or Esc) puts the pen down and saves.
//   - The card has a back. Turn Over shows the notes, how you met, keeping in touch and the
//     history; the front stays short.

// inkLine is one written line on the card: a small label, the value (one or more lines), and
// the ruling under it.
type inkLine struct {
	widget.BaseWidget
	label   string
	lines   []string
	icon    fyne.CanvasObject // a flag beside a phone number
	faint   bool              // a hint rather than something written
	onTap   func(at fyne.Position)
	onWrite func()
	hovered bool
}

func newInkLine(label string, lines []string, icon fyne.CanvasObject, onTap func(fyne.Position), onWrite func()) *inkLine {
	l := &inkLine{label: label, lines: lines, icon: icon, onTap: onTap, onWrite: onWrite}
	l.ExtendBaseWidget(l)
	return l
}

func (l *inkLine) Tapped(e *fyne.PointEvent) {
	if l.onTap != nil {
		l.onTap(e.AbsolutePosition)
	} else if l.onWrite != nil && l.faint {
		l.onWrite()
	}
}
func (l *inkLine) TappedSecondary(e *fyne.PointEvent) {
	if l.onTap != nil {
		l.onTap(e.AbsolutePosition)
	}
}
func (l *inkLine) DoubleTapped(*fyne.PointEvent) {
	if l.onWrite != nil {
		l.onWrite()
	}
}
func (l *inkLine) MouseIn(*desktop.MouseEvent)    { l.hovered = true; l.Refresh() }
func (l *inkLine) MouseMoved(*desktop.MouseEvent) {}
func (l *inkLine) MouseOut()                      { l.hovered = false; l.Refresh() }
func (l *inkLine) Cursor() desktop.Cursor {
	if l.onTap != nil || l.onWrite != nil {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

func (l *inkLine) CreateRenderer() fyne.WidgetRenderer {
	r := &inkRenderer{l: l}
	r.bg = canvas.NewRectangle(color.Transparent)
	r.bg.CornerRadius = 3
	r.lbl = canvas.NewText(strings.ToUpper(l.label), color.NRGBA{0x8A, 0x7E, 0x6C, 0xFF})
	r.lbl.TextSize = 10
	r.lbl.TextStyle.Bold = true
	// the words wrap at the page edge instead of running past it
	r.val = widget.NewLabel(strings.Join(l.lines, "\n"))
	r.val.Wrapping = fyne.TextWrapWord
	if l.faint {
		r.val.TextStyle.Italic = true
		r.val.Importance = widget.LowImportance
	}
	r.rule = canvas.NewLine(ruleInk)
	r.rule.StrokeWidth = 1
	return r
}

type inkRenderer struct {
	l    *inkLine
	bg   *canvas.Rectangle
	lbl  *canvas.Text
	val  *widget.Label
	rule *canvas.Line
}

const inkLabelW = 92

// valueX is where the words start: after the small label and the flag, if any.
func (r *inkRenderer) valueX() float32 {
	x := float32(inkLabelW + 8)
	if r.l.label == "" {
		x = 6
	}
	if r.l.icon != nil {
		x += 28
	}
	return x
}

func (r *inkRenderer) Layout(s fyne.Size) {
	r.bg.Move(fyne.NewPos(0, 1))
	r.bg.Resize(fyne.NewSize(s.Width, s.Height-2))
	r.lbl.Move(fyne.NewPos(6, 10))
	if r.l.icon != nil {
		r.l.icon.Move(fyne.NewPos(r.valueX()-28, 9))
		r.l.icon.Resize(fyne.NewSize(20, 14))
	}
	pad := theme.InnerPadding() // the label's own inset, so the words line up with the small label
	x := r.valueX() - pad
	w := fyne.Max(10, s.Width-x-2)
	r.val.Move(fyne.NewPos(x, -2))
	r.val.Resize(fyne.NewSize(w, r.val.MinSize().Height))
	r.rule.Position1 = fyne.NewPos(4, s.Height-1)
	r.rule.Position2 = fyne.NewPos(s.Width-4, s.Height-1)
}

func (r *inkRenderer) MinSize() fyne.Size {
	return fyne.NewSize(r.valueX()+40, fyne.Max(32, r.val.MinSize().Height))
}

func (r *inkRenderer) Refresh() {
	if r.l.hovered && (r.l.onTap != nil || r.l.onWrite != nil) {
		r.bg.FillColor = color.NRGBA{0x60, 0x50, 0x30, 0x12}
	} else {
		r.bg.FillColor = color.Transparent
	}
	r.bg.Refresh()
	r.Layout(r.l.Size())
}

func (r *inkRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.bg, r.lbl}
	if r.l.icon != nil {
		objs = append(objs, r.l.icon)
	}
	return append(objs, r.val, r.rule)
}
func (r *inkRenderer) Destroy() {}

// writeBox wraps something (the notes) so a double-click writes on it.
type writeBox struct {
	widget.BaseWidget
	content fyne.CanvasObject
	onWrite func()
}

func newWriteBox(content fyne.CanvasObject, onWrite func()) *writeBox {
	b := &writeBox{content: content, onWrite: onWrite}
	b.ExtendBaseWidget(b)
	return b
}
func (b *writeBox) DoubleTapped(*fyne.PointEvent) { b.onWrite() }
func (b *writeBox) Tapped(*fyne.PointEvent)       {}
func (b *writeBox) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(b.content)
}

// ---------- reading the card ----------

// openRead shows c's card for reading.
func (u *ui) openRead(c *Contact) {
	u.right.Objects = []fyne.CanvasObject{u.readCard(c)}
	u.right.Refresh()
}

// readCard is the card as printed: its front, or its back after Turn Over.
func (u *ui) readCard(c *Contact) fyne.CanvasObject {
	back := u.backID == c.ID
	var body fyne.CanvasObject
	if back {
		body = u.cardBack(c)
	} else {
		body = u.cardFront(c)
	}

	turnLabel := "Turn Over"
	turnHint := "notes & history"
	if back {
		turnHint = "details"
	}
	turn := widget.NewButtonWithIcon(turnLabel, theme.ViewRefreshIcon(), func() { u.learn("turn"); u.turnCard(c) })
	turn.Importance = widget.LowImportance
	hint := caption(turnHint)
	if u.learned("turn") { // says what's on the back until it's been turned over once
		hint.Hide()
	}

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
	share.Importance = widget.LowImportance

	var left fyne.CanvasObject
	if back {
		note := widget.NewButtonWithIcon("Add Note", theme.DocumentCreateIcon(), func() { u.addNote(c) })
		var keep *widget.Button
		keep = widget.NewButtonWithIcon("Keep in Touch", theme.MailReplyIcon(), func() {
			pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(keep)
			widget.ShowPopUpMenuAtPosition(u.keepInTouchMenu(c), u.win.Canvas(), pos.Add(fyne.NewPos(0, -270)))
		})
		note.Importance, keep.Importance = widget.LowImportance, widget.LowImportance
		left = container.NewHBox(note, keep)
	} else {
		write := widget.NewButtonWithIcon("Write", theme.DocumentCreateIcon(), func() { u.writeCard(nil) })
		write.Importance = widget.LowImportance
		left = write
	}
	cornerRoom := canvas.NewRectangle(color.Transparent) // the turned-up page corner sits here
	cornerRoom.SetMinSize(fyne.NewSize(26, 1))
	bar := container.NewBorder(widget.NewSeparator(), nil, left,
		container.NewHBox(share, hint, turn, cornerRoom), layout.NewSpacer())
	return container.NewBorder(nil, container.NewPadded(bar), nil, nil, container.NewVScroll(container.NewPadded(body)))
}

// printed is text in the organiser's serif (the app font in the plain look, or for Arabic).
func (u *ui) printed(s string, size float32, bold, italic bool, col color.Color) *canvas.Text {
	t := canvas.NewText(s, col)
	t.TextSize = size
	if u.cfg.ClassicLook {
		t.TextStyle = fyne.TextStyle{Bold: bold, Italic: italic}
	} else {
		t.TextStyle = serif(s, bold, italic)
	}
	return t
}

func (u *ui) cardFront(c *Contact) fyne.CanvasObject {
	ink := theme.Color(theme.ColorNameForeground)
	soft := color.NRGBA{0x6A, 0x60, 0x52, 0xFF}

	// who they are, beside the picture
	var who []fyne.CanvasObject
	formal := c.FormalName()
	if u.rightPage == nil { // the plain look has no page heading to carry the name
		name := c.DisplayName()
		if formal != "" {
			name = formal
		}
		who = append(who, u.printed(name, 22, true, false, ink))
	} else if formal != "" && formal != c.DisplayName() { // Dr., a middle name, PhD: the page heading has only first and last
		who = append(who, u.printed(formal, 17, false, false, ink))
	}
	if say := c.PhoneticName(); say != "" {
		who = append(who, u.printed("say “"+say+"”", 14, false, true, soft))
	}
	if c.AltName != "" {
		who = append(who, u.printed(c.AltName, 17, false, false, ink))
	}
	job := strings.Join(nonEmpty([]string{c.Title, c.Company}), " · ")
	if job != "" {
		jt := u.printed(job, 15, false, true, soft)
		if co := c.Company; co != "" && len(u.peopleAt(co)) > 1 { // others work there too: a company page
			jt.Text = job + "  ›"
			who = append(who, newCompanyLink(jt, func() { u.companyDialog(co) }))
		} else {
			who = append(who, jt)
		}
	}
	if c.Nickname != "" {
		who = append(who, u.printed("“"+c.Nickname+"”", 14, false, true, soft))
	}
	if c.Favorite {
		gold := color.NRGBA{0xB0, 0x82, 0x1E, 0xFF}
		star := canvas.NewText("★", gold) // the app font has a proper star; the serif's is tiny
		star.TextSize = 15
		who = append(who, container.NewHBox(star, u.printed("Favorite", 14, false, false, gold)))
	}
	if len(who) == 0 || (u.rightPage == nil && len(who) == 1) {
		who = append(who, caption("Double-click a line to write on it, or press Write."))
	}
	av := newAvatar(84)
	av.set(c, u.photo(c))
	av.onTap = func() { u.writeCard(nil) }
	head := container.NewBorder(nil, nil, container.NewPadded(av), nil,
		container.NewVBox(append([]fyne.CanvasObject{layout.NewSpacer()}, append(who, layout.NewSpacer())...)...))

	box := container.NewVBox(head)
	if a := u.actionsRow(c); a != nil {
		box.Add(a)
	}
	if lt := u.lastTalkedLine(c); lt != nil {
		box.Add(lt)
	}
	if pn := u.pencilNote(c); pn != nil {
		box.Add(pn)
	}
	if n := u.attention[c.ID]; n > 0 {
		review := widget.NewButtonWithIcon(plural(n, "suggestion")+" to tidy this card — Review…", theme.WarningIcon(),
			func() { u.cleanupFor([]*Contact{c}, c.DisplayName()) })
		review.Importance = widget.LowImportance
		review.Alignment = widget.ButtonAlignLeading
		box.Add(review)
	}

	cc := u.cfg.CountryCode
	for i, f := range c.Phones {
		i, f := i, f
		fl := newFlag()
		setFlag(fl, f.Value, cc)
		lbl := strings.ToLower(nonBlank(f.Label, "phone"))
		if i == 0 && len(c.Phones) > 1 {
			lbl += " ★"
		}
		box.Add(newInkLine(lbl, []string{f.Value}, fl, func(at fyne.Position) {
			n := NormalizePhone(f.Value, cc)
			items := []*fyne.MenuItem{
				fyne.NewMenuItem("Call "+f.Value, func() { u.open(c, "tel:"+n) }),
				fyne.NewMenuItem("Message", func() { u.open(c, "sms:"+n) }),
			}
			if CanWhatsApp(f.Value, cc) {
				items = append(items, fyne.NewMenuItem("WhatsApp", func() { u.open(c, "whatsapp://send?phone="+WhatsAppNumber(f.Value, cc)) }))
			}
			items = append(items, fyne.NewMenuItemSeparator(), u.copyItem(f.Value), u.writeItem(func(e *editor) fyne.Focusable { return e.phoneEntry(i) }))
			u.lineMenu(at, items...)
		}, func() { u.writeCard(func(e *editor) fyne.Focusable { return e.phoneEntry(i) }) }))
	}
	for i, f := range c.Emails {
		i, f := i, f
		lbl := "email"
		if l := strings.ToLower(f.Label); l != "" && l != "other" {
			lbl = l + " email"
		}
		box.Add(newInkLine(lbl, []string{f.Value}, nil, func(at fyne.Position) {
			u.lineMenu(at, fyne.NewMenuItem("Write an Email", func() { u.open(c, "mailto:"+f.Value) }),
				fyne.NewMenuItemSeparator(), u.copyItem(f.Value), u.writeItem(func(e *editor) fyne.Focusable { return e.emailEntry(i) }))
		}, func() { u.writeCard(func(e *editor) fyne.Focusable { return e.emailEntry(i) }) }))
	}
	for i, a := range c.Addresses {
		i, a := i, a
		if a.IsEmpty() {
			continue
		}
		var lines []string
		for _, s := range strings.Split(strings.TrimSpace(a.Street), "\n") {
			if s = strings.TrimSpace(s); s != "" {
				lines = append(lines, s)
			}
		}
		if l := strings.Join(nonEmpty([]string{a.City, a.Region, a.Postcode}), ", "); l != "" {
			lines = append(lines, l)
		}
		if a.Country != "" {
			lines = append(lines, a.Country)
		}
		box.Add(newInkLine(strings.ToLower(nonBlank(a.Label, "address")), lines, nil, func(at fyne.Position) {
			u.lineMenu(at, fyne.NewMenuItem("Show in Maps", func() { u.open(c, "https://maps.apple.com/?q="+url.QueryEscape(a.OneLine())) }),
				fyne.NewMenuItemSeparator(), u.copyItem(a.OneLine()), u.writeItem(func(e *editor) fyne.Focusable { return e.addrEntry(i) }))
		}, func() { u.writeCard(func(e *editor) fyne.Focusable { return e.addrEntry(i) }) }))
	}
	for i, f := range c.URLs {
		i, f := i, f
		link := f.Value
		if !strings.Contains(link, "://") {
			link = "https://" + link
		}
		box.Add(newInkLine(strings.ToLower(nonBlank(f.Label, "link")), []string{f.Value}, nil, func(at fyne.Position) {
			u.lineMenu(at, fyne.NewMenuItem("Open", func() { u.open(c, link) }),
				fyne.NewMenuItemSeparator(), u.copyItem(f.Value), u.writeItem(func(e *editor) fyne.Focusable { return e.urlEntry(i) }))
		}, func() { u.writeCard(func(e *editor) fyne.Focusable { return e.urlEntry(i) }) }))
	}
	if c.Birthday != "" {
		s := BirthdayText(c.Birthday)
		if d, turns, ok := c.NextBirthday(time.Now()); ok && d <= 31 {
			when := map[int]string{0: "today", 1: "tomorrow"}[d]
			if when == "" {
				when = "in " + plural(d, "day")
			}
			if turns > 0 {
				s += " — turns " + strconv.Itoa(turns) + " " + when
			} else {
				s += " — " + when
			}
		}
		box.Add(newInkLine("birthday", []string{s}, nil, func(at fyne.Position) {
			u.lineMenu(at, u.copyItem(BirthdayText(c.Birthday)), u.writeItem(func(e *editor) fyne.Focusable { return e.birthday }))
		}, func() { u.writeCard(func(e *editor) fyne.Focusable { return e.birthday }) }))
	}
	for i, f := range c.Dates { // anniversary and other dates
		i, f := i, f
		s := BirthdayText(f.Value)
		if d, years, ok := nextYearly(f.Value, time.Now()); ok && d <= 31 {
			when := map[int]string{0: "today", 1: "tomorrow"}[d]
			if when == "" {
				when = "in " + plural(d, "day")
			}
			if years > 0 {
				s += " — " + plural(years, "year") + " " + when
			} else {
				s += " — " + when
			}
		}
		box.Add(newInkLine(strings.ToLower(nonBlank(f.Label, "date")), []string{s}, nil, func(at fyne.Position) {
			u.lineMenu(at, u.copyItem(BirthdayText(f.Value)), u.writeItem(func(e *editor) fyne.Focusable { return e.dateEntry(i) }))
		}, func() { u.writeCard(func(e *editor) fyne.Focusable { return e.dateEntry(i) }) }))
	}
	for i, f := range c.Custom { // your own lines
		i, f := i, f
		box.Add(newInkLine(strings.ToLower(f.Label), []string{f.Value}, nil, func(at fyne.Position) {
			u.lineMenu(at, u.copyItem(f.Value), u.writeItem(func(e *editor) fyne.Focusable { return e.customEntry(i) }))
		}, func() { u.writeCard(func(e *editor) fyne.Focusable { return e.customEntry(i) }) }))
	}
	if len(c.Groups) > 0 {
		box.Add(newInkLine("groups", []string{strings.Join(c.Groups, ", ")}, nil, nil,
			func() { u.writeCard(func(e *editor) fyne.Focusable { return e.newGroup }) }))
	}

	// what isn't on the card yet, as one faint line
	var missing []string
	if len(c.Phones) == 0 {
		missing = append(missing, "phone")
	}
	if len(c.Emails) == 0 {
		missing = append(missing, "email")
	}
	if len(c.Addresses) == 0 {
		missing = append(missing, "address")
	}
	if c.Birthday == "" {
		missing = append(missing, "birthday")
	}
	if c.Company == "" {
		missing = append(missing, "company")
	}
	if len(missing) > 0 {
		add := newInkLine("", []string{"+ add " + strings.Join(missing, ", ") + "…"}, nil, nil, func() {
			u.writeCard(func(e *editor) fyne.Focusable {
				switch missing[0] {
				case "phone":
					return e.phoneEntry(0)
				case "email":
					return e.emailEntry(0)
				case "birthday":
					return e.birthday
				case "company":
					return e.company
				}
				return e.first
			})
		})
		add.faint = true
		box.Add(add)
	}
	box.Add(u.cardMeta(c))
	return box
}

func (u *ui) cardBack(c *Contact) fyne.CanvasObject {
	box := container.NewVBox()
	heading := func(s string) fyne.CanvasObject {
		return u.printed(s, 15, true, false, color.NRGBA{0x8A, 0x32, 0x1E, 0xFF})
	}
	box.Add(heading("Follow-ups"))
	if rows := u.followUpRows(c); len(rows) > 0 {
		for _, r := range rows {
			box.Add(r)
		}
	}
	addFU := newInkLine("", []string{"+ follow up on a date…"}, nil, nil, func() { u.followUpDialog([]*Contact{c}) })
	addFU.faint = true
	addFU.onTap = func(fyne.Position) { u.followUpDialog([]*Contact{c}) }
	box.Add(addFU)
	box.Add(heading("Notes"))
	writeNotes := func() { u.writeCard(func(e *editor) fyne.Focusable { return e.notes }) }
	if strings.TrimSpace(c.Notes) != "" {
		l := widget.NewLabel(c.Notes)
		l.Wrapping = fyne.TextWrapWord
		box.Add(newWriteBox(l, writeNotes))
	} else {
		n := newInkLine("", []string{"No notes yet — double-click to write"}, nil, nil, writeNotes)
		n.faint = true
		box.Add(n)
	}
	if c.HowWeMet != "" {
		box.Add(newInkLine("how we met", []string{c.HowWeMet}, nil, nil,
			func() { u.writeCard(func(e *editor) fyne.Focusable { return e.met }) }))
	}
	if lt := u.lastTalkedLine(c); lt != nil {
		box.Add(lt)
	}
	box.Add(widget.NewSeparator())
	box.Add(u.foldingSections(c))
	box.Add(u.cardMeta(c))
	return box
}

func (u *ui) cardMeta(c *Contact) fyne.CanvasObject {
	meta := []string{"Added " + c.Created.Format("2 Jan 2006")}
	if len(c.Sources) > 0 {
		meta = append(meta, "from "+strings.Join(c.Sources, ", "))
	}
	m := caption(strings.Join(meta, " · "))
	m.Wrapping = fyne.TextWrapWord
	return m
}

func (u *ui) lineMenu(at fyne.Position, items ...*fyne.MenuItem) {
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), u.win.Canvas(), at)
}

func (u *ui) copyItem(v string) *fyne.MenuItem {
	return fyne.NewMenuItem("Copy", func() {
		u.app.Clipboard().SetContent(v)
		u.flash("Copied " + v)
	})
}

func (u *ui) writeItem(focus func(*editor) fyne.Focusable) *fyne.MenuItem {
	return fyne.NewMenuItem("Change…", func() { u.writeCard(focus) })
}

// turnCard turns the open card over (front ↔ back).
func (u *ui) turnCard(c *Contact) {
	u.cardFlip(func() {
		if u.backID == c.ID {
			u.backID = ""
		} else {
			u.backID = c.ID
		}
		u.openRead(c)
	})
}

// ---------- writing on the card ----------

// writeCard picks up the pen: the open card becomes writable, with focus (if given) on the
// field to write in.
func (u *ui) writeCard(focus func(*editor) fyne.Focusable) {
	c := u.store.Get(u.selectedID)
	if c == nil {
		return
	}
	if u.ed == nil {
		u.openLive(c)
	}
	if u.ed == nil {
		return
	}
	var f fyne.Focusable = u.ed.first
	if focus != nil {
		if x := focus(u.ed); x != nil {
			f = x
		}
	}
	u.win.Canvas().Focus(f)
}

// finishWriting puts the pen down: saves if anything changed, and shows the card for reading.
func (u *ui) finishWriting() {
	if u.ed == nil {
		return
	}
	u.learn("write")
	if u.ed.dirty() {
		u.ed.save()
		return
	}
	u.ed = nil
	u.showDetail()
}

func (e *editor) phoneEntry(i int) fyne.Focusable {
	if i < len(e.phones.rows) {
		return e.phones.rows[i].value
	}
	return e.phones.add(Field{}).value
}
func (e *editor) emailEntry(i int) fyne.Focusable {
	if i < len(e.emails.rows) {
		return e.emails.rows[i].value
	}
	return e.emails.add(Field{}).value
}
func (e *editor) urlEntry(i int) fyne.Focusable {
	if i < len(e.urls.rows) {
		return e.urls.rows[i].value
	}
	return e.urls.add(Field{}).value
}
func (e *editor) dateEntry(i int) fyne.Focusable {
	if i < len(e.dates.rows) {
		return e.dates.rows[i].value
	}
	return e.dates.add(Field{}).value
}

func (e *editor) customEntry(i int) fyne.Focusable {
	if i < len(e.custom.rows) {
		return e.custom.rows[i].value
	}
	return e.custom.add(Field{}).value
}

// customLabels are the labels offered for your own lines: the usual ones, then any you've used.
func (u *ui) customLabels() []string {
	out := []string{"Spouse", "Children", "Diwaniya", "Office", "Assistant", "Prefers", "Note"}
	seen := map[string]bool{}
	for _, l := range out {
		seen[strings.ToLower(l)] = true
	}
	for _, c := range u.store.Contacts {
		for _, f := range c.Custom {
			if k := strings.ToLower(f.Label); !seen[k] && f.Label != "" {
				seen[k] = true
				out = append(out, f.Label)
			}
		}
	}
	return out
}

func (e *editor) addrEntry(i int) fyne.Focusable {
	if i < len(e.addrs.rows) {
		return e.addrs.rows[i].street
	}
	return e.addrs.add(Address{}).street
}

func nonBlank(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// cardEntry is a text field on the card. It behaves like any other, except that Esc puts the
// pen down (Fyne gives Esc only to the field that has the keyboard, never to the window).
type cardEntry struct {
	widget.Entry
}

// onCardEscape is what Esc does while writing on a card (set when the window is built).
var onCardEscape func()

func newCardEntry() *cardEntry {
	e := &cardEntry{}
	e.ExtendBaseWidget(e)
	return e
}

func newCardMultiLineEntry() *cardEntry {
	e := &cardEntry{}
	e.MultiLine = true
	e.Wrapping = fyne.TextWrapWord
	e.ExtendBaseWidget(e)
	return e
}

func (e *cardEntry) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyEscape && onCardEscape != nil {
		onCardEscape()
		return
	}
	e.Entry.TypedKey(k)
}

// typing reports whether the keyboard is in a text field.
func typing(f fyne.Focusable) bool {
	switch f.(type) {
	case *widget.Entry, *cardEntry, *widget.SelectEntry:
		return true
	}
	return false
}

// iconTap is a small clickable icon (the funnel in the search field).
type iconTap struct {
	widget.BaseWidget
	icon  *widget.Icon
	hint  string
	onTap func()
}

func newIconTap(res fyne.Resource, hint string, onTap func()) *iconTap {
	t := &iconTap{icon: widget.NewIcon(res), hint: hint, onTap: onTap}
	t.ExtendBaseWidget(t)
	return t
}
func (t *iconTap) Tapped(*fyne.PointEvent)             { t.onTap() }
func (t *iconTap) Cursor() desktop.Cursor              { return desktop.PointerCursor }
func (t *iconTap) MinSize() fyne.Size                  { return fyne.NewSize(24, 24) }
func (t *iconTap) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(t.icon) }

// learned reports whether a hint has been acted on already; learn records that it has.
func (u *ui) learned(k string) bool { return u.cfg.Learned[k] }

func (u *ui) learn(k string) {
	if u.cfg.Learned[k] {
		return
	}
	if u.cfg.Learned == nil {
		u.cfg.Learned = map[string]bool{}
	}
	u.cfg.Learned[k] = true
	_ = u.cfg.Save(u.store.Dir())
}
