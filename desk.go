package main

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Things on the open organiser do what they'd do in your hands:
//   the pen writes a new card · the strap closes the book · a page corner turns to the next
//   (or previous) letter · the pocket inside the cover takes business cards.
// Each lifts a little under the pointer and says what it does, so nothing has to be learnt.

// ---------- a hot spot with a hint ----------

// hotspot is an invisible area over something drawn on the book. The pointer becomes a hand,
// onHover lets the drawing react (the pen lifts), and the hint is drawn beside it (a pop-up
// would swallow the click).
type hotspot struct {
	widget.BaseWidget
	hint    fyne.CanvasObject
	onTap   func()
	onHover func(bool)
}

func newHotspot(hint string, tap func(), hover func(bool)) *hotspot {
	h := &hotspot{onTap: tap, onHover: hover}
	bg := canvas.NewRectangle(color.NRGBA{0x2A, 0x1A, 0x10, 0xEC})
	bg.CornerRadius = 6
	t := canvas.NewText(hint, color.NRGBA{0xF6, 0xE6, 0xCC, 0xFF})
	t.TextSize = 12
	t.TextStyle = serif("x", false, true)
	h.hint = container.NewStack(bg, container.NewPadded(t))
	h.hint.Hide()
	h.ExtendBaseWidget(h)
	return h
}

func (h *hotspot) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}
func (h *hotspot) Cursor() desktop.Cursor { return desktop.PointerCursor }
func (h *hotspot) MouseIn(*desktop.MouseEvent) {
	h.hint.Show()
	if h.onHover != nil {
		h.onHover(true)
	}
}
func (h *hotspot) MouseMoved(*desktop.MouseEvent) {}
func (h *hotspot) MouseOut() {
	h.hint.Hide()
	if h.onHover != nil {
		h.onHover(false)
	}
}
func (h *hotspot) Tapped(*fyne.PointEvent) {
	h.MouseOut()
	if h.onTap != nil {
		h.onTap()
	}
}

// ---------- page corners ----------

// curlImage is a dog-eared page corner: past the fold line the flap's back shows (lighter near
// the fold, a little darker at the tip); on the page side the flap throws a soft shadow.
// mirror puts it in the bottom-left corner instead of the bottom-right.
func curlImage(s int, mirror bool) image.Image {
	m := image.NewNRGBA(image.Rect(0, 0, s, s))
	fs := float64(s)
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			if mirror {
				fx = fs - fx
			}
			d := fx + fy - fs // distance past the fold line (x+y = s), in pixels·√2
			var c color.NRGBA
			switch {
			case d >= 0:
				t := math.Min(1, d/fs)
				v := 246 - 30*t
				c = color.NRGBA{uint8(v), uint8(v - 3), uint8(v - 12), 255}
			case d > -fs*0.22:
				a := (1 + d/(fs*0.22)) * 80
				c = color.NRGBA{0x30, 0x20, 0x10, uint8(a)}
			default:
				continue
			}
			i := y*m.Stride + x*4
			m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
	for t := 0; t < s; t++ { // the crease
		x, y := s-1-t, t
		if mirror {
			x = t
		}
		i := y*m.Stride + x*4
		m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = 0xA8, 0xA0, 0x90, 0xFF
	}
	return m
}

// pageCorner is the turned-up corner of a page; clicking it turns to the next letter (the
// right-hand page) or the previous one (the left-hand page). It curls further under the pointer.
type pageCorner struct {
	widget.BaseWidget
	small, big *canvas.Image
	hover      bool
	hint       fyne.CanvasObject
	onTap      func()
	mirror     bool
}

const cornerSmall, cornerBig = 30, 46

func newPageCorner(mirror bool, hint string, tap func()) *pageCorner {
	p := &pageCorner{mirror: mirror, onTap: tap}
	p.small = canvas.NewImageFromImage(curlImage(cornerSmall*2, mirror))
	p.big = canvas.NewImageFromImage(curlImage(cornerBig*2, mirror))
	p.big.Hide()
	h := newHotspot(hint, nil, nil)
	p.hint = h.hint
	p.ExtendBaseWidget(p)
	return p
}

