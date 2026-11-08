package main

import (
	_ "embed"
	"image"
	"image/color"
	"math"
	"math/rand"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
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
	deskColor   = color.NRGBA{0xEE, 0xEC, 0xE8, 0xFF}
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

type valueNoise struct{ g [256][256]float64 }

func newNoise(seed int64) *valueNoise {
	r := rand.New(rand.NewSource(seed))
	n := &valueNoise{}
	for y := range n.g {
		for x := range n.g[y] {
			n.g[y][x] = r.Float64()
		}
	}
	return n
}

func (n *valueNoise) at(x, y float64) float64 {
	x0, y0 := math.Floor(x), math.Floor(y)
	fx, fy := x-x0, y-y0
	fx, fy = fx*fx*(3-2*fx), fy*fy*(3-2*fy)
	i, j := int(x0)&255, int(y0)&255
	a, b := n.g[j][i], n.g[j][(i+1)&255]
	c, d := n.g[(j+1)&255][i], n.g[(j+1)&255][(i+1)&255]
	return a + (b-a)*fx + (c-a)*fy + (a-b-c+d)*fx*fy
}

func (n *valueNoise) fbm(x, y float64, oct int) float64 {
	s, amp, f, tot := 0.0, 0.5, 1.0, 0.0
	for i := 0; i < oct; i++ {
		s += amp * n.at(x*f, y*f)
		tot += amp
		amp *= 0.5
		f *= 2.03
	}
	return s / tot
}

func clamp8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// leatherImage is cognac calf: fine grain, darker creases, uneven burnished patches.
func leatherImage(w, h int, base [3]float64, seed int64) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	n := newNoise(seed)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x), float64(y)
			grain := n.fbm(fx/1.8, fy/1.8, 2)
			crease := math.Abs(n.fbm(fx/14, fy/5, 3)-0.5) * 2
			burnish := n.fbm(fx/120, fy/90, 4)
			v := 0.84 + 0.12*grain - 0.10*(1-crease)*(1-crease)*(1-crease) + 0.34*(burnish-0.5)
			i := y*img.Stride + x*4
			img.Pix[i+0] = clamp8(base[0] * v * 1.02)
			img.Pix[i+1] = clamp8(base[1] * v)
			img.Pix[i+2] = clamp8(base[2] * v * 0.96)
			img.Pix[i+3] = 255
		}
	}
	return img
}

// paperGrain is a faint transparent tooth over the white pages.
func paperGrain(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	n := newNoise(11)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := n.fbm(float64(x)/1.6, float64(y)/1.6, 2)
			i := y*img.Stride + x*4
			img.Pix[i], img.Pix[i+1], img.Pix[i+2] = 0x80, 0x78, 0x68
			img.Pix[i+3] = clamp8(6 + 18*math.Abs(v-0.5))
		}
	}
	return img
}

// chrome shades a cylinder across t (0..1): dark edges, bright band, soft reflection.
func chrome(t float64) float64 {
	v := 95 + 120*math.Sin(t*math.Pi)
	v += 110 * math.Exp(-math.Pow((t-0.32)/0.08, 2))
	v += 45 * math.Exp(-math.Pow((t-0.72)/0.1, 2))
	return v
}

// ringImage is one binder ring from above: a chrome loop crossing the spine into both pages.
func ringImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	cx, cy := float64(w)/2, float64(h)/2
	ax, ay := float64(w)/2-2, float64(h)/2-2
	thick := 0.30 // tube thickness as part of the radius
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := (float64(x)-cx)/ax, (float64(y)-cy)/ay
			d := math.Sqrt(dx*dx + dy*dy)
			i := y*img.Stride + x*4
			if d > 1 || d < 1-thick {
				// shadow on the paper just below the ring
				sd := math.Sqrt(dx*dx + (dy-0.18)*(dy-0.18))
				if sd <= 1.04 && sd >= 1-thick-0.04 && d > 1 {
					img.Pix[i+3] = 60
				}
				continue
			}
			t := (d - (1 - thick)) / thick // across the tube
			v := chrome(t)
			if dy > 0 {
				v *= 0.86 // the lower half is in shade
			}
			edge := math.Min(1, math.Min(t, 1-t)*10)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2] = clamp8(v), clamp8(v), clamp8(v*1.02)
			img.Pix[i+3] = clamp8(255 * edge)
		}
	}
	return img
}

