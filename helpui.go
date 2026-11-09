package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func (u *ui) help() {
	text := `**Finding people.** Start typing a name anywhere and it goes into the search field. The arrow keys walk the list. The funnel at the end of the search field opens a filter form: Name, Company, Country, City, From (where the card came from: LinkedIn, a spreadsheet, a business card) and Other. Only the people matching all of them are shown. Save as Smart Group keeps it in the sidebar. In the search field itself, from:linkedin does the same as From.

**The organiser.** It opens closed: click the cover or press any key. The coloured tabs on the left are your lists and groups; the A–Z tabs down the right edge show only the people filed under those letters (All shows everyone, ع holds Arabic names, # the rest). Click the strap on the right (or press ⇧⌘W) to close it; next time it opens at the same page, unless Settings → Open at says the first page. View → Filofax Look switches to the plain Mac look and back. Each phone number shows its country's flag.

**Finding people.** Type in the search field: names (Arabic or English, with or without hamza and accents), part of a phone number, a company, a note. If nobody matches exactly, the list tries again forgiving typos ("Mohamad Al-Mutari", "boubian") and its title says "no exact match, N close". ↑ and ↓ move through the results, Return edits, Esc clears. View → All Contacts, Favorites, Upcoming Birthdays and Recently Added are ⌘1 to ⌘4.

**Reaching people.** The round buttons under the name follow what the card holds: message, call, WhatsApp, mail, FaceTime, and LinkedIn, Telegram, Instagram, Messenger, X, Signal, Snapchat, TikTok, GitHub or Skype when there's a link or handle. *More* has number-based options. The app remembers when you last contacted someone.

**Editing.** Every card is editable where it is: click any field and type (⌘L or Return jumps to the first field). Save and Revert appear at the bottom once something changes, ⌘S saves, and if you move to another person, close the window or quit with unsaved changes, you're asked whether to update the card (Save, Don't Save or Cancel). Right-click a person in the list for Favorites, Connect, Share and Delete. Share (the arrow button) copies the card as text, saves a vCard or copies it to Apple Contacts.

**Last talked.** Card → Check Messages… (⇧⌘U) reads Messages, call history and Mail on this Mac (it needs Full Disk Access, which it asks for) and shows on each card when you were last in touch and how. For WhatsApp, export a chat (⋯ → Export Chat) and drop it on the window. Then it lists the people you write to or call often who aren't in your contacts. Nothing leaves the Mac.

**Keeping in touch.** Keep in Touch at the bottom of a card sets a reminder (every week … every year). View → Reach Out (⌘5) lists who's due, longest first, and a notification reminds you once a day, along with birthdays. Note (⌘J) adds a dated note to the card's timeline; "How we met" is on the Edit page.

**Quick Find.** The person icon in the menu bar opens Quick Find (also ⇧⌘O): type a name, press Return to copy the number. Your favorites are in that menu too, with call and message.

**Search filters.** company:boubyan · title:engineer · group:family · source:linkedin · city:salmiya · country:kuwait · code:965 · has:email · no:phone · talked:<30d · talked:>1y · is:favorite · is:due — and a minus in front reverses one (-group:work). Save any search as a Smart Group in the sidebar.

**The organiser does what it looks like.** The pen in its loop writes a new card. The turned-up corner at the bottom of each page turns to the previous or next letter. The pocket inside the front cover takes business cards: click it, or drop a photo of a card anywhere on the window. The strap closes the book. Each lifts a little under the pointer and says what it does. A card shows only what's written on it. Click a line for what it's for (call, message, email, open, copy). Double-click a line, or press Write, to write on the card: every field opens, and the one you clicked is ready to type in. Done or Esc puts the pen down and saves. Turn Over shows the back of the card: notes, how you met, keeping in touch and the history. On a card with several numbers, the star marks the main one; × crosses a line out. When the open card changes it gets an UPDATED stamp and a pencil note of what changed. The To Tidy tabs (with a red flag) are cards waiting for you to fix something.

**Today.** The first tab, Today (⌘0, also first in the Card menu and the toolbar), is the day's page: birthdays and anniversaries this week (call, message or WhatsApp them), people due a call from Keep in Touch (with Done to mark you've been in touch), people you met in the last two weeks from the calendar log (with Note), and cards waiting to be tidied (with Review). Click anyone to open their card. Typing a search shows the list again. Settings → Open at the first page now opens on Today.

**Follow-ups.** Keep in Touch → Follow Up on a Date… (Card menu, the Keep in Touch tab, right-click, or the back of a card) notes what to do for someone and when: "send the proposal", next Tuesday. On the day it's on the Today page with Later and Done; the Follow-ups tab lists everyone with one open, soonest first.

**Coming up.** The Today page shows meetings in the next three days with people you know, from the calendar log; click one to read their card before you go.

**Greetings.** Send Greetings… (Card menu, the Greetings tab, right-click) picks who (the clipped cards, Favorites, a group or everyone with a number), the occasion (Eid, Ramadan, National Day, New Year, a birthday, or your own words, in English or Arabic) and the message — {name} becomes each first name. Then it opens WhatsApp (or Messages) for each person in turn with the message ready; you press send there. Each card notes the greeting, and warns if they've had it this year.

**Company pages.** When others you know work at the same company, the job line on a card has a ›: click it to see everyone there, their roles and when you last talked.

**Tidy one by one.** Tidy Up → "Go through the cards that need it, one at a time" (or Tidy One by One on the Today page) shows each card that needs fixing with what's wrong, and Fix, Move to Trash, Skip or Stop, with a counter.

**Your own lines.** Write on a card and use "add a line of your own" under More About Them: Spouse, Children, Diwaniya, Office, or any label you type. They show on the card and are found by search.

**Names and dates.** Under the name, "prefix, middle name, suffix, how to say it" opens a row for Dr. or Eng., a middle name, Jr. or PhD, and the name spelled the way it's said; the card's heading then shows the whole name. Under Birthday, "add a date" keeps an anniversary or any date of your own; dates coming up this week are on Today. All of these go out in vCards and spreadsheets and come back in from Apple, Google and Outlook.

**Questions.** Type a question in the search field — "who do I know at Gulf Bank in marketing?", "people I haven't talked to in a year", "favorites in Kuwait without an email". It's turned into search filters on this Mac, shown under the field. With Cloud AI on, Return lets the AI read the question; only the question is sent, never a contact.

**Trash.** Move to Trash (Card menu, the Trash tab above the book, right-click, or the trash button on a card next to message) puts the open card, or the clipped cards, in the Trash straight away; the banner's Undo brings it back. Cards stay in the Trash until you empty it. Show Trash lists them with Put Back (or Put Back All); Empty Trash deletes them for good, after asking. The Card menu and the Trash tab show how many cards are in it.

**Menus and the organiser match.** The View menu and the tabs down the left have the same sections, names and order: the lists, To Tidy, Saved Searches, Groups, then Sort and Also in the Book (Calendar Log, Trash). The tabs show each group and saved search by name; in the menu use Go to Group… (⌘G) and Go to Saved Search…. The Card menu and the toolbar above the book also match each other.

**What a command acts on.** The Card menu and the toolbar above the organiser have the same ten commands, with the same names in the same order: Add Note, Favorite, Keep in Touch, Groups, Connections, Tidy Up, Find Duplicates, Share, Clip Cards and Check Messages (Messages, calls and Mail). They act on the clipped cards if any are clipped, otherwise on the open card. The toolbar says which at its left (“Acting on: Ali Hassan” or “3 clipped cards”). Tidy Up and Find Duplicates ask in their window, because they can also take the whole address book. Commands that can't act right now are dimmed. Connections with two cards clipped shows how those two are linked. Edit → Delete deletes the clipped cards if there are any, otherwise the open card, and always asks first, naming them.

**Seeing what changed.** After any change a banner at the bottom says what was done (“Clean Up applied to 3 Clipped Cards — 1 fix”), with *Show Changes* (each card, how it was in grey and how it is now in bold) and *Undo*. Cards that just changed have a blue dot in the list for a short while.

**Needs Attention** lists cards with something to tidy (a number without its country code, a typo in an email…); the card shows a Review button.

**Tidy notes with AI.** Card → Tidy Jumbled Notes with AI finds notes that are run together, full of export codes or repeats, and rewrites them readably, keeping every fact. It only runs with Cloud AI on (the note text goes to your OpenRouter model), asks first, and shows each rewrite before and after; one that seems to have lost a number, email or date isn't ticked.

**Merging.** Find Duplicates with cards clipped offers to merge them into one. The merge window shows where the cards disagree (name, job, birthday, photo) so you pick; every number, email, note, line of your own, date and follow-up is kept.

**Smart Add** (⌥⌘N). Paste an email signature, a business card's text or a WhatsApp contact message, or choose a business card photo. It's read privately on this Mac unless you switch on Cloud AI in Settings. You check the result before it's saved.

**Bring Everything In.** File → Bring Everything In is a checklist of every source, then clean-up and duplicates; each step is ticked once done.

**Importing (⌘O).** Click the funnel under the business-card pocket, or File → Import. It asks where your contacts are now (iPhone, Android, Gmail, Outlook, LinkedIn, another email, a spreadsheet, a note, Telegram, Facebook or Snapchat), perhaps one more question (is the iPhone on this Mac's iCloud? how will you send the file?), then shows numbered steps: what to do on that side, what you'll get, how to send it here, and the button that takes it in. Choose File opens in Downloads, where exports land. It brings people in from the Mac's Contacts app (and iPhone via iCloud), other phones (.vcf, Bluetooth or AirDrop), Gmail, Outlook, Yahoo or iCloud, LinkedIn, Telegram, Facebook, Snapchat, any spreadsheet (.csv or .xlsx, English or Arabic headers) and notes. You can also drag files onto the window. People you already have are recognised by number, email, profile link or name, in Arabic or English, and updated instead of duplicated. Anyone matched by name only is listed for you to check. Each phone, email, link and address shows where it came from, and the Timeline on each card records what happened without touching your notes.

**Undo (⌘Z, ⇧⌘Z).** Every change can be undone: edits, imports, clean-ups, merges, deletes. Edit → History lists this session's changes and the saved restore points.

**Many at once: paper clips.** Every card in the list has a paper clip at its left edge. Click it to clip that card; clip more and they gather under a yellow sticky note. ⌘-click and Shift-click (a range) work too, as do ⌘A for everyone shown (⇧⌘A unclips all) and Card → Clip Cards (⇧⌘S). While cards are clipped, every Card command and toolbar button acts on them. Take Clips Off ends it. Deleting several cards asks twice, and Return never confirms a delete: the red button has to be clicked.

**Duplicates and tidying.** Find Duplicates (⇧⌘D) merges the same person on several cards and removes repeats inside cards. Tidy Up writes every number the same way (+965 9999 8888), fixes names, emails and links, or, if you choose “Jumbled notes”, rewrites messy notes with Cloud AI. You see every change before it's made.

**Groups.** File → New Group (⇧⌘N), then tick groups on a card's Edit page. A new card made while a group is open joins it.

**Connections (six degrees).** Card → Connect to… (⌘K) links two people as friends, family, colleagues and so on. Card → How Are They Connected? (⇧⌘K) finds the shortest chain between two people, up to six steps. A vCard export (and Copy to Apple Contacts) carries each person's connections and Spouse / Children lines as related names; importing one links a related name to the person it names when only one card has that name, and otherwise keeps it as a line on the card.

**Calendar log (⇧⌘L).** Reads the Mac Calendar app or an .ics file; each card lists the meetings the person was in or mentioned in, and "last met".

**Photos and business cards.** Click the round picture to set the person's photo. A business card photo is read on this Mac and kept on the card.

**Names in two languages.** Each card can hold the name in the other language (محمد المطيري / Mohammed Al-Mutairi); Edit → Suggest fills common names.

**Privacy.** Your contacts live only on this Mac, in a folder only your user account can open, kept out of Spotlight. The app goes online only for Cloud AI (if you switch it on) and Help → Check for Updates.

**Safety.** Cards you move to the Trash stay there until you empty it (Card → Show Trash puts them back; Card → Empty Trash deletes them for good). A backup is kept for each of the last 30 days. File → Back Up To… saves everything to a drive you choose, encrypted if you give a password; File → Restore from Backup… brings one back. Undo history is kept when you quit.

**Keys:** ⌘N new card · ⇧⌘N new group · ⌥⌘N Smart Add · ⌘O import · ⌘F find · ⌘L type in the card · ⌘S save · ⌘⌫ delete · ⌘Z undo · ⇧⌘Z redo · ⌘A select all · ⌘1–4 lists · ⇧⌘F favorite · ⌘K connect · ⇧⌘K how connected · ⇧⌘D duplicates · ⇧⌘M merge · ⇧⌘L calendar · ⌘5 reach out · ⌘6 needs attention · ⌘J note · ⇧⌘U update last talked · ⇧⌘O quick find · ⌘, settings`
	r := widget.NewRichTextFromMarkdown(text)
	r.Wrapping = fyne.TextWrapWord
	d := newCustom("Smart Contacts Help", "Close", container.NewVScroll(r), u.win)
	d.Resize(fyne.NewSize(640, 560))
	d.Show()
}

// privacyText describes where the data is and what can leave the Mac.
func privacyText(dir string) string {
	return "Contacts, photos and 30 daily backups are stored only on this Mac, in\n" + dir +
		"\n(only your user can open it; kept out of Spotlight; not in iCloud Drive).\n" +
		"The app uses no network and no accounts unless Cloud AI is on, or you choose\n" +
		"Check for Updates (which asks GitHub for the newest build and sends no contacts).\n" +
		"FileVault (System Settings → Privacy & Security) encrypts it on disk."
}
