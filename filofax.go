package main

import (
	_ "embed"
	"image"
	"image/color"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The Filofax look: an open cognac-leather personal organiser lying on a desk. Stitched leather
// cover with a chrome pen in a loop on the left and a strap with a brass snap on the right; white
// ruled pages with grey title bars on a stack of page edges; a chrome six-ring mechanism down the
// middle; cream A–Z tabs (paired letters) down the right edge; and coloured card-stock index tabs
// for the lists and groups sticking out of the left page. All drawn by the app (no photos), sized
// to the window. Type is Libre Baskerville (SIL Open Font License, fonts/).

//go:embed fonts/serif.ttf
var serifRegular []byte

//go:embed fonts/serif-bold.ttf
var serifBold []byte

//go:embed fonts/serif-italic.ttf
var serifItalic []byte

var (
	resSerif       = fyne.NewStaticResource("serif.ttf", serifRegular)
	resSerifBold   = fyne.NewStaticResource("serif-bold.ttf", serifBold)
	resSerifItalic = fyne.NewStaticResource("serif-italic.ttf", serifItalic)
)

// ---------- colours ----------

var (
	deskColor   = color.NRGBA{0x2B, 0x2B, 0x2E, 0xFF} // dark grey around the book, like the title bar
	inkColor    = color.NRGBA{0x22, 0x24, 0x2A, 0xFF}
	inkFaint    = color.NRGBA{0x66, 0x68, 0x6E, 0xFF}
	paperColor  = color.NRGBA{0xF7, 0xF6, 0xF2, 0xFF}
	ruleColor   = color.NRGBA{0x8E, 0x97, 0xA3, 0x70}
	stitchColor = color.NRGBA{0xD8, 0xA0, 0x62, 0xD0}
	tabColors   = []color.NRGBA{ // muted divider colours
		{0xA8, 0x3A, 0x32, 0xFF}, {0xC2, 0x6E, 0x2C, 0xFF}, {0xC9, 0xA2, 0x3A, 0xFF}, {0x5E, 0x8A, 0x4A, 0xFF},
		{0x2F, 0x7A, 0x76, 0xFF}, {0x36, 0x5F, 0x9A, 0xFF}, {0x62, 0x4C, 0x92, 0xFF}, {0xA4, 0x50, 0x7C, 0xFF},
	}
)

// filofaxTheme: ink on paper, in a serif. A physical book doesn't go dark at night.
type filofaxTheme struct{ fyne.Theme }

func (t *filofaxTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return paperColor
	case theme.ColorNameForeground:
		return inkColor
	case theme.ColorNameDisabled:
		return inkFaint
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{0x9A, 0x9A, 0x96, 0xFF}
	case theme.ColorNameInputBackground:
		return color.NRGBA{0xFF, 0xFF, 0xFF, 0x90}
	case theme.ColorNameInputBorder:
		return color.NRGBA{0xC8, 0xCA, 0xCE, 0xFF}
	case theme.ColorNameSeparator:
		return ruleColor
	case theme.ColorNameButton:
		return color.NRGBA{0xE6, 0xE4, 0xDE, 0xFF}
	case theme.ColorNameHover:
		return color.NRGBA{0x60, 0x50, 0x30, 0x14}
	case theme.ColorNameSelection:
		return color.NRGBA{0xF3, 0xDE, 0x7A, 0x90} // highlighter
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground, theme.ColorNameHeaderBackground:
		return color.NRGBA{0xFA, 0xF9, 0xF6, 0xFF}
	case theme.ColorNameShadow:
		return color.NRGBA{0x20, 0x10, 0x05, 0x50}
	}
	return t.Theme.Color(n, theme.VariantLight)
}

// Font: names, fields and everything typed use the app's own font, which has Arabic. The
// organiser's printed parts (cover, tabs, page titles) ask for the serif by setting the
// Monospace style, which nothing else in the app uses — see serif().
func (t *filofaxTheme) Font(s fyne.TextStyle) fyne.Resource {
	if !s.Monospace {
		return t.Theme.Font(s)
	}
	switch {
	case s.Bold:
		return resSerifBold
	case s.Italic:
		return resSerifItalic
	}
	return resSerif
}

// serif is the text style for organiser print; text with Arabic keeps the app font.
func serif(text string, bold, italic bool) fyne.TextStyle {
	return fyne.TextStyle{Bold: bold, Italic: italic, Monospace: !hasArabic(text)}
}