// penImage is a chrome ballpoint seen from above, pointing down.
func penImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	body := float64(w) * 0.62
	for y := 0; y < h; y++ {
		fy := float64(y) / float64(h)
		half := body / 2
		switch {
		case fy < 0.03: // rounded top
			half *= 0.8
		case fy > 0.9: // taper to the tip
			half *= math.Max(0.08, (1-fy)/0.1)
		}
		for x := 0; x < w; x++ {
			dx := float64(x) - float64(w)/2 + 4
			i := y*img.Stride + x*4
			if math.Abs(dx) > half {
				continue
			}
			t := (dx + half) / (2 * half)
			v := chrome(t)
			if fy > 0.24 && fy < 0.245 || fy > 0.6 && fy < 0.605 { // rings on the barrel
				v *= 0.55
			}
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = clamp8(v), clamp8(v), clamp8(v*1.03), 255
		}
		// the clip
		if fy > 0.02 && fy < 0.36 {
			cx := float64(w)/2 + half - 1
			for x := int(cx); x < int(cx)+5 && x < w; x++ {
				t := float64(x-int(cx)) / 5
				v := chrome(t) * 0.95
				i := y*img.Stride + x*4
				img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = clamp8(v), clamp8(v), clamp8(v), 255
			}
		}
	}
	return img
}

// brassImage is the snap's domed brass stud.
func brassImage(d int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, d, d))
	c := float64(d) / 2
	for y := 0; y < d; y++ {
		for x := 0; x < d; x++ {
			dx, dy := (float64(x)-c)/c, (float64(y)-c)/c
			r := math.Sqrt(dx*dx + dy*dy)
			if r > 1 {
				continue
			}
			light := 0.55 + 0.45*(-dx*0.5-dy*0.7)*(1-r*0.5)
			spec := math.Exp(-((dx+0.35)*(dx+0.35)+(dy+0.4)*(dy+0.4))/0.03) * 120
			i := y*img.Stride + x*4
			img.Pix[i] = clamp8(200*light + spec)
			img.Pix[i+1] = clamp8(158*light + spec)
			img.Pix[i+2] = clamp8(70*light + spec*0.6)
			img.Pix[i+3] = clamp8(255 * math.Min(1, (1-r)*c))
		}
	}
	return img
}

var texCover, texLip, texGrain, texRing, texPen, texBrass image.Image

func loadTextures() {
	if texCover == nil {
		texCover = leatherImage(1000, 720, [3]float64{0x86, 0x45, 0x22}, 7)
		texLip = leatherImage(600, 600, [3]float64{0x5A, 0x2C, 0x16}, 9)
		texGrain = paperGrain(400, 560)
		texRing = ringImage(96, 30)
		texPen = penImage(30, 560)
		texBrass = brassImage(44)
	}
}

func img(i image.Image, mode canvas.ImageFill) *canvas.Image {
	c := canvas.NewImageFromImage(i)
	c.FillMode = mode
	return c
}

// ---------- the leather cover ----------

// leatherCover draws the open cover: rounded leather with an inner darker lip where the pages
// sit, stitching round both, a pen loop with the pen on the left and the strap on the right.
type leatherCover struct {
	widget.BaseWidget
	pageArea func() (fyne.Position, fyne.Size) // where the pages are, to put the lip under them
}

func (l *leatherCover) CreateRenderer() fyne.WidgetRenderer {
	loadTextures()
	r := &coverRenderer{l: l}
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
}

const coverMargin = 12

