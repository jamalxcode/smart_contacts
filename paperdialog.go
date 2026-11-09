package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"
	"strings"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
)

// Windows in the organiser are things on the desk:
//
//   - one that wants something filled in (a name, a note, a choice) is a sheet on a wooden
//     clipboard, held by a steel clip, its fields ruled lines ready to write on;
//   - one that only tells you something or asks you to confirm is a postage stamp: perforated
//     edges, an engraved skyline, a postmark.
//
// Every window in the app goes through these functions, which have the same shape as Fyne's
// (dialog.NewCustomConfirm and friends) and pick clipboard or stamp from what's in the window.
// In the plain Mac look, or while the book is closed, they make ordinary Fyne windows.

var paperUI *ui // the window the paper dialogs belong to

// ---------- the dialog ----------

type paperKind int

const (
	kindStamp paperKind = iota
	kindClipboard
)

type paperDialog struct {
	title            string
	content          fyne.CanvasObject
	confirm, dismiss string
	cb               func(bool)
	custom           []fyne.CanvasObject // buttons the caller made (SetButtons)
	hasCustom        bool
	importance       widget.Importance
	size             fyne.Size
	onClosed         []func()
	keyOK            func() // what Return does, when the caller said (trackDialog)
	keyOKSet         bool
	w                fyne.Window

	clipboard bool // always a clipboard (a wizard of choices has no text fields to spot)

	fallback dialog.Dialog
	overlay  *fyne.Container
	sheet    *paperSheet
	open     bool
}

func newPaper(title, confirm, dismiss string, content fyne.CanvasObject, cb func(bool), w fyne.Window) *paperDialog {
	return &paperDialog{title: title, confirm: confirm, dismiss: dismiss, content: content, cb: cb, w: w, importance: widget.HighImportance}
}

// newConfirm is dialog.NewCustomConfirm.
func newConfirm(title, confirm, dismiss string, content fyne.CanvasObject, cb func(bool), w fyne.Window) *paperDialog {
	return newPaper(title, confirm, dismiss, content, cb, w)
}

// showCustomConfirm is dialog.ShowCustomConfirm.
func showCustomConfirm(title, confirm, dismiss string, content fyne.CanvasObject, cb func(bool), w fyne.Window) {
	newConfirm(title, confirm, dismiss, content, cb, w).Show()
}

// newCustom is dialog.NewCustom: one button that closes it.
func newCustom(title, dismiss string, content fyne.CanvasObject, w fyne.Window) *paperDialog {
	return newPaper(title, "", dismiss, content, nil, w)
}

// newCustomWithoutButtons is dialog.NewCustomWithoutButtons.
func newCustomWithoutButtons(title string, content fyne.CanvasObject, w fyne.Window) *paperDialog {
	d := newPaper(title, "", "", content, nil, w)
	d.hasCustom = true
	return d
}

// newForm is dialog.NewForm.
func newForm(title, confirm, dismiss string, items []*widget.FormItem, cb func(bool), w fyne.Window) *paperDialog {
	return newPaper(title, confirm, dismiss, widget.NewForm(items...), cb, w)
}

// showForm is dialog.ShowForm.
func showForm(title, confirm, dismiss string, items []*widget.FormItem, cb func(bool), w fyne.Window) {
	newForm(title, confirm, dismiss, items, cb, w).Show()
}

// newInformation is dialog.NewInformation.
func newInformation(title, message string, w fyne.Window) *paperDialog {
	l := widget.NewLabel(message)
	l.Wrapping = fyne.TextWrapWord
	return newPaper(title, "", "OK", l, nil, w)
}

// newError is dialog.NewError, with the error written as a sentence.
func newError(err error, w fyne.Window) *paperDialog {
	msg := err.Error()
	if r := []rune(msg); len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
		msg = string(r)
	}
	if !strings.HasSuffix(msg, ".") && !strings.HasSuffix(msg, "?") && !strings.HasSuffix(msg, "!") {
		msg += "."
	}
	return newInformation("Something went wrong", msg, w)
}

func (d *paperDialog) SetOnClosed(f func()) {
	d.onClosed = append(d.onClosed, f)
	if d.fallback != nil {
		d.fallback.SetOnClosed(d.closed)
	}
}
func (d *paperDialog) SetDismissText(s string)                  { d.dismiss = s }
func (d *paperDialog) SetConfirmText(s string)                  { d.confirm = s }
func (d *paperDialog) SetConfirmImportance(i widget.Importance) { d.importance = i }
func (d *paperDialog) SetButtons(b []fyne.CanvasObject)         { d.custom, d.hasCustom = b, true }
func (d *paperDialog) Resize(s fyne.Size) {
	d.size = s
	if d.fallback != nil {
		d.fallback.Resize(s)
	}
	if d.overlay != nil {
		d.overlay.Refresh()
	}
}
func (d *paperDialog) MinSize() fyne.Size {
	if d.sheet != nil {
		return d.sheet.MinSize()
	}
	return d.content.MinSize()
}
func (d *paperDialog) Refresh() {
	if d.fallback != nil {
		d.fallback.Refresh()
	}
	if d.overlay != nil {
		d.overlay.Refresh()
	}
}

