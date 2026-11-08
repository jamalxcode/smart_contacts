package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Choosing several cards works like a desk: clip them together. Every row has a paper clip at
// its left edge, faint until it's clipped; click it to clip or unclip that card (no mode to
// switch on first). The clipped cards gather under a yellow sticky note with what can be done
// to all of them.

// clipPath is a paperclip's wire as a polyline, in a 16×40 box.
func clipPath() [][2]float64 {
	var p [][2]float64
	arc := func(cx, cy, r, a0, a1 float64) {
		for i := 0; i <= 16; i++ {
			a := a0 + (a1-a0)*float64(i)/16
			p = append(p, [2]float64{cx + r*math.Cos(a), cy + r*math.Sin(a)})
		}
	}
	p = append(p, [2]float64{3, 31})
	arc(8, 7, 5, math.Pi, 2*math.Pi)      // over the top, left to right
	arc(9.5, 33, 3.5, 0, math.Pi)         // round the bottom, right to left
	arc(9.5, 11, 2.5, math.Pi, 2*math.Pi) // the inner loop's top
	p = append(p, [2]float64{12, 27})
	return p
}

func segDist(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy + 1e-9)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

// clipImage draws the clip at scale k: shiny steel, or a faint grey "slot" when ghost.
func clipImage(k int, ghost bool) image.Image {
	w, h := 16*k, 40*k
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	path := clipPath()
	half := 0.95 // the wire's half-thickness, in box units
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px, py := (float64(x)+0.5)/float64(k), (float64(y)+0.5)/float64(k)
			d := 99.0
			for i := 1; i < len(path); i++ {
				d = math.Min(d, segDist(px, py, path[i-1][0], path[i-1][1], path[i][0], path[i][1]))
			}
			cov := math.Max(0, math.Min(1, (half-d)*float64(k)+0.5))
			if cov <= 0 {
				continue
			}
			var c color.NRGBA
			if ghost {
				c = color.NRGBA{0x9A, 0x94, 0x88, uint8(70 * cov)}
			} else {
				// across the wire: bright in the middle, darker at the edges (a round wire)
				v := chrome(0.5 + 0.5*(d/half)*math.Copysign(1, px-8))
				g := 70 + 170*v // dark steel with a bright highlight: plainly "on"
				c = color.NRGBA{uint8(g), uint8(g), uint8(math.Min(255, g+14*(1-v))), uint8(255 * cov)}
			}
			i := y*m.Stride + x*4
			m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
	return m
}

func pngResource(name string, im image.Image) fyne.Resource {
	var b bytes.Buffer
	_ = png.Encode(&b, im)
	return fyne.NewStaticResource(name, b.Bytes())
}

var clipOn, clipOff, clipIcon fyne.Resource

func loadClips() {
	if clipOn == nil {
		clipOn = pngResource("clip.png", clipImage(4, false))
		clipOff = pngResource("clip-ghost.png", clipImage(4, true))
		clipIcon = clipOn
	}
}

// rowClip is the clip at the left of a list row.
type rowClip struct {
	widget.BaseWidget
	img   *canvas.Image
	onTap func()
}

func newRowClip() *rowClip {
	loadClips()
	c := &rowClip{img: canvas.NewImageFromResource(clipOff)}
	c.img.FillMode = canvas.ImageFillContain
	c.ExtendBaseWidget(c)
	return c
}

func (c *rowClip) set(on bool) {
	r := clipOff
	if on {
		r = clipOn
	}
	if c.img.Resource != r {
		c.img.Resource = r
		c.img.Refresh()
	}
}
func (c *rowClip) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}
func (c *rowClip) Cursor() desktop.Cursor { return desktop.PointerCursor }
func (c *rowClip) CreateRenderer() fyne.WidgetRenderer {
	return &clipRenderer{c: c}
}

type clipRenderer struct{ c *rowClip }

func (r *clipRenderer) Layout(s fyne.Size) {
	// the clip grips the card's top edge, so it rides a little high
	r.c.img.Move(fyne.NewPos((s.Width-12)/2, -4))
	r.c.img.Resize(fyne.NewSize(12, 30))
}
func (r *clipRenderer) MinSize() fyne.Size           { return fyne.NewSize(18, 30) }
func (r *clipRenderer) Refresh()                     { r.Layout(r.c.Size()); canvas.Refresh(r.c) }
func (r *clipRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.c.img} }
func (r *clipRenderer) Destroy()                     {}

// toggleClip clips or unclips a card from its row; the first clip starts clipping, and taking
// the last one off ends it.
func (u *ui) toggleClip(id string) {
	if u.busy() {
		return
	}
	if !u.selectMode {
		u.toggleSelectMode()
	}
	if u.picked[id] {
		delete(u.picked, id)
	} else {
		u.picked[id] = true
	}
	u.selAnchor = id
	if len(u.picked) == 0 {
		u.toggleSelectMode() // nothing clipped any more
		return
	}
	u.list.Refresh()
	u.updateBulkBar()
}

// stickyNote is the yellow note the clipped cards' commands are written on.
func stickyNote(content fyne.CanvasObject) fyne.CanvasObject {
	paper := canvas.NewLinearGradient(color.NRGBA{0xFF, 0xF4, 0xA8, 0xFF}, color.NRGBA{0xF8, 0xE8, 0x88, 0xFF}, 90)
	shadow := canvas.NewRectangle(color.NRGBA{0x30, 0x20, 0x00, 0x40})
	tape := canvas.NewRectangle(color.NRGBA{0xFF, 0xFF, 0xFF, 0x70}) // a strip of tape holding it on
	return &fyne.Container{Layout: &noteLayout{}, Objects: []fyne.CanvasObject{shadow, paper, tape, content}}
}

type noteLayout struct{}

func (noteLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return objs[3].MinSize().Add(fyne.NewSize(20, 26))
}
func (noteLayout) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	objs[0].Move(fyne.NewPos(3, 5))
	objs[0].Resize(s.Subtract(fyne.NewSize(2, 4)))
	objs[1].Move(fyne.NewPos(0, 2))
	objs[1].Resize(s.Subtract(fyne.NewSize(4, 4)))
	objs[2].Move(fyne.NewPos(s.Width/2-34, -4))
	objs[2].Resize(fyne.NewSize(68, 14))
	objs[3].Move(fyne.NewPos(10, 14))
	objs[3].Resize(s.Subtract(fyne.NewSize(24, 22)))
}