func (r *coverRenderer) Layout(s fyne.Size) {
	r.desk.Resize(s)
	m := float32(coverMargin)
	cw, ch := s.Width-2*m-22, s.Height-2*m // leaves room for the strap on the right
	r.shadow.Move(fyne.NewPos(m+4, m+7))
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
	r.pen.Move(fyne.NewPos(lx+4, m+ch*0.14))
	r.pen.Resize(fyne.NewSize(30, ch*0.72))
	r.loop.Move(fyne.NewPos(lx-6, m+ch*0.33))
	r.loop.Resize(fyne.NewSize(48, 56))
	r.loopShade.Move(r.loop.Position())
	r.loopShade.Resize(r.loop.Size())
	// strap with the brass snap, over the right edge
	sw, sh := float32(86), float32(118)
	sx, sy := m+cw-50, m+ch/2-sh/2
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

// paperPage is one white page with a grey title bar, on a small stack of page edges, with the
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
	p.title = canvas.NewText("", color.NRGBA{0x4A, 0x4E, 0x56, 0xFF})
	p.title.TextStyle = serif("", true, false)
	p.title.TextSize = 13
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
	r.bar = canvas.NewHorizontalGradient(color.NRGBA{0xC4, 0xC8, 0xCE, 0xFF}, color.NRGBA{0xD6, 0xD9, 0xDD, 0xFF})
	r.barShape = canvas.NewRectangle(color.NRGBA{0xCB, 0xCF, 0xD4, 0xFF})
	r.barShape.CornerRadius = 7
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
	r.barShape.Move(fyne.NewPos(inL, 12))
	r.barShape.Resize(fyne.NewSize(s.Width-inL-inR, 22))
	r.bar.Move(fyne.NewPos(inL+2, 13))
	r.bar.Resize(fyne.NewSize(s.Width-inL-inR-4, 20))
	r.p.title.Move(fyne.NewPos(inL+12, 12+(22-r.p.title.MinSize().Height)/2))
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
	r.plate = canvas.NewHorizontalGradient(color.NRGBA{0x88, 0x8C, 0x92, 0xFF}, color.NRGBA{0xE8, 0xEA, 0xEE, 0xFF})
	r.plate2 = canvas.NewHorizontalGradient(color.NRGBA{0xF2, 0xF3, 0xF5, 0xFF}, color.NRGBA{0x7A, 0x7E, 0x84, 0xFF})
	r.capTop = canvas.NewRadialGradient(color.NRGBA{0xF6, 0xF7, 0xF9, 0xFF}, color.NRGBA{0x80, 0x84, 0x8A, 0xFF})
	r.capBot = canvas.NewRadialGradient(color.NRGBA{0xF6, 0xF7, 0xF9, 0xFF}, color.NRGBA{0x80, 0x84, 0x8A, 0xFF})
	return r
}

type ringsRenderer struct {
	b              *binderRings
	plate, plate2  *canvas.LinearGradient
	capTop, capBot *canvas.RadialGradient
	rings          []fyne.CanvasObject
	lastH          float32
}

