package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// closedCover is the organiser shut: the front cover with the binding on the left, stitching,
// the title blind-embossed and gilded, and the strap fastened with its brass snap. A click (or
// any key) opens it.
type closedCover struct {
	widget.BaseWidget
	count  int
	onOpen func()
}

func (c *closedCover) Tapped(*fyne.PointEvent) {
	if c.onOpen != nil {
		c.onOpen()
	}
}
func (c *closedCover) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (c *closedCover) CreateRenderer() fyne.WidgetRenderer {
	loadTextures()
	r := &closedRenderer{c: c}
	r.desk = canvas.NewRectangle(deskColor)
	r.shadow = canvas.NewRectangle(color.NRGBA{0x30, 0x18, 0x08, 0x60})
	r.shadow.CornerRadius = 26
	r.leather = img(texCover, canvas.ImageFillStretch)
	r.edge = canvas.NewRectangle(color.Transparent)
	r.edge.StrokeColor = color.NRGBA{0x3A, 0x1A, 0x0A, 0xC0}
	r.edge.StrokeWidth = 3
	r.edge.CornerRadius = 22
	r.spine = canvas.NewHorizontalGradient(color.NRGBA{0x20, 0x0C, 0x04, 0xB0}, color.NRGBA{0x20, 0x0C, 0x04, 0x00})
	r.ridge = canvas.NewLine(color.NRGBA{0x20, 0x0C, 0x04, 0x90})
	r.ridge.StrokeWidth = 2
	r.ridgeHi = canvas.NewLine(color.NRGBA{0xE8, 0xB8, 0x88, 0x40})
	r.ridgeHi.StrokeWidth = 1
	mk := func(s string, size float32, col color.Color, st fyne.TextStyle) *canvas.Text {
		t := canvas.NewText(s, col)
		t.TextSize = size
		t.TextStyle = st
		t.Alignment = fyne.TextAlignCenter
		return t
	}
	gold := color.NRGBA{0xE2, 0xBE, 0x72, 0xFF}
	deep := color.NRGBA{0x2A, 0x10, 0x04, 0xC0}
	r.titleShadow = mk("Smart Contacts", 40, deep, serif("x", true, false))
	r.title = mk("Smart Contacts", 40, gold, serif("x", true, false))
	r.subShadow = mk("Address Book", 18, deep, serif("x", false, true))
	r.sub = mk("Address Book", 18, gold, serif("x", false, true))
	r.rule = canvas.NewLine(color.NRGBA{0xE2, 0xBE, 0x72, 0xB0})
	r.rule.StrokeWidth = 1.2
	r.hint = mk("", 13, color.NRGBA{0xF0, 0xD8, 0xB8, 0xC8}, serif("x", false, true))
	r.strap = img(texCover, canvas.ImageFillStretch)
	r.strapShade = canvas.NewVerticalGradient(color.NRGBA{0xFF, 0xE0, 0xC0, 0x30}, color.NRGBA{0x20, 0x08, 0x00, 0x60})
	r.strapEdge = canvas.NewRectangle(color.Transparent)
	r.strapEdge.StrokeColor = color.NRGBA{0x3A, 0x1A, 0x0A, 0xC0}
	r.strapEdge.StrokeWidth = 2
	r.strapEdge.CornerRadius = 14
	r.brass = img(texBrass, canvas.ImageFillContain)
	for i := 0; i < 4; i++ {
		m := canvas.NewImageFromImage(cornerMask(28, i))
		m.FillMode = canvas.ImageFillStretch
		r.corners = append(r.corners, m)
	}
	return r
}

type closedRenderer struct {
	c                                  *closedCover
	desk, shadow, edge, strapEdge      *canvas.Rectangle
	leather, strap, brass              *canvas.Image
	spine, strapShade                  *canvas.LinearGradient
	ridge, ridgeHi, rule               *canvas.Line
	title, titleShadow, sub, subShadow *canvas.Text
	hint                               *canvas.Text
	corners, stitches                  []fyne.CanvasObject
	last                               fyne.Size
}