func (p *pageCorner) CreateRenderer() fyne.WidgetRenderer {
	return &cornerRenderer{p: p}
}
func (p *pageCorner) Cursor() desktop.Cursor { return desktop.PointerCursor }
func (p *pageCorner) MouseIn(*desktop.MouseEvent) {
	p.hover = true
	p.small.Hide()
	p.big.Show()
	p.hint.Show()
}
func (p *pageCorner) MouseMoved(*desktop.MouseEvent) {}
func (p *pageCorner) MouseOut() {
	p.hover = false
	p.big.Hide()
	p.small.Show()
	p.hint.Hide()
}
func (p *pageCorner) Tapped(*fyne.PointEvent) {
	if p.onTap != nil {
		p.onTap()
	}
}

type cornerRenderer struct{ p *pageCorner }

func (r *cornerRenderer) Layout(s fyne.Size) {
	// the images sit in the corner of this square (bottom-right, or bottom-left when mirrored)
	place := func(im *canvas.Image, n float32) {
		x := s.Width - n
		if r.p.mirror {
			x = 0
		}
		im.Move(fyne.NewPos(x, s.Height-n))
		im.Resize(fyne.NewSize(n, n))
	}
	place(r.p.small, cornerSmall)
	place(r.p.big, cornerBig)
}
func (r *cornerRenderer) MinSize() fyne.Size { return fyne.NewSize(cornerBig, cornerBig) }
func (r *cornerRenderer) Refresh()           { r.Layout(r.p.Size()); canvas.Refresh(r.p) }
func (r *cornerRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.p.small, r.p.big}
}
func (r *cornerRenderer) Destroy() {}

// turnLetter moves to the next (+1) or previous (-1) A–Z tab that has people, from All
// onwards; it stops at the ends.
func (u *ui) turnLetter(dir int) {
	cur := 0
	for i, l := range letterTabs {
		if l == u.letter || (l == "All" && u.letter == "") {
			cur = i
		}
	}
	for i := cur + dir; i >= 0 && i < len(letterTabs); i += dir {
		l := letterTabs[i]
		if l == "All" || u.letterUsed(l) {
			if l == "All" {
				u.letter = ""
			} else {
				u.letter = l
			}
			u.pageTurn(dir)
			u.refreshList()
			return
		}
	}
	u.flash("That's the last page")
}

func (u *ui) letterUsed(tab string) bool {
	for k := range u.usedLetters {
		if letterIn(tab, k) {
			return true
		}
	}
	return false
}

// ---------- the pocket for business cards ----------

// cardPocket is the leather pocket inside the front cover with a business card sticking out.
// Click it (or drop a card's photo anywhere on the window) to add someone from a card.
type cardPocket struct {
	widget.BaseWidget
	u     *ui
	hover bool
	hint  fyne.CanvasObject
}

func newCardPocket(u *ui) *cardPocket {
	p := &cardPocket{u: u}
	p.hint = newHotspot("Add someone from a business card — click, or drop a photo of the card", nil, nil).hint
	p.ExtendBaseWidget(p)
	return p
}

func (p *cardPocket) Cursor() desktop.Cursor { return desktop.PointerCursor }
func (p *cardPocket) MouseIn(*desktop.MouseEvent) {
	p.hover = true
	p.hint.Show()
	p.Refresh()
}
func (p *cardPocket) MouseMoved(*desktop.MouseEvent) {}
func (p *cardPocket) MouseOut() {
	p.hover = false
	p.hint.Hide()
	p.Refresh()
}
func (p *cardPocket) Tapped(*fyne.PointEvent) {
	p.MouseOut()
	p.u.smartAdd()
}

