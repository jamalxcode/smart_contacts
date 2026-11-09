package main

import (
	"context"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (u *ui) settings() {
	cc := newCardEntry()
	cc.SetText(u.cfg.CountryCode)
	sortBy := widget.NewRadioGroup([]string{"First name", "Last name"}, nil)
	sortBy.Horizontal = true
	sortBy.SetSelected(map[bool]string{false: "First name", true: "Last name"}[u.cfg.SortByLast])
	key := newPasswordEntry()
	key.SetText(u.cfg.OpenRouterKey)
	key.SetPlaceHolder("sk-or-… (optional)")
	model := widget.NewSelectEntry([]string{DefaultAIModel, "anthropic/claude-sonnet-4.5", "openai/gpt-5-mini", "google/gemini-2.5-flash"})
	model.SetText(u.cfg.AIModel)
	allow := widget.NewCheck("Allow Smart Add to send pasted text to OpenRouter (it leaves this Mac)", nil)
	allow.SetChecked(u.cfg.AllowCloudAI)
	test := widget.NewButton("Test AI", nil)
	syncAI := func(on bool) {
		for _, w := range []fyne.Disableable{key, model, test} {
			if on {
				w.Enable()
			} else {
				w.Disable()
			}
		}
	}
	allow.OnChanged = syncAI
	syncAI(u.cfg.AllowCloudAI)
	result := caption("")
	result.Wrapping = fyne.TextWrapWord
	test.OnTapped = func() {
		result.SetText("Testing…")
		k, m, c := key.Text, model.Text, strings.TrimSpace(cc.Text)
		go func() {
			got, _, err := ParseAI(context.Background(), k, m, "Jane Doe, Head of Design at Example Co, +1 415 555 0100", c)
			fyne.Do(func() {
				switch {
				case err != nil:
					result.SetText("Didn't work: " + err.Error())
				case got.First == "":
					result.SetText("The model answered but missed the name; try another model.")
				default:
					result.SetText("Works — it read “" + got.DisplayName() + ", " + got.Title + "”.")
				}
			})
		}()
	}
	keyNote := caption("Off (default): Smart Add reads text on this Mac only and the app never uses the network. " +
		"The key is stored in config.json in the data folder (not the Keychain), readable only by your user.")
	keyNote.Wrapping = fyne.TextWrapWord
	folder := widget.NewButtonWithIcon("Show Data Folder", theme.FolderOpenIcon(), func() { RevealInFinder(u.store.Dir() + "/contacts.json") })
	autoAct := widget.NewCheck("Update “last talked” from Messages, calls and Mail when the app opens", nil)
	autoAct.SetChecked(u.cfg.AutoActivity)
	notify := widget.NewCheck("Remind me of birthdays and people to reach out to", nil)
	notify.SetChecked(!u.cfg.NoNotify)
	coverChk := widget.NewCheck("Show the closed organiser when the app opens", nil)
	coverChk.SetChecked(!u.cfg.NoCover)
	openAt := widget.NewRadioGroup([]string{"The last page I was on", "The first page (All Contacts)"}, nil)
	openAt.SetSelected(map[bool]string{false: "The last page I was on", true: "The first page (All Contacts)"}[u.cfg.OpenAtFirst])
	// Laid out like a Mac settings window: titled groups on a light panel, the whole thing
	// scrolling, and Cancel / Save always in their own row at the bottom.
	section := func(title string, items ...*widget.FormItem) fyne.CanvasObject {
		head := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		bg := canvas.NewRectangle(color.NRGBA{0xFF, 0xFF, 0xFF, 0x80})
		bg.CornerRadius = 10
		bg.StrokeColor = color.NRGBA{0, 0, 0, 0x18}
		bg.StrokeWidth = 1
		return container.NewVBox(head, container.NewStack(bg, container.NewPadded(widget.NewForm(items...))))
	}
	privacy := caption(privacyText(u.store.Dir()))
	privacy.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(
		section("General",
			widget.NewFormItem("Your country code", cc),
			widget.NewFormItem("Sort by", sortBy),
			widget.NewFormItem("History", autoAct),
			widget.NewFormItem("Reminders", notify)),
		section("Organiser",
			widget.NewFormItem("When the app opens", coverChk),
			widget.NewFormItem("Open at", openAt)),
		section("Cloud AI",
			widget.NewFormItem("Cloud AI", allow),
			widget.NewFormItem("OpenRouter key", key),
			widget.NewFormItem("AI model", model),
			widget.NewFormItem("", container.NewVBox(keyNote, container.NewHBox(test, result)))),
		section("Privacy",
			widget.NewFormItem("", container.NewVBox(privacy, container.NewHBox(folder, layout.NewSpacer())))),
	)
	d := newConfirm("Settings", "Save", "Cancel", container.NewVScroll(container.NewPadded(content)), func(ok bool) {
		if !ok {
			return
		}
		if code := digitsOnly(cc.Text); code != "" && len(code) <= 4 {
			u.cfg.CountryCode = code
		} else {
			showInfo("Country code not changed", "The country code should be digits only, like 965. The other settings were saved.", u.win)
		}
		u.cfg.SortByLast = sortBy.Selected == "Last name"
		u.cfg.OpenRouterKey = strings.TrimSpace(key.Text)
		u.cfg.AllowCloudAI = allow.Checked
		u.cfg.AutoActivity = autoAct.Checked
		u.cfg.NoNotify = !notify.Checked
		u.cfg.NoCover = !coverChk.Checked
		u.cfg.OpenAtFirst = openAt.Selected == "The first page (All Contacts)"
		u.cfg.AIModel = strings.TrimSpace(model.Text)
		if u.cfg.AIModel == "" {
			u.cfg.AIModel = DefaultAIModel
		}
		if err := u.cfg.Save(u.store.Dir()); err != nil {
			showErr(err, u.win)
		}
		u.refreshAll()
		u.flash("Settings saved")
	}, u.win)
	d.SetConfirmImportance(widget.HighImportance)
	trackDialog(d, d.Confirm)
	d.Resize(fyne.NewSize(700, 640))
	d.Show()
}
