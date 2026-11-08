package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// appVersion is the running version ("1.0.18"), set by the build.
func (u *ui) appVersion() string {
	return fyne.CurrentApp().Metadata().Version
}

// checkForUpdates looks for a newer build and, if there is one, offers to install it.
func (u *ui) checkForUpdates() {
	wait := dialog.NewCustomWithoutButtons("Checking for updates…", widget.NewProgressBarInfinite(), u.win)
	wait.Show()
	go func() {
		r, err := FetchLatest()
		fyne.Do(func() {
			wait.Hide()
			if err != nil {
				dialog.ShowError(err, u.win)
				return
			}
			cur := u.appVersion()
			if !r.NewerThan(cur) {
				dialog.ShowInformation("You're up to date", fmt.Sprintf("Smart Contacts %s is the newest version.", cur), u.win)
				return
			}
			u.offerUpdate(r, cur)
		})
	}()
}

func (u *ui) offerUpdate(r *Release, cur string) {
	notes := releaseNotes(r.Notes)
	head := widget.NewLabel(fmt.Sprintf("Smart Contacts %s is available — you have %s.", r.Version, cur))
	head.Wrapping = fyne.TextWrapWord
	what := widget.NewRichTextFromMarkdown(notes)
	what.Wrapping = fyne.TextWrapWord
	info := caption("Your contacts aren't touched: they stay in their own folder. The app closes, updates and reopens by itself.")
	info.Wrapping = fyne.TextWrapWord
	body := container.NewBorder(head, info, nil, nil, container.NewVScroll(what))
	d := dialog.NewCustomConfirm("Update Available", "Update Now", "Later", body, func(ok bool) {
		if ok {
			u.leaveEditor(func() { u.installUpdate(r) }, nil)
		}
	}, u.win)
	d.Resize(fyne.NewSize(620, 460))
	d.Show()
}

// releaseNotes keeps the useful part of the release text: the change list, without the build
// line and commit sign-off lines.
func releaseNotes(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "Co-Authored-By:") || strings.HasPrefix(t, "Claude-Session:") ||
			strings.HasPrefix(t, "Built from ") {
			continue
		}
		out = append(out, l)
	}
	n := strings.TrimSpace(strings.Join(out, "\n"))
	if n == "" {
		return "Fixes and improvements."
	}
	return n
}

func (u *ui) installUpdate(r *Release) {
	bundle := appBundle()
	if bundle == "" {
		dialog.ShowInformation("Can't update this copy",
			"This copy isn't running as an installed app. Install with the command in the README:\n\n"+
				"curl -fsSL https://raw.githubusercontent.com/jamalxcode/smart_contacts/main/install.sh | bash", u.win)
		return
	}
	if !canWrite(filepath.Dir(bundle)) {
		dialog.ShowInformation("Can't update here",
			"Your user can't change "+filepath.Dir(bundle)+". Update from Terminal with the install command in the README.", u.win)
		return
	}
	bar := widget.NewProgressBar()
	status := widget.NewLabel("Downloading " + r.Version + "…")
	prog := dialog.NewCustomWithoutButtons("Updating Smart Contacts", container.NewVBox(status, bar), u.win)
	prog.Resize(fyne.NewSize(420, 140))
	prog.Show()
	go func() {
		app, err := DownloadUpdate(func(p float64) { fyne.Do(func() { bar.SetValue(p) }) })
		fyne.Do(func() {
			if err != nil {
				prog.Hide()
				dialog.ShowError(err, u.win)
				return
			}
			status.SetText("Installing — the app will reopen in a moment…")
			if err := u.store.Save(); err != nil { // everything is saved already; this is belt and braces
				prog.Hide()
				dialog.ShowError(fmt.Errorf("couldn't save before updating: %w", err), u.win)
				return
			}
			if err := InstallAndRelaunch(app, bundle); err != nil {
				prog.Hide()
				dialog.ShowError(err, u.win)
				return
			}
			fyne.CurrentApp().Quit()
		})
	}()
}

// about shows the version and where the data is.
func (u *ui) about() {
	md := fyne.CurrentApp().Metadata()
	msg := fmt.Sprintf("Smart Contacts %s (build %d)\n\nA private address book — contacts stay on this Mac.\nData folder: %s",
		md.Version, md.Build, u.store.Dir())
	dialog.ShowCustomConfirm("About Smart Contacts", "Check for Updates…", "Close", widget.NewLabel(msg), func(ok bool) {
		if ok {
			u.checkForUpdates()
		}
	}, u.win)
}
