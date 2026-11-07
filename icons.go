package main

import (
	"bytes"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"golang.org/x/image/draw"
)

// Material-style 24px icons the Fyne theme doesn't have. Themed, so they follow light/dark mode.
func svgIcon(name, path string) fyne.Resource {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#000000" d="` + path + `"/></svg>`
	return theme.NewThemedResource(fyne.NewStaticResource(name+".svg", []byte(svg)))
}

var (
	iconPhone   = svgIcon("phone", "M6.62 10.79c1.44 2.83 3.76 5.14 6.59 6.59l2.2-2.2c.27-.27.67-.36 1.02-.24 1.12.37 2.33.57 3.57.57.55 0 1 .45 1 1V20c0 .55-.45 1-1 1-9.39 0-17-7.61-17-17 0-.55.45-1 1-1h3.5c.55 0 1 .45 1 1 0 1.25.2 2.45.57 3.57.11.35.03.74-.25 1.02l-2.2 2.2z")
	iconMessage = svgIcon("message", "M20 2H4c-1.1 0-1.99.9-1.99 2L2 22l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zM6 9h12v2H6V9zm8 5H6v-2h8v2zm4-6H6V6h12v2z")
	iconChat    = svgIcon("chat", "M12 2C6.48 2 2 6.48 2 12c0 1.85.5 3.58 1.38 5.07L2 22l4.93-1.38A9.94 9.94 0 0 0 12 22c5.52 0 10-4.48 10-10S17.52 2 12 2zm4.6 13.6c-.2.55-1.15 1.05-1.6 1.1-.42.05-.95.07-1.53-.1-.35-.11-.8-.26-1.38-.51-2.43-1.05-4.01-3.49-4.13-3.65-.12-.16-.98-1.31-.98-2.5s.62-1.77.85-2.01c.22-.24.48-.3.64-.3h.46c.15 0 .35-.06.55.42.2.49.69 1.68.75 1.8.06.12.1.26.02.42-.08.16-.12.26-.24.4-.12.14-.25.31-.36.42-.12.12-.24.25-.1.49.14.24.62 1.02 1.33 1.65.91.81 1.68 1.06 1.92 1.18.24.12.38.1.52-.06.14-.16.6-.7.76-.94.16-.24.32-.2.54-.12.22.08 1.4.66 1.64.78.24.12.4.18.46.28.06.1.06.58-.14 1.13z")
	iconStar    = svgIcon("star", "M12 17.27L18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z")
	iconStarOff = svgIcon("staroff", "M22 9.24l-7.19-.62L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21 12 17.27 18.18 21l-1.63-7.03L22 9.24zM12 15.4l-3.76 2.27 1-4.28-3.32-2.88 4.38-.38L12 6.1l1.71 4.04 4.38.38-3.32 2.88 1 4.28L12 15.4z")
	iconPlace   = svgIcon("place", "M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7zm0 9.5c-1.38 0-2.5-1.12-2.5-2.5s1.12-2.5 2.5-2.5 2.5 1.12 2.5 2.5-1.12 2.5-2.5 2.5z")
	iconCake    = svgIcon("cake", "M12 6c1.11 0 2-.9 2-2 0-.38-.1-.73-.29-1.03L12 0l-1.71 2.97c-.19.3-.29.65-.29 1.03 0 1.1.9 2 2 2zm4.6 9.99l-1.07-1.07-1.08 1.07c-1.3 1.3-3.58 1.31-4.89 0l-1.07-1.07-1.09 1.07C6.75 16.64 5.88 17 4.96 17c-.73 0-1.4-.23-1.96-.61V21c0 .55.45 1 1 1h16c.55 0 1-.45 1-1v-4.61c-.56.38-1.23.61-1.96.61-.92 0-1.79-.36-2.44-1.01zM18 9h-5V7h-2v2H6c-1.66 0-3 1.34-3 3v1.54c0 1.08.88 1.96 1.96 1.96.52 0 1.02-.2 1.38-.57l2.14-2.13 2.13 2.13c.74.74 2.03.74 2.77 0l2.14-2.13 2.13 2.13c.37.37.86.57 1.38.57 1.08 0 1.96-.88 1.96-1.96V12C21 10.34 19.66 9 18 9z")
	iconSparkle = svgIcon("sparkle", "M19 9l1.25-2.75L23 5l-2.75-1.25L19 1l-1.25 2.75L15 5l2.75 1.25L19 9zm-7.5.5L9 4 6.5 9.5 1 12l5.5 2.5L9 20l2.5-5.5L17 12l-5.5-2.5zM19 15l-1.25 2.75L15 19l2.75 1.25L19 23l1.25-2.75L23 19l-2.75-1.25L19 15z")
	iconLink    = svgIcon("link", "M3.9 12c0-1.71 1.39-3.1 3.1-3.1h4V7H7c-2.76 0-5 2.24-5 5s2.24 5 5 5h4v-1.9H7c-1.71 0-3.1-1.39-3.1-3.1zM8 13h8v-2H8v2zm9-6h-4v1.9h4c1.71 0 3.1 1.39 3.1 3.1s-1.39 3.1-3.1 3.1h-4V17h4c2.76 0 5-2.24 5-5s-2.24-5-5-5z")
	iconGroup   = svgIcon("group", "M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z")
	iconLabel   = svgIcon("label", "M17.63 5.84C17.27 5.33 16.67 5 16 5L5 5.01C3.9 5.01 3 5.9 3 7v10c0 1.1.9 1.99 2 1.99L16 19c.67 0 1.27-.33 1.63-.84L22 12l-4.37-6.16z")
	iconMerge   = svgIcon("merge", "M17 20.41L18.41 19 15 15.59 13.59 17 17 20.41zM7.5 8H11v5.59L5.59 19 7 20.41l6-6V8h3.5L12 3.5 7.5 8z")
	iconBrush   = svgIcon("brush", "M7 14c-1.66 0-3 1.34-3 3 0 1.31-1.16 2-2 2 .92 1.22 2.49 2 4 2 2.21 0 4-1.79 4-4 0-1.66-1.34-3-3-3zm13.71-9.37l-1.34-1.34a.996.996 0 0 0-1.41 0L9 12.25 11.75 15l8.96-8.96a.996.996 0 0 0 0-1.41z")
)