func (d *paperDialog) closed() {
	for _, f := range d.onClosed {
		f()
	}
}

// Hide closes it without answering.
func (d *paperDialog) Hide() {
	if d.fallback != nil {
		d.fallback.Hide()
		return
	}
	if !d.open {
		return
	}
	d.open = false
	if paperUI != nil && paperUI.modalLayer != nil {
		paperUI.modalLayer.Remove(d.overlay)
		// Remove only lays the layer out again; without a refresh the window stays painted
		// until something else redraws (a click did, so Close worked and Esc seemed not to)
		paperUI.modalLayer.Refresh()
	}
	d.closed()
}

// Dismiss closes it as the Cancel button does.
func (d *paperDialog) Dismiss() {
	if d.fallback != nil {
		d.fallback.Dismiss()
		return
	}
	d.Hide()
	if d.cb != nil {
		d.cb(false)
	}
}

// Confirm closes it as the confirm button does.
func (d *paperDialog) Confirm() {
	if d.fallback != nil {
		if c, ok := d.fallback.(*dialog.ConfirmDialog); ok {
			c.Confirm()
		}
		return
	}
	d.Hide()
	if d.cb != nil {
		d.cb(true)
	}
}

// paperOK reports whether the desk-style windows can be used now.
func paperOK() bool {
	u := paperUI
	return u != nil && !u.cfg.ClassicLook && !u.coverShown && u.modalLayer != nil && u.bookContent != nil &&
		u.win.Content() == u.bookContent
}

func (d *paperDialog) Show() {
	if !paperOK() {
		d.showFallback()
		return
	}
	if d.open {
		return
	}
	u := paperUI
	kind := kindStamp
	if d.clipboard || hasInput(d.content) {
		kind = kindClipboard
	}
	var buttons []fyne.CanvasObject
	if d.hasCustom {
		buttons = d.custom
	} else {
		if d.dismiss != "" {
			buttons = append(buttons, newPaperButton(d.dismiss, btnPlain, d.Dismiss))
		}
		if d.confirm != "" {
			k := btnPrimary
			if d.importance == widget.DangerImportance {
				k = btnDanger
			}
			buttons = append(buttons, newPaperButton(d.confirm, k, d.Confirm))
		}
	}
	d.sheet = newPaperSheet(kind, d.title, d.content, buttons)
	dim := canvas.NewRectangle(color.NRGBA{0x10, 0x08, 0x02, 0x70})
	block := newTapSink(canvas.NewRectangle(color.Transparent)) // clicks outside go nowhere
	d.overlay = container.New(&sheetPlacer{d: d}, dim, block, d.sheet)
	u.modalLayer.Add(d.overlay)
	d.open = true
	// Esc and Return work every time it's shown (a window built once and shown again, like
	// Import, used to lose them after its first close)
	ok := d.Confirm
	if d.confirm == "" || d.importance == widget.DangerImportance {
		ok = nil // Return never confirms a destructive action
	}
	if d.keyOKSet {
		ok = d.keyOK
	}
	trackDialog(d, ok)
	// it slides onto the desk
	d.sheet.drop = -26
	// Only the sheet moves: refreshing the whole overlay every frame redrew every label and
	// picture on it, which made big windows (Import) slow to appear.
	place := &sheetPlacer{d: d}
	a := fyne.NewAnimation(170*time.Millisecond, func(t float32) {
		d.sheet.drop = -26 * (1 - t) * (1 - t)
		if d.overlay != nil {
			place.Layout(d.overlay.Objects, d.overlay.Size())
		}
	})
	a.Start()
	// Focus the first field; with none (a wizard of choices, a stamp), take the keyboard off
	// whatever is behind — or the search field there keeps it and eats Esc and Return.
	if f := firstInput(d.content); kind == kindClipboard && f != nil {
		u.win.Canvas().Focus(f)
	} else {
		u.win.Canvas().Unfocus()
	}
}

