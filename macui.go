package main

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Pieces that make the app look and behave like a Mac app (Apple Human Interface Guidelines).

// ---------- round action button (Contacts' message / call / video / mail row) ----------

type roundAction struct {
	widget.BaseWidget
	icon    fyne.Resource
	label   string
	onTap   func()
	hovered bool
	circle  *canvas.Circle
}

func newRoundAction(label string, icon fyne.Resource, tap func()) *roundAction {
	r := &roundAction{icon: icon, label: label, onTap: tap}
	r.ExtendBaseWidget(r)
	return r
}

const roundSize = 40

func (r *roundAction) Tapped(*fyne.PointEvent) {
	if r.onTap != nil {
		r.onTap()
	}
}
func (r *roundAction) Cursor() desktop.Cursor { return desktop.PointerCursor }
func (r *roundAction) MouseIn(*desktop.MouseEvent) {
	r.hovered = true
	r.Refresh()
}
func (r *roundAction) MouseMoved(*desktop.MouseEvent) {}
func (r *roundAction) MouseOut() {
	r.hovered = false
	r.Refresh()
}

func (r *roundAction) CreateRenderer() fyne.WidgetRenderer {
	r.circle = canvas.NewCircle(theme.Color(theme.ColorNameButton))
	ic := canvas.NewImageFromResource(theme.NewPrimaryThemedResource(r.icon))
	ic.FillMode = canvas.ImageFillContain
	// Contacts names its own buttons in lower case (message, call, video, mail); apps keep their
	// own names (WhatsApp, LinkedIn).
	label := map[string]string{"Call": "call", "Message": "message", "Email": "mail", "FaceTime": "video", "More": "more"}[r.label]
	if label == "" {
		label = r.label
	}
	txt := canvas.NewText(label, theme.Color(theme.ColorNamePrimary))
	txt.TextSize = theme.Size(theme.SizeNameCaptionText)
	txt.Alignment = fyne.TextAlignCenter
	return &roundRenderer{r: r, icon: ic, text: txt}
}

type roundRenderer struct {
	r    *roundAction
	icon *canvas.Image
	text *canvas.Text
}

func (rr *roundRenderer) MinSize() fyne.Size {
	w := fyne.Max(roundSize+8, rr.text.MinSize().Width+4)
	return fyne.NewSize(fyne.Max(w, 56), roundSize+4+rr.text.MinSize().Height)
}
func (rr *roundRenderer) Layout(s fyne.Size) {
	x := (s.Width - roundSize) / 2
	rr.r.circle.Move(fyne.NewPos(x, 0))
	rr.r.circle.Resize(fyne.NewSize(roundSize, roundSize))
	ic := float32(18)
	rr.icon.Move(fyne.NewPos(x+(roundSize-ic)/2, (roundSize-ic)/2))
	rr.icon.Resize(fyne.NewSize(ic, ic))
	rr.text.Move(fyne.NewPos(0, roundSize+3))
	rr.text.Resize(fyne.NewSize(s.Width, rr.text.MinSize().Height))
}
func (rr *roundRenderer) Refresh() {
	c := theme.Color(theme.ColorNameButton)
	if rr.r.hovered {
		c = theme.Color(theme.ColorNameInputBorder)
	}
	rr.r.circle.FillColor = c
	rr.text.Color = theme.Color(theme.ColorNamePrimary)
	rr.r.circle.Refresh()
	rr.icon.Refresh()
	rr.text.Refresh()
}
func (rr *roundRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{rr.r.circle, rr.icon, rr.text}
}
func (rr *roundRenderer) Destroy() {}

// ---------- right-click menu on any object ----------

// contextArea wraps content so a right-click (or Control-click) shows a menu. Ordinary clicks
// are handed to onTap (a list row selects itself).
type contextArea struct {
	widget.BaseWidget
	content fyne.CanvasObject
	data    string // what the row shows (a contact ID), for the menu
	menu    func() *fyne.Menu
	onTap   func()           // Fyne gives every click to the widget that takes any click, so pass left clicks on
	mods    fyne.KeyModifier // keys held at the last mouse press (⌘ or Shift for selecting)
	win     fyne.Window
}

func newContextArea(content fyne.CanvasObject, win fyne.Window, menu func() *fyne.Menu) *contextArea {
	c := &contextArea{content: content, menu: menu, win: win}
	c.ExtendBaseWidget(c)
	return c
}

func (c *contextArea) MouseDown(e *desktop.MouseEvent) { c.mods = e.Modifier }
func (c *contextArea) MouseUp(*desktop.MouseEvent)     {}

func (c *contextArea) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}

func (c *contextArea) TappedSecondary(e *fyne.PointEvent) {
	if c.menu == nil {
		return
	}
	if m := c.menu(); m != nil && len(m.Items) > 0 {
		widget.ShowPopUpMenuAtPosition(m, c.win.Canvas(), e.AbsolutePosition)
	}
}

func (c *contextArea) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.content)
}

// ---------- tinted sidebar ----------

// sidebarBackground is the source-list tint behind the left column; it follows light/dark mode.
func sidebarBackground(a fyne.App) *canvas.Rectangle {
	r := canvas.NewRectangle(sidebarColor(a.Settings().ThemeVariant()))
	a.Settings().AddListener(func(s fyne.Settings) {
		fyne.Do(func() {
			r.FillColor = sidebarColor(s.ThemeVariant())
			r.Refresh()
		})
	})
	return r
}

