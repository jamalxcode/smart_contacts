package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (u *ui) smartAdd() {
	in := widget.NewMultiLineEntry()
	in.Wrapping = fyne.TextWrapWord
	in.SetMinRowsVisible(10)
	in.SetPlaceHolder("Paste an email signature, a business card's text, a WhatsApp contact message, a LinkedIn blurb…")
	if clip := strings.TrimSpace(u.app.Clipboard().Content()); clip != "" && len(clip) < 4000 && (reEmail.MatchString(clip) || rePhone.MatchString(clip)) {
		in.SetText(clip) // looks like contact details: save a paste
	}
	mode := "Private: read on this Mac only. Nothing you paste leaves this computer."
	if u.cloudAI() {
		mode = "Cloud AI is ON: this text will be sent to " + u.cfg.AIModel + " via OpenRouter (turn it off in Settings to keep everything on this Mac)."
	}
	note := caption(mode)
	note.Wrapping = fyne.TextWrapWord
	progress := widget.NewProgressBarInfinite()
	progress.Hide()
	var d *paperDialog
	cardBtn := widget.NewButtonWithIcon("Business Card Image…", theme.FileImageIcon(), func() {
		d.Hide()
		u.chooseCardImage()
	})
	cardNote := caption("…or read a photo or screenshot of a business card (the text is read on this Mac; the image is kept with the contact). You can also drag a card image onto the window.")
	cardNote.Wrapping = fyne.TextWrapWord
	content := container.NewBorder(nil, container.NewVBox(note, progress, widget.NewSeparator(), container.NewBorder(nil, nil, nil, cardBtn, cardNote)), nil, nil, in)
	d = newConfirm("Smart Add", "Read It", "Cancel", content, func(ok bool) {
		text := strings.TrimSpace(in.Text)
		if !ok || text == "" {
			return
		}
		u.runSmartAdd(text)
	}, u.win)
	d.Resize(fyne.NewSize(600, 420))
	d.Show()
	u.win.Canvas().Focus(in)
}

func (u *ui) runSmartAdd(text string) { u.runSmartAddCard(text, nil) }

// runSmartAddCard reads text (typed, pasted or recognised from a card image) into a contact.
// card is the business card image, kept with whichever contact is saved.
func (u *ui) runSmartAddCard(text string, card []byte) {
	cc := u.cfg.CountryCode
	if !u.cloudAI() {
		u.finishSmartAddCard(ParseLocal(text, cc), "Read privately on this Mac — check the details, then Save.", card)
		return
	}
	prog := newCustomWithoutButtons("Reading…", container.NewVBox(widget.NewLabel("Asking "+u.cfg.AIModel+"…"), widget.NewProgressBarInfinite()), u.win)
	prog.Show()
	go func() {
		c, confident, err := ParseAI(context.Background(), u.cfg.OpenRouterKey, u.cfg.AIModel, text, cc)
		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				u.finishSmartAddCard(ParseLocal(text, cc), "The AI didn't answer ("+err.Error()+"), so this was read on this Mac — check it, then Save.", card)
				return
			}
			msg := "Filled in by AI — check the details, then Save."
			if !confident {
				msg = "The text may describe more than one person; this is the AI's best single contact. Check it, then Save."
			}
			u.finishSmartAddCard(c, msg, card)
		})
	}()
}

// cloudAI is true only when the user has explicitly allowed sending text off this Mac AND set a key.
func (u *ui) cloudAI() bool {
	return u.cfg.AllowCloudAI && strings.TrimSpace(u.cfg.OpenRouterKey) != ""
}

// finishSmartAddCard opens the editor with what was read. If the person is already in the book,
// it asks whether to update that card (adding what's new) or add a new person.
func (u *ui) finishSmartAddCard(c *Contact, banner string, card []byte) {
	c.Tidy()
	if card != nil {
		banner += "\nThe business card image will be kept with this contact."
	}
	x := NewDupIndex(u.store.Contacts, u.cfg.CountryCode).Find(c)
	if x == nil {
		u.leaveEditor(func() { u.openEditorCard(nil, c, banner, card, "added this contact") }, nil)
		return
	}
	merged := x.Clone()
	Merge(merged, []*Contact{c}, u.cfg.CountryCode)
	adds := mergeAdds(x, c, u.cfg.CountryCode)
	what := "nothing new"
	if len(adds) > 0 {
		what = strings.Join(adds, ", ")
	}
	msg := widget.NewLabel(fmt.Sprintf("This looks like %s, who is already in your contacts.\n\nUpdating adds: %s.", x.DisplayName(), what))
	msg.Wrapping = fyne.TextWrapWord
	var d *paperDialog
	update := widget.NewButton("Update "+x.DisplayName(), func() {
		d.Hide()
		u.leaveEditor(func() {
			u.selectedID = x.ID
			u.reselect()
			u.openEditorCard(x, merged, banner+"\nUpdated from the new details — check, then Save.", card, "updated this contact")
		}, nil)
	})
	update.Importance = widget.HighImportance
	add := widget.NewButton("Add as a new person", func() {
		d.Hide()
		u.leaveEditor(func() { u.openEditorCard(nil, c, banner, card, "added this contact") }, nil)
	})
	cancel := widget.NewButton("Cancel", func() { d.Hide() })
	d = newCustomWithoutButtons("Already in your contacts?", container.NewVBox(msg, update, add, cancel), u.win)
	d.Resize(fyne.NewSize(520, 300))
	d.Show()
}

// chooseCardImage picks a business card photo or screenshot and reads it.
func (u *ui) chooseCardImage() {
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
		u.readCardImage(r.URI().Name(), b)
	}, u.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".jpg", ".jpeg", ".png", ".heic", ".heif", ".tiff", ".JPG", ".JPEG", ".PNG", ".HEIC"}))
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

// readCardImage recognises the text on a card image (on this Mac) and continues as Smart Add.
func (u *ui) readCardImage(name string, data []byte) {
	img, err := ToJPEG(data, name)
	if err != nil {
		showErr(err, u.win)
		return
	}
	prog := newCustomWithoutButtons("Reading the business card…", container.NewVBox(
		widget.NewLabel("Recognising the text on this Mac."), widget.NewProgressBarInfinite()), u.win)
	prog.Show()
	go func() {
		text, err := ReadCardText(img)
		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				showErr(err, u.win)
				return
			}
			u.runSmartAddCard(text, img)
		})
	}()
}

// ---------- import ----------