func (d *paperDialog) showFallback() {
	switch {
	case d.hasCustom:
		c := dialog.NewCustomWithoutButtons(d.title, d.content, d.w)
		if len(d.custom) > 0 {
			c.SetButtons(d.custom)
		}
		d.fallback = c
	case d.confirm != "":
		c := dialog.NewCustomConfirm(d.title, d.confirm, d.dismiss, d.content, d.cb, d.w)
		c.SetConfirmImportance(d.importance)
		d.fallback = c
	default:
		d.fallback = dialog.NewCustom(d.title, d.dismiss, d.content, d.w)
	}
	if d.size.Width > 0 || d.size.Height > 0 {
		d.fallback.Resize(d.size)
	}
	if len(d.onClosed) > 0 {
		d.fallback.SetOnClosed(d.closed)
	}
	d.fallback.Show()
}

// sheetPlacer centres the clipboard or stamp, at its requested size within the window.
type sheetPlacer struct{ d *paperDialog }

func (l *sheetPlacer) MinSize(objs []fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (l *sheetPlacer) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	objs[0].Resize(s)
	objs[1].Resize(s)
	sh := l.d.sheet
	m := sh.MinSize()
	w := fyne.Max(m.Width, fyne.Max(l.d.size.Width+60, 440))
	h := fyne.Max(m.Height, l.d.size.Height+120)
	w, h = fyne.Min(w, s.Width-40), fyne.Min(h, s.Height-40)
	sh.Resize(fyne.NewSize(w, h))
	sh.Move(fyne.NewPos((s.Width-w)/2, (s.Height-h)/2+sh.drop))
}

// hasInput reports whether a window asks for something to be filled in or chosen.
func hasInput(o fyne.CanvasObject) bool {
	switch t := o.(type) {
	case nil:
		return false
	case *widget.Entry, *cardEntry, *widget.SelectEntry, *widget.Select, *widget.Check, *widget.CheckGroup,
		*widget.RadioGroup, *widget.Slider, *widget.Form, *widget.List:
		return true
	case *fyne.Container:
		for _, c := range t.Objects {
			if hasInput(c) {
				return true
			}
		}
	case *container.Scroll:
		return hasInput(t.Content)
	case *container.ThemeOverride:
		return hasInput(t.Content)
	case *container.AppTabs:
		for _, it := range t.Items {
			if hasInput(it.Content) {
				return true
			}
		}
	}
	return false
}

// firstInput finds the first text field, to put the keyboard in it.
func firstInput(o fyne.CanvasObject) fyne.Focusable {
	switch t := o.(type) {
	case *widget.Entry:
		return t
	case *cardEntry:
		return t
	case *widget.SelectEntry:
		return t
	case *widget.Form:
		for _, it := range t.Items {
			if f := firstInput(it.Widget); f != nil {
				return f
			}
		}
	case *fyne.Container:
		for _, c := range t.Objects {
			if f := firstInput(c); f != nil {
				return f
			}
		}
	case *container.Scroll:
		return firstInput(t.Content)
	case *container.ThemeOverride:
		return firstInput(t.Content)
	}
	return nil
}

// ---------- the sheet: clipboard or stamp ----------

type paperSheet struct {
	widget.BaseWidget
	kind    paperKind
	title   string
	content fyne.CanvasObject
	buttons []fyne.CanvasObject
	drop    float32 // how far above its place it is while it slides in
}

func newPaperSheet(kind paperKind, title string, content fyne.CanvasObject, buttons []fyne.CanvasObject) *paperSheet {
	s := &paperSheet{kind: kind, title: title, content: content, buttons: buttons}
	s.ExtendBaseWidget(s)
	return s
}

func (s *paperSheet) CreateRenderer() fyne.WidgetRenderer {
	r := &sheetRenderer{s: s}
	r.shadow = canvas.NewRectangle(color.NRGBA{0, 0, 0, 0x60})
	r.shadow.CornerRadius = 16
	if s.kind == kindClipboard {
		r.board = canvas.NewImageFromImage(image.NewNRGBA(image.Rect(0, 0, 1, 1))) // drawn to size in Layout
		r.board.FillMode = canvas.ImageFillStretch
		r.paper = canvas.NewRectangle(color.NRGBA{0xFA, 0xF5, 0xE8, 0xFF})
		r.paper.CornerRadius = 6
		r.paperShadow = canvas.NewRectangle(color.NRGBA{0x30, 0x18, 0x05, 0x50})
		r.paperShadow.CornerRadius = 6
		r.grain = canvas.NewImageFromImage(texGrain)
		r.grain.FillMode = canvas.ImageFillStretch
		r.clip = canvas.NewImageFromImage(clipboardClipImage())
		r.clip.FillMode = canvas.ImageFillStretch
	} else {
		r.stamp = canvas.NewImageFromImage(image.NewNRGBA(image.Rect(0, 0, 1, 1)))
		r.stamp.FillMode = canvas.ImageFillStretch
		r.post = canvas.NewImageFromImage(postmarkImage())
		r.post.FillMode = canvas.ImageFillStretch
	}
	ink := color.NRGBA{0x2A, 0x22, 0x1A, 0xFF}
	r.titleT = canvas.NewText(s.title, ink)
	r.titleT.TextSize = 22
	r.titleT.TextStyle = serif(s.title, true, s.kind == kindClipboard)
	r.titleT.Alignment = fyne.TextAlignCenter
	r.rule = canvas.NewRectangle(color.NRGBA{0x8A, 0x7A, 0x60, 0x70})
	th := fyne.CurrentApp().Settings().Theme()
	body := s.content
	inkChoices(body)             // tick boxes and radio buttons keep an ink outline on the paper
	if s.kind == kindClipboard { // fields are lines to write on
		body = container.NewStack(s.content, newRuleOverlay(s.content))
	}
	r.body = container.NewThemeOverride(body, &liveTheme{th})
	if s.kind == kindStamp {
		r.body = container.NewThemeOverride(body, &stampTheme{th})
	}
	r.btnRow = container.NewHBox(s.buttons...)
	return r
}

