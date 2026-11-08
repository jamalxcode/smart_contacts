package main

import (
	_ "embed"
	"fmt"
	"image/color"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Bringing contacts in is a short conversation on a clipboard rather than a wall of choices:
//
//  1. Where are your contacts now? (iPhone, Android, Gmail, Outlook, LinkedIn, a spreadsheet…)
//  2. Sometimes one more question (is the iPhone on this Mac's iCloud? how will you send the file?)
//  3. Numbered steps for what to do on that side, what you'll get, how to send it here, and the
//     button that takes it in.
//
// Every guide ends on the same promise: it's all read on this Mac, people you already have are
// updated rather than duplicated, and a backup is saved first.

//go:embed assets/import_funnel.png
var importFunnelPNG []byte

//go:embed assets/import_funnel_menu.png
var importFunnelMenuPNG []byte

var (
	resImportFunnel = fyne.NewStaticResource("import_funnel.png", importFunnelPNG)
	resImportMenu   = fyne.NewStaticResource("import_funnel_menu.png", importFunnelMenuPNG)
)

// ---------- what the wizard knows ----------

// wizQuestion is a page with a question and its answers.
type wizQuestion struct {
	ask     string
	note    string
	answers []wizAnswer
}

type wizAnswer struct {
	label, detail string
	icon          fyne.Resource
	next          *wizQuestion // another question…
	guide         *wizGuide    // …or the steps
}

// wizGuide is the numbered steps for one source, ending in the button that imports.
type wizGuide struct {
	title  string
	steps  []string
	expect string // what you'll have at the end
	link   string // a web page to open for step 1, if any
	linkTo string // its button's words
	finish wizFinish
}

type wizFinish int

const (
	finishFile    wizFinish = iota // choose the file (opens in Downloads)
	finishMac                      // read the Mac's Contacts app
	finishReceive                  // wait for a phone to send the file
	finishPaste                    // paste text
	finishNone                     // nothing to import (explains why)
)

func (u *ui) importQuestions() *wizQuestion {
	sendAsk := func(made string) *wizQuestion {
		return &wizQuestion{
			ask:  "How will you send the file to this Mac?",
			note: made,
			answers: []wizAnswer{
				{label: "Email it to myself", detail: "the surest way: email, WhatsApp, Google Drive…", icon: theme.MailSendIcon(), guide: &wizGuide{
					title: "Send it to yourself",
					steps: []string{
						"On the phone, share the contacts file by email (or a chat you can open on this Mac).",
						"On this Mac, open the message and save the attachment — it goes to Downloads.",
						"Press Choose File below and pick it.",
					},
					expect: "A .vcf file in Downloads.",
					finish: finishFile,
				}},
				{label: "AirDrop", detail: "iPhone, or an Android that can share to AirDrop", icon: theme.UploadIcon(), guide: &wizGuide{
					title: "Send it with AirDrop",
					steps: []string{
						"On the phone, tap Share on the contacts file and choose AirDrop.",
						"Pick this Mac. Accept it here if the Mac asks.",
						"The file lands in your Downloads folder.",
						"Press Choose File below — it opens in Downloads — and pick the file.",
					},
					expect: "A .vcf file (a vCard) in Downloads.",
					finish: finishFile,
				}},
				{label: "Bluetooth", detail: "needs one setting on this Mac first", icon: theme.MediaFastForwardIcon(), guide: &wizGuide{
					title: "Send it over Bluetooth",
					steps: []string{
						"Let this Mac receive files: System Settings → General → Sharing, click ⓘ next to Bluetooth Sharing and turn it on. (Open Sharing Settings below goes straight there.)",
						"In the same place set When receiving items to Accept and Save (or Ask What to Do), and the folder to Downloads. Click Done.",
						"Pair the phone with this Mac if it isn't already (System Settings → Bluetooth).",
						"Press Start Waiting below. The app watches Downloads for the file.",
						"On the phone, share the contacts file, choose Bluetooth, then this Mac. Accept it on the Mac if asked — it opens here by itself.",
					},
					expect: "The app finds the .vcf file as soon as it lands in Downloads. If the phone says the Mac refused it, Bluetooth Sharing is still off.",
					link:   "x-apple.systempreferences:com.apple.Sharing-Settings.extension", linkTo: "Open Sharing Settings",
					finish: finishReceive,
				}},
			},
		}
	}

	iphone := &wizQuestion{
		ask:  "Is that iPhone signed in to the same Apple Account as this Mac, with Contacts on in iCloud?",
		note: "On the iPhone: Settings → your name → iCloud → Contacts.",
		answers: []wizAnswer{
			{label: "Yes, same Apple Account", icon: theme.ConfirmIcon(), guide: &wizGuide{
				title: "Bring in this Mac's contacts",
				steps: []string{
					"Nothing to do on the iPhone: its contacts are already on this Mac through iCloud.",
					"Press Import Now below.",
					"If macOS asks whether Smart Contacts may read your contacts, press Allow (it only reads them, once).",
				},
				expect: "Everyone from the iPhone, plus any Google or Exchange accounts added on this Mac.",
				finish: finishMac,
			}},
			{label: "No, or I'm not sure", icon: theme.QuestionIcon(), guide: &wizGuide{
				title: "Export them from iCloud on the web",
				steps: []string{
					"Open icloud.com/contacts and sign in with the iPhone's Apple Account.",
					"Click a contact, then press ⌘A to select everyone.",
					"Click the ⋯ (or the gear) and choose Export vCard.",
					"The file goes to Downloads. Press Choose File below and pick it.",
				},
				expect: "One .vcf file with everyone in it.",
				link:   "https://www.icloud.com/contacts", linkTo: "Open icloud.com",
				finish: finishFile,
			}},
		},
	}

	google := &wizGuide{
		title: "Export from Google Contacts",
		steps: []string{
			"Open contacts.google.com and sign in.",
			"Click Export (on the left, or under ⋮ at the top).",
			"Choose All contacts and the format Google CSV, then press Export.",
			"The file goes to Downloads. Press Choose File below and pick it.",
		},
		expect: "A file called contacts.csv.",
		link:   "https://contacts.google.com", linkTo: "Open Google Contacts",
		finish: finishFile,
	}
	// Most Android phones keep their contacts in the Google account: then there's nothing to send
	android := &wizQuestion{
		ask:  "Does that phone keep its contacts in your Google account?",
		note: "Most Android phones do. On the phone: Contacts → ☰ or your picture → the account shown is where they're saved.",
		answers: []wizAnswer{
			{label: "Yes, in Google", detail: "the easiest way — nothing to send", icon: theme.ConfirmIcon(), guide: google},
			{label: "No, or I'm not sure", detail: "make a file on the phone and send it", icon: theme.QuestionIcon(),
				next: sendAsk("First make the file: in the Contacts app, ☰ or ⋮ → Settings (or Manage contacts) → Export → Export to .vcf file. It saves to the phone's storage.")},
		},
	}

	return &wizQuestion{
		ask:  "Where are the contacts you want to bring in?",
		note: "Everything is read on this Mac. People you already have are updated, not duplicated, and a backup is saved first.",
		answers: []wizAnswer{
			{label: "iPhone", detail: "or this Mac's Contacts app", icon: iconPhone, next: iphone},
			{label: "Android phone", detail: "Samsung, Pixel, Huawei, Xiaomi…", icon: iconPhone, next: android},
			{label: "Gmail / Google", icon: theme.MailComposeIcon(), guide: google},
			{label: "Outlook / Hotmail", icon: theme.MailComposeIcon(), guide: &wizGuide{
				title: "Export from Outlook",
				steps: []string{
					"Open outlook.com/people and sign in.",
					"Choose Manage contacts (or ⋯) → Export contacts.",
					"Pick All contacts and press Export.",
					"The file goes to Downloads. Press Choose File below and pick it.",
				},
				expect: "A file called contacts.csv.",
				link:   "https://outlook.live.com/people", linkTo: "Open Outlook People",
				finish: finishFile,
			}},
			{label: "LinkedIn", icon: iconLink, guide: &wizGuide{
				title: "Download your LinkedIn connections",
				steps: []string{
					"Open LinkedIn → Me → Settings & Privacy → Data privacy → Get a copy of your data.",
					"Choose Connections only, then Request archive.",
					"LinkedIn emails you a link, usually within ten minutes (sometimes a day).",
					"Download the archive from the email and open it: inside is Connections.csv.",
					"Press Choose File below and pick Connections.csv.",
				},
				expect: "Names, companies, job titles and emails (where people share them).",
				link:   "https://www.linkedin.com/mypreferences/d/download-my-data", linkTo: "Open LinkedIn",
				finish: finishFile,
			}},
			{label: "Another email account", detail: "Yahoo, iCloud web, work mail…", icon: theme.MailComposeIcon(), guide: &wizGuide{
				title: "Export from your email",
				steps: []string{
					"Sign in to the email account on the web and open its contacts or address book.",
					"Look for Export (often under ⋯ or Settings) and choose vCard or CSV.",
					"The file goes to Downloads. Press Choose File below and pick it.",
				},
				expect: "A .vcf or .csv file.",
				finish: finishFile,
			}},
			{label: "A spreadsheet", detail: "Excel, Numbers or CSV", icon: theme.GridIcon(), guide: &wizGuide{
				title: "Bring in a spreadsheet",
				steps: []string{
					"Keep one person per row, with headings in the first row (Name, Phone, Email… in English or Arabic).",
					"From Numbers, use File → Export To → CSV. Excel files (.xlsx) work as they are.",
					"Press Choose File below and pick it.",
					"Check the columns are matched the way you want, then import.",
				},
				expect: "A window that shows which column is which before anything is added.",
				finish: finishFile,
			}},
			{label: "A note or some text", detail: "a list from Notes, WhatsApp, an email…", icon: theme.DocumentIcon(), guide: &wizGuide{
				title: "Bring in a list of people",
				steps: []string{
					"Put one person per line (\"Ali 9999 8888\"), or separate people with an empty line.",
					"Copy the text, then press Paste Text below — or save it as a .txt file and use Choose File.",
					"Check the people found, then import.",
				},
				expect: "Each line read as a name with its number or email.",
				finish: finishPaste,
			}},
			{label: "Telegram", icon: iconMessage, guide: &wizGuide{
				title: "Export your Telegram contacts",
				steps: []string{
					"Open Telegram Desktop on this Mac (the phone app can't export).",
					"Settings → Advanced → Export Telegram data.",
					"Tick only Contacts list, choose the format Machine-readable JSON, and export.",
					"Press Choose File below and pick result.json from the export folder.",
				},
				expect: "Your Telegram contacts with their phone numbers.",
				finish: finishFile,
			}},
			{label: "Facebook or Snapchat friends", icon: iconPeople, guide: &wizGuide{
				title: "Download your friends list",
				steps: []string{
					"Facebook: Settings → Download your information → choose Friends, format JSON. Snapchat: Settings → My Data.",
					"When the download is ready, open it and find your_friends.json (Facebook) or friends.json (Snapchat).",
					"Press Choose File below and pick it.",
				},
				expect: "Names only, so they're matched to people you already have (they don't make new cards).",
				finish: finishFile,
			}},
			{label: "Instagram or X", icon: iconLink, guide: &wizGuide{
				title: "Instagram and X can't be imported",
				steps: []string{
					"Their exports list only usernames or account numbers — nothing to match a person by.",
					"Instead, add a person's profile link on their card: double-click the card and add a link.",
				},
				expect: "",
				finish: finishNone,
			}},
		},
	}
}

// ---------- the wizard window ----------

// importHub opens the import wizard (File → Import…, and the funnel in the organiser).
func (u *ui) importHub() {
	var d *paperDialog
	page := container.NewStack()
	var history []func()
	backBtn := newPaperButton("Back", btnPlain, nil)
	closeBtn := newPaperButton("Close", btnPlain, func() { d.Hide() })
	var showQ func(q *wizQuestion)
	var showG func(g *wizGuide)
	set := func(o fyne.CanvasObject) {
		page.Objects = []fyne.CanvasObject{o}
		page.Refresh()
		if len(history) > 1 {
			backBtn.Show()
		} else {
			backBtn.Hide()
		}
	}
	backBtn.onTap = func() {
		if len(history) > 1 {
			history = history[:len(history)-1]
			history[len(history)-1]()
		}
	}
	go1 := func(f func()) {
		history = append(history, f)
		f()
	}
	showQ = func(q *wizQuestion) {
		box := container.NewVBox()
		ask := widget.NewLabelWithStyle(q.ask, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		ask.Wrapping = fyne.TextWrapWord
		box.Add(ask)
		if q.note != "" {
			n := caption(q.note)
			n.Wrapping = fyne.TextWrapWord
			box.Add(n)
		}
		grid := container.NewGridWithColumns(2)
		for _, a := range q.answers {
			a := a
			grid.Add(newAnswerCard(a.label, a.detail, a.icon, func() {
				if a.next != nil {
					go1(func() { showQ(a.next) })
				} else if a.guide != nil {
					go1(func() { showG(a.guide) })
				}
			}))
		}
		box.Add(grid)
		set(container.NewVScroll(box))
	}
	showG = func(g *wizGuide) {
		box := container.NewVBox()
		t := widget.NewLabelWithStyle(g.title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		t.Wrapping = fyne.TextWrapWord
		box.Add(t)
		for i, s := range g.steps {
			box.Add(newNumberedStep(i+1, s))
		}
		if g.expect != "" {
			e := caption("What you'll have: " + g.expect)
			e.Wrapping = fyne.TextWrapWord
			box.Add(e)
		}
		var acts []fyne.CanvasObject
		if g.link != "" {
			link := g.link
			acts = append(acts, newPaperButton(g.linkTo, btnPlain, func() {
				if p, err := url.Parse(link); err == nil {
					_ = u.app.OpenURL(p)
				}
			}))
		}
		switch g.finish {
		case finishFile:
			acts = append(acts, newPaperButton("Choose File…", btnPrimary, func() { d.Hide(); u.importFileFrom(downloadsDir()) }))
		case finishMac:
			acts = append(acts, newPaperButton("Import Now", btnPrimary, func() { d.Hide(); u.importMac() }))
		case finishReceive:
			acts = append(acts, newPaperButton("Start Waiting", btnPrimary, func() { d.Hide(); u.receiveFromPhone() }))
		case finishPaste:
			acts = append(acts,
				newPaperButton("Choose File…", btnPlain, func() { d.Hide(); u.importFileFrom(downloadsDir()) }),
				newPaperButton("Paste Text…", btnPrimary, func() { d.Hide(); u.pasteImport() }))
		}
		if len(acts) > 0 {
			box.Add(newCardGap(6))
			box.Add(container.NewHBox(append([]fyne.CanvasObject{layout.NewSpacer()}, acts...)...))
		}
		set(container.NewVScroll(box))
	}
	go1(func() { showQ(u.importQuestions()) })

	funnel := canvas.NewImageFromResource(resImportFunnel)
	funnel.FillMode = canvas.ImageFillContain
	funnel.SetMinSize(fyne.NewSize(64, 64))
	content := container.NewBorder(nil, nil, container.NewVBox(funnel), nil, page)
	d = newCustomWithoutButtons("Import Contacts", content, u.win)
	d.clipboard = true // you're answering questions
	d.SetButtons([]fyne.CanvasObject{backBtn, closeBtn})
	d.Resize(fyne.NewSize(700, 560))
	d.Show()
}

func downloadsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Downloads")
}

// importFileFrom picks a file to import, starting in dir (Downloads, where exports land).
func (u *ui) importFileFrom(dir string) {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil || r == nil {
			return
		}
		defer r.Close()
		b, err := io.ReadAll(io.LimitReader(r, 512<<20))
		if err != nil {
			showErr(err, u.win)
			return
		}
		u.importData(r.URI().Name(), b)
	}, u.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".vcf", ".vcard", ".csv", ".tsv", ".xlsx", ".txt", ".json", ".ics", ".VCF", ".CSV", ".XLSX", ".TXT", ".JSON"}))
	if dir != "" {
		if l, err := storage.ListerForURI(storage.NewFileURI(dir)); err == nil {
			d.SetLocation(l)
		}
	}
	d.Resize(fyne.NewSize(760, 520))
	d.Show()
}

