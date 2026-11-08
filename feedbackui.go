package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Feedback after a change, the Mac way: say what happened, where, and offer Undo. Every saved
// change (one edit, a bulk command, a clean-up, an undo) shows a banner at the bottom of the
// window: "Cleaned up 12 cards: phones, names" with Show Changes and Undo; the changed cards
// get a blue dot in the list for a little while. Show Changes lists each card before → after.

var changedDot = color.NRGBA{0x0A, 0x84, 0xFF, 0xFF} // the system blue of Mail's unread dot

// scopeName names what a command acts on: a person, "3 Clipped Cards", or "All 453 Cards".
func (u *ui) scopeName(cs []*Contact) string {
	switch {
	case len(cs) == 1:
		return cs[0].DisplayName()
	case len(cs) == len(u.store.Contacts):
		return fmt.Sprintf("All %d Cards", len(cs))
	case u.selectMode && len(cs) == len(u.picked):
		return fmt.Sprintf("%d Clipped Cards", len(cs))
	}
	return fmt.Sprintf("%d Cards", len(cs))
}

// ---------- the banner ----------

// tapSink keeps clicks on the banner's background from reaching the list underneath.
type tapSink struct {
	widget.BaseWidget
	content fyne.CanvasObject
}

func newTapSink(o fyne.CanvasObject) *tapSink {
	t := &tapSink{content: o}
	t.ExtendBaseWidget(t)
	return t
}
func (t *tapSink) Tapped(*fyne.PointEvent)          {}
func (t *tapSink) TappedSecondary(*fyne.PointEvent) {}
func (t *tapSink) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.content)
}

// toastLayout puts the banner at the bottom centre of the window, above the status line.
type toastLayout struct{}

func (toastLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (toastLayout) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	for _, o := range objs {
		m := o.MinSize()
		w := fyne.Min(fyne.Max(m.Width, 620), s.Width-40)
		o.Resize(fyne.NewSize(w, m.Height))
		o.Move(fyne.NewPos((s.Width-w)/2, s.Height-m.Height-46))
	}
}

type toast struct {
	box    *fyne.Container // the layer that sits over the window
	card   fyne.CanvasObject
	title  *widget.Label
	detail *widget.Label
	show   *widget.Button
	undo   *widget.Button
	shown  time.Time
	gen    int
}

func (u *ui) newToast() *toast {
	t := &toast{}
	t.title = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	t.title.Wrapping = fyne.TextWrapWord
	t.detail = widget.NewLabel("")
	t.detail.Wrapping = fyne.TextWrapWord
	t.detail.SizeName = theme.SizeNameCaptionText
	t.detail.Importance = widget.LowImportance
	t.show = widget.NewButton("Show Changes", nil)
	t.undo = widget.NewButtonWithIcon("Undo", theme.ContentUndoIcon(), nil)
	t.undo.Importance = widget.HighImportance
	closeBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() { t.hide() })
	closeBtn.Importance = widget.LowImportance
	check := canvas.NewImageFromResource(theme.NewColoredResource(theme.ConfirmIcon(), theme.ColorNameSuccess))
	check.SetMinSize(fyne.NewSize(22, 22))
	bg := canvas.NewRectangle(color.NRGBA{0xFC, 0xFB, 0xF8, 0xF8})
	bg.CornerRadius = 12
	bg.StrokeColor = color.NRGBA{0, 0, 0, 0x28}
	bg.StrokeWidth = 1
	shadow := canvas.NewRectangle(color.NRGBA{0, 0, 0, 0x30})
	shadow.CornerRadius = 14
	text := container.NewVBox(t.title, t.detail)
	row := container.NewBorder(nil, nil, container.NewCenter(check),
		container.NewHBox(container.NewCenter(t.show), container.NewCenter(t.undo), container.NewCenter(closeBtn)), text)
	t.card = newTapSink(container.NewStack(
		container.New(&shadowOffset{}, shadow), bg, container.NewPadded(row)))
	t.card.Hide()
	t.box = container.New(toastLayout{}, t.card)
	return t
}

// shadowOffset draws its one child slightly larger and lower: a soft drop shadow.
type shadowOffset struct{}