type sheetRenderer struct {
	s                                *paperSheet
	shadow, paper, paperShadow, rule *canvas.Rectangle
	board, grain, clip               *canvas.Image
	stamp, post                      *canvas.Image
	titleT                           *canvas.Text
	body                             fyne.CanvasObject
	btnRow                           *fyne.Container
	stampSize                        fyne.Size
}

// insets: where the content starts and ends inside the sheet
func (r *sheetRenderer) insets() (left, top, right, bottom float32) {
	if r.s.kind == kindClipboard {
		return 46, 116, 46, 34 // board 18 + paper margin 28; the clip and title above
	}
	return 44, 160, 44, 38 // inside the perforations, under the engraving and title
}

func (r *sheetRenderer) MinSize() fyne.Size {
	l, t, rt, b := r.insets()
	cm := r.body.MinSize()
	bm := r.btnRow.MinSize()
	w := fyne.Max(fyne.Max(cm.Width, bm.Width), r.titleT.MinSize().Width+40)
	return fyne.NewSize(w+l+rt, t+cm.Height+16+bm.Height+b)
}

func (r *sheetRenderer) Layout(sz fyne.Size) {
	l, t, rt, b := r.insets()
	if r.s.kind == kindClipboard {
		r.shadow.Move(fyne.NewPos(8, 14))
		r.shadow.Resize(sz)
	} else { // a stamp lies flat: a soft shadow just inside its perforations
		r.shadow.FillColor = color.NRGBA{0, 0, 0, 0x48}
		r.shadow.Move(fyne.NewPos(10, 14))
		r.shadow.Resize(sz.Subtract(fyne.NewSize(12, 12)))
	}
	if r.s.kind == kindClipboard {
		if r.stampSize != sz && sz.Width > 10 && sz.Height > 10 {
			r.stampSize = sz
			r.board.Image = boardImage(int(sz.Width), int(sz.Height))
			r.board.Refresh()
		}
		r.board.Resize(sz)
		pp := fyne.NewPos(18, 30)
		ps := fyne.NewSize(sz.Width-36, sz.Height-48)
		r.paperShadow.Move(pp.Add(fyne.NewPos(2, 4)))
		r.paperShadow.Resize(ps)
		r.paper.Move(pp)
		r.paper.Resize(ps)
		r.grain.Move(pp)
		r.grain.Resize(ps)
		cw := fyne.Min(200, sz.Width*0.5)
		r.clip.Move(fyne.NewPos((sz.Width-cw)/2, -22))
		r.clip.Resize(fyne.NewSize(cw, cw*0.36))
		r.titleT.Move(fyne.NewPos(0, 66))
		r.titleT.Resize(fyne.NewSize(sz.Width, 30))
		r.rule.Move(fyne.NewPos(46, 102))
		r.rule.Resize(fyne.NewSize(sz.Width-92, 1))
	} else {
		if r.stampSize != sz && sz.Width > 10 && sz.Height > 10 {
			r.stampSize = sz
			r.stamp.Image = stampSheetImage(int(sz.Width), int(sz.Height))
			r.stamp.Refresh()
		}
		r.stamp.Resize(sz)
		ps := float32(112)
		r.post.Move(fyne.NewPos(sz.Width-ps-10, 18))
		r.post.Resize(fyne.NewSize(ps, ps))
		r.titleT.Move(fyne.NewPos(0, 112))
		r.titleT.Resize(fyne.NewSize(sz.Width, 30))
		r.rule.Move(fyne.NewPos(44, 148))
		r.rule.Resize(fyne.NewSize(sz.Width-88, 1))
	}
	bm := r.btnRow.MinSize()
	r.btnRow.Resize(bm)
	r.btnRow.Move(fyne.NewPos(sz.Width-rt-bm.Width, sz.Height-b-bm.Height))
	r.body.Move(fyne.NewPos(l, t))
	r.body.Resize(fyne.NewSize(sz.Width-l-rt, sz.Height-t-b-bm.Height-16))
}