// ---------- procedural textures ----------

// leatherCover draws the open cover: rounded leather with an inner darker lip where the pages
// sit, stitching round both, a pen loop with the pen on the left and the strap on the right.
type leatherCover struct {
	widget.BaseWidget
	pageArea func() (fyne.Position, fyne.Size) // where the pages are, to put the lip under them
	penUp    bool                              // the pointer is on the pen: it lifts out of its loop
	strapOut bool                              // the pointer is on the strap: it pulls away from the snap
	r        *coverRenderer
}

// lift moves the pen or strap without redrawing the stitching.
func (l *leatherCover) lift() {
	if l.r != nil {
		l.r.Layout(l.Size())
		canvas.Refresh(l)
	}
}

func (l *leatherCover) CreateRenderer() fyne.WidgetRenderer {
	loadTextures()
	r := &coverRenderer{l: l}
	l.r = r
	r.desk = canvas.NewRectangle(deskColor)
	r.shadow = canvas.NewRectangle(color.NRGBA{0x30, 0x18, 0x08, 0x50})
	r.shadow.CornerRadius = 26
	r.leather = img(texCover, canvas.ImageFillStretch)
	r.edge = canvas.NewRectangle(color.Transparent)
	r.edge.StrokeColor = color.NRGBA{0x3A, 0x1A, 0x0A, 0xC0}
	r.edge.StrokeWidth = 3
	r.edge.CornerRadius = 22
	r.lip = img(texLip, canvas.ImageFillStretch)
	r.lipEdge = canvas.NewRectangle(color.Transparent)
	r.lipEdge.StrokeColor = color.NRGBA{0x2A, 0x12, 0x06, 0xA0}
	r.lipEdge.StrokeWidth = 2
	r.lipEdge.CornerRadius = 8
	// pen loop and pen
	r.loop = img(texCover, canvas.ImageFillStretch)
	r.loopShade = canvas.NewVerticalGradient(color.NRGBA{0xFF, 0xE0, 0xC0, 0x30}, color.NRGBA{0x20, 0x08, 0x00, 0x70})
	r.pen = img(texPen, canvas.ImageFillContain)
	// strap and snap
	r.strap = img(texCover, canvas.ImageFillStretch)
	r.strapShade = canvas.NewVerticalGradient(color.NRGBA{0xFF, 0xE0, 0xC0, 0x30}, color.NRGBA{0x20, 0x08, 0x00, 0x60})
	r.strapEdge = canvas.NewRectangle(color.Transparent)
	r.strapEdge.StrokeColor = color.NRGBA{0x3A, 0x1A, 0x0A, 0xC0}
	r.strapEdge.StrokeWidth = 2
	r.strapEdge.CornerRadius = 14
	r.brass = img(texBrass, canvas.ImageFillContain)
	for i := 0; i < 4; i++ { // corner masks round the leather's corners
		m := canvas.NewImageFromImage(cornerMask(28, i))
		m.FillMode = canvas.ImageFillStretch
		r.corners = append(r.corners, m)
	}
	return r
}

// cornerMask is desk colour outside a quarter circle (which corner: 0 TL, 1 TR, 2 BL, 3 BR).
func cornerMask(s, which int) image.Image {
	m := image.NewNRGBA(image.Rect(0, 0, s, s))
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			cx, cy := float64(s), float64(s)
			if which == 1 || which == 3 {
				cx = 0
			}
			if which >= 2 {
				cy = 0
			}
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			a := math.Max(0, math.Min(1, d-float64(s)+0.5))
			i := y*m.Stride + x*4
			m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = deskColor.R, deskColor.G, deskColor.B, clamp8(255*a)
		}
	}
	return m
}

type coverRenderer struct {
	l                           *leatherCover
	desk, shadow, edge, lipEdge *canvas.Rectangle
	leather, lip, loop, pen     *canvas.Image
	strap, brass                *canvas.Image
	strapEdge                   *canvas.Rectangle
	loopShade, strapShade       *canvas.LinearGradient
	corners                     []fyne.CanvasObject
	stitches                    []fyne.CanvasObject
	last                        fyne.Size
	strapWasOut                 bool
}

const coverMargin = 3 // the leather runs almost to the window edge