func (shadowOffset) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (shadowOffset) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(-2, 3))
		o.Resize(fyne.NewSize(s.Width+4, s.Height+3))
	}
}

func (t *toast) hide() {
	t.gen++
	t.card.Hide()
}

// showToast puts up the banner. undoFn nil hides the Undo button; changes nil hides Show Changes.
func (u *ui) showToast(title, detail string, changes []CardChange, undoLabel string, undoFn func()) {
	t := u.toast
	if t == nil {
		return
	}
	t.gen++
	gen := t.gen
	t.title.SetText(title)
	t.detail.SetText(detail)
	if detail == "" {
		t.detail.Hide()
	} else {
		t.detail.Show()
	}
	if len(changes) > 0 {
		t.show.OnTapped = func() { title := t.title.Text; t.hide(); u.showChanges(title, changes) }
		t.show.Show()
	} else {
		t.show.Hide()
	}
	if undoFn != nil {
		t.undo.SetText(undoLabel)
		t.undo.OnTapped = func() { t.hide(); undoFn() }
		t.undo.Show()
	} else {
		t.undo.Hide()
	}
	t.shown = time.Now()
	t.card.Show()
	t.box.Refresh()
	go func() {
		time.Sleep(10 * time.Second)
		fyne.Do(func() {
			if t.gen == gen {
				t.hide()
			}
		})
	}()
}

// retitleToast lets the command's own words ("Tidied 3 notes") replace the generic title of a
// banner that has just appeared.
func (u *ui) retitleToast(msg string) bool {
	t := u.toast
	if t == nil || !t.card.Visible() || time.Since(t.shown) > 1500*time.Millisecond {
		return false
	}
	// "(⌘Z to undo)" and "(backup … saved first)" stay in the status line: the banner has Undo
	if i := strings.LastIndex(msg, " ("); i > 0 && strings.HasSuffix(strings.TrimSpace(msg), ")") {
		msg = strings.TrimSpace(msg[:i])
	}
	msg = strings.TrimSpace(msg)
	if msg == t.title.Text {
		return true
	}
	// the command's words become the title; what changed moves to the line below
	d := t.title.Text
	if t.detail.Text != "" {
		d += " — " + t.detail.Text
	}
	t.title.SetText(msg)
	t.detail.SetText(d)
	t.detail.Show()
	return true
}

// afterChange is called once a save made an undoable change: the banner and the blue dots.
func (u *ui) afterChange(changes []CardChange, verb string) {
	if len(changes) == 0 {
		return
	}
	u.markRecent(changes)
	for _, c := range changes { // the open card changed: stamp it
		if c.ID == u.selectedID && c.Kind != "deleted" {
			u.stampCard()
			break
		}
	}
	title := summariseChanges(changes)
	if verb != "" {
		title = verb + ": " + title
	}
	detail := ""
	if len(changes) > 1 {
		var names []string
		for i, c := range changes {
			if i == 4 {
				names = append(names, fmt.Sprintf("and %d more", len(changes)-4))
				break
			}
			names = append(names, c.Name)
		}
		detail = strings.Join(names, ", ")
	}
	if verb == "" { // an ordinary change: offer Undo
		u.showToast(title, detail, changes, "Undo", u.undo)
	} else if verb == "Undid" {
		u.showToast(title, detail, changes, "Redo", u.redo)
	} else {
		u.showToast(title, detail, changes, "Undo", u.undo)
	}
}

// noteFields remembers which fields the last change altered on each card (the pencil note).
func (u *ui) noteFields(changes []CardChange) {
	u.recentFields = map[string][]string{}
	for _, c := range changes {
		if c.Kind != "changed" {
			continue
		}
		var f []string
		for _, x := range c.Fields {
			f = append(f, strings.ToLower(x.Field))
		}
		u.recentFields[c.ID] = f
	}
}

// pencilNote is the note in pencil on a card that just changed: "pencilled in just now: phones".
func (u *ui) pencilNote(c *Contact) fyne.CanvasObject {
	f := u.recentFields[c.ID]
	if len(f) == 0 {
		return nil
	}
	t := canvas.NewText("pencilled in just now: "+strings.Join(f, ", "), color.NRGBA{0x6A, 0x66, 0x60, 0xFF})
	t.TextSize = 12
	t.TextStyle = serif("x", false, true)
	ic := canvas.NewImageFromResource(theme.DocumentCreateIcon())
	ic.SetMinSize(fyne.NewSize(14, 14))
	return container.NewHBox(container.NewCenter(ic), t)
}