func (r *ringsRenderer) Layout(s fyne.Size) {
	pw := float32(22)
	x := (s.Width - pw) / 2
	r.plate.Move(fyne.NewPos(x, 8))
	r.plate.Resize(fyne.NewSize(pw/2, s.Height-16))
	r.plate2.Move(fyne.NewPos(x+pw/2, 8))
	r.plate2.Resize(fyne.NewSize(pw/2, s.Height-16))
	r.capTop.Move(fyne.NewPos(x-6, -6))
	r.capTop.Resize(fyne.NewSize(pw+12, 26))
	r.capBot.Move(fyne.NewPos(x-6, s.Height-20))
	r.capBot.Resize(fyne.NewSize(pw+12, 26))
	if s.Height == r.lastH {
		return
	}
	r.lastH = s.Height
	r.rings = r.rings[:0]
	n := int((s.Height - 60) / 110)
	if n > 6 {
		n = 6
	}
	if n < 2 {
		n = 2
	}
	gap := (s.Height - 60) / float32(n-1)
	for i := 0; i < n; i++ {
		ring := img(texRing, canvas.ImageFillStretch)
		w := s.Width + 56 // reaches into the punched holes of both pages
		ring.Move(fyne.NewPos(-28, 30+gap*float32(i)-15))
		ring.Resize(fyne.NewSize(w, 30))
		r.rings = append(r.rings, ring)
	}
}
func (r *ringsRenderer) MinSize() fyne.Size { return fyne.NewSize(44, 200) }
func (r *ringsRenderer) Refresh()           { r.lastH = -1; r.Layout(r.b.Size()); canvas.Refresh(r.b) }
func (r *ringsRenderer) Objects() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{r.plate, r.plate2, r.capTop, r.capBot}, r.rings...)
}
func (r *ringsRenderer) Destroy() {}

// ---------- coloured index tabs (lists and groups), out of the left page ----------

type indexTab struct {
	widget.BaseWidget
	title, count string
	col          color.NRGBA
	selected     bool
	header       bool
	onTap        func()
	hover        bool
}