func (r *coverRenderer) Layout(s fyne.Size) {
	r.desk.Resize(s)
	m := float32(coverMargin)
	cw, ch := s.Width-2*m-26, s.Height-2*m-2 // leaves room for the strap on the right
	r.shadow.Move(fyne.NewPos(m+1, m+2))
	r.shadow.Resize(fyne.NewSize(cw, ch))
	r.leather.Move(fyne.NewPos(m, m))
	r.leather.Resize(fyne.NewSize(cw, ch))
	r.edge.Move(fyne.NewPos(m, m))
	r.edge.Resize(fyne.NewSize(cw, ch))
	cs := float32(26)
	pos := []fyne.Position{{X: m, Y: m}, {X: m + cw - cs, Y: m}, {X: m, Y: m + ch - cs}, {X: m + cw - cs, Y: m + ch - cs}}
	for i, c := range r.corners {
		c.Move(pos[i])
		c.Resize(fyne.NewSize(cs, cs))
	}
	// the lip under the pages
	pp, ps := r.l.pageArea()
	r.lip.Move(pp.Subtract(fyne.NewPos(12, 12)))
	r.lip.Resize(ps.Add(fyne.NewSize(24, 24)))
	r.lipEdge.Move(pp.Subtract(fyne.NewPos(12, 12)))
	r.lipEdge.Resize(ps.Add(fyne.NewSize(24, 24)))
	// pen loop on the left, pen through it
	lx := m + 18
	penY := m + ch*0.14
	if r.l.penUp {
		penY -= 14
	}
	r.pen.Move(fyne.NewPos(lx+4, penY))
	r.pen.Resize(fyne.NewSize(30, ch*0.72))
	r.loop.Move(fyne.NewPos(lx-6, m+ch*0.33))
	r.loop.Resize(fyne.NewSize(48, 56))
	r.loopShade.Move(r.loop.Position())
	r.loopShade.Resize(r.loop.Size())
	// strap with the brass snap, over the right edge
	sw, sh := float32(86), float32(118)
	sx, sy := m+cw-50, m+ch/2-sh/2
	if r.l.strapOut {
		sx += 7 // pulled a little towards you
	}
	r.strap.Move(fyne.NewPos(sx, sy))
	r.strap.Resize(fyne.NewSize(sw, sh))
	r.strapShade.Move(fyne.NewPos(sx, sy))
	r.strapShade.Resize(fyne.NewSize(sw, sh))
	r.strapEdge.Move(fyne.NewPos(sx, sy))
	r.strapEdge.Resize(fyne.NewSize(sw, sh))
	r.brass.Move(fyne.NewPos(sx+sw-54, sy+sh/2-21))
	r.brass.Resize(fyne.NewSize(42, 42))
	if s != r.last {
		r.last = s
		r.stitches = r.stitches[:0]
		r.stitchRect(m+9, m+9, cw-18, ch-18)
		r.stitchRect(pp.X-20, pp.Y-20, ps.Width+40, ps.Height+40)
		r.stitchRect(lx-2, m+ch*0.33+5, 40, 46)
		r.stitchRect(sx+7, sy+7, sw-14, sh-14)
	}
	if r.l.strapOut != r.strapWasOut { // the strap's stitches move with it
		r.strapWasOut = r.l.strapOut
		r.last = fyne.Size{}
		r.Layout(s)
	}
}

// stitchRect lays a line of stitches round a rectangle.
func (r *coverRenderer) stitchRect(x, y, w, h float32) {
	dash, gap := float32(7), float32(5)
	add := func(x1, y1, x2, y2 float32) {
		l := canvas.NewLine(stitchColor)
		l.StrokeWidth = 1.6
		l.Position1, l.Position2 = fyne.NewPos(x1, y1), fyne.NewPos(x2, y2)
		r.stitches = append(r.stitches, l)
	}
	for t := float32(10); t+dash < w-10; t += dash + gap {
		add(x+t, y, x+t+dash, y)
		add(x+t, y+h, x+t+dash, y+h)
	}
	for t := float32(10); t+dash < h-10; t += dash + gap {
		add(x, y+t, x, y+t+dash)
		add(x+w, y+t, x+w, y+t+dash)
	}
}

