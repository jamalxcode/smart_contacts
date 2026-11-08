package main

import (
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

// The Filofax look: an open leather organiser. Coloured card-stock index tabs on the left (the
// lists and groups), a strip of A–Z tabs, then the pages — a ruled page of names and, past the
// binder rings, the person's page. The textures are drawn by the app itself (no image files):
// grained leather, cream paper with fibres, ruled lines and a red margin, punched holes, and
// metal rings with highlights.

// ---------- colours ----------

var (
	inkColor    = color.NRGBA{0x1F, 0x2A, 0x44, 0xFF} // fountain-pen blue-black
	inkFaint    = color.NRGBA{0x5E, 0x63, 0x70, 0xFF}
	paperColor  = color.NRGBA{0xF7, 0xF1, 0xE1, 0xFF}
	ruleColor   = color.NRGBA{0x9F, 0xBC, 0xD8, 0x90}
	marginColor = color.NRGBA{0xD9, 0x6B, 0x6B, 0xA0}
	tabColors   = []color.NRGBA{ // Filofax divider colours
		{0xC6, 0x3B, 0x34, 0xFF}, {0xE0, 0x7B, 0x2E, 0xFF}, {0xE9, 0xC2, 0x3F, 0xFF}, {0x6B, 0xA3, 0x4F, 0xFF},
		{0x2E, 0x8C, 0x86, 0xFF}, {0x3C, 0x6E, 0xB4, 0xFF}, {0x6E, 0x55, 0xA8, 0xFF}, {0xC0, 0x5A, 0x8E, 0xFF},
	}
)

// filofaxTheme: ink on paper. The organiser is a physical object, so it doesn't turn dark at night.
type filofaxTheme struct{ fyne.Theme }

func (t *filofaxTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	light := theme.VariantLight
	switch n {
	case theme.ColorNameBackground:
		return paperColor
	case theme.ColorNameForeground:
		return inkColor
	case theme.ColorNameDisabled:
		return inkFaint
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{0x8A, 0x86, 0x7C, 0xFF}
	case theme.ColorNameInputBackground:
		return color.NRGBA{0xFF, 0xFC, 0xF2, 0xC0}
	case theme.ColorNameInputBorder:
		return color.NRGBA{0xC9, 0xBE, 0xA4, 0xFF}
	case theme.ColorNameSeparator:
		return color.NRGBA{0xD8, 0xCB, 0xAE, 0xFF}
	case theme.ColorNameButton:
		return color.NRGBA{0xEC, 0xE2, 0xC9, 0xFF}
	case theme.ColorNameHover:
		return color.NRGBA{0x8A, 0x6A, 0x30, 0x18}
	case theme.ColorNameSelection:
		return color.NRGBA{0xF2, 0xD8, 0x6A, 0x80} // highlighter yellow
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground, theme.ColorNameHeaderBackground:
		return color.NRGBA{0xFB, 0xF6, 0xE8, 0xFF}
	case theme.ColorNameShadow:
		return color.NRGBA{0x2A, 0x14, 0x08, 0x50}
	}
	return t.Theme.Color(n, light)
}

// ---------- procedural textures ----------

// valueNoise is smooth noise in 0..1, used for leather grain and paper fibres.
type valueNoise struct {
	g [256][256]float64
}

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

// leatherImage is pebble-grained oxblood leather with a soft vignette.
func leatherImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	n := newNoise(7)
	base := [3]float64{0x6A, 0x24, 0x1E}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x), float64(y)
			pebble := n.fbm(fx/3.1, fy/3.1, 2)              // fine pebbles
			ridge := math.Abs(n.fbm(fx/9, fy/9, 3)-0.5) * 2 // creases between them
			broad := n.fbm(fx/180, fy/180, 3)               // dye variation
			v := 0.78 + 0.22*pebble - 0.18*(1-ridge)*(1-ridge) + 0.16*(broad-0.5)
			dx, dy := (fx/float64(w))-0.5, (fy/float64(h))-0.5
			v *= 1 - 0.55*(dx*dx+dy*dy) // vignette
			img.Pix[y*img.Stride+x*4+0] = clamp8(base[0] * v)
			img.Pix[y*img.Stride+x*4+1] = clamp8(base[1] * v)
			img.Pix[y*img.Stride+x*4+2] = clamp8(base[2] * v)
			img.Pix[y*img.Stride+x*4+3] = 255
		}
	}
	return img
}

