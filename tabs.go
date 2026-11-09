package main

import (
	"image/color"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type indexTab struct {
	widget.BaseWidget
	title, count string
	col          color.NRGBA
	selected     bool
	header       bool
	flag         bool // a To Tidy tab: muted, with its count on a red flag
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
	flag := canvas.NewRectangle(color.NRGBA{0xC6, 0x28, 0x28, 0xFF})
	flag.CornerRadius = 8
	if !t.flag {
		flag.Hide()
	} else {
		count.TextStyle.Bold = true
		count.Color = color.White
	}
	return &tabRenderer{t: t, shadow: shadow, card: card, sheen: sheen, title: title, count: count, flag: flag}
}

type tabRenderer struct {
	t            *indexTab
	shadow, card *canvas.Rectangle
	flag         *canvas.Rectangle
	sheen        *canvas.LinearGradient
	title, count *canvas.Text
}

func (r *tabRenderer) MinSize() fyne.Size {
	if r.t.header {
		return fyne.NewSize(120, 28)
	}
	return fyne.NewSize(150, 31)
}

func (r *tabRenderer) Layout(s fyne.Size) {
	if r.t.header { // a small paper label with the heading in black, easy to read on leather
		r.title.Text = r.t.title
		ts := r.title.MinSize()
		r.card.Move(fyne.NewPos(10, 4))
		r.card.Resize(fyne.NewSize(ts.Width+16, 20))
		r.shadow.Move(fyne.NewPos(11, 6))
		r.shadow.Resize(fyne.NewSize(ts.Width+16, 20))
		r.title.Move(fyne.NewPos(18, 4+(20-ts.Height)/2))
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
	cs := r.count.MinSize()
	// a long name fits its tab: a little smaller first, then cut short with "…"
	avail := s.Width - cs.Width - 16 - 8 - (out + 9)
	if r.t.count == "" {
		avail = s.Width - 16 - (out + 9)
	}
	r.title.Text, r.title.TextSize = r.t.title, 13.5
	for r.title.MinSize().Width > avail && r.title.TextSize > 11 {
		r.title.TextSize -= 0.5
	}
	for rs := []rune(r.t.title); r.title.MinSize().Width > avail && len(rs) > 4; {
		rs = rs[:len(rs)-1]
		r.title.Text = strings.TrimSpace(string(rs)) + "…"
	}
	r.title.Move(fyne.NewPos(out+9, (s.Height-4-r.title.MinSize().Height)/2+1))
	r.count.Move(fyne.NewPos(s.Width-cs.Width-16, (s.Height-4-cs.Height)/2+1)) // clear of the page edge
	fw := fyne.Max(cs.Width+10, 18)
	r.flag.Move(fyne.NewPos(s.Width-16-cs.Width/2-fw/2, (s.Height-4-16)/2+1))
	r.flag.Resize(fyne.NewSize(fw, 16))
}

func (r *tabRenderer) Refresh() {
	r.card.FillColor = r.t.col
	r.count.Text = r.t.count
	if r.t.header {
		r.title.Color = color.NRGBA{0x14, 0x10, 0x0C, 0xFF}
		r.title.TextSize = 11
		r.card.FillColor = color.NRGBA{0xFB, 0xF7, 0xEC, 0xFF}
		r.card.CornerRadius = 3
		r.card.StrokeColor = color.NRGBA{0xC8, 0xBC, 0xA4, 0xFF}
		r.card.StrokeWidth = 1
		r.shadow.CornerRadius = 3
	}
	r.Layout(r.t.Size())
	canvas.Refresh(r.t)
}

func (r *tabRenderer) Objects() []fyne.CanvasObject {
	if r.t.header {
		return []fyne.CanvasObject{r.shadow, r.card, r.title}
	}
	return []fyne.CanvasObject{r.shadow, r.card, r.sheen, r.title, r.flag, r.count}
}
func (r *tabRenderer) Destroy() {}

func (u *ui) filofaxTabs() fyne.CanvasObject {
	u.tabBox = container.NewVBox()
	// the lists and groups scroll; Sort and Also in the Book stay at the bottom (as in View)
	return container.NewBorder(nil, u.organiserFooter(), nil, nil, container.NewVScroll(u.tabBox))
}

func (u *ui) refreshTabs() {
	if u.tabBox == nil {
		return
	}
	u.tabBox.Objects = nil
	ci := 0
	tidy := false
	for _, it := range u.sideItems {
		it := it
		t := &indexTab{title: it.title}
		if it.key == grpHeader {
			t.header = true
			t.title = strings.ToUpper(it.title)
			tidy = it.title == "To Tidy"
		} else {
			t.col = tabColors[ci%len(tabColors)]
			ci++
			if tidy {
				t.flag = true
				t.col = color.NRGBA{0x8A, 0x84, 0x7A, 0xFF} // muted stone: a job list, not people
			}
			if it.count > 0 {
				t.count = itoa(it.count)
			}
			t.selected = it.key == u.group
			t.onTap = func() {
				if u.group == it.key {
					return
				}
				u.pageTurn(u.tabOrder(it.key) - u.tabOrder(u.group))
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
	u.usedLetters = used
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
			was := u.letter
			if l == "All" {
				u.letter = ""
			} else {
				u.letter = l
			}
			if was != u.letter {
				u.pageTurn(letterIndex(u.letter) - letterIndex(was))
			}
			u.refreshList()
		}
		t.ExtendBaseWidget(t)
		u.letterBox.Add(t)
	}
	u.letterBox.Refresh()
}