func (r *coverRenderer) MinSize() fyne.Size { return fyne.NewSize(600, 400) }
func (r *coverRenderer) Refresh()           { r.last = fyne.Size{}; r.Layout(r.l.Size()); canvas.Refresh(r.l) }
func (r *coverRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.desk, r.shadow, r.leather, r.edge}
	objs = append(objs, r.corners...)
	objs = append(objs, r.lip, r.lipEdge)
	objs = append(objs, r.stitches...)
	objs = append(objs, r.pen, r.loop, r.loopShade, r.strap, r.strapShade, r.strapEdge, r.brass)
	return objs
}
func (r *coverRenderer) Destroy() {}

// ---------- pages ----------

// paperPage is one white page with a printed heading, on a small stack of page edges, with the
// shadow the rings and the binding cast at its inner edge.
type paperPage struct {
	widget.BaseWidget
	content   fyne.CanvasObject
	innerLeft bool // the binding is on its left (the right-hand page)
	title     *canvas.Text
	ruled     bool
}

func newPaperPage(content fyne.CanvasObject, innerLeft, ruled bool) *paperPage {
	p := &paperPage{content: content, innerLeft: innerLeft, ruled: ruled}
	p.title = canvas.NewText("", color.NRGBA{0x5A, 0x22, 0x18, 0xFF}) // printed in the refill's red-brown
	p.title.TextStyle = serif("", true, false)
	p.title.TextSize = 15
	p.ExtendBaseWidget(p)
	return p
}

func (p *paperPage) SetTitle(s string) {
	if p.title.Text != s {
		p.title.Text = s
		p.title.TextStyle = serif(s, true, false)
		p.title.Refresh()
	}
}

func (p *paperPage) CreateRenderer() fyne.WidgetRenderer {
	loadTextures()
	r := &pageRenderer{p: p}
	for i := 0; i < 3; i++ { // the stack of pages underneath
		e := canvas.NewRectangle(color.NRGBA{0xEC, 0xEA, 0xE3, 0xFF})
		e.StrokeColor = color.NRGBA{0xC8, 0xC4, 0xB8, 0xFF}
		e.StrokeWidth = 1
		e.CornerRadius = 6
		r.stack = append(r.stack, e)
	}
	r.shadow = canvas.NewRectangle(color.NRGBA{0x10, 0x05, 0x00, 0x55})
	r.shadow.CornerRadius = 8
	r.sheet = canvas.NewRectangle(paperColor)
	r.sheet.CornerRadius = 6
	r.grain = img(texGrain, canvas.ImageFillStretch)
	in, out := color.NRGBA{0x50, 0x48, 0x38, 0x48}, color.NRGBA{0, 0, 0, 0}
	if p.innerLeft {
		r.gutter = canvas.NewHorizontalGradient(in, out)
	} else {
		r.gutter = canvas.NewHorizontalGradient(out, in)
	}
	// the page's heading is printed: ink on paper over a double rule, as on Filofax refills
	r.bar = canvas.NewHorizontalGradient(color.NRGBA{0x6E, 0x2A, 0x1E, 0xB0}, color.NRGBA{0x6E, 0x2A, 0x1E, 0x40})
	r.barShape = canvas.NewRectangle(color.NRGBA{0x6E, 0x2A, 0x1E, 0x60})
	return r
}

type pageRenderer struct {
	p                       *paperPage
	stack                   []*canvas.Rectangle
	shadow, sheet, barShape *canvas.Rectangle
	grain                   *canvas.Image
	gutter, bar             *canvas.LinearGradient
	rules                   []fyne.CanvasObject
	lastH                   float32
}

const pageBar = 34 // the grey title bar's height incl. space

