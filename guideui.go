package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// bringEverythingIn is a checklist that walks through every source, then clean-up and
// de-duplication, ticking off what's done. It shows on first launch and from File.
func (u *ui) bringEverythingIn() {
	has := func(names ...string) bool {
		for _, n := range names {
			for _, src := range u.store.SourceNames() {
				if strings.EqualFold(src, n) || strings.HasPrefix(strings.ToLower(src), strings.ToLower(n)) {
					return true
				}
			}
		}
		return false
	}
	var d *dialog.CustomDialog
	type step struct {
		title, how string
		done       bool
		action     string
		run        func()
	}
	steps := []step{
		{"Mac & iPhone contacts", "Everything in the Mac's Contacts app, including your iPhone through iCloud.", has("Mac Contacts"), "Import", u.importMac},
		{"Google / Gmail", "contacts.google.com → Export → Google CSV or vCard, then choose the file.", has("Google Contacts", "Gmail"), "Choose File…", u.importFile},
		{"Outlook / work email", "outlook.com → People → Manage → Export contacts.", has("Outlook"), "Choose File…", u.importFile},
		{"LinkedIn", "Settings & Privacy → Data privacy → Get a copy of your data → Connections.", has("LinkedIn"), "Choose File…", u.importFile},
		{"Other phones", "Send the phone's contacts over Bluetooth or AirDrop; the app picks them up.", has("File:", "Android phone"), "Receive…", u.receiveFromPhone},
		{"Telegram", "Telegram Desktop → Export Telegram data → Contacts list (JSON).", has("Telegram"), "Choose File…", u.importFile},
		{"Facebook / Snapchat friends", "Their data downloads (JSON); names are matched to people you have.", has("Facebook", "Snapchat"), "Choose File…", u.importFile},
		{"Calendar", "Links meetings to the people in them.", len(u.cal.Events) > 0, "Read…", u.calendarDialog},
		{"Clean up", "Fix numbers, emails and notes across everyone.", !u.cfg.LastCleanup.IsZero(), "Clean Up…", u.cleanup},
		{"Look for duplicates", "Merge anyone who ended up on two cards.", !u.cfg.LastDedupe.IsZero(), "Look…", u.duplicates},
	}
	box := container.NewVBox()
	done, marked := 0, false
	for i, st := range steps {
		st := st
		icon := widget.NewIcon(theme.CheckButtonIcon())
		if st.done {
			icon.SetResource(theme.CheckButtonCheckedIcon())
			done++
		}
		title := widget.NewLabelWithStyle(fmt.Sprintf("%d. %s", i+1, st.title), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		how := caption(st.how)
		how.Wrapping = fyne.TextWrapWord
		btn := widget.NewButton(st.action, func() { d.Hide(); st.run() })
		if !st.done && !marked { // the next step to do
			btn.Importance = widget.HighImportance
			marked = true
		}
		box.Add(container.NewBorder(nil, nil, icon, container.NewVBox(btn), container.NewVBox(title, how)))
	}
	intro := widget.NewLabel(fmt.Sprintf("Bring all your contacts into one place, one source at a time — %d of %d done. Skip any you don't use. People you already have are matched and updated, never duplicated, and every step can be undone.", done, len(steps)))
	intro.Wrapping = fyne.TextWrapWord
	d = dialog.NewCustom("Bring Everything In", "Close", container.NewBorder(intro, nil, nil, nil, container.NewVScroll(box)), u.win)
	d.Resize(fyne.NewSize(720, 640))
	d.Show()
}

// backupTo writes a .zip of everything (contacts, photos, business cards, calendar, backups)
// to a folder the user picks — an external drive, for example.
func (u *ui) backupTo() {
	d := dialog.NewFolderOpen(func(dir fyne.ListableURI, err error) {
		if err != nil || dir == nil {
			return
		}
		write := func() {
			name := filepath.Join(dir.Path(), "Smart Contacts backup "+time.Now().Format("2006-01-02 1504")+".zip")
			if err := zipDataDir(u.store.Dir(), name); err != nil {
				showErr(err, u.win)
				return
			}
			u.flash("Backed up to " + name)
		}
		if InCloudFolder(dir.Path()) {
			u.confirmCB("This folder syncs to the cloud", "A backup there leaves this Mac. Use an external drive or a local folder to keep it private. Save there anyway?", "Save Anyway", false, func(ok bool) {
				if ok {
					write()
				}
			}, u.win)
			return
		}
		write()
	}, u.win)
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

// zipDataDir zips the data folder. The settings file goes in without the OpenRouter key.
func zipDataDir(src, dest string) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	z := zip.NewWriter(f)
	err = filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if strings.HasPrefix(filepath.Base(p), ".tmp-") {
			return nil
		}
		w, err := z.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if rel == "config.json" {
			var cfg map[string]any
			b, _ := os.ReadFile(p)
			_ = json.Unmarshal(b, &cfg)
			delete(cfg, "openRouterKey")
			b, _ = json.MarshalIndent(cfg, "", "  ")
			_, err = w.Write(b)
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(w, in)
		return err
	})
	if err != nil {
		z.Close()
		os.Remove(dest)
		return err
	}
	return z.Close()
}