func (t *indexTab) Tapped(*fyne.PointEvent) {
	if t.onTap != nil && !t.header {
		t.onTap()
	}
}
func (t *indexTab) Cursor() desktop.Cursor {
	if t.header {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}
func (t *indexTab) MouseIn(*desktop.MouseEvent)    { t.hover = true; t.Refresh() }
func (t *indexTab) MouseMoved(*desktop.MouseEvent) {}
func (t *indexTab) MouseOut()                      { t.hover = false; t.Refresh() }

func (t *indexTab) CreateRenderer() fyne.WidgetRenderer {
	shadow := canvas.NewRectangle(color.NRGBA{0x20, 0x08, 0x00, 0x60})
	shadow.CornerRadius = 6
	card := canvas.NewRectangle(t.col)
	card.CornerRadius = 6
	sheen := canvas.NewVerticalGradient(color.NRGBA{0xFF, 0xFF, 0xFF, 0x48}, color.NRGBA{0, 0, 0, 0x1C})
	title := canvas.NewText(t.title, color.White)
	title.TextStyle = serif(t.title, true, false)
	title.TextSize = 12
	count := canvas.NewText(t.count, color.NRGBA{0xFF, 0xFF, 0xFF, 0xD8})
	count.TextSize = 11
	return &tabRenderer{t: t, shadow: shadow, card: card, sheen: sheen, title: title, count: count}
}

type tabRenderer struct {
	t            *indexTab
	shadow, card *canvas.Rectangle
	sheen        *canvas.LinearGradient
	title, count *canvas.Text
}

func (r *tabRenderer) MinSize() fyne.Size {
	if r.t.header {
		return fyne.NewSize(120, 20)
	}
	return fyne.NewSize(150, 31)
}

func (r *tabRenderer) Layout(s fyne.Size) {
	if r.t.header {
		r.title.Move(fyne.NewPos(14, 4))
		return
	}
	// tabs come out of the page to the left; the chosen one sticks out furthest
	out := float32(14)
	if r.t.selected {
		out = 0
	} else if r.t.hover {
		out = 7
	}
	w := s.Width - out + 10 // tucked under the page edge
	r.shadow.Move(fyne.NewPos(out+1, 3))
	r.shadow.Resize(fyne.NewSize(w, s.Height-4))
	r.card.Move(fyne.NewPos(out, 1))
	r.card.Resize(fyne.NewSize(w, s.Height-4))
	r.sheen.Move(fyne.NewPos(out, 1))
	r.sheen.Resize(fyne.NewSize(w, (s.Height-4)/2))
	r.title.Move(fyne.NewPos(out+9, (s.Height-4-r.title.MinSize().Height)/2+1))
	cs := r.count.MinSize()
	r.count.Move(fyne.NewPos(s.Width-cs.Width-16, (s.Height-4-cs.Height)/2+1)) // clear of the page edge
}

func (r *tabRenderer) Refresh() {
	r.card.FillColor = r.t.col
	r.title.Text, r.count.Text = r.t.title, r.t.count
	if r.t.header {
		r.title.Color = color.NRGBA{0xF0, 0xD8, 0xB8, 0xD0}
		r.title.TextSize = 10
	}
	r.Layout(r.t.Size())
	canvas.Refresh(r.t)
}

func (r *tabRenderer) Objects() []fyne.CanvasObject {
	if r.t.header {
		return []fyne.CanvasObject{r.title}
	}
	return []fyne.CanvasObject{r.shadow, r.card, r.sheen, r.title, r.count}
}
func (r *tabRenderer) Destroy() {}

func (u *ui) filofaxTabs() fyne.CanvasObject {
	u.tabBox = container.NewVBox()
	return container.NewVScroll(u.tabBox)
}

func (u *ui) refreshTabs() {
	if u.tabBox == nil {
		return
	}
	u.tabBox.Objects = nil
	ci := 0
	for _, it := range u.sideItems {
		it := it
		t := &indexTab{title: it.title}
		if it.key == grpHeader {
			t.header = true
			t.title = strings.ToUpper(it.title)
		} else {
			t.col = tabColors[ci%len(tabColors)]
			ci++
			if it.count > 0 {
				t.count = itoa(it.count)
			}
			t.selected = it.key == u.group
			t.onTap = func() {
				u.group = it.key
				u.cfg.LastGroup = it.key
				u.refreshTabs()
				u.refreshList()
			}
		}
		t.ExtendBaseWidget(t)
		u.tabBox.Add(t)
	}
	u.tabBox.Refresh()
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// ---------- A–Z tabs down the right edge ----------

// letterOf is the index letter a contact files under: A–Z, ع for Arabic names, # for the rest.
func letterOf(c *Contact, byLast bool) string {
	name := strings.TrimSpace(c.First)
	if byLast && strings.TrimSpace(c.Last) != "" {
		name = c.Last
	}
	if name == "" {
		name = c.DisplayName()
	}
	low := strings.ToLower(name) // "Al-Mutairi" files under M, as in a paper address book
	for _, p := range []string{"al-", "al ", "el-", "ال"} {
		if byLast && strings.HasPrefix(low, p) && len([]rune(name)) > len([]rune(p))+1 {
			name = string([]rune(name)[len([]rune(p)):])
			break
		}
	}
	for _, r := range fold(name) {
		switch {
		case r >= 'a' && r <= 'z':
			return string(unicode.ToUpper(r))
		case unicode.Is(unicode.Arabic, r):
			return "ع"
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			return "#"
		}
	}
	return "#"
}

// The tabs pair letters, as on organiser dividers.
var letterTabs = []string{"All", "AB", "CD", "EF", "GH", "IJ", "KL", "MN", "OP", "QR", "ST", "UV", "WX", "YZ", "ع", "#"}

// letterIn reports whether a contact's letter belongs to the chosen tab.
func letterIn(tab, l string) bool {
	if tab == "" || tab == "All" {
		return true
	}
	if tab == "ع" || tab == "#" {
		return l == tab
	}
	return strings.Contains(tab, l)
}

type letterTab struct {
	widget.BaseWidget
	letters  string
	selected bool
	empty    bool
	onTap    func()
}

func (t *letterTab) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}
func (t *letterTab) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (t *letterTab) CreateRenderer() fyne.WidgetRenderer {
	shadow := canvas.NewRectangle(color.NRGBA{0x20, 0x10, 0x00, 0x50})
	shadow.CornerRadius = 5
	bg := canvas.NewRectangle(color.NRGBA{0xEF, 0xEB, 0xDF, 0xFF})
	bg.CornerRadius = 5
	bg.StrokeColor = color.NRGBA{0xC9, 0xC2, 0xB0, 0xFF}
	bg.StrokeWidth = 1
	r := &letterRenderer{t: t, bg: bg, shadow: shadow}
	parts := []string{t.letters}
	if rs := []rune(t.letters); len(rs) == 2 { // "AB" shows A over B
		parts = []string{string(rs[0]), string(rs[1])}
	}
	for _, p := range parts {
		x := canvas.NewText(p, inkColor)
		x.TextStyle = serif(p, true, false)
		x.TextSize = 12
		if p == "All" {
			x.TextSize = 9
		}
		x.Alignment = fyne.TextAlignCenter
		r.txt = append(r.txt, x)
	}
	return r
}

type letterRenderer struct {
	t          *letterTab
	bg, shadow *canvas.Rectangle
	txt        []*canvas.Text
}

func (r *letterRenderer) MinSize() fyne.Size { return fyne.NewSize(30, 24) }
func (r *letterRenderer) Layout(s fyne.Size) {
	out := float32(5) // tabs come out of the right page; the chosen one further
	if r.t.selected {
		out = 0
	}
	w := s.Width - out
	r.shadow.Move(fyne.NewPos(-8, 2))
	r.shadow.Resize(fyne.NewSize(w+9, s.Height-3))
	r.bg.Move(fyne.NewPos(-10, 1))
	r.bg.Resize(fyne.NewSize(w+10, s.Height-3))
	lh := float32(0)
	for _, t := range r.txt {
		lh += t.MinSize().Height - 3
	}
	y := (s.Height - lh) / 2
	for _, t := range r.txt {
		t.Move(fyne.NewPos(0, y-1))
		t.Resize(fyne.NewSize(w, t.MinSize().Height))
		y += t.MinSize().Height - 3
	}
}
func (r *letterRenderer) Refresh() {
	c := inkColor
	switch {
	case r.t.selected:
		r.bg.FillColor = paperColor
		c = color.NRGBA{0x9A, 0x2A, 0x22, 0xFF}
	case r.t.empty:
		r.bg.FillColor = color.NRGBA{0xE2, 0xDC, 0xCC, 0xFF}
		c = color.NRGBA{0xA8, 0xA2, 0x94, 0xFF}
	default:
		r.bg.FillColor = color.NRGBA{0xEF, 0xEB, 0xDF, 0xFF}
	}
	for _, t := range r.txt {
		t.Color = c
	}
	r.Layout(r.t.Size())
	canvas.Refresh(r.t)
}
func (r *letterRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.shadow, r.bg}
	for _, t := range r.txt {
		objs = append(objs, t)
	}
	return objs
}
func (r *letterRenderer) Destroy() {}