// paperGrain is a transparent overlay of fibres and tooth, stretched over a page.
func paperGrain(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	n := newNoise(11)
	r := rand.New(rand.NewSource(3))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := n.fbm(float64(x)/2.2, float64(y)/2.2, 2)
			f := n.fbm(float64(x)/40, float64(y)/7, 2) // long fibres
			a := 10 + 26*math.Abs(v-0.5) + 18*math.Max(0, f-0.62)
			if r.Intn(900) == 0 {
				a += 40 // a fleck
			}
			img.Pix[y*img.Stride+x*4+0] = 0x7A
			img.Pix[y*img.Stride+x*4+1] = 0x60
			img.Pix[y*img.Stride+x*4+2] = 0x38
			img.Pix[y*img.Stride+x*4+3] = clamp8(a)
		}
	}
	return img
}

// ringImage is one binder ring seen from above: a chrome tube with a highlight and shadow.
func ringImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	cx, cy := float64(w)/2, float64(h)/2
	R, tube := float64(w)/2-3, float64(h)/2-2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// an ellipse band
			dx, dy := (float64(x)-cx)/R, (float64(y)-cy)/tube
			d := math.Sqrt(dx*dx + dy*dy)
			if d > 1 {
				// soft shadow below the ring
				if dy > 0 && d < 1.25 {
					img.Pix[y*img.Stride+x*4+3] = clamp8(70 * (1.25 - d) / 0.25)
				}
				continue
			}
			t := (float64(y) - (cy - tube)) / (2 * tube) // 0 top .. 1 bottom
			shade := 0.55 + 0.45*math.Cos(t*math.Pi*1.1-0.4)
			spec := math.Exp(-math.Pow((t-0.28)/0.07, 2)) * 0.9
			v := 120 + 110*shade + 120*spec
			edge := math.Min(1, (1-d)*6)
			img.Pix[y*img.Stride+x*4+0] = clamp8(v)
			img.Pix[y*img.Stride+x*4+1] = clamp8(v * 0.98)
			img.Pix[y*img.Stride+x*4+2] = clamp8(v * 0.95)
			img.Pix[y*img.Stride+x*4+3] = clamp8(255 * edge)
		}
	}
	return img
}

var (
	texLeather, texGrain, texRing image.Image
)

func loadTextures() {
	if texLeather == nil {
		texLeather = leatherImage(900, 640)
		texGrain = paperGrain(420, 600)
		texRing = ringImage(80, 26)
	}
}

// ---------- paper page ----------

// paperPage is a sheet of ruled organiser paper behind content: cream stock with grain, blue
// rules, a red margin, punched holes on the binder side and a drop shadow.
type paperPage struct {
	widget.BaseWidget
	content   fyne.CanvasObject
	holesLeft bool // which side the punched holes are on
	ruled     bool
}

func newPaperPage(content fyne.CanvasObject, holesLeft, ruled bool) *paperPage {
	p := &paperPage{content: content, holesLeft: holesLeft, ruled: ruled}
	p.ExtendBaseWidget(p)
	return p
}

func (p *paperPage) CreateRenderer() fyne.WidgetRenderer {
	loadTextures()
	shadow := canvas.NewRectangle(color.NRGBA{0x10, 0x05, 0x02, 0x70})
	shadow.CornerRadius = 4
	sheet := canvas.NewRectangle(paperColor)
	sheet.CornerRadius = 3
	grain := canvas.NewImageFromImage(texGrain)
	grain.FillMode = canvas.ImageFillStretch
	edge := canvas.NewHorizontalGradient(color.NRGBA{0x80, 0x60, 0x30, 0x30}, color.NRGBA{0, 0, 0, 0})
	if !p.holesLeft {
		edge = canvas.NewHorizontalGradient(color.NRGBA{0, 0, 0, 0}, color.NRGBA{0x80, 0x60, 0x30, 0x30})
	}
	margin := canvas.NewLine(marginColor)
	margin.StrokeWidth = 1.2
	return &paperRenderer{p: p, shadow: shadow, sheet: sheet, grain: grain, edge: edge, margin: margin}
}

type paperRenderer struct {
	p             *paperPage
	shadow, sheet *canvas.Rectangle
	grain         *canvas.Image
	edge          *canvas.LinearGradient
	margin        *canvas.Line
	rules         []fyne.CanvasObject
	holes         []fyne.CanvasObject
	lastH         float32
}

const ruleGap = 28