func (r *pageRenderer) Layout(s fyne.Size) {
	// page edges peek out on the outer side and bottom
	for i, e := range r.stack {
		o := float32(3-i) * 2.5
		if r.p.innerLeft {
			e.Move(fyne.NewPos(0, o))
		} else {
			e.Move(fyne.NewPos(-o, o))
		}
		e.Resize(fyne.NewSize(s.Width+o, s.Height))
	}
	r.shadow.Move(fyne.NewPos(2, 4))
	r.shadow.Resize(s)
	r.sheet.Resize(s)
	r.grain.Resize(s)
	gw := float32(34)
	if r.p.innerLeft {
		r.gutter.Move(fyne.NewPos(0, 0))
	} else {
		r.gutter.Move(fyne.NewPos(s.Width-gw, 0))
	}
	r.gutter.Resize(fyne.NewSize(gw, s.Height))
	inL, inR := float32(28), float32(16)
	if !r.p.innerLeft {
		inL, inR = 16, 28
	}
	r.bar.Move(fyne.NewPos(inL+4, 33)) // thick rule
	r.bar.Resize(fyne.NewSize(s.Width-inL-inR-8, 1.6))
	r.barShape.Move(fyne.NewPos(inL+4, 36.5)) // thin rule
	r.barShape.Resize(fyne.NewSize(s.Width-inL-inR-8, 0.7))
	r.p.title.Move(fyne.NewPos(inL+6, 31-r.p.title.MinSize().Height))
	if r.p.ruled && s.Height != r.lastH {
		r.lastH = s.Height
		r.rules = r.rules[:0]
		for y := float32(pageBar + 30); y < s.Height-14; y += 29 {
			l := canvas.NewLine(color.NRGBA{0x8E, 0x97, 0xA3, 0x38})
			l.Position1, l.Position2 = fyne.NewPos(inL, y), fyne.NewPos(s.Width-inR, y)
			r.rules = append(r.rules, l)
		}
	}
	r.p.content.Move(fyne.NewPos(inL-4, pageBar+6))
	r.p.content.Resize(fyne.NewSize(s.Width-inL-inR+8, s.Height-pageBar-14))
}

func (r *pageRenderer) MinSize() fyne.Size {
	m := r.p.content.MinSize()
	return fyne.NewSize(m.Width+48, m.Height+pageBar+20)
}
func (r *pageRenderer) Refresh() { r.Layout(r.p.Size()); canvas.Refresh(r.p) }
func (r *pageRenderer) Objects() []fyne.CanvasObject {
	var objs []fyne.CanvasObject
	for _, e := range r.stack {
		objs = append(objs, e)
	}
	objs = append(objs, r.shadow, r.sheet, r.grain, r.gutter, r.barShape, r.bar, r.p.title)
	objs = append(objs, r.rules...)
	return append(objs, r.p.content)
}
func (r *pageRenderer) Destroy() {}

// ---------- the ring mechanism ----------

type binderRings struct{ widget.BaseWidget }

func newBinderRings() *binderRings { b := &binderRings{}; b.ExtendBaseWidget(b); return b }

func (b *binderRings) CreateRenderer() fyne.WidgetRenderer {
	loadTextures()
	r := &ringsRenderer{b: b}
	r.plateShadow = canvas.NewRectangle(color.NRGBA{0x20, 0x10, 0x05, 0x50})
	r.plateShadow.CornerRadius = 10
	r.plate = img(texPlate, canvas.ImageFillStretch)
	r.capTop = img(texCap, canvas.ImageFillStretch)
	r.capBot = img(texCap, canvas.ImageFillStretch)
	return r
}

type ringsRenderer struct {
	b              *binderRings
	plateShadow    *canvas.Rectangle
	plate          *canvas.Image
	capTop, capBot *canvas.Image
	rings          []fyne.CanvasObject
	lastH          float32
}

func (r *ringsRenderer) Layout(s fyne.Size) {
	pw := float32(40)
	x := (s.Width - pw) / 2
	r.plateShadow.Move(fyne.NewPos(x+3, 14))
	r.plateShadow.Resize(fyne.NewSize(pw, s.Height-24))
	r.plate.Move(fyne.NewPos(x, 10))
	r.plate.Resize(fyne.NewSize(pw, s.Height-20))
	r.capTop.Move(fyne.NewPos(x-8, -4))
	r.capTop.Resize(fyne.NewSize(pw+16, 34))
	r.capBot.Move(fyne.NewPos(x-8, s.Height-30))
	r.capBot.Resize(fyne.NewSize(pw+16, 34))
	if s.Height == r.lastH {
		return
	}
	r.lastH = s.Height
	r.rings = r.rings[:0]
	n := int((s.Height - 120) / 105)
	if n > 6 {
		n = 6
	}
	if n < 2 {
		n = 2
	}
	gap := (s.Height - 150) / float32(n-1)
	for i := 0; i < n; i++ {
		ring := img(texRing, canvas.ImageFillOriginal) // drawn at its real size: no stretching
		ring.Move(fyne.NewPos((s.Width-ringW)/2, 75+gap*float32(i)-ringH/2))
		ring.Resize(fyne.NewSize(ringW, ringH))
		r.rings = append(r.rings, ring)
	}
}
func (r *ringsRenderer) MinSize() fyne.Size { return fyne.NewSize(56, 240) }
func (r *ringsRenderer) Refresh()           { r.lastH = -1; r.Layout(r.b.Size()); canvas.Refresh(r.b) }
func (r *ringsRenderer) Objects() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{r.plateShadow, r.plate, r.capTop, r.capBot}, r.rings...)
}
func (r *ringsRenderer) Destroy() {}

