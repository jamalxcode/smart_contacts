package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
)

// Moving around the organiser looks like turning pages, and a change leaves a mark on the card:
//   - choosing another tab or letter turns the list page (a picture of the old page folds away
//     towards the rings, or the outer edge going back, and the new one is underneath);
//   - when the card that's open changes, an UPDATED stamp lands on it and fades.

// tabOrder and letterIndex say which way a page turn goes (forward is down the tabs / A to Z).
func (u *ui) tabOrder(key string) int {
	for i, it := range u.sideItems {
		if it.key == key {
			return i
		}
	}
	return 0
}

func letterIndex(l string) int {
	if l == "" {
		return 0
	}
	for i, t := range letterTabs {
		if t == l {
			return i
		}
	}
	return 0
}

// overlay returns the window's top layer for animations (nil in the classic look).
func (u *ui) overlay() *fyne.Container {
	if u.cfg.ClassicLook || u.coverShown {
		return nil
	}
	return u.flipLayer
}

// pageTurn turns the list page: dir > 0 forward, dir < 0 back.
func (u *ui) pageTurn(dir int) {
	layer := u.overlay()
	if layer == nil || u.leftPage == nil || dir == 0 {
		return
	}
	cv := u.win.Canvas()
	shot := cv.Capture()
	if shot == nil {
		return
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(u.leftPage)
	size := u.leftPage.Size()
	scale := float32(shot.Bounds().Dx()) / cv.Size().Width
	r := image.Rect(int(pos.X*scale), int(pos.Y*scale), int((pos.X+size.Width)*scale), int((pos.Y+size.Height)*scale))
	old := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(old, old.Bounds(), shot, r.Min, draw.Src)
	page := canvas.NewImageFromImage(old)
	page.FillMode = canvas.ImageFillStretch
	page.ScaleMode = canvas.ImageScaleFastest
	// the turning edge darkens as it lifts
	edge := canvas.NewHorizontalGradient(color.NRGBA{0, 0, 0, 0}, color.NRGBA{0x20, 0x10, 0x00, 0x70})
	if dir < 0 {
		edge = canvas.NewHorizontalGradient(color.NRGBA{0x20, 0x10, 0x00, 0x70}, color.NRGBA{0, 0, 0, 0})
	}
	place := func(t float32) {
		e := t * t * (3 - 2*t) // ease in and out
		w := size.Width * (1 - e)
		x := pos.X // going back: the page folds towards the outer (left) edge
		if dir > 0 {
			x = pos.X + size.Width - w // forward: towards the rings
		}
		page.Move(fyne.NewPos(x, pos.Y))
		page.Resize(fyne.NewSize(w, size.Height))
		ew := fyne.Min(w, 40)
		ex := x + w - ew
		if dir < 0 {
			ex = x
		}
		edge.Move(fyne.NewPos(ex, pos.Y))
		edge.Resize(fyne.NewSize(ew, size.Height))
	}
	place(0)
	layer.Objects = append(layer.Objects, page, edge)
	layer.Refresh()
	a := fyne.NewAnimation(340*time.Millisecond, func(t float32) {
		place(t)
		page.Refresh()
		edge.Refresh()
	})
	a.Curve = fyne.AnimationLinear
	a.Start()
	go func() {
		time.Sleep(400 * time.Millisecond)
		fyne.Do(func() {
			layer.Remove(page)
			layer.Remove(edge)
		})
	}()
}

// ---------- the UPDATED stamp ----------

var stampCache image.Image

// stampImage is "UPDATED" in red ink inside a double border, tilted like a rubber stamp, with
// the ink a little uneven.
func stampImage() image.Image {
	if stampCache != nil {
		return stampCache
	}
	const k = 2 // drawn at twice the size it's shown, for sharp edges
	w, h := 190*k, 64*k
	flat := image.NewNRGBA(image.Rect(0, 0, w, h))
	ink := color.NRGBA{0xB3, 0x26, 0x1E, 0xFF}
	box := func(in int, t int) {
		for y := in; y < h-in; y++ {
			for x := in; x < w-in; x++ {
				if x < in+t || x >= w-in-t || y < in+t || y >= h-in-t {
					flat.SetNRGBA(x, y, ink)
				}
			}
		}
	}
	box(2*k, 3*k)
	box(8*k, 1*k)
	if f, err := opentype.Parse(serifBold); err == nil {
		if face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: float64(30 * k), DPI: 72}); err == nil {
			d := &font.Drawer{Dst: flat, Src: image.NewUniform(ink), Face: face}
			tw := d.MeasureString("UPDATED").Ceil()
			d.Dot = fixed.P((w-tw)/2, h/2+11*k)
			d.DrawString("UPDATED")
		}
	}
	// uneven ink: knock some of it back
	n := newNoise(7)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*flat.Stride + x*4
			if flat.Pix[i+3] == 0 {
				continue
			}
			v := n.fbm(float64(x)/9, float64(y)/9, 3)
			flat.Pix[i+3] = clamp8(255 * math.Min(1, 0.45+v))
		}
	}
	// tilt it
	a := -8 * math.Pi / 180
	ow, oh := int(float64(w)*math.Cos(-a)+float64(h)*math.Sin(-a)), int(float64(w)*math.Sin(-a)+float64(h)*math.Cos(-a))
	out := image.NewNRGBA(image.Rect(0, 0, ow, oh))
	cx, cy := float64(w)/2, float64(h)/2
	ox, oy := float64(ow)/2, float64(oh)/2
	c, s := math.Cos(a), math.Sin(a)
	m := f64.Aff3{c, -s, ox - c*cx + s*cy, s, c, oy - s*cx - c*cy}
	xdraw.BiLinear.Transform(out, m, flat, flat.Bounds(), xdraw.Over, nil)
	stampCache = out
	return out
}

// stampCard lands an UPDATED stamp on the open card and lets it fade.
func (u *ui) stampCard() {
	layer := u.overlay()
	if layer == nil || u.rightPage == nil {
		return
	}
	im := stampImage()
	st := canvas.NewImageFromImage(im)
	st.FillMode = canvas.ImageFillStretch
	b := im.Bounds()
	w, h := float32(b.Dx())/2, float32(b.Dy())/2
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(u.rightPage)
	size := u.rightPage.Size()
	at := fyne.NewPos(pos.X+size.Width-w-40, pos.Y+46)
	layer.Objects = append(layer.Objects, st)
	// it comes down a little larger and settles, then stays a moment and fades
	land := fyne.NewAnimation(180*time.Millisecond, func(t float32) {
		k := 1.25 - 0.25*t
		st.Move(fyne.NewPos(at.X-(w*k-w)/2, at.Y-(h*k-h)/2))
		st.Resize(fyne.NewSize(w*k, h*k))
		st.Translucency = float64(0.6 * (1 - t))
		st.Refresh()
	})
	land.Start()
	go func() {
		time.Sleep(1800 * time.Millisecond)
		fyne.Do(func() {
			fade := fyne.NewAnimation(500*time.Millisecond, func(t float32) {
				st.Translucency = float64(t)
				st.Refresh()
			})
			fade.Start()
		})
		time.Sleep(600 * time.Millisecond)
		fyne.Do(func() { layer.Remove(st) })
	}()
}