func (r *paperRenderer) Layout(s fyne.Size) {
	r.shadow.Move(fyne.NewPos(3, 4))
	r.shadow.Resize(s)
	r.sheet.Resize(s)
	r.grain.Resize(s)
	ew := float32(26)
	if r.p.holesLeft {
		r.edge.Move(fyne.NewPos(0, 0))
	} else {
		r.edge.Move(fyne.NewPos(s.Width-ew, 0))
	}
	r.edge.Resize(fyne.NewSize(ew, s.Height))
	mx := float32(30) // the margin sits just inside the holes, left of the writing
	if !r.p.holesLeft {
		mx = 12
	}
	r.margin.Position1 = fyne.NewPos(mx, 0)
	r.margin.Position2 = fyne.NewPos(mx, s.Height)
	// rules and holes depend on the height
	if s.Height != r.lastH {
		r.lastH = s.Height
		r.rules = r.rules[:0]
		if r.p.ruled {
			for y := float32(64); y < s.Height-8; y += ruleGap {
				l := canvas.NewLine(ruleColor)
				l.StrokeWidth = 1
				l.Position1, l.Position2 = fyne.NewPos(6, y), fyne.NewPos(s.Width-6, y)
				r.rules = append(r.rules, l)
			}
		}
		r.holes = r.holes[:0]
		n := int(s.Height / 95)
		if n > 8 {
			n = 8
		}
		if n < 2 {
			n = 2
		}
		gap := s.Height / float32(n+1)
		for i := 1; i <= n; i++ {
			hole := canvas.NewCircle(color.NRGBA{0x3A, 0x18, 0x10, 0xFF}) // the leather shows through
			hole.StrokeColor = color.NRGBA{0xC8, 0xB9, 0x96, 0xFF}
			hole.StrokeWidth = 1
			d := float32(11)
			x := float32(9)
			if !r.p.holesLeft {
				x = s.Width - 9 - d
			}
			hole.Move(fyne.NewPos(x, gap*float32(i)-d/2))
			hole.Resize(fyne.NewSize(d, d))
			r.holes = append(r.holes, hole)
		}
	} else {
		for _, o := range r.rules {
			l := o.(*canvas.Line)
			l.Position2 = fyne.NewPos(s.Width-6, l.Position1.Y)
		}
	}
	inset := fyne.NewPos(36, 6)
	right := float32(8)
	if !r.p.holesLeft {
		inset = fyne.NewPos(18, 6)
		right = 26 // clear of the holes
	}
	r.p.content.Move(inset)
	r.p.content.Resize(fyne.NewSize(s.Width-inset.X-right, s.Height-12))
}

func (r *paperRenderer) MinSize() fyne.Size {
	m := r.p.content.MinSize()
	return fyne.NewSize(m.Width+48, m.Height+12)
}
func (r *paperRenderer) Refresh() { r.Layout(r.p.Size()); canvas.Refresh(r.p) }
func (r *paperRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.shadow, r.sheet, r.grain, r.edge}
	objs = append(objs, r.rules...)
	objs = append(objs, r.margin)
	objs = append(objs, r.holes...)
	return append(objs, r.p.content)
}
func (r *paperRenderer) Destroy() {}

// ---------- binder rings ----------

type binderRings struct{ widget.BaseWidget }

func newBinderRings() *binderRings { b := &binderRings{}; b.ExtendBaseWidget(b); return b }

func (b *binderRings) CreateRenderer() fyne.WidgetRenderer {
	loadTextures()
	spine := canvas.NewVerticalGradient(color.NRGBA{0x2C, 0x0E, 0x0A, 0xFF}, color.NRGBA{0x44, 0x16, 0x12, 0xFF})
	return &ringsRenderer{b: b, spine: spine}
}

type ringsRenderer struct {
	b     *binderRings
	spine *canvas.LinearGradient
	rings []fyne.CanvasObject
	lastH float32
}

func (r *ringsRenderer) Layout(s fyne.Size) {
	r.spine.Move(fyne.NewPos(s.Width/2-6, 0))
	r.spine.Resize(fyne.NewSize(12, s.Height))
	if s.Height == r.lastH {
		return
	}
	r.lastH = s.Height
	r.rings = r.rings[:0]
	n := int(s.Height / 95)
	if n > 8 {
		n = 8
	}
	if n < 2 {
		n = 2
	}
	gap := s.Height / float32(n+1)
	for i := 1; i <= n; i++ {
		img := canvas.NewImageFromImage(texRing)
		img.FillMode = canvas.ImageFillStretch
		w := s.Width + 28 // reaches into the holes of both pages
		img.Move(fyne.NewPos(-14, gap*float32(i)-13))
		img.Resize(fyne.NewSize(w, 26))
		r.rings = append(r.rings, img)
	}
}
func (r *ringsRenderer) MinSize() fyne.Size { return fyne.NewSize(26, 100) }
func (r *ringsRenderer) Refresh()           { r.lastH = -1; r.Layout(r.b.Size()); canvas.Refresh(r.b) }
func (r *ringsRenderer) Objects() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{r.spine}, r.rings...)
}
func (r *ringsRenderer) Destroy() {}