// avatarColors are calm colours readable with white initials in both themes.
var avatarColors = []color.NRGBA{
	{0x5B, 0x6C, 0xE0, 0xFF}, {0x2E, 0x9C, 0x8F, 0xFF}, {0xD9, 0x7A, 0x34, 0xFF}, {0xC2, 0x4D, 0x7A, 0xFF},
	{0x7A, 0x5C, 0xC9, 0xFF}, {0x3C, 0x8D, 0xC4, 0xFF}, {0x6F, 0x9A, 0x3A, 0xFF}, {0xB8, 0x5A, 0x4A, 0xFF},
}

func avatarColor(id string) color.NRGBA {
	h := fnv.New32a()
	h.Write([]byte(id))
	return avatarColors[h.Sum32()%uint32(len(avatarColors))]
}

// ShrinkPhoto scales a photo down to at most max px on its longest side and re-encodes it as JPEG,
// so a 6 MB camera photo becomes ~60 KB. Images it can't decode come back unchanged.
func ShrinkPhoto(b []byte, max int) []byte {
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return b
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w <= max && h <= max && len(b) < 300_000 {
		return b
	}
	nw, nh := w, h
	if w > h && w > max {
		nw, nh = max, h*max/w
	} else if h >= w && h > max {
		nw, nh = w*max/h, max
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85}); err != nil {
		return b
	}
	return out.Bytes()
}

// CanDecodeImage reports whether b is a JPEG or PNG this app can show.
func CanDecodeImage(b []byte) bool {
	_, _, err := image.DecodeConfig(bytes.NewReader(b))
	return err == nil
}

// CirclePhoto centre-crops a photo to a square, scales it to size px and makes everything
// outside the circle transparent, so the avatar is round on any toolkit version.
// Undecodable images come back unchanged.
func CirclePhoto(b []byte, size int) []byte {
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return b
	}
	r := img.Bounds()
	side := r.Dx()
	if r.Dy() < side {
		side = r.Dy()
	}
	sq := image.Rect(r.Min.X+(r.Dx()-side)/2, r.Min.Y+(r.Dy()-side)/2, 0, 0)
	sq.Max = sq.Min.Add(image.Pt(side, side))
	scaled := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, sq, draw.Src, nil)
	out := image.NewNRGBA(scaled.Bounds())
	c := float64(size) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-c, float64(y)+0.5-c
			d := c - sqrt(dx*dx+dy*dy) // distance inside the edge, for a soft 1px edge
			if d <= 0 {
				continue
			}
			p := color.NRGBAModel.Convert(scaled.At(x, y)).(color.NRGBA)
			if d < 1 {
				p.A = uint8(float64(p.A) * d)
			}
			out.SetNRGBA(x, y, p)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return b
	}
	return buf.Bytes()
}

func sqrt(v float64) float64 { return math.Sqrt(v) }
