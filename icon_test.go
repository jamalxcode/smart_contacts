package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"testing"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// TestMakeIcon draws Icon.png, the app icon: the leather-bound organiser closed, with its gold
// stitching, strap and brass snap, the way the window shows it. It only runs when asked:
//
//	MAKE_ICON=Icon.png go test -run TestMakeIcon .
func TestMakeIcon(t *testing.T) {
	out := os.Getenv("MAKE_ICON")
	if out == "" {
		t.Skip("set MAKE_ICON=path to draw the icon")
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, appIcon(1024)); err != nil {
		t.Fatal(err)
	}
}

// roundRect adds a rounded rectangle to a rasterizer.
func roundRect(r *vector.Rasterizer, x0, y0, x1, y1, rad float32) {
	const k = 0.5523 // control-point distance for a quarter circle
	r.MoveTo(x0+rad, y0)
	r.LineTo(x1-rad, y0)
	r.CubeTo(x1-rad+rad*k, y0, x1, y0+rad-rad*k, x1, y0+rad)
	r.LineTo(x1, y1-rad)
	r.CubeTo(x1, y1-rad+rad*k, x1-rad+rad*k, y1, x1-rad, y1)
	r.LineTo(x0+rad, y1)
	r.CubeTo(x0+rad-rad*k, y1, x0, y1-rad+rad*k, x0, y1-rad)
	r.LineTo(x0, y0+rad)
	r.CubeTo(x0, y0+rad-rad*k, x0+rad-rad*k, y0, x0+rad, y0)
	r.ClosePath()
}

func rrMask(s int, x0, y0, x1, y1, rad float32) *image.Alpha {
	r := vector.NewRasterizer(s, s)
	roundRect(r, x0, y0, x1, y1, rad)
	m := image.NewAlpha(image.Rect(0, 0, s, s))
	r.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

// blurAlpha softens a mask (three box blurs, close to a Gaussian) for shadows.
func blurAlpha(m *image.Alpha, rad int) *image.Alpha {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	src := make([]float64, w*h)
	for i := range src {
		src[i] = float64(m.Pix[i])
	}
	tmp := make([]float64, w*h)
	for pass := 0; pass < 3; pass++ {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				var s float64
				n := 0
				for d := -rad; d <= rad; d++ {
					if xx := x + d; xx >= 0 && xx < w {
						s += src[y*w+xx]
						n++
					}
				}
				tmp[y*w+x] = s / float64(n)
			}
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				var s float64
				n := 0
				for d := -rad; d <= rad; d++ {
					if yy := y + d; yy >= 0 && yy < h {
						s += tmp[yy*w+x]
						n++
					}
				}
				src[y*w+x] = s / float64(n)
			}
		}
	}
	out := image.NewAlpha(m.Rect)
	for i, v := range src {
		out.Pix[i] = clamp8(v)
	}
	return out
}

// paint lays src over dst through mask, at an overall opacity.
func paint(dst *image.NRGBA, src image.Image, mask image.Image, op float64) {
	if op < 1 {
		mask = &fadeMask{mask, op}
	}
	draw.DrawMask(dst, dst.Bounds(), src, image.Point{}, mask, image.Point{}, draw.Over)
}

type fadeMask struct {
	m  image.Image
	op float64
}

func (f *fadeMask) ColorModel() color.Model { return color.AlphaModel }
func (f *fadeMask) Bounds() image.Rectangle { return f.m.Bounds() }
func (f *fadeMask) At(x, y int) color.Color {
	_, _, _, a := f.m.At(x, y).RGBA()
	return color.Alpha{uint8(float64(a>>8) * f.op)}
}