// ---------- coloured index tabs (the lists and groups) ----------

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
	shadow := canvas.NewRectangle(color.NRGBA{0x10, 0x04, 0x02, 0x60})
	shadow.CornerRadius = 7
	card := canvas.NewRectangle(t.col)
	card.CornerRadius = 7
	sheen := canvas.NewVerticalGradient(color.NRGBA{0xFF, 0xFF, 0xFF, 0x40}, color.NRGBA{0, 0, 0, 0x22})
	title := canvas.NewText(t.title, color.White)
	title.TextStyle.Bold = true
	title.TextSize = 13
	count := canvas.NewText(t.count, color.NRGBA{0xFF, 0xFF, 0xFF, 0xD0})
	count.TextSize = 12
	count.Alignment = fyne.TextAlignTrailing
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
		return fyne.NewSize(120, 22)
	}
	return fyne.NewSize(150, 34)
}

func (r *tabRenderer) Layout(s fyne.Size) {
	if r.t.header {
		r.title.Move(fyne.NewPos(6, 4))
		return
	}
	// a selected tab sticks out further, towards the pages
	inset := float32(18)
	if r.t.selected {
		inset = 0
	} else if r.t.hover {
		inset = 9
	}
	w := s.Width - inset
	r.shadow.Move(fyne.NewPos(inset+2, 3))
	r.shadow.Resize(fyne.NewSize(w, s.Height-4))
	r.card.Move(fyne.NewPos(inset, 1))
	r.card.Resize(fyne.NewSize(w+8, s.Height-4)) // runs under the page edge
	r.sheen.Move(fyne.NewPos(inset, 1))
	r.sheen.Resize(fyne.NewSize(w+8, (s.Height-4)/2))
	r.title.Move(fyne.NewPos(inset+10, (s.Height-4-r.title.MinSize().Height)/2+1))
	cs := r.count.MinSize()
	r.count.Move(fyne.NewPos(s.Width-cs.Width-4, (s.Height-4-cs.Height)/2+1))
}