func (r *sheetRenderer) Refresh() {
	r.titleT.Text = r.s.title
	r.titleT.Refresh()
	r.Layout(r.s.Size())
}

func (r *sheetRenderer) Objects() []fyne.CanvasObject {
	if r.s.kind == kindClipboard {
		return []fyne.CanvasObject{r.shadow, r.board, r.paperShadow, r.paper, r.grain, r.clip, r.titleT, r.rule, r.body, r.btnRow}
	}
	return []fyne.CanvasObject{r.shadow, r.stamp, r.titleT, r.rule, r.body, r.btnRow, r.post}
}
func (r *sheetRenderer) Destroy() {}

// stampTheme: text on the stamp is ink on cream, with no boxes.
type stampTheme struct{ fyne.Theme }

func (t *stampTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameInputBackground, theme.ColorNameInputBorder:
		return color.Transparent
	}
	return t.Theme.Color(n, v)
}

// ---------- buttons on paper ----------

type paperBtnKind int

const (
	btnPlain   paperBtnKind = iota // cream card
	btnPrimary                     // navy enamel
	btnDanger                      // red enamel
)

type paperButton struct {
	widget.BaseWidget
	text    string
	kind    paperBtnKind
	onTap   func()
	hover   bool
	pressed bool
}

func newPaperButton(text string, kind paperBtnKind, onTap func()) *paperButton {
	b := &paperButton{text: text, kind: kind, onTap: onTap}
	b.ExtendBaseWidget(b)
	return b
}

func (b *paperButton) Tapped(*fyne.PointEvent) {
	if b.onTap != nil {
		b.onTap()
	}
}
func (b *paperButton) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (b *paperButton) MouseIn(*desktop.MouseEvent)    { b.hover = true; b.Refresh() }
func (b *paperButton) MouseMoved(*desktop.MouseEvent) {}
func (b *paperButton) MouseOut()                      { b.hover, b.pressed = false, false; b.Refresh() }
func (b *paperButton) MouseDown(*desktop.MouseEvent)  { b.pressed = true; b.Refresh() }
func (b *paperButton) MouseUp(*desktop.MouseEvent)    { b.pressed = false; b.Refresh() }

func (b *paperButton) CreateRenderer() fyne.WidgetRenderer {
	r := &pbRenderer{b: b}
	r.shadow = canvas.NewRectangle(color.NRGBA{0x20, 0x10, 0x00, 0x50})
	r.shadow.CornerRadius = 9
	r.body = canvas.NewRectangle(color.White)
	r.body.CornerRadius = 9
	r.body.StrokeWidth = 1
	r.shine = canvas.NewRectangle(color.NRGBA{0xFF, 0xFF, 0xFF, 0x40})
	r.shine.CornerRadius = 8
	r.text = canvas.NewText(b.text, color.White)
	r.text.TextSize = 14
	r.text.TextStyle.Bold = true
	r.text.Alignment = fyne.TextAlignCenter
	r.Refresh()
	return r
}

type pbRenderer struct {
	b                   *paperButton
	shadow, body, shine *canvas.Rectangle
	text                *canvas.Text
}

func (r *pbRenderer) MinSize() fyne.Size {
	return fyne.NewSize(fyne.Max(110, r.text.MinSize().Width+40), 38)
}
func (r *pbRenderer) Layout(s fyne.Size) {
	dy := float32(0)
	if r.b.pressed {
		dy = 1
	}
	r.shadow.Move(fyne.NewPos(0, 3))
	r.shadow.Resize(s)
	r.body.Move(fyne.NewPos(0, dy))
	r.body.Resize(s)
	r.shine.Move(fyne.NewPos(2, 2+dy))
	r.shine.Resize(fyne.NewSize(s.Width-4, s.Height*0.45))
	ts := r.text.MinSize()
	r.text.Move(fyne.NewPos(0, (s.Height-ts.Height)/2+dy))
	r.text.Resize(fyne.NewSize(s.Width, ts.Height))
}
func (r *pbRenderer) Refresh() {
	var fill, edge, ink color.NRGBA
	switch r.b.kind {
	case btnPrimary:
		fill, edge, ink = color.NRGBA{0x2A, 0x4E, 0x86, 0xFF}, color.NRGBA{0x14, 0x2C, 0x52, 0xFF}, color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	case btnDanger:
		fill, edge, ink = color.NRGBA{0xA8, 0x30, 0x28, 0xFF}, color.NRGBA{0x66, 0x16, 0x10, 0xFF}, color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	default:
		fill, edge, ink = color.NRGBA{0xF4, 0xEE, 0xDF, 0xFF}, color.NRGBA{0xA8, 0x98, 0x7C, 0xFF}, color.NRGBA{0x2A, 0x22, 0x1A, 0xFF}
	}
	k := 1.0
	if r.b.hover {
		k = 1.08
	}
	if r.b.pressed {
		k = 0.9
	}
	sc := func(v uint8) uint8 { return clamp8(float64(v) * k) }
	r.body.FillColor = color.NRGBA{sc(fill.R), sc(fill.G), sc(fill.B), 0xFF}
	r.body.StrokeColor = edge
	r.text.Color = ink
	r.text.Text = r.b.text
	r.shadow.Refresh()
	r.body.Refresh()
	r.shine.Refresh()
	r.text.Refresh()
	r.Layout(r.b.Size())
}
func (r *pbRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.shadow, r.body, r.shine, r.text}
}
func (r *pbRenderer) Destroy() {}

