package main

import (
	"fmt"
	"io"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// photoMenu opens from a click on the card's picture: choose or remove the personal photo.
func (u *ui) photoMenu(c *Contact, anchor fyne.CanvasObject) {
	items := []*fyne.MenuItem{fyne.NewMenuItem("Choose Photo…", func() { u.choosePersonalPhoto(c) })}
	if c.HasPhoto {
		items = append(items, fyne.NewMenuItem("Remove Photo", func() {
			_ = u.store.SetPhoto(c, nil)
			delete(u.photoCache, c.ID)
			u.store.Put(c)
			u.save()
		}))
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), u.win.Canvas(), pos.Add(fyne.NewPos(0, anchor.Size().Height)))
}

func (u *ui) choosePersonalPhoto(c *Contact) {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil || r == nil {
			return
		}
		defer r.Close()
		b, err := io.ReadAll(io.LimitReader(r, 60<<20))
		if err != nil {
			showErr(err, u.win)
			return
		}
		img, err := ToJPEG(b, r.URI().Name()) // iPhone HEIC photos are converted on the Mac
		if err != nil {
			showErr(err, u.win)
			return
		}
		if err := u.store.SetPhoto(c, ShrinkPhoto(img, 512)); err != nil {
			showErr(err, u.win)
			return
		}
		delete(u.photoCache, c.ID)
		u.store.Put(c)
		u.save()
		u.flash("Photo set for " + c.DisplayName())
	}, u.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".jpg", ".jpeg", ".png", ".heic", ".heif", ".JPG", ".JPEG", ".PNG", ".HEIC"}))
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

// cardsSection shows the business card images kept with a contact.
func (u *ui) cardsSection(c *Contact) []fyne.CanvasObject {
	if len(c.CardImages) == 0 {
		return nil
	}
	grid := container.NewGridWrap(fyne.NewSize(300, 200))
	for i := len(c.CardImages) - 1; i >= 0; i-- {
		ci := c.CardImages[i]
		path := u.store.CardImagePath(ci.File)
		img := canvas.NewImageFromFile(path)
		img.FillMode = canvas.ImageFillContain
		img.SetMinSize(fyne.NewSize(290, 150))
		open := widget.NewButtonWithIcon("", theme.ZoomInIcon(), func() { u.showCardImage(c, ci) })
		open.Importance = widget.LowImportance
		reveal := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() { RevealInFinder(path) })
		reveal.Importance = widget.LowImportance
		del := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
			u.confirmCB("Delete this card image?", "The contact's details stay; only the picture is deleted.", "Delete", true, func(ok bool) {
				if ok {
					u.store.RemoveCardImage(c, ci.File)
					u.store.Put(c)
					u.save()
				}
			}, u.win)
		})
		del.Importance = widget.LowImportance
		info := caption(fmt.Sprintf("%s · %s", ci.Added.Format("2 Jan 2006"), ci.Note))
		info.Truncation = fyne.TextTruncateEllipsis
		grid.Add(container.NewBorder(nil, container.NewBorder(nil, nil, nil, container.NewHBox(open, reveal, del), info), nil, nil, img))
	}
	return []fyne.CanvasObject{grid}
}

func (u *ui) showCardImage(c *Contact, ci CardImage) {
	img := canvas.NewImageFromFile(u.store.CardImagePath(ci.File))
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(760, 480))
	d := dialog.NewCustom(c.DisplayName()+" — business card", "Close", img, u.win)
	d.Resize(fyne.NewSize(820, 600))
	d.Show()
}
