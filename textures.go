package main

import (
	"image"
	"math"
	"math/rand"

	"fyne.io/fyne/v2/canvas"
)

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

// ringImage is one ring of the binder seen from above: a thick chrome loop whose two ends go
// down through punched holes in the left and right pages, with its shadow on the paper.
func ringImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	cx, cy := float64(w)/2, float64(h)/2
	ax, ay := float64(w)/2-6, float64(h)/2-3
	const tube = 5.5 // the wire's thickness in pixels, the same all the way round
	set := func(i int, r, g, b, a float64) {
		oa := float64(img.Pix[i+3]) / 255
		na := a / 255
		out := na + oa*(1-na)
		if out == 0 {
			return
		}
		mix := func(o uint8, n float64) uint8 { return clamp8((n*na + float64(o)*oa*(1-na)) / out) }
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = mix(img.Pix[i], r), mix(img.Pix[i+1], g), mix(img.Pix[i+2], b)
		img.Pix[i+3] = clamp8(out * 255)
	}
	inRing := func(dx, dy float64) (bool, float64) {
		dOut := math.Sqrt((dx/ax)*(dx/ax) + (dy/ay)*(dy/ay))
		dIn := math.Sqrt((dx/(ax-tube))*(dx/(ax-tube)) + (dy/(ay-tube))*(dy/(ay-tube)))
		if dOut > 1 || dIn < 1 || dOut == 0 {
			return false, 0
		}
		f := (1 - 1/dIn) / (1/dOut - 1/dIn) // 0 inner edge .. 1 outer edge
		return true, f
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*img.Stride + x*4
			fx, fy := float64(x)+0.5, float64(y)+0.5
			// punched holes where the ring goes into the pages
			for _, hx := range []float64{7, float64(w) - 7} {
				if hd := math.Hypot((fx-hx)/5, (fy-cy)/4.2); hd < 1 {
					set(i, 0x4A, 0x30, 0x1C, 220*math.Min(1, (1-hd)*4))
				}
			}
			if ok, _ := inRing(fx-cx-1.5, fy-cy-3); ok { // shadow on the paper
				set(i, 0x30, 0x20, 0x10, 60)
			}
			ok, f := inRing(fx-cx, fy-cy)
			if !ok {
				continue
			}
			v := chrome(f)
			if fy > cy {
				v *= 0.84 // the far side of the loop is in shade
			}
			edge := math.Min(1, math.Min(f, 1-f)*6)
			set(i, v, v, v*1.02, 255*edge)
		}
	}
	return img
}

// plateImage is the binder's spine plate in cross-section: two chrome rods side by side,
// stretched to any height.
func plateImage(w int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, 4))
	for x := 0; x < w; x++ {
		half := float64(w) / 2
		t := math.Mod(float64(x), half) / half
		v := chrome(t)
		if math.Abs(float64(x)-half) < 1.2 {
			v *= 0.45 // the seam between the rods
		}
		for y := 0; y < 4; y++ {
			i := y*img.Stride + x*4
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = clamp8(v), clamp8(v), clamp8(v*1.03), 255
		}
	}
	return img
}

// capImage is a domed chrome end cap of the ring mechanism.
func capImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	cx, cy := float64(w)/2, float64(h)/2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := (float64(x)-cx)/(cx-1), (float64(y)-cy)/(cy-1)
			d := math.Sqrt(dx*dx + dy*dy)
			if d > 1 {
				continue
			}
			v := chrome((dx + 1) / 2)
			v += 90 * math.Exp(-((dx+0.25)*(dx+0.25)+(dy+0.35)*(dy+0.35))/0.04)
			v *= 0.85 + 0.15*(1-d)
			i := y*img.Stride + x*4
			img.Pix[i], img.Pix[i+1], img.Pix[i+2] = clamp8(v), clamp8(v), clamp8(v*1.03)
			img.Pix[i+3] = clamp8(255 * math.Min(1, (1-d)*12))
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

var texCover, texLip, texGrain, texRing, texPen, texBrass, texPlate, texCap image.Image

func loadTextures() {
	if texCover == nil {
		texCover = leatherImage(1000, 720, [3]float64{0x86, 0x45, 0x22}, 7)
		texLip = leatherImage(600, 600, [3]float64{0x5A, 0x2C, 0x16}, 9)
		texGrain = paperGrain(400, 560)
		texRing = ringImage(ringW, ringH)
		texPlate = plateImage(44)
		texCap = capImage(60, 40)
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

var feltCache image.Image

func feltImage() image.Image {
	if feltCache == nil {
		const w, h = 600, 600
		m := image.NewNRGBA(image.Rect(0, 0, w, h))
		n := newNoise(11)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := n.fbm(float64(x)/3, float64(y)/3, 3) - 0.5 // fine fibres
				i := y*m.Stride + x*4
				g := float64(deskColor.R) + v*14
				m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = clamp8(g), clamp8(g), clamp8(g+2), 255
			}
		}
		feltCache = m
	}
	return feltCache
}