// a ring's size: just wider than the gutter, so it dips into each page's punched hole
const ringW, ringH = 104, 30

// ---------- coloured index tabs (lists and groups), out of the left page ----------

// stackFill spreads its items evenly down the full height.
type stackFill struct{}

func (s *stackFill) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(30, float32(len(objs))*22)
}
func (s *stackFill) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	if len(objs) == 0 {
		return
	}
	h := size.Height / float32(len(objs))
	for i, o := range objs {
		o.Move(fyne.NewPos(0, float32(i)*h))
		o.Resize(fyne.NewSize(size.Width, h))
	}
}

// fixedWidth gives content a set width.
type fixedWidth struct{ w float32 }

func (f *fixedWidth) MinSize(objs []fyne.CanvasObject) fyne.Size {
	h := float32(0)
	for _, o := range objs {
		h = fyne.Max(h, o.MinSize().Height)
	}
	return fyne.NewSize(f.w, h)
}
func (f *fixedWidth) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
}

// ---------- assembling the book ----------

// organiserLayout places everything on the cover: index tabs | left page | rings | right page | A–Z.
type organiserLayout struct {
	cover                        *leatherCover
	tabs, left, rings, right, az fyne.CanvasObject
	status, strap                fyne.CanvasObject
	pen                          *hotspot
	prev, next                   *pageCorner
	pocket                       *cardPocket
	importer                     *importFunnel
	pagesPos                     fyne.Position
	pagesSize                    fyne.Size
}

func (l *organiserLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(900, 560) }