// stitches dashes a rounded rectangle's outline: short gold thread strokes with a dark hole
// shadow, like the cover's saddle stitching.
func stitches(s int, x0, y0, x1, y1, rad, dash, gap, width float32) *image.Alpha {
	r := vector.NewRasterizer(s, s)
	// walk the outline as points
	var pts [][2]float32
	add := func(x, y float32) { pts = append(pts, [2]float32{x, y}) }
	arc := func(cx, cy, a0 float32) {
		for i := 0; i <= 24; i++ {
			a := float64(a0) + math.Pi/2*float64(i)/24
			add(cx+rad*float32(math.Cos(a)), cy+rad*float32(math.Sin(a)))
		}
	}
	add(x0+rad, y0)
	add(x1-rad, y0)
	arc(x1-rad, y0+rad, -math.Pi/2)
	add(x1, y1-rad)
	arc(x1-rad, y1-rad, 0)
	add(x0+rad, y1)
	arc(x0+rad, y1-rad, math.Pi/2)
	add(x0, y0+rad)
	arc(x0+rad, y0+rad, math.Pi)
	pos, on := float32(0), true
	left := dash
	for i := 1; i < len(pts); i++ {
		ax, ay, bx, by := pts[i-1][0], pts[i-1][1], pts[i][0], pts[i][1]
		seg := float32(math.Hypot(float64(bx-ax), float64(by-ay)))
		t := float32(0)
		for seg-t > 0 {
			step := float32(math.Min(float64(left), float64(seg-t)))
			if on {
				sx, sy := ax+(bx-ax)*t/seg, ay+(by-ay)*t/seg
				ex, ey := ax+(bx-ax)*(t+step)/seg, ay+(by-ay)*(t+step)/seg
				nx, ny := -(by-ay)/seg*width/2, (bx-ax)/seg*width/2
				r.MoveTo(sx+nx, sy+ny)
				r.LineTo(ex+nx, ey+ny)
				r.LineTo(ex-nx, ey-ny)
				r.LineTo(sx-nx, sy-ny)
				r.ClosePath()
			}
			t += step
			left -= step
			if left <= 0 {
				on = !on
				if on {
					left = dash
				} else {
					left = gap
				}
			}
		}
		pos += seg
	}
	m := image.NewAlpha(image.Rect(0, 0, s, s))
	r.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

func shift(m *image.Alpha, dx, dy int) *image.Alpha {
	out := image.NewAlpha(m.Rect)
	draw.Draw(out, out.Rect.Add(image.Pt(dx, dy)), m, image.Point{}, draw.Src)
	return out
}

func intersect(a, b *image.Alpha) *image.Alpha {
	out := image.NewAlpha(a.Rect)
	for i := range out.Pix {
		out.Pix[i] = uint8(int(a.Pix[i]) * int(b.Pix[i]) / 255)
	}
	return out
}

func subtract(a, b *image.Alpha) *image.Alpha {
	out := image.NewAlpha(a.Rect)
	for i := range out.Pix {
		out.Pix[i] = uint8(int(a.Pix[i]) * (255 - int(b.Pix[i])) / 255)
	}
	return out
}

func scaled(src image.Image, s int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, s, s))
	xdraw.CatmullRom.Scale(out, out.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	return out
}

func uni(r, g, b uint8) *image.Uniform { return image.NewUniform(color.NRGBA{r, g, b, 255}) }

// vertical shading: light from the top left, so the cover is a touch brighter there.
func shade(s int, top, bottom float64) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, s, s))
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			t := (float64(y) + 0.4*float64(x)) / (1.4 * float64(s))
			a := top + (bottom-top)*t
			i := y*m.Stride + x*4
			if a >= 0 {
				m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = 255, 230, 200, clamp8(a*255)
			} else {
				m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = 20, 8, 0, clamp8(-a*255)
			}
		}
	}
	return m
}