// ---------- confirmations with verbs ----------

// confirm asks with buttons named for what they do ("Delete" / "Cancel"), as the HIG asks,
// instead of Yes / No. destructive colours the button red.
func (u *ui) confirm(title, message, verb string, destructive bool, do func()) {
	l := widget.NewLabel(message)
	l.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustomConfirm(title, verb, "Cancel", container.NewPadded(l), func(ok bool) {
		if ok {
			do()
		}
	}, u.win)
	ok := d.Confirm
	if destructive {
		d.SetConfirmImportance(widget.DangerImportance)
		ok = d.Dismiss // Return never confirms a destructive action: it has to be clicked
	} else {
		d.SetConfirmImportance(widget.HighImportance)
	}
	d.Resize(fyne.NewSize(440, 0))
	trackDialog(d, ok)
	d.Show()
}

// fieldLabel shows the standard labels in lower case, as Contacts does ("mobile", "work"),
// and keeps names of services and custom labels as typed ("WhatsApp", "LinkedIn").
func fieldLabel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "mobile", "home", "work", "other", "main", "landline", "fax", "home fax", "work fax", "pager",
		"personal", "school", "homepage", "email", "iphone", "website", "blog", "profile":
		if strings.EqualFold(s, "iphone") {
			return "iPhone"
		}
		return strings.ToLower(strings.TrimSpace(s))
	}
	return s
}

// rowMenu is the right-click menu of a contact in the list.
func (u *ui) rowMenu(id string) *fyne.Menu {
	c := u.store.Get(id)
	if c == nil || u.busy() {
		return nil
	}
	if u.selectMode && u.picked[id] { // right-click on the selection: commands for all of it
		return u.bulkMenu(u.pickedContacts())
	}
	fav := "Add to Favorites"
	if c.Favorite {
		fav = "Remove from Favorites"
	}
	head := fyne.NewMenuItem(c.DisplayName(), nil) // says which card these commands act on
	head.Disabled = true
	return fyne.NewMenu("",
		head,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(fav, func() { u.toggleFavorite(c) }),
		fyne.NewMenuItem("Connect to…", func() { u.connectDialog(c) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Copy Card as Text", func() { u.copyCard(c) }),
		fyne.NewMenuItem("Save as vCard…", func() { u.exportVCF([]*Contact{c}, c.DisplayName()) }),
		fyne.NewMenuItem("Copy to Apple Contacts…", func() { u.sendToMacContacts([]*Contact{c}) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(selectLabel(u, id), func() { u.clickWithKeys(id, fyne.KeyModifierShortcutDefault) }),
		fyne.NewMenuItem("Select All", u.selectAllCards),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Clean Up This Card…", func() { u.cleanupFor([]*Contact{c}, "") }),
		fyne.NewMenuItem("Delete "+c.DisplayName()+"…", func() { u.deleteContact(c) }),
	)
}

func selectLabel(u *ui, id string) string {
	if u.selectMode {
		return "Add to Selection"
	}
	return "Select"
}

// confirmCB is confirm for callers that want the answer (the window argument is ignored; it
// keeps the call shape of dialog.ShowConfirm).
func (u *ui) confirmCB(title, message, verb string, destructive bool, cb func(bool), _ fyne.Window) {
	l := widget.NewLabel(message)
	l.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustomConfirm(title, verb, "Cancel", container.NewPadded(l), cb, u.win)
	ok := d.Confirm
	if destructive {
		d.SetConfirmImportance(widget.DangerImportance)
		ok = d.Dismiss // Return never confirms a destructive action: it has to be clicked
	} else {
		d.SetConfirmImportance(widget.HighImportance)
	}
	d.Resize(fyne.NewSize(460, 0))
	trackDialog(d, ok)
	d.Show()
}

// ---------- Esc and Return in dialogs ----------

// Mac dialogs close with Esc (Cancel) and accept with Return (the default button). Fyne's don't,
// so the dialogs the app shows are tracked and the window's key handler does it.
type keyDialog struct {
	d  dialog.Dialog
	ok func()
}

var keyDialogs []keyDialog

func trackDialog(d dialog.Dialog, ok func()) {
	keyDialogs = append(keyDialogs, keyDialog{d, ok})
	d.SetOnClosed(func() {
		for i := len(keyDialogs) - 1; i >= 0; i-- {
			if keyDialogs[i].d == d {
				keyDialogs = append(keyDialogs[:i], keyDialogs[i+1:]...)
				break
			}
		}
	})
}

// dialogKey handles Esc / Return for the top dialog; it reports whether it did.
func dialogKey(k *fyne.KeyEvent) bool {
	if len(keyDialogs) == 0 {
		return false
	}
	top := keyDialogs[len(keyDialogs)-1]
	switch k.Name {
	case fyne.KeyEscape:
		top.d.Dismiss()
		return true
	case fyne.KeyReturn, fyne.KeyEnter:
		if top.ok != nil {
			top.ok()
		} else {
			top.d.Dismiss()
		}
		return true
	}
	return false
}

// showInfo and showErr are dialog.ShowInformation and dialog.ShowError that close with Esc or Return.
func showInfo(title, message string, w fyne.Window) {
	d := dialog.NewInformation(title, message, w)
	trackDialog(d, nil)
	d.Show()
}

func showErr(err error, w fyne.Window) {
	d := dialog.NewError(err, w)
	trackDialog(d, nil)
	d.Show()
}