func (l *organiserLayout) Layout(_ []fyne.CanvasObject, s fyne.Size) {
	m := float32(coverMargin)
	top, bottom := m+22, s.Height-m-42 // room under the pages for the gilded status line
	h := bottom - top
	penW := float32(64)
	tabsW := float32(165)
	if s.Width < 1180 { // a smaller screen: slimmer cover parts so the pages keep their width
		penW, tabsW = 50, 132
	}
	azW := float32(30)
	x := m + penW
	pocketH, importH := float32(0), float32(0)
	if l.pocket != nil {
		pocketH = 104
	}
	if l.importer != nil {
		importH = 92
	}
	l.tabs.Move(fyne.NewPos(x, top+pageBar+2))
	l.tabs.Resize(fyne.NewSize(tabsW, h-pageBar-10-pocketH-importH))
	if l.pocket != nil { // the business-card pocket, low inside the front cover
		py := bottom - pocketH - importH + 4
		l.pocket.Move(fyne.NewPos(x+6, py))
		l.pocket.Resize(fyne.NewSize(tabsW-14, pocketH-6))
		hs := l.pocket.hint.MinSize()
		l.pocket.hint.Move(fyne.NewPos(x+6, py-hs.Height-6))
		l.pocket.hint.Resize(hs)
	}
	if l.importer != nil { // the import funnel, right under the pocket
		l.importer.Move(fyne.NewPos(x+6, bottom-importH+2))
		l.importer.Resize(fyne.NewSize(tabsW-14, importH-4))
	}
	x += tabsW
	pagesRight := s.Width - m - 22 - 58 - azW // room for the A–Z tabs and the strap
	total := pagesRight - x
	ringsW := float32(56)
	leftW := float32(math.Max(260, math.Min(400, float64(total)*0.42)))
	l.left.Move(fyne.NewPos(x, top))
	l.left.Resize(fyne.NewSize(leftW, h))
	l.rings.Move(fyne.NewPos(x+leftW, top-14))
	l.rings.Resize(fyne.NewSize(ringsW, h+28))
	rx := x + leftW + ringsW
	l.right.Move(fyne.NewPos(rx, top))
	l.right.Resize(fyne.NewSize(pagesRight-rx, h))
	l.az.Move(fyne.NewPos(pagesRight, top+14))
	l.az.Resize(fyne.NewSize(azW, h-28))
	if l.status != nil { // the status line, small, on the leather under the pages
		l.status.Move(fyne.NewPos(x-6, bottom+6))
		l.status.Resize(fyne.NewSize(pagesRight-x, 24))
	}
	l.pagesPos = fyne.NewPos(x, top)
	l.pagesSize = fyne.NewSize(pagesRight-x, h)
	if l.prev != nil { // turned-up corners: bottom-left of the left page, bottom-right of the right
		cs := float32(cornerBig)
		l.prev.Move(fyne.NewPos(x, bottom-cs))
		l.prev.Resize(fyne.NewSize(cs, cs))
		l.next.Move(fyne.NewPos(pagesRight-cs, bottom-cs))
		l.next.Resize(fyne.NewSize(cs, cs))
		ph := l.prev.hint.MinSize()
		l.prev.hint.Move(fyne.NewPos(x+cs+4, bottom-ph.Height-4))
		l.prev.hint.Resize(ph)
		nh := l.next.hint.MinSize()
		l.next.hint.Move(fyne.NewPos(pagesRight-cs-nh.Width-4, bottom-nh.Height-4))
		l.next.hint.Resize(nh)
	}
	if l.pen != nil { // over the drawn pen (same sums as the cover's)
		ch := s.Height - 2*m - 2
		px, py := m+18, m+ch*0.14
		l.pen.Move(fyne.NewPos(px, py))
		l.pen.Resize(fyne.NewSize(38, ch*0.72))
		hs := l.pen.hint.MinSize()
		l.pen.hint.Move(fyne.NewPos(px+44, py+10))
		l.pen.hint.Resize(hs)
	}
	l.cover.Move(fyne.NewPos(0, 0))
	l.cover.Resize(s)
	if l.strap != nil { // over the drawn strap (same sums as the cover's)
		cw, ch := s.Width-2*m-26, s.Height-2*m-2
		sw, sh := float32(86), float32(118)
		l.strap.Move(fyne.NewPos(m+cw-50, m+ch/2-sh/2))
		l.strap.Resize(fyne.NewSize(sw, sh))
		if h, ok := l.strap.(*strapHit); ok {
			hs := h.hint.MinSize()
			h.hint.Move(fyne.NewPos(m+cw-58-hs.Width, m+ch/2-hs.Height/2))
			h.hint.Resize(hs)
		}
	}
}

// filofaxLayout builds the open organiser around the list and the card.
func (u *ui) filofaxLayout(listPane fyne.CanvasObject) fyne.CanvasObject {
	loadTextures()
	tabs := u.filofaxTabs()
	u.leftPage = newPaperPage(listPane, false, false)
	u.rightPage = newPaperPage(u.right, true, false)
	rings := newBinderRings()
	az := u.letterStrip()
	status := newStatusLabel(container.NewThemeOverride(u.status, &inkTextTheme{&filofaxTheme{newAppTheme()}}))
	u.statusPlate = status
	strap := newStrapHit(u)
	lay := &organiserLayout{tabs: tabs, left: u.leftPage, rings: rings, right: u.rightPage, az: az, status: status, strap: strap}
	cover := &leatherCover{pageArea: func() (fyne.Position, fyne.Size) { return lay.pagesPos, lay.pagesSize }}
	cover.ExtendBaseWidget(cover)
	lay.cover = cover
	strap.cover = cover
	lay.pen = newHotspot("Write a new card", func() { u.editContact(nil) }, func(on bool) { cover.penUp = on; cover.lift() })
	lay.prev = newPageCorner(true, "Previous letter", func() { u.turnLetter(-1) })
	lay.next = newPageCorner(false, "Next letter", func() { u.turnLetter(+1) })
	lay.pocket = newCardPocket(u)
	lay.importer = newImportFunnel(u.importHub)
	// order matters: the A–Z and index tabs sit under the page edges, the rings over them
	return container.New(lay, cover, tabs, az, u.leftPage, u.rightPage, rings, status, lay.pocket, lay.importer,
		lay.prev, lay.next, lay.pen, strap, strap.hint, lay.pen.hint, lay.prev.hint, lay.next.hint, lay.pocket.hint)
}

