package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// The card is a printed page: each value sits on a ruled line, like writing in a real address
// book. Fyne draws a box round every text field, so the card's theme makes that box invisible
// until the field is clicked (then the blue focus box shows you're writing), and ruleOverlay,
// a transparent sheet over the card, draws one ruled line under each field, wherever it is.

var ruleInk = color.NRGBA{0x7C, 0x8C, 0xA6, 0x70} // the faint blue of printed ruling

type ruleOverlay struct {
	widget.BaseWidget
	content fyne.CanvasObject // what to rule: the card's fields
}

func newRuleOverlay(content fyne.CanvasObject) *ruleOverlay {
	r := &ruleOverlay{content: content}
	r.ExtendBaseWidget(r)
	return r
}

func (r *ruleOverlay) CreateRenderer() fyne.WidgetRenderer { return &ruleRenderer{o: r} }

type ruleRenderer struct {
	o     *ruleOverlay
	lines []fyne.CanvasObject
	pool  []*canvas.Line
}

// fields finds the text fields and pop-up menus on the card, with their positions relative to
// the overlay (which covers the same area as the content).
func ruledFields(o fyne.CanvasObject, at fyne.Position, out *[]ruledField) {
	if o == nil || !o.Visible() {
		return
	}
	switch t := o.(type) {
	case *fyne.Container:
		for _, c := range t.Objects {
			ruledFields(c, at.Add(c.Position()), out)
		}
	case *container.ThemeOverride:
		ruledFields(t.Content, at, out)
	case *container.Scroll:
		// a scroller inside the card would need its offset; the card has none
	case *widget.Entry:
		*out = append(*out, ruledField{at, t.Size(), t.MultiLine})
	case *widget.SelectEntry:
		*out = append(*out, ruledField{at, t.Size(), false})
	case *widget.Select:
		*out = append(*out, ruledField{at, t.Size(), false})
	}
}

type ruledField struct {
	pos   fyne.Position
	size  fyne.Size
	multi bool
}

func (r *ruleRenderer) line() *canvas.Line {
	n := len(r.lines)
	if n < len(r.pool) {
		return r.pool[n]
	}
	l := canvas.NewLine(ruleInk)
	l.StrokeWidth = 1
	r.pool = append(r.pool, l)
	return l
}

func (r *ruleRenderer) Layout(fyne.Size) {
	var fs []ruledField
	// the content sits at the overlay's own origin (they share a Stack)
	ruledFields(r.o.content, fyne.NewPos(0, 0), &fs)
	r.lines = r.lines[:0]
	for _, f := range fs {
		if f.size.Width < 20 {
			continue
		}
		x1, x2 := f.pos.X+4, f.pos.X+f.size.Width-4
		ys := []float32{f.pos.Y + f.size.Height - 5}
		if f.multi { // a notes box is several ruled lines
			ys = ys[:0]
			for y := f.pos.Y + 30; y < f.pos.Y+f.size.Height-2; y += 22 {
				ys = append(ys, y)
			}
		}
		for _, y := range ys {
			l := r.line()
			l.Position1, l.Position2 = fyne.NewPos(x1, y), fyne.NewPos(x2, y)
			r.lines = append(r.lines, l)
		}
	}
}

func (r *ruleRenderer) MinSize() fyne.Size { return fyne.NewSize(0, 0) }
func (r *ruleRenderer) Refresh() {
	r.Layout(r.o.Size())
	for _, l := range r.lines {
		l.Refresh()
	}
}
func (r *ruleRenderer) Objects() []fyne.CanvasObject { return r.lines }
func (r *ruleRenderer) Destroy()                     {}