// appIcon draws the closed organiser on Apple's icon grid (the book stands within the
// 824-point body of a 1024 canvas, with a soft shadow below).
func appIcon(s int) image.Image {
	k := float32(s) / 1024
	out := image.NewNRGBA(image.Rect(0, 0, s, s))
	// leather, drawn at half size and enlarged so the grain still reads when the icon is small
	cover := scaled(leatherImage(s/2, s/2, [3]float64{0x8E, 0x48, 0x22}, 7), s)
	back := scaled(leatherImage(s/2, s/2, [3]float64{0x5E, 0x2E, 0x16}, 9), s)
	strapLeather := scaled(leatherImage(s/3, s/3, [3]float64{0x7A, 0x3C, 0x1C}, 13), s)

	// the book's outline
	bx0, by0, bx1, by1 := 176*k, 96*k, 812*k, 928*k
	rad := 54 * k

	// shadow on the desk
	sh := blurAlpha(rrMask(s, bx0+6*k, by0+26*k, bx1+22*k, by1+20*k, rad), int(18*k))
	paint(out, uni(0, 0, 0), sh, 0.55)

	// back cover, a little larger, and the page block between (seen at the fore-edge)
	paint(out, back, rrMask(s, bx0, by0+4*k, bx1+20*k, by1+8*k, rad), 1)
	pages := rrMask(s, bx0+40*k, by0+18*k, bx1+10*k, by1-6*k, 24*k)
	paint(out, uni(0xF3, 0xEA, 0xD6), pages, 1)
	// page edges: fine lines down the fore-edge
	lines := image.NewNRGBA(image.Rect(0, 0, s, s))
	for x := int(bx1 - 4*k); x < int(bx1+10*k); x++ {
		if (x/int(math.Max(1, float64(3*k))))%2 == 0 {
			for y := 0; y < s; y++ {
				i := y*lines.Stride + x*4
				lines.Pix[i], lines.Pix[i+1], lines.Pix[i+2], lines.Pix[i+3] = 0xB8, 0xA6, 0x84, 0xB0
			}
		}
	}
	paint(out, lines, pages, 1)

	// front cover
	front := rrMask(s, bx0, by0, bx1, by1, rad)
	paint(out, cover, front, 1)
	paint(out, shade(s, 0.10, -0.22), front, 1)
	// a burnished edge: darker just inside the outline
	inner := blurAlpha(rrMask(s, bx0+14*k, by0+14*k, bx1-14*k, by1-14*k, rad-10*k), int(10*k))
	paint(out, uni(0x2A, 0x10, 0x04), subtract(front, inner), 0.55)

	// the spine: a fold line near the left, with a highlight beside it
	spineX := bx0 + 62*k
	fold := rrMask(s, spineX-3*k, by0+6*k, spineX+3*k, by1-6*k, 3*k)
	paint(out, uni(0x2A, 0x10, 0x04), fold, 0.8)
	paint(out, uni(0xD8, 0x9A, 0x62), shift(fold, int(5*k), 0), 0.35)
	paint(out, uni(0x20, 0x0A, 0x00), intersect(front, rrMask(s, bx0, by0, spineX, by1, rad)), 0.18)

	// gold stitching round the cover, and down the spine
	gold := uni(0xE6, 0xC2, 0x72)
	st := stitches(s, bx0+26*k, by0+26*k, bx1-26*k, by1-26*k, rad-22*k, 22*k, 13*k, 6*k)
	paint(out, uni(0x1E, 0x0A, 0x00), blurAlpha(shift(st, int(2*k), int(3*k)), int(2*k)), 0.7)
	paint(out, gold, st, 1)
	st2 := stitches(s, spineX+28*k, by0+62*k, bx1-62*k, by1-62*k, 16*k, 22*k, 13*k, 6*k)
	paint(out, uni(0x1E, 0x0A, 0x00), blurAlpha(shift(st2, int(2*k), int(3*k)), int(2*k)), 0.6)
	paint(out, gold, st2, 0.85)

	// the title, gold-blocked into the leather
	drawTitle(out, k, (spineX+bx1-30*k)/2, 300*k)

	// the strap across the fore-edge, with its brass snap
	sx0, sy0, sx1, sy1 := 660*k, 590*k, 872*k, 800*k
	strap := rrMask(s, sx0, sy0, sx1, sy1, 34*k)
	paint(out, uni(0, 0, 0), blurAlpha(shift(strap, int(4*k), int(12*k)), int(12*k)), 0.6)
	paint(out, strapLeather, strap, 1)
	paint(out, shade(s, 0.12, -0.25), strap, 1)
	sst := stitches(s, sx0+16*k, sy0+16*k, sx1-16*k, sy1-16*k, 22*k, 16*k, 10*k, 5*k)
	paint(out, uni(0x1E, 0x0A, 0x00), shift(sst, int(1*k), int(2*k)), 0.6)
	paint(out, gold, sst, 0.9)
	d := int(96 * k)
	stud := brassImage(d)
	cx, cy := int((sx0+sx1)/2-6*k), int((sy0+sy1)/2)
	studShadow := image.NewAlpha(out.Rect)
	r := vector.NewRasterizer(s, s)
	ox, oy := float32(cx)+5*k, float32(cy)+9*k
	for i := 0; i <= 48; i++ {
		a := 2 * math.Pi * float64(i) / 48
		px, py := ox+float32(d)/2*float32(math.Cos(a)), oy+float32(d)/2*float32(math.Sin(a))
		if i == 0 {
			r.MoveTo(px, py)
		} else {
			r.LineTo(px, py)
		}
	}
	r.ClosePath()
	r.Draw(studShadow, studShadow.Bounds(), image.Opaque, image.Point{})
	paint(out, uni(0, 0, 0), blurAlpha(studShadow, int(6*k)), 0.6)
	draw.Draw(out, image.Rect(cx-d/2, cy-d/2, cx+d/2, cy+d/2), stud, image.Point{}, draw.Over)
	return out
}

