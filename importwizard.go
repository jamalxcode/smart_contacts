package main

import (
	_ "embed"
	"fmt"
	"image/color"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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

// wizGuide is one source's guide: what it does, numbered steps (each with what you should see),
// what to check before importing, what to try if it doesn't work, and the button that imports.
type wizGuide struct {
	title   string
	intro   string    // what this does, in a sentence or two
	steps   []wizStep // what to do, in order
	check   []string  // confirm these before pressing the import button
	trouble []string  // if it doesn't work
	expect  string    // what you'll have at the end
	link    string    // a page or setting to open, if any
	linkTo  string    // its button's words
	finish  wizFinish
}

// wizStep is one thing to do, and what you should see when it's done.
type wizStep struct {
	do, see string
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
	makeFile := wizStep{
		do: "On the phone, make the contacts file.\n" +
			"• Samsung: Contacts → ☰ (menu) → Manage contacts → Import or export contacts → Export → choose Internal storage → tick All → Done → Export.\n" +
			"• Pixel and most others (Google Contacts app): Fix & manage (bottom of the screen) → Export to file → Save.",
		see: "A message that the contacts were exported, and a file ending in .vcf (often Contacts.vcf) in the phone's Files or My Files app.",
	}
	email := &wizGuide{
		title: "Send the file to yourself",
		intro: "The surest way: you send the contacts file to yourself by email (or WhatsApp, Telegram, Google Drive), open it on this Mac and bring it in.",
		steps: []wizStep{
			makeFile,
			{"On the phone, open Files (or My Files), find the .vcf file (look in Downloads or Internal storage), tap Share and choose Gmail or your email app — or WhatsApp to yourself.", "A new message with Contacts.vcf attached."},
			{"Send it to an email address you can open on this Mac.", "The message arrives on the Mac within a minute."},
			{"On the Mac, open the message and download the attachment (in Mail: click it and choose Save to Downloads; in Gmail: the ⬇ on the attachment).", "The .vcf file in your Downloads folder."},
			{"Press Choose File… at the bottom of this window and pick the .vcf file.", "A window listing the people found — nothing is added until you press Import there."},
		},
		check:   []string{"The file ends in .vcf (a vCard).", "It's in Downloads — Choose File opens there."},
		trouble: []string{"No Export option on the phone? The contacts are probably in Google — press Back twice and choose Yes, in Google.", "The attachment is too big for email? Use Google Drive or WhatsApp to yourself."},
		expect:  "Everyone from the phone, with their numbers and emails.",
		finish:  finishFile,
	}
	airdrop := &wizGuide{
		title: "Send it with AirDrop",
		intro: "AirDrop sends the file straight to this Mac over the air. A few newer Android phones (such as the Pixel 10, through Quick Share) can send to AirDrop; most can't — if yours has no such option, use Send it to myself.",
		steps: []wizStep{
			makeFile,
			{"On this Mac, open Finder → AirDrop (in the sidebar, or Go → AirDrop) and at the bottom set \"Allow me to be discovered by\" to Everyone for 10 Minutes.", "This Mac's name shows in the AirDrop window."},
			{"On the phone, open Files, find the .vcf file, tap Share → Quick Share (or AirDrop) and choose this Mac.", "This Mac's name in the list, then \"Sent\" on the phone."},
			{"If the Mac asks, click Accept.", "The .vcf file in your Downloads folder."},
			{"Press Choose File… at the bottom of this window and pick the .vcf file.", "A window listing the people found — nothing is added until you press Import there."},
		},
		check:   []string{"Wi-Fi and Bluetooth are both on, on the phone and on the Mac.", "The phone and the Mac are within a few metres."},
		trouble: []string{"The Mac doesn't appear on the phone? Keep Finder's AirDrop window open, and set discovery to Everyone for 10 Minutes.", "The phone has no AirDrop option? Use Send it to myself or Bluetooth instead."},
		expect:  "A .vcf file in Downloads.",
		finish:  finishFile,
	}
	bluetooth := &wizGuide{
		title: "Send it over Bluetooth",
		intro: "Bluetooth sends the file from the phone to this Mac without the internet. macOS turns away Bluetooth files until you switch on Bluetooth Sharing, so that comes first.",
		steps: []wizStep{
			{"Press Open Sharing Settings at the bottom of this window (or open System Settings → General → Sharing).", "The Sharing settings, with Bluetooth Sharing in the list."},
			{"Turn on the switch next to Bluetooth Sharing.", "The switch turns blue (on)."},
			{"Click ⓘ next to Bluetooth Sharing. Set \"When receiving items\" to Accept and Save, and \"Folder for accepted items\" to Downloads. Click Done.", "Back in the Sharing list, with Bluetooth Sharing on."},
			{"Pair the phone with this Mac if you haven't: on the Mac open System Settings → Bluetooth; on the phone open Settings → Bluetooth (or Connections → Bluetooth) and tap this Mac; confirm the same code on both.", "The phone under My Devices on the Mac (Connected or Not Connected — both are fine)."},
			makeFile,
			{"Back in Smart Contacts, press Start Waiting at the bottom of this window. The app watches Downloads for the file for 15 minutes.", "A window saying \"Waiting for the file…\"."},
			{"On the phone, open Files, find the .vcf file, tap Share → Bluetooth and choose this Mac.", "\"Sending…\" then \"Sent\" on the phone; a moment later the list of people found opens here by itself."},
		},
		check:   []string{"Bluetooth Sharing is on (step 2) and set to Accept and Save (step 3) — without these the phone says the Mac refused the file.", "\"When receiving items\" isn't set to Never Allow.", "Bluetooth is on, on both."},
		trouble: []string{"The phone says \"refused\" or \"failed\"? Bluetooth Sharing is still off, or set to Never Allow or Ask What to Do — go back to steps 2 and 3.", "The Mac doesn't appear on the phone? Unpair both and pair again, with Bluetooth settings open on the Mac.", "Still stuck? Use Send it to myself — it always works."},
		expect:  "The app finds Contacts.vcf as soon as it lands in Downloads.",
		link:    "x-apple.systempreferences:com.apple.Sharing-Settings.extension", linkTo: "Open Sharing Settings",
		finish: finishReceive,
	}
	sendAsk := &wizQuestion{
		ask:  "How will you send the contacts file to this Mac?",
		note: "Each way starts by making the file on the phone, and shows you how.",
		answers: []wizAnswer{
			{label: "Send it to myself", detail: "email, WhatsApp, Google Drive — the surest way", icon: theme.MailSendIcon(), guide: email},
			{label: "AirDrop", detail: "only some newer Android phones, like the Pixel 10", icon: theme.UploadIcon(), guide: airdrop},
			{label: "Bluetooth", detail: "works with most phones — needs one setting on this Mac first", icon: theme.MediaFastForwardIcon(), guide: bluetooth},
		},
	}

	google := &wizGuide{
		title: "Export from Google Contacts",
		intro: "If the contacts are in your Google account, you download them from the Google Contacts website on this Mac — nothing to send from the phone.",
		steps: []wizStep{
			{"Press Open Google Contacts at the bottom of this window. It opens contacts.google.com in your browser; sign in with the Google account the phone uses if asked.", "Your contacts listed, with a count at the top (\"Contacts (452)\")."},
			{"Click ⋮ (More actions) at the top of the list and choose Export. If you don't see ⋮, tick the box on one contact first, then the box at the top → All.", "A small window: \"Export contacts\"."},
			{"Leave \"Contacts\" chosen (that's everyone), choose vCard (for iOS Contacts) — it keeps photos and notes — and press Export.", "A file called contacts.vcf in your Downloads folder."},
			{"Come back to Smart Contacts, press Choose File… at the bottom of this window and pick contacts.vcf.", "A window listing the people found — nothing is added until you press Import there."},
		},
		check:   []string{"You signed in with the same account the phone uses (its Contacts app shows which).", "The number of contacts at the top looks right."},
		trouble: []string{"Fewer contacts than on the phone? Some are saved on the phone only — use Back and send the file instead.", "Several Google accounts? Export each one; people already in your book are updated, not duplicated."},
		expect:  "One file, contacts.vcf, with everyone in that account.",
		link:    "https://contacts.google.com", linkTo: "Open Google Contacts",
		finish: finishFile,
	}

	iphone := &wizQuestion{
		ask:  "Is that iPhone signed in to the same Apple Account as this Mac, with Contacts on in iCloud?",
		note: "To check on the iPhone: Settings → your name → iCloud → Contacts is on. On this Mac: System Settings → your name → iCloud → Contacts.",
		answers: []wizAnswer{
			{label: "Yes, same Apple Account", detail: "one press", icon: theme.ConfirmIcon(), guide: &wizGuide{
				title: "Bring in this Mac's contacts",
				intro: "The iPhone's contacts are already on this Mac through iCloud, so Smart Contacts reads them here. It only reads; nothing is changed in Apple's Contacts.",
				steps: []wizStep{
					{"Press Import Now at the bottom of this window.", "\"Reading Mac Contacts…\", and macOS may ask whether Smart Contacts can use Contacts."},
					{"If it asks, press OK (or Allow).", "A preview of the people found."},
					{"Look through the list of people found and press Import.", "A note at the top saying how many were added and updated."},
				},
				check:   []string{"Contacts is on in iCloud on both — iPhone: Settings → your name → iCloud → Contacts; Mac: System Settings → your name → iCloud → Contacts.", "Open Apple's Contacts app once — if your iPhone people are there, they'll come in."},
				trouble: []string{"It says it isn't allowed? System Settings → Privacy & Security → Contacts (and Automation) → turn on Smart Contacts, then try again.", "Nobody new appears? The iPhone isn't syncing to this Mac — use Back and choose No."},
				expect:  "Everyone from the iPhone, plus any Google or Exchange accounts added on this Mac.",
				finish:  finishMac,
			}},
			{label: "No, or I'm not sure", detail: "export from icloud.com instead", icon: theme.QuestionIcon(), guide: &wizGuide{
				title: "Export them from iCloud on the web",
				intro: "You download the iPhone's contacts from icloud.com as one file, then bring that file in.",
				steps: []wizStep{
					{"Press Open icloud.com at the bottom of this window and sign in with the iPhone's Apple Account.", "Your contacts listed at icloud.com/contacts."},
					{"Open the Select menu at the top of the list and choose Select All Contacts.", "Everyone highlighted, with a count of how many are selected."},
					{"Click the Actions button (⋯) and choose Export vCard.", "A .vcf file in your Downloads folder."},
					{"Come back to Smart Contacts, press Choose File… at the bottom of this window and pick that .vcf file.", "A window listing the people found — nothing is added until you press Import there."},
				},
				check:   []string{"The count in step 2 matches the iPhone.", "The file ends in .vcf."},
				trouble: []string{"Contacts isn't on icloud.com? On the iPhone, turn on Settings → your name → iCloud → Contacts, wait a few minutes, and reload."},
				expect:  "One .vcf file with everyone in it.",
				link:    "https://www.icloud.com/contacts", linkTo: "Open icloud.com",
				finish: finishFile,
			}},
		},
	}

	android := &wizQuestion{
		ask:  "Does that phone keep its contacts in your Google account?",
		note: "Most Android phones do. On the phone: open Contacts and tap ☰ or your picture — the account shown is where they're saved.",
		answers: []wizAnswer{
			{label: "Yes, in Google", detail: "the easiest way — nothing to send", icon: theme.ConfirmIcon(), guide: google},
			{label: "No, or I'm not sure", detail: "make a file on the phone and send it", icon: theme.QuestionIcon(), next: sendAsk},
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
				intro: "You download your Outlook (or Hotmail) contacts from the Outlook website as a file, then bring it in.",
				steps: []wizStep{
					{"Press Open Outlook People at the bottom of this window and sign in.", "Your contacts on the People page."},
					{"On the toolbar, open Manage contacts and choose Export contacts.", "An \"Export contacts\" panel."},
					{"Choose All contacts and press Export.", "A file called contacts.csv in your Downloads folder."},
					{"Come back to Smart Contacts, press Choose File… at the bottom of this window and pick contacts.csv.", "A window listing the people found, with how each column was read — nothing is added until you press Import there."},
				},
				check:   []string{"You signed in with the right account (work and personal are separate).", "The file is contacts.csv."},
				trouble: []string{"No Export option? On a work account it may be switched off by your company — ask IT, or export from the Outlook app on a PC (File → Open & Export → Import/Export → Export to a file → CSV)."},
				expect:  "A file called contacts.csv.",
				link:    "https://outlook.live.com/people", linkTo: "Open Outlook People",
				finish: finishFile,
			}},
			{label: "LinkedIn", icon: iconLink, guide: &wizGuide{
				title: "Download your LinkedIn connections",
				intro: "LinkedIn emails you a file of your connections. It takes a little while to arrive, so this is two sittings: ask now, import when the email comes.",
				steps: []wizStep{
					{"Press Open LinkedIn at the bottom of this window and sign in. (On LinkedIn it's Me → Settings & Privacy → Data privacy → Get a copy of your data.)", "A page asking which data you want."},
					{"Choose \"Want something in particular?\", tick only Connections, then Request archive (it may ask for your password).", "\"Your download is being prepared\"."},
					{"Wait for LinkedIn's email — usually about ten minutes, sometimes up to a day.", "An email \"Your LinkedIn data archive is ready\"."},
					{"Open the link in the email, press Download archive, then double-click the downloaded .zip in Downloads.", "A folder with Connections.csv inside."},
					{"Come back to Smart Contacts, press Choose File… at the bottom of this window and pick Connections.csv.", "A window listing the people found — nothing is added until you press Import there."},
				},
				check:   []string{"You picked Connections.csv (not Messages or Profile).", "Many connections hide their email — that's LinkedIn, not a fault."},
				trouble: []string{"No email after a day? Check Spam, or request the archive again."},
				expect:  "Names, companies, job titles and emails where people share them.",
				link:    "https://www.linkedin.com/mypreferences/d/download-my-data", linkTo: "Open LinkedIn",
				finish: finishFile,
			}},
			{label: "Another email account", detail: "Yahoo, iCloud web, work mail…", icon: theme.MailComposeIcon(), guide: &wizGuide{
				title: "Export from your email",
				intro: "Most email services can download their address book as a file. The names of the menus differ, but the steps are the same.",
				steps: []wizStep{
					{"Sign in to the email account on the web and open its Contacts or Address Book.", "Your contacts listed."},
					{"Look for Export — often under ⋯, the gear, or Settings → Contacts.", "A choice of formats."},
					{"Choose vCard (.vcf) or CSV and export everyone.", "A file downloads."},
					{"Come back to Smart Contacts, press Choose File… at the bottom of this window and pick it.", "A window listing the people found — nothing is added until you press Import there."},
				},
				check:   []string{"The file ends in .vcf or .csv."},
				trouble: []string{"iCloud mail: use iPhone → No, or I'm not sure. Gmail: use Gmail / Google. Outlook or Hotmail: use Outlook / Hotmail."},
				expect:  "A .vcf or .csv file.",
				finish:  finishFile,
			}},
			{label: "A spreadsheet", detail: "Excel, Numbers or CSV", icon: theme.GridIcon(), guide: &wizGuide{
				title: "Bring in a spreadsheet",
				intro: "Any list of people in rows works. You'll see which column is which before anything is added, and can change it.",
				steps: []wizStep{
					{"Check the sheet has one person per row, and headings in the first row (Name, Phone, Email, Company… in English or Arabic).", "Headings across the top, people below."},
					{"From Numbers, use File → Export To → CSV. Excel files (.xlsx) work as they are.", "A .csv or .xlsx file."},
					{"Press Choose File… at the bottom of this window and pick it.", "A window listing the people found, with a \"Column matching\" section."},
					{"Open Column matching and check each column is read the way you want (change any that aren't), then press Import.", "A note at the top saying how many were added."},
				},
				check:   []string{"Phone numbers without a country code are taken as your country (Kuwait, +965) — change it in Settings → Your country code.", "Only the first sheet of an Excel file is read."},
				trouble: []string{"A column isn't recognised? Choose what it holds under Column matching."},
				expect:  "A window showing which column is which before anything is added.",
				finish:  finishFile,
			}},
			{label: "A note or some text", detail: "a list from Notes, WhatsApp, an email…", icon: theme.DocumentIcon(), guide: &wizGuide{
				title: "Bring in a list of people",
				intro: "Paste a list of names and numbers from anywhere; each person is read from their line.",
				steps: []wizStep{
					{"Put one person per line (\"Ali Hassan 9999 8888\"), or separate people with an empty line.", "A tidy list in Notes or an email."},
					{"Select the text and copy it (⌘C).", "Nothing yet — it's on the clipboard."},
					{"Press Paste Text… at the bottom of this window, click in the box, paste (⌘V) and press Read. (Or save it as a .txt file and use Choose File….)", "A window listing the people found."},
					{"Check the people found, then press Import.", "A note at the top saying how many were added."},
				},
				check:   []string{"Each person has a name and a number or email — lines without either are skipped."},
				trouble: []string{"Two people read as one? Put an empty line between them."},
				expect:  "Each line read as a name with its number or email.",
				finish:  finishPaste,
			}},
			{label: "Telegram", icon: iconMessage, guide: &wizGuide{
				title: "Export your Telegram contacts",
				intro: "Telegram's desktop app can export your contacts with their phone numbers. The phone app can't.",
				steps: []wizStep{
					{"Press Get Telegram Desktop at the bottom of this window, download it for macOS, install and open it, and sign in with your phone.", "Your chats in Telegram Desktop."},
					{"Open Settings → Advanced → Export Telegram data.", "A window of things to export."},
					{"Untick everything except Contacts list, set Format to Machine-readable JSON, and press Export.", "\"Done\", and a Telegram Desktop folder in Downloads."},
					{"Come back to Smart Contacts, press Choose File… at the bottom of this window and pick result.json inside that folder.", "A window listing the people found — nothing is added until you press Import there."},
				},
				check:   []string{"The format was JSON, not HTML."},
				trouble: []string{"No Export option? You're in the Mac App Store \"Telegram\" app — it can't export. Use Telegram Desktop from desktop.telegram.org."},
				expect:  "Your Telegram contacts with their phone numbers.",
				link:    "https://desktop.telegram.org", linkTo: "Get Telegram Desktop",
				finish: finishFile,
			}},
			{label: "Facebook or Snapchat friends", icon: iconPeople, guide: &wizGuide{
				title: "Download your friends list",
				intro: "Facebook and Snapchat give names only (no numbers), so they're matched to people already in your book — they add a profile link, not new cards.",
				steps: []wizStep{
					{"Facebook: press Open Facebook at the bottom of this window (Accounts Center → Your information and permissions → Export your information). Create an export → Export to device → Customize information: tick only Friends and followers → Format JSON → Start export.", "\"Your export is in progress\"."},
					{"Snapchat instead: in the app, Settings → My Data → turn on Export JSON files, tick Friends → Next → All time → Submit.", "A note that they'll email you when it's ready."},
					{"When the email or notification comes (minutes to a day), download the .zip and double-click it in Downloads.", "A folder of files."},
					{"Press Choose File… at the bottom of this window and pick connections/friends/your_friends.json (Facebook) or json/friends.json (Snapchat).", "The friends matched to people in your book, for you to check before pressing Import."},
				},
				check:   []string{"The format was JSON."},
				trouble: []string{"Few matches? Names often differ (nicknames, Arabic vs English) — matched ones are listed for you to check."},
				expect:  "Names only, matched to people you already have.",
				link:    "https://accountscenter.facebook.com/info_and_permissions/dyi", linkTo: "Open Facebook",
				finish: finishFile,
			}},
			{label: "Instagram or X", icon: iconLink, guide: &wizGuide{
				title: "Instagram and X can't be imported",
				intro: "Their exports list only usernames or account numbers — nothing to match a person by.",
				steps: []wizStep{
					{"Open the person's card, press Write, and add their profile link under Web & Social.", "The link on the card, with an Instagram or X button."},
				},
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
		para := func(s string) *widget.Label {
			l := widget.NewLabel(s)
			l.Wrapping = fyne.TextWrapWord
			return l
		}
		head := func(s string) {
			h := widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			box.Add(h)
		}
		if g.intro != "" {
			box.Add(para(g.intro))
		}
		if len(g.steps) > 0 {
			head("Steps")
		}
		for i, st := range g.steps {
			box.Add(newNumberedStep(i+1, st.do, st.see))
			// The step that opens a page also carries the link itself, right there.
			if g.link != "" && g.linkTo != "" && strings.Contains(st.do, g.linkTo) {
				if p, err := url.Parse(g.link); err == nil {
					text := g.link
					if p.Scheme == "http" || p.Scheme == "https" {
						text = p.Host + strings.TrimSuffix(p.Path, "/")
					} else {
						text = g.linkTo
					}
					h := widget.NewHyperlink("→ "+text, p)
					h.Wrapping = fyne.TextWrapBreak
					pad := canvas.NewRectangle(color.Transparent)
					pad.SetMinSize(fyne.NewSize(38, 1))
					box.Add(container.NewBorder(nil, nil, pad, nil, h))
				}
			}
		}
		if len(g.check) > 0 {
			head("Check before you continue")
			for _, c := range g.check {
				box.Add(container.NewBorder(nil, nil, container.NewVBox(widget.NewIcon(theme.ConfirmIcon())), nil, para(c)))
			}
		}
		if len(g.trouble) > 0 {
			head("If it doesn't work")
			for _, c := range g.trouble {
				box.Add(container.NewBorder(nil, nil, container.NewVBox(widget.NewIcon(theme.WarningIcon())), nil, para(c)))
			}
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
		if len(acts) == 0 {
			set(container.NewVScroll(box))
			return
		}
		// The buttons the steps mention stay in sight at the bottom, however far you scroll.
		rule := canvas.NewRectangle(color.NRGBA{0x8A, 0x6A, 0x4A, 0x60})
		rule.SetMinSize(fyne.NewSize(1, 1))
		bar := container.NewVBox(rule, container.NewHBox(append([]fyne.CanvasObject{caption("The steps' buttons:"), layout.NewSpacer()}, acts...)...))
		set(container.NewBorder(nil, bar, nil, nil, container.NewVScroll(box)))
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

// newNumberedStep is "①  Do this." with the number in an ink circle, and under it in italics
// what you should see when it's done.
func newNumberedStep(n int, text string, see ...string) fyne.CanvasObject {
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
	body := container.NewVBox(l)
	if len(see) > 0 && see[0] != "" {
		s := widget.NewLabel("You should see: " + see[0])
		s.Wrapping = fyne.TextWrapWord
		s.TextStyle.Italic = true
		s.Importance = widget.LowImportance
		body = container.New(&tightVBox{}, l, s)
	}
	return container.NewBorder(nil, nil, container.NewVBox(badge), nil, body)
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