// ---------- pieces ----------

// answerCard is one answer: a small card with an icon, the answer and a line under it.
type answerCard struct {
	widget.BaseWidget
	label, detail string
	icon          fyne.Resource
	onTap         func()
	hover         bool
}

func newAnswerCard(label, detail string, icon fyne.Resource, onTap func()) *answerCard {
	a := &answerCard{label: label, detail: detail, icon: icon, onTap: onTap}
	a.ExtendBaseWidget(a)
	return a
}
func (a *answerCard) Tapped(*fyne.PointEvent)        { a.onTap() }
func (a *answerCard) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (a *answerCard) MouseIn(*desktop.MouseEvent)    { a.hover = true; a.Refresh() }
func (a *answerCard) MouseMoved(*desktop.MouseEvent) {}
func (a *answerCard) MouseOut()                      { a.hover = false; a.Refresh() }

func (a *answerCard) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.NRGBA{0xFF, 0xFD, 0xF6, 0xFF})
	bg.CornerRadius = 8
	bg.StrokeWidth = 1
	bg.StrokeColor = color.NRGBA{0xC8, 0xBA, 0x9C, 0xFF}
	shadow := canvas.NewRectangle(color.NRGBA{0x40, 0x28, 0x10, 0x30})
	shadow.CornerRadius = 8
	var icon *canvas.Image
	if a.icon != nil {
		icon = canvas.NewImageFromResource(theme.NewColoredResource(a.icon, theme.ColorNameForeground))
		icon.FillMode = canvas.ImageFillContain
	}
	l := canvas.NewText(a.label, cardBlack)
	l.TextSize = 14
	l.TextStyle.Bold = true
	dt := canvas.NewText(a.detail, color.NRGBA{0x6A, 0x5E, 0x4C, 0xFF})
	dt.TextSize = 11
	r := &answerRenderer{a: a, bg: bg, shadow: shadow, icon: icon, l: l, d: dt}
	r.Refresh()
	return r
}