func (u *ui) letterStrip() fyne.CanvasObject {
	u.letterBox = container.New(&stackFill{})
	return u.letterBox
}

// refreshLetters marks the chosen tab and greys tabs nobody files under.
func (u *ui) refreshLetters(used map[string]bool) {
	if u.letterBox == nil {
		return
	}
	u.letterBox.Objects = nil
	for _, l := range letterTabs {
		l := l
		any := l == "All"
		for k := range used {
			if l != "All" && letterIn(l, k) {
				any = true
			}
		}
		t := &letterTab{letters: l, selected: l == u.letter || (l == "All" && u.letter == ""), empty: !any}
		t.onTap = func() {
			if l == "All" {
				u.letter = ""
			} else {
				u.letter = l
			}
			u.refreshList()
		}
		t.ExtendBaseWidget(t)
		u.letterBox.Add(t)
	}
	u.letterBox.Refresh()
}

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
	pagesPos                     fyne.Position
	pagesSize                    fyne.Size
}

func (l *organiserLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(1000, 560) }

func (l *organiserLayout) Layout(_ []fyne.CanvasObject, s fyne.Size) {
	m := float32(coverMargin)
	top, bottom := m+30, s.Height-m-26
	h := bottom - top
	penW := float32(70)
	tabsW := float32(165)
	azW := float32(30)
	x := m + penW
	l.tabs.Move(fyne.NewPos(x, top+pageBar+2))
	l.tabs.Resize(fyne.NewSize(tabsW, h-pageBar-10))
	x += tabsW
	pagesRight := s.Width - m - 22 - 58 - azW // room for the A–Z tabs and the strap
	total := pagesRight - x
	ringsW := float32(46)
	leftW := float32(math.Max(300, math.Min(400, float64(total)*0.4)))
	l.left.Move(fyne.NewPos(x, top))
	l.left.Resize(fyne.NewSize(leftW, h))
	l.rings.Move(fyne.NewPos(x+leftW, top-10))
	l.rings.Resize(fyne.NewSize(ringsW, h+20))
	rx := x + leftW + ringsW
	l.right.Move(fyne.NewPos(rx, top))
	l.right.Resize(fyne.NewSize(pagesRight-rx, h))
	l.az.Move(fyne.NewPos(pagesRight, top+14))
	l.az.Resize(fyne.NewSize(azW, h-28))
	l.pagesPos = fyne.NewPos(x, top)
	l.pagesSize = fyne.NewSize(pagesRight-x, h)
	l.cover.Move(fyne.NewPos(0, 0))
	l.cover.Resize(s)
}