// toggleLook switches between the Filofax organiser and the classic Mac panes.
func (u *ui) toggleLook() {
	u.leaveEditor(func() {
		u.cfg.ClassicLook = !u.cfg.ClassicLook
		_ = u.cfg.Save(u.store.Dir())
		u.ed = nil
		u.leftPage, u.rightPage = nil, nil
		u.build()
		u.refreshAll()
	}, nil)
}

// ---------- the closed organiser ----------

// leatherTextTheme: light tan writing for text placed on the leather (the status line).
// inkTextTheme: black print, for words on a paper label.
type inkTextTheme struct{ fyne.Theme }

func (t *inkTextTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if n == theme.ColorNameForeground || n == theme.ColorNameDisabled {
		return color.NRGBA{0x14, 0x10, 0x0C, 0xFF}
	}
	return t.Theme.Color(n, v)
}

// statusLabel is the status line printed in black on a paper label stuck to the leather under
// the pages, as wide as its words.
type statusLabel struct {
	widget.BaseWidget
	text fyne.CanvasObject
}

func newStatusLabel(text fyne.CanvasObject) *statusLabel {
	l := &statusLabel{text: text}
	l.ExtendBaseWidget(l)
	return l
}

func (l *statusLabel) CreateRenderer() fyne.WidgetRenderer {
	shadow := canvas.NewRectangle(color.NRGBA{0x10, 0x05, 0x00, 0x60})
	shadow.CornerRadius = 3
	paper := canvas.NewRectangle(color.NRGBA{0xFB, 0xF7, 0xEC, 0xFF})
	paper.CornerRadius = 3
	paper.StrokeColor = color.NRGBA{0xC8, 0xBC, 0xA4, 0xFF}
	paper.StrokeWidth = 1
	return &statusRenderer{l: l, shadow: shadow, paper: paper}
}

type statusRenderer struct {
	l             *statusLabel
	shadow, paper *canvas.Rectangle
}

func (r *statusRenderer) Layout(s fyne.Size) {
	m := r.l.text.MinSize()
	w := fyne.Min(m.Width+8, s.Width)
	r.paper.Resize(fyne.NewSize(w, s.Height))
	r.shadow.Move(fyne.NewPos(1, 2))
	r.shadow.Resize(fyne.NewSize(w, s.Height))
	r.l.text.Move(fyne.NewPos(4, (s.Height-m.Height)/2))
	r.l.text.Resize(fyne.NewSize(w-8, m.Height))
}
func (r *statusRenderer) MinSize() fyne.Size { return fyne.NewSize(10, 24) }
func (r *statusRenderer) Refresh()           { r.Layout(r.l.Size()); canvas.Refresh(r.l) }
func (r *statusRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.shadow, r.paper, r.l.text}
}
func (r *statusRenderer) Destroy() {}

// fitStatus resizes the status line's paper label to its words.
func (u *ui) fitStatus() {
	if u.statusPlate != nil {
		u.statusPlate.Refresh()
	}
}

type leatherTextTheme struct{ fyne.Theme }

func (t *leatherTextTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if n == theme.ColorNameForeground || n == theme.ColorNameDisabled {
		return color.NRGBA{0xE6, 0xC2, 0x72, 0xF0} // gilded, like the title on the cover
	}
	return t.Theme.Color(n, v)
}

// closedSize is the window size that fits the closed book for a given height.
// windowGrey is what shows around the book: dark grey, like a desk in a dim room, instead of white.
var windowGrey = deskColor

// onGrey puts o on the dark grey desk: a faint felt texture, so the book sits on something.
func onGrey(o fyne.CanvasObject) fyne.CanvasObject {
	felt := canvas.NewImageFromImage(feltImage())
	felt.FillMode = canvas.ImageFillStretch
	return container.NewStack(canvas.NewRectangle(windowGrey), felt, o)
}

// sizeWindow changes the window size. On the Mac it animates, keeps the right edge (the cover
// swings open to the left) and keeps the whole window on screen; elsewhere it just resizes.
func (u *ui) sizeWindow(sz fyne.Size, animate bool) {
	if fitWindow(u.win, sz, animate) {
		return
	}
	u.win.Resize(sz)
	go func() { // some window managers apply a resize only after the new content is laid out
		time.Sleep(150 * time.Millisecond)
		fyne.Do(func() { u.win.Resize(sz) })
	}()
}