// ---------- pictures: wood, the clip, the stamp, the postmark ----------

var woodCache, clipCache, postCache image.Image

// woodImage is the clipboard's hardboard: warm brown with long grain.
func woodImage() image.Image {
	if woodCache != nil {
		return woodCache
	}
	w, h := 520, 700
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	n := newNoise(21)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x), float64(y)
			warp := n.fbm(fx/180, fy/40, 3) * 9
			ring := 0.5 + 0.5*math.Sin(fx/11+warp)
			fine := n.fbm(fx/2.5, fy/40, 2)
			v := 0.78 + 0.12*ring + 0.10*fine - 0.08*n.fbm(fx/90, fy/90, 3)
			i := y*img.Stride + x*4
			img.Pix[i] = clamp8(168 * v)
			img.Pix[i+1] = clamp8(112 * v)
			img.Pix[i+2] = clamp8(64 * v)
			img.Pix[i+3] = 255
		}
	}
	woodCache = img
	return img
}

// boardImage is the wood cut to the clipboard's size, with rounded corners and a bevelled edge.
func boardImage(w, h int) image.Image {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(out, out.Bounds(), woodImage(), woodImage().Bounds(), xdraw.Src, nil)
	const rad = 16.0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			cx, cy := math.Max(rad, math.Min(float64(w)-rad, fx)), math.Max(rad, math.Min(float64(h)-rad, fy))
			d := rad - math.Hypot(fx-cx, fy-cy) // distance inside the edge
			i := y*out.Stride + x*4
			if d <= 0 {
				out.Pix[i+3] = 0
				continue
			}
			a := math.Min(1, d)
			edge := math.Min(math.Min(fx, fy), math.Min(float64(w)-fx, float64(h)-fy))
			k := 1.0
			if edge < 4 { // the cut edge catches the light on top, darker below
				k = 0.8 + 0.1*edge/4
				if fy < 6 {
					k = 1.15
				}
			}
			out.Pix[i] = clamp8(float64(out.Pix[i]) * k)
			out.Pix[i+1] = clamp8(float64(out.Pix[i+1]) * k)
			out.Pix[i+2] = clamp8(float64(out.Pix[i+2]) * k)
			out.Pix[i+3] = clamp8(255 * a)
		}
	}
	return out
}

// clipboardClipImage is the steel clip: a ring at the top over a curved lever plate, with
// two rivets.
func clipboardClipImage() image.Image {
	if clipCache != nil {
		return clipCache
	}
	const k = 2
	w, h := 200*k, 72*k
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	set := func(x, y int, v float64, a float64) {
		if x < 0 || y < 0 || x >= w || y >= h || a <= 0 {
			return
		}
		i := y*img.Stride + x*4
		oa := float64(img.Pix[i+3]) / 255
		na := math.Min(1, a)
		out := na + oa*(1-na)
		mix := func(o uint8, nv float64) uint8 { return clamp8((nv*na + float64(o)*oa*(1-na)) / out) }
		g := clamp8(v)
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = mix(img.Pix[i], float64(g)), mix(img.Pix[i+1], float64(g)), mix(img.Pix[i+2], math.Min(255, float64(g)+8))
		img.Pix[i+3] = clamp8(out * 255)
	}
	// shadow under the plate
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/k, float64(y)/k
			// the plate: a rounded trapezoid from y 30 to 66
			if fy >= 32 && fy <= 70 {
				t := (fy - 32) / 38
				half := 64 + 30*t
				if math.Abs(fx-100) <= half {
					d := math.Min(half-math.Abs(fx-100), math.Min(fy-32, 70-fy))
					set(x+3*k, y+4*k, 0, 0.35*math.Min(1, d/3))
				}
			}
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/k, float64(y)/k
			// the ring at the top
			rd := math.Hypot(fx-100, fy-22)
			if rd >= 11 && rd <= 19 {
				t := (rd - 11) / 8
				set(x, y, chrome(t), math.Min(1, math.Min(rd-11, 19-rd)*k))
			}
			// the plate
			if fy >= 30 && fy <= 66 {
				t := (fy - 30) / 36
				half := 62 + 30*t
				if dx := math.Abs(fx - 100); dx <= half {
					edge := math.Min(half-dx, math.Min(fy-30, 66-fy))
					v := chrome(0.15+0.8*t) * (0.92 + 0.08*math.Cos(dx/half*math.Pi/2))
					set(x, y, v, math.Min(1, edge*k))
				}
			}
			// rivets
			for _, rx := range []float64{58, 142} {
				if d := math.Hypot(fx-rx, fy-52); d <= 5 {
					set(x, y, 230-25*d, math.Min(1, (5-d)*k))
				}
			}
		}
	}
	clipCache = img
	return img
}