// ---------- blue dots on changed cards ----------

func (u *ui) markRecent(changes []CardChange) {
	u.recentGen++
	gen := u.recentGen
	u.recent = map[string]bool{}
	for _, c := range changes {
		if c.Kind != "deleted" {
			u.recent[c.ID] = true
		}
	}
	u.list.Refresh()
	go func() {
		time.Sleep(20 * time.Second)
		fyne.Do(func() {
			if u.recentGen == gen {
				u.recent = nil
				u.recentFields = nil
				u.list.Refresh()
			}
		})
	}()
}

func newChangedDot() *fyne.Container {
	d := canvas.NewCircle(changedDot)
	box := container.New(&dotLayout{}, d)
	return box
}

type dotLayout struct{}

func (dotLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(10, 10) }
func (dotLayout) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	for _, o := range objs {
		o.Resize(fyne.NewSize(8, 8))
		o.Move(fyne.NewPos((s.Width-8)/2, (s.Height-8)/2))
	}
}

// ---------- Show Changes ----------

// showChanges lists every card a change touched, each field before → after.
func (u *ui) showChanges(title string, changes []CardChange) {
	box := container.NewVBox()
	var d dialog.Dialog
	const most = 300
	for i, ch := range changes {
		if i == most {
			box.Add(caption(fmt.Sprintf("…and %d more cards.", len(changes)-most)))
			break
		}
		ch := ch
		tag := map[string]string{"added": "New card", "changed": "Changed", "deleted": "Moved to the Trash"}[ch.Kind]
		name := widget.NewButton(ch.Name, nil)
		name.Alignment = widget.ButtonAlignLeading
		name.Importance = widget.LowImportance
		if ch.Kind == "deleted" {
			name.Disable()
		} else {
			name.OnTapped = func() {
				d.Hide()
				u.showContact(ch.ID)
			}
		}
		badge := canvas.NewText(tag, map[string]color.Color{
			"added":   color.NRGBA{0x24, 0x8A, 0x3D, 0xFF},
			"changed": changedDot,
			"deleted": color.NRGBA{0xC0, 0x39, 0x2B, 0xFF},
		}[ch.Kind])
		badge.TextSize = theme.CaptionTextSize()
		badge.TextStyle.Bold = true
		box.Add(container.NewBorder(nil, nil, nil, container.NewCenter(badge), name))
		for _, f := range ch.Fields {
			before := f.Before
			if before == "" {
				before = "—"
			}
			after := f.After
			if after == "" {
				after = "(removed)"
			}
			if ch.Kind == "added" {
				l := widget.NewLabel(after)
				l.Wrapping = fyne.TextWrapWord
				box.Add(container.NewBorder(nil, nil, container.NewGridWrap(fyne.NewSize(140, 30), caption(f.Field)), nil, l))
				continue
			}
			was := widget.NewLabel(before)
			was.Wrapping = fyne.TextWrapWord
			was.Importance = widget.LowImportance
			now := widget.NewLabel(after)
			now.Wrapping = fyne.TextWrapWord
			now.TextStyle.Bold = true
			arrow := widget.NewLabel("→")
			box.Add(container.NewBorder(nil, nil, container.NewGridWrap(fyne.NewSize(140, 30), caption(f.Field)), nil,
				container.NewBorder(nil, nil, nil, nil, container.NewGridWithColumns(2,
					was, container.NewBorder(nil, nil, arrow, nil, now)))))
		}
		box.Add(widget.NewSeparator())
	}
	head := widget.NewLabel("Grey is how it was, bold is how it is now. Click a name to open the card. Edit → Undo (⌘Z) puts it all back.")
	head.Wrapping = fyne.TextWrapWord
	cd := newCustom(title, "Done", container.NewBorder(head, nil, nil, nil, container.NewVScroll(box)), u.win)
	d = cd
	cd.Resize(fyne.NewSize(820, 620))
	trackDialog(cd, nil)
	cd.Show()
}
