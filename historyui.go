package main

import (
	"fmt"
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

// undo reverses the last change (an edit, delete, merge, import, clean-up…).
func (u *ui) undo() {
	if f, ok := u.win.Canvas().Focused().(fyne.Shortcutable); ok && u.ed != nil {
		f.TypedShortcut(&fyne.ShortcutUndo{}) // while typing, ⌘Z undoes the typing
		return
	}
	label, err := u.store.Undo()
	if err != nil {
		u.flash("Nothing to undo")
		return
	}
	u.afterRestore()
	u.flash("Undid: " + label + "   (⌘⇧Z to redo)")
	u.afterChange(u.store.UndoneChanges(), "Undid")
}

func (u *ui) redo() {
	if f, ok := u.win.Canvas().Focused().(fyne.Shortcutable); ok && u.ed != nil {
		f.TypedShortcut(&fyne.ShortcutRedo{})
		return
	}
	label, err := u.store.Redo()
	if err != nil {
		u.flash("Nothing to redo")
		return
	}
	u.afterRestore()
	u.flash("Redid: " + label)
	u.afterChange(u.store.LastChanges(), "Redid")
}

func (u *ui) afterRestore() {
	u.photoCache = map[string]fyne.Resource{}
	u.invalidate()
	if u.store.Get(u.selectedID) == nil {
		u.selectedID = ""
	}
	u.refreshAll()
	u.refreshMenu()
}

// historyDialog lists this session's changes (each can be undone back to) and the saved
// restore points (backups before imports, clean-ups and de-duplication, and daily backups).
func (u *ui) historyDialog() {
	var d *dialog.CustomDialog
	box := container.NewVBox()
	steps := u.store.History()
	box.Add(widget.NewLabelWithStyle("Changes this session", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	if len(steps) == 0 {
		box.Add(caption("No changes yet."))
	}
	for i, st := range steps {
		i, st := i, st
		btn := widget.NewButtonWithIcon("Undo back to here", theme.ContentUndoIcon(), func() {
			what := "this change"
			if i > 0 {
				what = fmt.Sprintf("this change and the %d after it", i)
			}
			u.confirmCB("Undo "+what+"?", "“"+st.Label+"” and everything after it will be reversed. Redo (⌘⇧Z) can bring them back.", "Undo", false, func(ok bool) {
				if !ok {
					return
				}
				if err := u.store.UndoTo(i); err != nil {
					showErr(err, u.win)
					return
				}
				u.afterRestore()
				d.Hide()
				u.flash("Undid back to before “" + st.Label + "”")
			}, u.win)
		})
		btn.Importance = widget.LowImportance
		box.Add(container.NewBorder(nil, nil, nil, btn, container.NewVBox(widget.NewLabel(st.Label), caption(st.Time.Format("15:04:05")))))
	}
	box.Add(widget.NewSeparator())
	box.Add(widget.NewLabelWithStyle("Restore points (saved on this Mac)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	box.Add(caption("Saved automatically before every import, clean-up and de-duplication, and once a day. Restoring replaces the whole address book with that copy — and can itself be undone."))
	pts := u.store.RestorePoints()
	if len(pts) == 0 {
		box.Add(caption("None yet."))
	}
	for i, p := range pts {
		if i == 40 {
			box.Add(caption(fmt.Sprintf("…and %d older ones in the backups folder", len(pts)-40)))
			break
		}
		p := p
		fi, _ := os.Stat(p)
		when := ""
		if fi != nil {
			when = fi.ModTime().Format("2 Jan 2006 · 15:04")
		}
		btn := widget.NewButtonWithIcon("Restore", theme.HistoryIcon(), func() {
			u.confirmCB("Restore this copy?", "Your address book will be replaced with the copy from "+when+". You can undo this.", "Restore", false, func(ok bool) {
				if !ok {
					return
				}
				if err := u.store.RestoreFile(p); err != nil {
					showErr(err, u.win)
					return
				}
				u.afterRestore()
				d.Hide()
				u.flash("Restored the copy from " + when + " (⌘Z to undo)")
			}, u.win)
		})
		btn.Importance = widget.LowImportance
		box.Add(container.NewBorder(nil, nil, nil, btn, container.NewVBox(widget.NewLabel(restoreName(p)), caption(when))))
	}
	d = dialog.NewCustom("History", "Close", container.NewVScroll(box), u.win)
	d.Resize(fyne.NewSize(620, 600))
	d.Show()
}

func restoreName(p string) string {
	b := strings.TrimSuffix(filepath.Base(p), ".json")
	for prefix, name := range map[string]string{"before-import-": "Before an import", "before-cleanup-": "Before a clean-up",
		"before-dedupe-": "Before de-duplicating", "before-restore-": "Before a restore", "contacts-": "Daily backup"} {
		if strings.HasPrefix(b, prefix) {
			return name
		}
	}
	return b
}

var _ = time.Now