// stampSheetImage is a postage stamp the size of the window: cream paper with perforated
// edges, a fine inner frame, and an engraved skyline across the top.
func stampSheetImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	const r = 7.0 // perforation radius
	const pitch = 19.0
	paper := color.NRGBA{0xF6, 0xEE, 0xDA, 0xFF}
	n := newNoise(5)
	inHole := func(x, y float64) bool {
		// holes along each edge, centred on the edge line
		nearest := func(v, length float64) float64 {
			count := math.Floor(length / pitch)
			off := (length - count*pitch) / 2
			k := math.Round((v - off) / pitch)
			return off + k*pitch
		}
		if y < r+1 {
			if math.Hypot(x-nearest(x, float64(w)), y) < r {
				return true
			}
		}
		if y > float64(h)-r-1 {
			if math.Hypot(x-nearest(x, float64(w)), float64(h)-y) < r {
				return true
			}
		}
		if x < r+1 {
			if math.Hypot(x, y-nearest(y, float64(h))) < r {
				return true
			}
		}
		if x > float64(w)-r-1 {
			if math.Hypot(float64(w)-x, y-nearest(y, float64(h))) < r {
				return true
			}
		}
		return false
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			if inHole(fx, fy) {
				continue
			}
			v := 0.94 + 0.06*n.fbm(fx/2, fy/2, 2) - 0.05*n.fbm(fx/120, fy/120, 3)
			i := y*img.Stride + x*4
			img.Pix[i] = clamp8(float64(paper.R) * v)
			img.Pix[i+1] = clamp8(float64(paper.G) * v)
			img.Pix[i+2] = clamp8(float64(paper.B) * v)
			img.Pix[i+3] = 255
		}
	}
	// a fine double frame inside the perforations
	frame := color.NRGBA{0x8A, 0x6E, 0x48, 0xFF}
	box := func(in int, alpha uint8) {
		for x := in; x < w-in; x++ {
			for _, y := range []int{in, h - in - 1} {
				if (x/4)%2 == 0 {
					img.SetNRGBA(x, y, color.NRGBA{frame.R, frame.G, frame.B, alpha})
				}
			}
		}
		for y := in; y < h-in; y++ {
			for _, x := range []int{in, w - in - 1} {
				if (y/4)%2 == 0 {
					img.SetNRGBA(x, y, color.NRGBA{frame.R, frame.G, frame.B, alpha})
				}
			}
		}
	}
	box(16, 0xB0)
	// the engraving: a skyline in sepia, across the top
	sky := skylineImage(w-56, 76)
	draw.Draw(img, image.Rect(28, 26, w-28, 26+76), sky, image.Point{}, draw.Over)
	return img
}

// skylineImage is an engraved city: hatched sky, buildings with windows, a spire or two.
func skylineImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	bg := color.NRGBA{0xDE, 0xD0, 0xAE, 0xFF}
	ink := color.NRGBA{0x6E, 0x58, 0x3C, 0xFF}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := bg
			if (x+y)%5 == 0 { // engraved hatching in the sky
				c = color.NRGBA{0xCC, 0xBB, 0x96, 0xFF}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	rnd := rand.New(rand.NewSource(7))
	ground := h - 8
	x := 0
	for x < w {
		bw := 10 + rnd.Intn(26)
		bh := 14 + rnd.Intn(h-26)
		if rnd.Intn(5) == 0 {
			bh = h - 14 // a tower
		}
		top := ground - bh
		shade := color.NRGBA{0xA8, 0x94, 0x70, 0xFF}
		if rnd.Intn(2) == 0 {
			shade = color.NRGBA{0x96, 0x80, 0x5E, 0xFF}
		}
		for yy := top; yy < ground; yy++ {
			for xx := x; xx < x+bw && xx < w; xx++ {
				c := shade
				if xx == x || xx == x+bw-1 || yy == top {
					c = ink
				} else if (yy-top)%5 == 2 && (xx-x)%4 == 2 { // windows
					c = color.NRGBA{0xEA, 0xDE, 0xC0, 0xFF}
				}
				img.SetNRGBA(xx, yy, c)
			}
		}
		if bh > h-20 && rnd.Intn(2) == 0 { // a spire
			cx := x + bw/2
			for yy := top - 10; yy < top; yy++ {
				img.SetNRGBA(cx, yy, ink)
			}
		}
		x += bw - 2 + rnd.Intn(4)
	}
	for yy := ground; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			img.SetNRGBA(xx, yy, color.NRGBA{0x8A, 0x74, 0x52, 0xFF})
		}
	}
	// a thin engraved frame
	for xx := 0; xx < w; xx++ {
		img.SetNRGBA(xx, 0, ink)
		img.SetNRGBA(xx, h-1, ink)
	}
	for yy := 0; yy < h; yy++ {
		img.SetNRGBA(0, yy, ink)
		img.SetNRGBA(w-1, yy, ink)
	}
	return img
}