func (p *cardPocket) CreateRenderer() fyne.WidgetRenderer {
	loadTextures()
	r := &pocketRenderer{p: p}
	r.card = canvas.NewRectangle(color.NRGBA{0xFB, 0xFA, 0xF5, 0xFF})
	r.card.StrokeColor = color.NRGBA{0xB8, 0xB0, 0xA0, 0xFF}
	r.card.StrokeWidth = 1
	r.card.CornerRadius = 2
	r.name = canvas.NewText("Business Cards", color.NRGBA{0x3A, 0x2A, 0x1E, 0xFF})
	r.name.TextSize = 11
	r.name.TextStyle = serif("x", true, false)
	for i := 0; i < 2; i++ {
		l := canvas.NewRectangle(color.NRGBA{0xA8, 0xA4, 0x98, 0xFF})
		r.lines = append(r.lines, l)
	}
	r.leather = img(texCover, canvas.ImageFillStretch)
	r.shade = canvas.NewVerticalGradient(color.NRGBA{0x10, 0x05, 0x00, 0xB0}, color.NRGBA{0x10, 0x05, 0x00, 0x30})
	r.edge = canvas.NewRectangle(color.Transparent)
	r.edge.StrokeColor = color.NRGBA{0x2A, 0x12, 0x06, 0xC0}
	r.edge.StrokeWidth = 2
	r.edge.CornerRadius = 6
	r.label = canvas.NewText("drop or click to add", color.NRGBA{0xE8, 0xC8, 0x82, 0xE0})
	r.label.TextSize = 11
	r.label.TextStyle = serif("x", false, true)
	r.label.Alignment = fyne.TextAlignCenter
	return r
}

type pocketRenderer struct {
	p           *cardPocket
	card, edge  *canvas.Rectangle
	lines       []*canvas.Rectangle
	name, label *canvas.Text
	leather     *canvas.Image
	shade       *canvas.LinearGradient
	stitches    []fyne.CanvasObject
	last        fyne.Size
}

func (r *pocketRenderer) Layout(s fyne.Size) {
	ph := s.Height * 0.62 // the pocket
	py := s.Height - ph - 16
	lift := float32(0)
	if r.p.hover {
		lift = 8
	}
	cw := s.Width - 34
	r.card.Move(fyne.NewPos(17, py-30-lift))
	r.card.Resize(fyne.NewSize(cw, 60))
	r.name.Move(fyne.NewPos(26, py-26-lift))
	for i, l := range r.lines {
		l.Move(fyne.NewPos(26, py-8-lift+float32(i)*6))
		l.Resize(fyne.NewSize(cw*0.55-float32(i)*14, 1.4))
	}
	r.leather.Move(fyne.NewPos(6, py))
	r.leather.Resize(fyne.NewSize(s.Width-12, ph))
	r.shade.Move(fyne.NewPos(6, py))
	r.shade.Resize(fyne.NewSize(s.Width-12, ph))
	r.edge.Move(fyne.NewPos(6, py))
	r.edge.Resize(fyne.NewSize(s.Width-12, ph))
	r.label.Move(fyne.NewPos(0, s.Height-15))
	r.label.Resize(fyne.NewSize(s.Width, 14))
	if s != r.last {
		r.last = s
		r.stitches = r.stitches[:0]
		y := py + 6
		for x := float32(16); x+7 < s.Width-16; x += 12 {
			l := canvas.NewLine(stitchColor)
			l.StrokeWidth = 1.4
			l.Position1, l.Position2 = fyne.NewPos(x, y), fyne.NewPos(x+7, y)
			r.stitches = append(r.stitches, l)
		}
	}
}

func (r *pocketRenderer) MinSize() fyne.Size { return fyne.NewSize(140, 104) }
func (r *pocketRenderer) Refresh()           { r.Layout(r.p.Size()); canvas.Refresh(r.p) }
func (r *pocketRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.card, r.name}
	for _, l := range r.lines {
		objs = append(objs, l)
	}
	objs = append(objs, r.leather, r.shade, r.edge)
	objs = append(objs, r.stitches...)
	return append(objs, r.label)
}
func (r *pocketRenderer) Destroy() {}