// drawTitle writes "Smart Contacts" with a rule and "Address Book" under it, in gold leaf
// pressed into the cover (a dark edge below each letter, a bright one above).
func drawTitle(out *image.NRGBA, k float32, cx, base float32) {
	bold, err1 := opentype.Parse(serifBold)
	ital, err2 := opentype.Parse(serifItalic)
	if err1 != nil || err2 != nil {
		return
	}
	write := func(f *opentype.Font, size float64, text string, y float32) {
		face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size * float64(k), DPI: 72, Hinting: font.HintingNone})
		if err != nil {
			return
		}
		d := &font.Drawer{Face: face}
		w := d.MeasureString(text)
		at := fixed.Point26_6{X: fixed.Int26_6(cx*64) - w/2, Y: fixed.Int26_6(y * 64)}
		for _, p := range []struct {
			dx, dy float32
			c      color.NRGBA
		}{
			{0, 3, color.NRGBA{0x20, 0x0A, 0x00, 0xC0}}, // pressed-in shadow
			{0, -1.5, color.NRGBA{0xFF, 0xEC, 0xB0, 0x90}},
			{0, 0, color.NRGBA{0xE2, 0xBA, 0x66, 0xFF}},
		} {
			d.Dst, d.Src = out, image.NewUniform(p.c)
			d.Dot = at.Add(fixed.Point26_6{X: fixed.Int26_6(p.dx * k * 64), Y: fixed.Int26_6(p.dy * k * 64)})
			d.DrawString(text)
		}
	}
	write(bold, 96, "Smart", base)
	write(bold, 96, "Contacts", base+100*k)
	// a gold rule
	r := image.Rect(int(cx-120*k), int(base+140*k), int(cx+120*k), int(base+146*k))
	draw.Draw(out, r.Add(image.Pt(0, int(2*k))), uni(0x20, 0x0A, 0x00), image.Point{}, draw.Over)
	draw.Draw(out, r, uni(0xE2, 0xBA, 0x66), image.Point{}, draw.Over)
	write(ital, 54, "Address Book", base+214*k)
}