func (r *tabRenderer) Refresh() {
	r.card.FillColor = r.t.col
	r.title.Text, r.count.Text = r.t.title, r.t.count
	if r.t.header {
		r.title.Color = color.NRGBA{0xE8, 0xD6, 0xB8, 0xC0}
		r.title.TextSize = 11
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

// filofaxTabs shows the sidebar items as coloured index tabs.
func (u *ui) filofaxTabs() fyne.CanvasObject {
	u.tabBox = container.NewVBox()
	return container.NewVScroll(container.NewPadded(u.tabBox))
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

// ---------- A–Z letter tabs ----------

// letterOf is the index letter a contact files under: A–Z, ع for Arabic names, # for the rest.
func letterOf(c *Contact, byLast bool) string {
	name := strings.TrimSpace(c.First)
	if byLast && strings.TrimSpace(c.Last) != "" {
		name = c.Last
	}
	if name == "" {
		name = c.DisplayName()
	}
	// "Al-Mutairi" files under M, as in a paper address book
	low := strings.ToLower(name)
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

var letterTabs = append([]string{"All"}, append(strings.Split("A B C D E F G H I J K L M N O P Q R S T U V W X Y Z", " "), "ع", "#")...)

type letterTab struct {
	widget.BaseWidget
	letter   string
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
	bg := canvas.NewRectangle(color.NRGBA{0xE9, 0xDD, 0xC0, 0xFF})
	bg.CornerRadius = 4
	txt := canvas.NewText(t.letter, inkColor)
	txt.TextStyle.Bold = true
	txt.TextSize = 11
	txt.Alignment = fyne.TextAlignCenter
	return &letterRenderer{t: t, bg: bg, txt: txt}
}

type letterRenderer struct {
	t   *letterTab
	bg  *canvas.Rectangle
	txt *canvas.Text
}

func (r *letterRenderer) MinSize() fyne.Size { return fyne.NewSize(34, 18) }
func (r *letterRenderer) Layout(s fyne.Size) {
	inset := float32(6)
	if r.t.selected {
		inset = 0
	}
	r.bg.Move(fyne.NewPos(inset, 0.5))
	r.bg.Resize(fyne.NewSize(s.Width-inset+6, s.Height-1))
	r.txt.Move(fyne.NewPos(inset, (s.Height-r.txt.MinSize().Height)/2))
	r.txt.Resize(fyne.NewSize(s.Width-inset, r.txt.MinSize().Height))
}
func (r *letterRenderer) Refresh() {
	switch {
	case r.t.selected:
		r.bg.FillColor = color.NRGBA{0xF6, 0xEF, 0xDC, 0xFF}
		r.txt.Color = color.NRGBA{0xA0, 0x2A, 0x24, 0xFF}
	case r.t.empty:
		r.bg.FillColor = color.NRGBA{0xD6, 0xC8, 0xA6, 0xFF}
		r.txt.Color = color.NRGBA{0x9A, 0x8E, 0x74, 0xFF}
	default:
		r.bg.FillColor = color.NRGBA{0xE9, 0xDD, 0xC0, 0xFF}
		r.txt.Color = inkColor
	}
	if r.t.letter == "All" {
		r.txt.TextSize = 10
	}
	r.Layout(r.t.Size())
	canvas.Refresh(r.t)
}
func (r *letterRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bg, r.txt} }
func (r *letterRenderer) Destroy()                     {}

// letterStrip is the column of A–Z tabs; "All" shows everyone.
func (u *ui) letterStrip() fyne.CanvasObject {
	u.letterBox = container.New(&stackFill{})
	return u.letterBox
}

// refreshLetters marks the chosen letter and greys letters nobody files under.
func (u *ui) refreshLetters(used map[string]bool) {
	if u.letterBox == nil {
		return
	}
	u.letterBox.Objects = nil
	for _, l := range letterTabs {
		l := l
		t := &letterTab{letter: l, selected: l == u.letter || (l == "All" && u.letter == ""), empty: l != "All" && !used[l]}
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

// stackFill spreads its items evenly down the full height (the letter tabs fill the page edge).
type stackFill struct{}

func (s *stackFill) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(34, float32(len(objs))*15)
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

// filofaxLayout is the open organiser: leather, coloured tabs, A–Z tabs, the ruled page of names,
// the binder rings and the person's page.
func (u *ui) filofaxLayout(listPane fyne.CanvasObject) fyne.CanvasObject {
	loadTextures()
	leather := canvas.NewImageFromImage(texLeather)
	leather.FillMode = canvas.ImageFillStretch
	stitch := canvas.NewRectangle(color.Transparent) // stitched border round the cover
	stitch.StrokeColor = color.NRGBA{0xD9, 0xB9, 0x8A, 0x70}
	stitch.StrokeWidth = 1.2
	stitch.CornerRadius = 10

	tabs := container.New(&fixedWidth{172}, u.filofaxTabs())
	letters := container.New(&fixedWidth{34}, u.letterStrip())
	left := container.New(&fixedWidth{350}, newPaperPage(listPane, false, true))
	rings := container.New(&fixedWidth{26}, newBinderRings())
	right := newPaperPage(u.right, true, false)
	book := container.NewBorder(nil, nil, container.NewHBox(tabs, letters, left, rings), nil, right)

	statusBg := canvas.NewRectangle(color.NRGBA{0xEC, 0xE2, 0xC9, 0xFF})
	status := container.NewStack(statusBg, container.NewPadded(u.status))
	cover := container.NewStack(leather, container.New(&insetLayout{8}, stitch), container.New(&insetLayout{16}, book))
	return container.NewBorder(nil, status, nil, nil, cover)
}

// insetLayout fills its area less a margin on every side.
type insetLayout struct{ m float32 }

func (l *insetLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	s := fyne.NewSize(0, 0)
	for _, o := range objs {
		s = s.Max(o.MinSize())
	}
	return s.Add(fyne.NewSize(2*l.m, 2*l.m))
}
func (l *insetLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(l.m, l.m))
		o.Resize(size.Subtract(fyne.NewSize(2*l.m, 2*l.m)))
	}
}

// toggleLook switches between the Filofax organiser and the classic Mac panes.
func (u *ui) toggleLook() {
	u.leaveEditor(func() {
		u.cfg.ClassicLook = !u.cfg.ClassicLook
		_ = u.cfg.Save(u.store.Dir())
		u.ed = nil
		u.build()
		u.refreshAll()
	}, nil)
}
