// Smart Contacts — a native Mac address book in Go (Fyne).
// Contacts are stored only on this Mac: ~/Library/Application Support/Smart Contacts.
package main

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func main() {
	a := app.NewWithID("company.sala.smartcontacts")
	a.Settings().SetTheme(newAppTheme())
	dir := DataDir()
	store, err := OpenStore(dir)
	if err != nil {
		w := a.NewWindow("Smart Contacts")
		w.Resize(fyne.NewSize(560, 240))
		w.SetContent(widget.NewLabel(" "))
		w.Show()
		dialog.ShowError(fmt.Errorf("couldn't open your contacts: %w", err), w)
		a.Run()
		os.Exit(1)
	}
	cfg := LoadConfig(dir)
	if len(os.Args) > 2 && os.Args[1] == "--dump-scripts" { // for the build check: write the macOS scripts out
		d := os.Args[2]
		_ = os.MkdirAll(d, 0o755)
		_ = os.WriteFile(d+"/contacts.applescript", []byte(macContactsScript), 0o644)
		_ = os.WriteFile(d+"/calendar.applescript", []byte(calendarScript(730, 180)), 0o644)
		_ = os.WriteFile(d+"/ocr.js", []byte(ocrScript), 0o644)
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "--read-card" { // for the build check: OCR an image and print the contact
		data, err := os.ReadFile(os.Args[2])
		if err == nil {
			var text string
			if text, err = ReadCardText(data); err == nil {
				c := ParseLocal(text, "965")
				fmt.Printf("TEXT:\n%s\nCONTACT: %s | %s | %s | %v | %v\n", text, c.FullName(), c.Title, c.Company, c.Phones, c.Emails)
			}
		}
		if err != nil {
			fmt.Println("ERROR:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "--import" { // smart_contacts --import file.vcf (handy for testing)
		if f, err := os.Open(os.Args[2]); err == nil {
			cards, _ := ParseVCards(f)
			f.Close()
			for _, c := range cards {
				if len(c.Photo) > 0 {
					_ = store.SetPhoto(c.Contact, ShrinkPhoto(c.Photo, 512))
				}
				store.Put(c.Contact)
			}
			_ = store.Save()
			fmt.Printf("imported %d contacts\n", len(cards))
		}
		return
	}
	u := newUI(a, store, cfg)
	if len(os.Args) > 2 && os.Args[1] == "--preview" { // open straight into an import preview
		if data, err := os.ReadFile(os.Args[2]); err == nil {
			name := os.Args[2]
			go func() { fyne.Do(func() { u.importData(name, data) }) }()
		}
	}
	u.win.ShowAndRun()
}