// filofaxLayout builds the open organiser around the list and the card.
func (u *ui) filofaxLayout(listPane fyne.CanvasObject) fyne.CanvasObject {
	loadTextures()
	tabs := u.filofaxTabs()
	u.leftPage = newPaperPage(listPane, false, false)
	u.rightPage = newPaperPage(u.right, true, false)
	rings := newBinderRings()
	az := u.letterStrip()
	lay := &organiserLayout{tabs: tabs, left: u.leftPage, rings: rings, right: u.rightPage, az: az}
	cover := &leatherCover{pageArea: func() (fyne.Position, fyne.Size) { return lay.pagesPos, lay.pagesSize }}
	cover.ExtendBaseWidget(cover)
	lay.cover = cover
	// order matters: the A–Z and index tabs sit under the page edges, the rings over them
	book := container.New(lay, cover, tabs, az, u.leftPage, u.rightPage, rings)
	statusBg := canvas.NewRectangle(deskColor)
	status := container.NewStack(statusBg, container.NewPadded(u.status))
	return container.NewBorder(nil, status, nil, nil, book)
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
	// a closed organiser is about half as wide as an open one
	ch := s.Height - 60
	cw := float32(math.Min(float64(ch)*0.72, float64(s.Width)-140))
	x, y := (s.Width-cw)/2-20, float32(30)
	r.shadow.Move(fyne.NewPos(x+6, y+9))
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
	sw, sh := float32(120), float32(124)
	sx, sy := x+cw-80, y+ch/2-sh/2
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
	u.leaveEditor(func() {
		u.saveWindow() // remembers the page it was left at
		cv := &closedCover{count: len(u.store.Contacts), onOpen: u.openBook}
		cv.ExtendBaseWidget(cv)
		u.coverShown = true
		u.win.SetContent(cv)
	}, nil)
}

// openBook opens the organiser at the last page (or the first, if Settings say so).
func (u *ui) openBook() {
	if !u.coverShown {
		return
	}
	u.coverShown = false
	if u.cfg.OpenAtFirst {
		u.group, u.letter, u.selectedID = grpAll, "", ""
	}
	u.win.SetContent(u.bookContent)
	u.refreshAll()
	if i := u.indexOf(u.selectedID); i >= 0 {
		u.list.ScrollTo(i)
	}
	u.win.Canvas().Focus(u.search)
}

// restorePage puts back the list, A–Z tab and person the organiser was left at.
func (u *ui) restorePage() {
	if u.cfg.OpenAtFirst {
		u.group, u.letter, u.selectedID = grpAll, "", ""
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
