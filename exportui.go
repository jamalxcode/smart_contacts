package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

func (u *ui) importMac() {
	prog := newCustomWithoutButtons("Reading Mac Contacts…", container.NewVBox(
		widget.NewLabel("If macOS asks, allow Smart Contacts to use Contacts."), widget.NewProgressBarInfinite()), u.win)
	prog.Show()
	go func() {
		cards, err := ReadMacContacts()
		fyne.Do(func() {
			prog.Hide()
			if err != nil {
				showErr(err, u.win)
				return
			}
			u.previewImport(&Batch{Source: "Mac Contacts", Format: "Mac Contacts app", Cards: cards})
		})
	}()
}

// sendToMacContacts copies cards into Apple's Contacts app, after warning that Apple's app
// may sync them to iCloud — the one way contacts could leave this Mac from here.
func (u *ui) sendToMacContacts(cs []*Contact) {
	if len(cs) == 0 {
		return
	}
	what := fmt.Sprintf("these %d contacts", len(cs))
	if len(cs) == 1 {
		what = cs[0].DisplayName()
	}
	msg := widget.NewLabel("This copies " + what + " into Apple's Contacts app.\n\n" +
		"If iCloud Contacts is turned on (System Settings → your name → iCloud → Contacts),\n" +
		"Apple will sync the copy to iCloud and your other devices — it will no longer be only on this Mac.\n\n" +
		"Smart Contacts itself keeps everything on this Mac either way.")
	showCustomConfirm("Copy to Apple Contacts?", "Copy Anyway", "Cancel", msg, func(ok bool) {
		if ok {
			if err := OpenInMacContacts(u.store, cs); err != nil {
				showErr(err, u.win)
			}
		}
	}, u.win)
}

// confirmExportPlace warns before writing contacts into a folder that syncs off this Mac.
func (u *ui) confirmExportPlace(w fyne.URIWriteCloser, write func()) {
	if !InCloudFolder(w.URI().Path()) {
		write()
		return
	}
	showCustomConfirm("This folder syncs to the cloud", "Save There Anyway", "Cancel",
		widget.NewLabel("“"+w.URI().Name()+"” is going into a cloud-synced folder (iCloud Drive, Dropbox, Google Drive…),\n"+
			"so the contacts would leave this Mac. Choose a folder like Documents or Desktop to keep them private."),
		func(ok bool) {
			if ok {
				write()
				return
			}
			w.Close()
			_ = storage.Delete(w.URI()) // don't leave an empty file in the cloud folder
		}, u.win)
}

// ---------- export ----------

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, s)
	if i := strings.Index(s, " ("); i > 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func (u *ui) exportVCF(cs []*Contact, name string) {
	if len(cs) == 0 {
		return
	}
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil || w == nil {
			return
		}
		u.confirmExportPlace(w, func() {
			defer w.Close()
			for _, c := range cs {
				if err := WriteVCardRelated(w, c, u.store.Photo(c), u.store.RelatedFor(c)); err != nil {
					showErr(err, u.win)
					return
				}
			}
			u.flash(fmt.Sprintf("Saved %d contacts to %s", len(cs), w.URI().Name()))
		})
	}, u.win)
	d.SetFileName(safeName(name) + ".vcf")
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

func (u *ui) exportCSV() { u.exportCSVFor(u.filtered) }

// exportCSVFor saves some contacts as a spreadsheet.
func (u *ui) exportCSVFor(cs []*Contact) {
	if len(cs) == 0 {
		return
	}
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil || w == nil {
			return
		}
		u.confirmExportPlace(w, func() {
			defer w.Close()
			if err := WriteCSV(w, cs); err != nil {
				showErr(err, u.win)
				return
			}
			u.flash(fmt.Sprintf("Saved %d contacts to %s", len(cs), w.URI().Name()))
		})
	}, u.win)
	name := u.groupTitle()
	if len(cs) != len(u.filtered) {
		name = fmt.Sprintf("%d contacts", len(cs))
	}
	d.SetFileName(safeName(name) + ".csv")
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

// ---------- duplicates ----------