type answerRenderer struct {
	a          *answerCard
	bg, shadow *canvas.Rectangle
	icon       *canvas.Image
	l, d       *canvas.Text
}

func (r *answerRenderer) Layout(s fyne.Size) {
	lift := float32(0)
	if r.a.hover {
		lift = -1
	}
	r.shadow.Move(fyne.NewPos(1, 3))
	r.shadow.Resize(s)
	r.bg.Move(fyne.NewPos(0, lift))
	r.bg.Resize(s)
	x := float32(12)
	if r.icon != nil {
		r.icon.Move(fyne.NewPos(12, s.Height/2-11+lift))
		r.icon.Resize(fyne.NewSize(22, 22))
		x = 44
	}
	if r.a.detail == "" {
		r.l.Move(fyne.NewPos(x, (s.Height-r.l.MinSize().Height)/2+lift))
	} else {
		r.l.Move(fyne.NewPos(x, 8+lift))
		r.d.Move(fyne.NewPos(x, 28+lift))
	}
}
func (r *answerRenderer) MinSize() fyne.Size {
	w := fyne.Max(r.l.MinSize().Width, r.d.MinSize().Width) + 56
	return fyne.NewSize(w, 50)
}
func (r *answerRenderer) Refresh() {
	if r.a.hover {
		r.bg.FillColor = color.NRGBA{0xFF, 0xF4, 0xD8, 0xFF}
		r.bg.StrokeColor = color.NRGBA{0xA8, 0x88, 0x50, 0xFF}
	} else {
		r.bg.FillColor = color.NRGBA{0xFF, 0xFD, 0xF6, 0xFF}
		r.bg.StrokeColor = color.NRGBA{0xC8, 0xBA, 0x9C, 0xFF}
	}
	r.bg.Refresh()
	r.Layout(r.a.Size())
}
func (r *answerRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.shadow, r.bg}
	if r.icon != nil {
		objs = append(objs, r.icon)
	}
	return append(objs, r.l, r.d)
}
func (r *answerRenderer) Destroy() {}