func (r *closedRenderer) Layout(s fyne.Size) {
	r.desk.Resize(s)
	// the book fills the window, leaving only room for the strap's overhang
	ch := s.Height - 6
	cw := s.Width - 6 - 40
	x, y := float32(3), float32(3)
	r.shadow.Move(fyne.NewPos(x+1, y+2))
	r.shadow.Resize(fyne.NewSize(cw, ch))
	r.leather.Move(fyne.NewPos(x, y))
	r.leather.Resize(fyne.NewSize(cw, ch))
	r.edge.Move(fyne.NewPos(x, y))
	r.edge.Resize(fyne.NewSize(cw, ch))
	r.spine.Move(fyne.NewPos(x, y))
	r.spine.Resize(fyne.NewSize(46, ch))
	r.ridge.Position1, r.ridge.Position2 = fyne.NewPos(x+40, y+6), fyne.NewPos(x+40, y+ch-6)
	r.ridgeHi.Position1, r.ridgeHi.Position2 = fyne.NewPos(x+42, y+6), fyne.NewPos(x+42, y+ch-6)
	cs := float32(26)
	pos := []fyne.Position{{X: x, Y: y}, {X: x + cw - cs, Y: y}, {X: x, Y: y + ch - cs}, {X: x + cw - cs, Y: y + ch - cs}}
	for i, c := range r.corners {
		c.Move(pos[i])
		c.Resize(fyne.NewSize(cs, cs))
	}
	tx, tw := x+46, cw-46
	ty := y + ch*0.30
	for _, t := range []*canvas.Text{r.title, r.titleShadow} {
		t.Resize(fyne.NewSize(tw, t.MinSize().Height))
	}
	r.titleShadow.Move(fyne.NewPos(tx+1.5, ty+1.5))
	r.title.Move(fyne.NewPos(tx, ty))
	ty += r.title.MinSize().Height + 10
	r.rule.Position1, r.rule.Position2 = fyne.NewPos(tx+tw*0.3, ty), fyne.NewPos(tx+tw*0.7, ty)
	ty += 12
	for _, t := range []*canvas.Text{r.sub, r.subShadow} {
		t.Resize(fyne.NewSize(tw, t.MinSize().Height))
	}
	r.subShadow.Move(fyne.NewPos(tx+1, ty+1))
	r.sub.Move(fyne.NewPos(tx, ty))
	r.hint.Text = itoa(r.c.count) + " contacts  ·  click to open"
	r.hint.Resize(fyne.NewSize(tw, r.hint.MinSize().Height))
	r.hint.Move(fyne.NewPos(tx, y+ch-60))
	// strap over the right edge, fastened
	sw, sh := float32(116), float32(124)
	sx, sy := x+cw-78, y+ch/2-sh/2
	r.strap.Move(fyne.NewPos(sx, sy))
	r.strap.Resize(fyne.NewSize(sw, sh))
	r.strapShade.Move(fyne.NewPos(sx, sy))
	r.strapShade.Resize(fyne.NewSize(sw, sh))
	r.strapEdge.Move(fyne.NewPos(sx, sy))
	r.strapEdge.Resize(fyne.NewSize(sw, sh))
	r.brass.Move(fyne.NewPos(sx+18, sy+sh/2-23))
	r.brass.Resize(fyne.NewSize(46, 46))
	if s != r.last {
		r.last = s
		r.stitches = r.stitches[:0]
		cr := &coverRenderer{}
		cr.stitchRect(x+12, y+12, cw-24, ch-24)
		cr.stitchRect(x+56, y+28, cw-84, ch-56)
		cr.stitchRect(sx+8, sy+8, sw-16, sh-16)
		r.stitches = cr.stitches
	}
}

func (r *closedRenderer) MinSize() fyne.Size { return fyne.NewSize(500, 420) }
func (r *closedRenderer) Refresh()           { r.last = fyne.Size{}; r.Layout(r.c.Size()); canvas.Refresh(r.c) }
func (r *closedRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.desk, r.shadow, r.leather, r.edge, r.spine, r.ridge, r.ridgeHi}
	objs = append(objs, r.corners...)
	objs = append(objs, r.stitches...)
	return append(objs, r.titleShadow, r.title, r.rule, r.subShadow, r.sub, r.hint, r.strap, r.strapShade, r.strapEdge, r.brass)
}
func (r *closedRenderer) Destroy() {}

