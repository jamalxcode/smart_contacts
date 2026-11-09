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
	var d *paperDialog
	type step struct {
		title, how string
		done       bool
		action     string
		run        func()
	}
	steps := []step{
		{"Mac & iPhone contacts", "Everything in the Mac's Contacts app, including your iPhone through iCloud.", has("Mac Contacts"), "Import", u.importMac},
		{"Google / Gmail", "Download them from contacts.google.com — step by step.", has("Google Contacts", "Gmail"), "Show Me How…", func() { u.importGuide("Gmail / Google") }},
		{"Outlook / work email", "Download them from Outlook's People page — step by step.", has("Outlook"), "Show Me How…", func() { u.importGuide("Outlook / Hotmail") }},
		{"LinkedIn", "Ask LinkedIn for your data archive, then bring in Connections.csv — step by step.", has("LinkedIn"), "Show Me How…", func() { u.importGuide("LinkedIn") }},
		{"Other phones", "Android and other phones — through Google, or by sending the contacts file.", has("File:", "Android phone"), "Show Me How…", func() { u.importGuide("Android phone") }},
		{"Telegram", "Export your contacts from Telegram Desktop — step by step.", has("Telegram"), "Show Me How…", func() { u.importGuide("Telegram") }},
		{"Facebook / Snapchat friends", "Their data downloads; names are matched to people you have.", has("Facebook", "Snapchat"), "Show Me How…", func() { u.importGuide("Facebook or Snapchat friends") }},
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
	d = newCustom("Bring Everything In", "Close", container.NewBorder(intro, nil, nil, nil, container.NewVScroll(box)), u.win)
	d.Resize(fyne.NewSize(720, 640))
	d.Show()
}

// backupTo writes a .zip of everything (contacts, photos, business cards, calendar, backups)
// to a folder the user picks — an external drive, for example.
func (u *ui) backupTo() {
	pw1, pw2 := newPasswordEntry(), newPasswordEntry()
	pw1.SetPlaceHolder("Optional")
	pw2.SetPlaceHolder("Type it again")
	info := caption("With a password the backup is encrypted (AES-256): safe on a USB stick or another drive. " +
		"Without the password it can't be opened — keep it somewhere safe; it isn't stored anywhere.")
	info.Wrapping = fyne.TextWrapWord
	form := newForm("Back Up To…", "Choose Folder…", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Password", pw1),
		widget.NewFormItem("Confirm", pw2),
		widget.NewFormItem("", info),
	}, func(ok bool) {
		if !ok {
			return
		}
		if pw1.Text != pw2.Text {
			showInfo("Passwords don't match", "Type the same password twice, or leave both empty for a plain backup.", u.win)
			return
		}
		u.chooseBackupFolder(pw1.Text)
	}, u.win)
	form.Resize(fyne.NewSize(520, 0))
	form.Show()
}

func (u *ui) chooseBackupFolder(password string) {
	d := dialog.NewFolderOpen(func(dir fyne.ListableURI, err error) {
		if err != nil || dir == nil {
			return
		}
		write := func() {
			base := filepath.Join(dir.Path(), "Smart Contacts backup "+time.Now().Format("2006-01-02 1504"))
			if password == "" {
				if err := zipDataDir(u.store.Dir(), base+".zip"); err != nil {
					showErr(err, u.win)
					return
				}
				u.flash("Backed up to " + base + ".zip")
				return
			}
			plain, err := zipDataBytes(u.store.Dir())
			if err == nil {
				var sealed []byte
				if sealed, err = EncryptBackup(plain, password); err == nil {
					err = os.WriteFile(base+".scbackup", sealed, 0o600)
				}
			}
			if err != nil {
				showErr(err, u.win)
				return
			}
			u.flash("Encrypted backup saved to " + base + ".scbackup")
		}
		if InCloudFolder(dir.Path()) && password == "" {
			u.confirmCB("This folder syncs to the cloud", "A backup there leaves this Mac. Use an external drive, add a password, or save there anyway?", "Save Anyway", false, func(ok bool) {
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

// restoreBackup brings back a backup made with Back Up To… (plain .zip or encrypted .scbackup).
func (u *ui) restoreBackup() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil || r == nil {
			return
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			showErr(err, u.win)
			return
		}
		if IsEncryptedBackup(data) {
			pw := newPasswordEntry()
			f := newForm("Encrypted Backup", "Open", "Cancel", []*widget.FormItem{widget.NewFormItem("Password", pw)}, func(ok bool) {
				if !ok {
					return
				}
				plain, err := DecryptBackup(data, pw.Text)
				if err != nil {
					showErr(err, u.win)
					return
				}
				u.confirmRestore(plain)
			}, u.win)
			f.Resize(fyne.NewSize(420, 0))
			f.Show()
			u.win.Canvas().Focus(pw)
			return
		}
		u.confirmRestore(data)
	}, u.win)
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

func (u *ui) confirmRestore(zipBytes []byte) {
	u.confirmCB("Restore this backup?", "Your address book will be replaced with the one in the backup. Photos, business cards and calendar log come back too; your settings stay. "+
		"A copy of the current address book is saved first, and ⌘Z undoes the restore.", "Restore", false, func(ok bool) {
		if !ok {
			return
		}
		path, cleanup, err := RestoreFiles(zipBytes, u.store.Dir())
		if err != nil {
			showErr(err, u.win)
			return
		}
		defer cleanup()
		if err := u.store.RestoreFile(path); err != nil {
			showErr(err, u.win)
			return
		}
		u.cal = LoadCalendar(u.store.Dir())
		u.act = LoadActivity(u.store.Dir())
		u.photoCache = map[string]fyne.Resource{}
		u.invalidate()
		u.refreshAll()
		u.flash(fmt.Sprintf("Restored %d contacts from the backup", len(u.store.Contacts)))
	}, u.win)
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