// newNumberedStep is "①  Do this." with the number in an ink circle.
func newNumberedStep(n int, text string) fyne.CanvasObject {
	ring := canvas.NewCircle(color.Transparent)
	ring.StrokeColor = color.NRGBA{0x8A, 0x32, 0x1E, 0xFF}
	ring.StrokeWidth = 1.6
	num := canvas.NewText(fmt.Sprint(n), color.NRGBA{0x8A, 0x32, 0x1E, 0xFF})
	num.TextSize = 13
	num.TextStyle = serif("1", true, false)
	num.Alignment = fyne.TextAlignCenter
	badge := container.New(&badgeLayout{}, ring, num)
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return container.NewBorder(nil, nil, container.NewVBox(badge), nil, l)
}

type badgeLayout struct{}

func (badgeLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(34, 34) }
func (badgeLayout) Layout(objs []fyne.CanvasObject, s fyne.Size) {
	objs[0].Move(fyne.NewPos(5, 7))
	objs[0].Resize(fyne.NewSize(24, 24))
	m := objs[1].MinSize()
	objs[1].Move(fyne.NewPos(5, 7+(24-m.Height)/2))
	objs[1].Resize(fyne.NewSize(24, m.Height))
}

// ---------- the funnel in the organiser ----------

// importFunnel is the Import button under the business-card pocket: the funnel with cards
// flying into it, and "Import Contacts" on a paper label.
type importFunnel struct {
	widget.BaseWidget
	onTap func()
	hover bool
}