// showCover closes the organiser.
func (u *ui) showCover() {
	if u.cfg.ClassicLook || u.bookContent == nil {
		return
	}
	u.learn("strap")
	u.leaveEditor(func() {
		u.saveWindow() // remembers the page it was left at
		u.openSize = u.win.Canvas().Size()
		cv := &closedCover{count: len(u.store.Contacts), onOpen: u.openBook}
		cv.ExtendBaseWidget(cv)
		u.coverShown = true
		u.win.SetContent(onGrey(cv))
		u.sizeWindow(closedSize(u.openSize.Height), true) // the window takes the shape of the closed book
	}, nil)
}

// openBook opens the organiser at the last page (or the first, if Settings say so).
func (u *ui) openBook() {
	if !u.coverShown {
		return
	}
	u.coverShown = false
	if u.cfg.OpenAtFirst {
		u.group, u.letter, u.selectedID = grpToday, "", "" // the first page is Today
	}
	u.win.SetContent(u.bookContent)
	if sz := u.openSize; sz.Width > 0 {
		u.sizeWindow(sz, true) // opens leftwards, like the cover swinging open, and stays on screen
	}
	u.refreshAll()
	if i := u.indexOf(u.selectedID); i >= 0 {
		u.list.ScrollTo(i)
	}
	u.win.Canvas().Focus(u.search)
	if !u.closeTipShown && !u.learned("strap") { // until it's been closed with the strap once
		u.closeTipShown = true
		u.flash("To close the organiser, click the strap on the right (or press ⇧⌘W)")
	}
}

// restorePage puts back the list, A–Z tab and person the organiser was left at.
func (u *ui) restorePage() {
	if u.cfg.OpenAtFirst {
		u.group, u.letter, u.selectedID = grpToday, "", "" // the first page is Today
		return
	}
	if u.cfg.LastGroup != "" {
		u.group = u.cfg.LastGroup
	}
	u.letter = u.cfg.LastLetter
	if u.store.Get(u.cfg.LastContact) != nil {
		u.selectedID = u.cfg.LastContact
	}
}

func closedSize(h float32) fyne.Size {
	if h < 480 {
		h = 480
	}
	return fyne.NewSize(h*0.70+40, h)
}

// ---------- closing the book with the strap ----------

// strapHit sits over the strap on the open organiser: the pointer turns into a hand, a hint pops
// up, and a click asks to close the book.
type strapHit struct {
	widget.BaseWidget
	u     *ui
	cover *leatherCover
	hint  fyne.CanvasObject // drawn on the book, beside the strap (a pop-up would swallow the click)
}

func newStrapHit(u *ui) *strapHit {
	s := &strapHit{u: u}
	bg := canvas.NewRectangle(color.NRGBA{0x2A, 0x1A, 0x10, 0xE8})
	bg.CornerRadius = 6
	t := canvas.NewText("Click the strap to close the organiser", color.NRGBA{0xF6, 0xE6, 0xCC, 0xFF})
	t.TextSize = 12
	t.TextStyle = serif("x", false, true)
	s.hint = container.NewStack(bg, container.NewPadded(t))
	s.hint.Hide()
	s.ExtendBaseWidget(s)
	return s
}

func (s *strapHit) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}
func (s *strapHit) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (s *strapHit) MouseIn(*desktop.MouseEvent)    { s.hint.Show(); s.pull(true) }
func (s *strapHit) MouseMoved(*desktop.MouseEvent) {}
func (s *strapHit) MouseOut()                      { s.hint.Hide(); s.pull(false) }

// pull draws the strap a little away from the snap while the pointer is on it.
func (s *strapHit) pull(on bool) {
	if s.cover != nil && s.cover.strapOut != on {
		s.cover.strapOut = on
		s.cover.lift()
	}
}

func (s *strapHit) Tapped(*fyne.PointEvent) {
	s.MouseOut()
	s.u.confirmCB("Close the organiser?", "It will open again at this page.", "Close", false, func(ok bool) {
		if ok {
			s.u.showCover()
		}
	}, s.u.win)
}