// postmarkImage is a round postmark with today's date and wavy cancellation lines, in uneven
// red-brown ink, tilted as if stamped by hand.
func postmarkImage() image.Image {
	if postCache != nil {
		return postCache
	}
	const k = 2
	s := 120 * k
	flat := image.NewNRGBA(image.Rect(0, 0, s, s))
	ink := color.NRGBA{0x9A, 0x3A, 0x2A, 0xFF}
	c := float64(s) / 2
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			d := math.Hypot(float64(x)-c, float64(y)-c) / k
			if (d > 52 && d < 56) || (d > 44 && d < 46) {
				flat.SetNRGBA(x, y, ink)
			}
		}
	}
	if f, err := opentype.Parse(serifBold); err == nil {
		write := func(text string, size float64, cy int) {
			face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size * k, DPI: 72})
			if err != nil {
				return
			}
			d := &font.Drawer{Dst: flat, Src: image.NewUniform(ink), Face: face}
			tw := d.MeasureString(text).Ceil()
			d.Dot = fixed.P((s-tw)/2, cy*k)
			d.DrawString(text)
		}
		now := time.Now()
		write("SMART", 11, 44)
		write(strings.ToUpper(now.Format("2 Jan")), 15, 66)
		write(fmt.Sprint(now.Year()), 11, 84)
	}
	// uneven ink
	n := newNoise(3)
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			i := y*flat.Stride + x*4
			if flat.Pix[i+3] == 0 {
				continue
			}
			v := n.fbm(float64(x)/7, float64(y)/7, 3)
			flat.Pix[i+3] = clamp8(255 * math.Min(0.85, 0.25+v))
		}
	}
	a := -14 * math.Pi / 180
	out := image.NewNRGBA(image.Rect(0, 0, s, s))
	cs, sn := math.Cos(a), math.Sin(a)
	m := f64.Aff3{cs, -sn, c - cs*c + sn*c, sn, cs, c - sn*c - cs*c}
	xdraw.BiLinear.Transform(out, m, flat, flat.Bounds(), xdraw.Over, nil)
	postCache = out
	return out
}

// inkChoiceTheme outlines tick boxes and radio buttons in ink. (The paper's theme hides input
// borders so text fields are ruled lines, and Fyne draws these with the same colour.)
type inkChoiceTheme struct{ fyne.Theme }

func (t *inkChoiceTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameInputBorder:
		return color.NRGBA{0x6A, 0x5C, 0x48, 0xFF}
	case theme.ColorNameInputBackground:
		return color.NRGBA{0xFF, 0xFF, 0xFF, 0x90}
	}
	return t.Theme.Color(n, v)
}

// inkChoices wraps every tick box and radio group inside o in inkChoiceTheme.
func inkChoices(o fyne.CanvasObject) {
	th := &inkChoiceTheme{fyne.CurrentApp().Settings().Theme()}
	wrap := func(x fyne.CanvasObject) (fyne.CanvasObject, bool) {
		switch x.(type) {
		case *widget.Check, *widget.CheckGroup, *widget.RadioGroup:
			return container.NewThemeOverride(x, th), true
		}
		return x, false
	}
	switch t := o.(type) {
	case *fyne.Container:
		for i, c := range t.Objects {
			if w, ok := wrap(c); ok {
				t.Objects[i] = w
			} else {
				inkChoices(c)
			}
		}
	case *container.Scroll:
		if w, ok := wrap(t.Content); ok {
			t.Content = w
		} else {
			inkChoices(t.Content)
		}
	case *widget.Form:
		for _, it := range t.Items {
			if w, ok := wrap(it.Widget); ok {
				it.Widget = w
			} else {
				inkChoices(it.Widget)
			}
		}
	}
}