func newImportFunnel(onTap func()) *importFunnel {
	f := &importFunnel{onTap: onTap}
	f.ExtendBaseWidget(f)
	return f
}
func (f *importFunnel) Tapped(*fyne.PointEvent)        { f.onTap() }
func (f *importFunnel) Cursor() desktop.Cursor         { return desktop.PointerCursor }
func (f *importFunnel) MouseIn(*desktop.MouseEvent)    { f.hover = true; f.Refresh() }
func (f *importFunnel) MouseMoved(*desktop.MouseEvent) {}
func (f *importFunnel) MouseOut()                      { f.hover = false; f.Refresh() }

func (f *importFunnel) CreateRenderer() fyne.WidgetRenderer {
	img := canvas.NewImageFromResource(resImportFunnel)
	img.FillMode = canvas.ImageFillContain
	plate := canvas.NewRectangle(color.NRGBA{0xFB, 0xF7, 0xEC, 0xFF})
	plate.CornerRadius = 4
	plate.StrokeColor = color.NRGBA{0xC8, 0xBC, 0xA4, 0xFF}
	plate.StrokeWidth = 1
	label := canvas.NewText("Import Contacts", cardBlack)
	label.TextSize = 12
	label.TextStyle = serif("x", true, false)
	label.Alignment = fyne.TextAlignCenter
	return &funnelRenderer{f: f, img: img, plate: plate, label: label}
}

type funnelRenderer struct {
	f     *importFunnel
	img   *canvas.Image
	plate *canvas.Rectangle
	label *canvas.Text
}

func (r *funnelRenderer) Layout(s fyne.Size) {
	lift := float32(0)
	if r.f.hover {
		lift = 4
	}
	is := s.Height - 26
	r.img.Move(fyne.NewPos((s.Width-is)/2, -lift))
	r.img.Resize(fyne.NewSize(is, is))
	lw := r.label.MinSize().Width + 18
	r.plate.Move(fyne.NewPos((s.Width-lw)/2, s.Height-24))
	r.plate.Resize(fyne.NewSize(lw, 22))
	r.label.Move(fyne.NewPos(0, s.Height-24+(22-r.label.MinSize().Height)/2))
	r.label.Resize(fyne.NewSize(s.Width, r.label.MinSize().Height))
}
func (r *funnelRenderer) MinSize() fyne.Size { return fyne.NewSize(120, 96) }
func (r *funnelRenderer) Refresh()           { r.Layout(r.f.Size()); canvas.Refresh(r.f) }
func (r *funnelRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.img, r.plate, r.label}
}
func (r *funnelRenderer) Destroy() {}
