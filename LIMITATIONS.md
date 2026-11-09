# What Smart Contacts won't do, and what's next

Smart Contacts keeps every contact on this Mac. These are the deliberate limits, plus the ideas
that haven't been built yet.

## Won't be built (privacy)

- **Profiling people:** searching Google, X, LinkedIn or other sites for every contact and
  compiling what each person does, which groups they belong to or what they're involved in.
- **Reverse lookups:** finding someone's identity or social accounts from their phone number or
  email. That depends on data brokers and scraping, which LinkedIn and X forbid.
- **Background tracking:** watching people's online activity over time.
- **Online searches of contacts:** no LinkedIn search, social media search, X search or Google
  search of contacts from inside the app. Profile links are added by hand under
  *Edit → Web & Social*.

## Possible, but left out to keep contacts private

- **Reading your messages and calls:** scanning Messages, WhatsApp, Mail or call history to work
  out who you talk to most, or to fill in "last contacted" automatically.
- **Fetching contact photos online:** from Gravatar, WhatsApp or similar (it sends emails and
  numbers to outside servers).
- **Placing contacts on a map:** turning addresses into map locations needs an online service.
- **Checking numbers with a carrier:** confirming a number is live or which network it's on.
- **Asking AI about the whole address book:** questions like "who do I know in banking?" would
  send every contact to a cloud model.
- **Cloud backup or sync of contacts:** iCloud, Google or any server.
- **Uploading the address book to find people:** "find your friends" or Truecaller-style
  caller-ID matching.
- **Creating contacts from your inbox or calendar:** scanning email senders and meeting guests.
- **Looking up company details online:** website, size or industry for each company.
- **Watching Apple Contacts in the background:** importing is a one-time copy you start yourself;
  later changes there don't flow in automatically.

## Possible, but risky to automate

- **Guessing relationships:** same surname = family, same address = household. Too often wrong;
  only same-company links are automatic.
- **Ranking people:** "importance" or relationship-strength scores.
- **Inferring sensitive traits** (religion, health, nationality) from names or notes.
- **Matching faces** of contacts in your photos.
- **Messaging contacts on your behalf:** automatic birthday wishes or bulk WhatsApp sent without
  you reviewing each one.
- **Sharing your network map:** exporting who-knows-whom gives away other people's relationships.
- **Merging duplicates automatically:** every merge is reviewed by you first.
- **Deleting stale contacts:** for example, people you haven't contacted in two years.
- **Overwriting names with automatic transliteration** (Arabic ↔ English).
- **Assigning groups automatically** from company or notes.
- **Exporting contacts to marketing tools:** bulk-email lists or CRMs.

## Current limits on contact data (could be lifted)

- Contacts live on one Mac only; there is no conflict handling if the file is edited elsewhere.
- Contact data has no encryption of its own; it relies on FileVault.
- Typo tolerance in search starts at 4 letters and only kicks in when nothing matches exactly;
  numbers, job titles, notes and addresses are matched exactly.
- Google, Outlook and LinkedIn come in through files you export (vCard / CSV), not a live link.
- Facebook and Snapchat exports list friends by name only: they enrich people already in the book
  and never create cards unless you tick them. Instagram and X exports hold only usernames or
  account numbers, so they can't be matched at all; add those links by hand.
- macOS can't pull contacts from a phone over Bluetooth by itself; the phone has to send a .vcf
  (Bluetooth Sharing or AirDrop), which the Import wizard's Bluetooth guide picks up from Downloads.
  Bluetooth Sharing must be on and set to Accept and Save, or the phone reports the Mac refused it.
  The guide and the waiting window show whether Bluetooth Sharing is on (read live from macOS);
  "Accept and Save" can't be read, so the guide still asks you to check it.
- WhatsApp has no contacts export (it uses the phone's address book) — import the phone instead.
- LinkedIn messages open the person's LinkedIn page (where the Message button is); LinkedIn has no
  direct "message this person" link. X opens the profile for the same reason.
- "Telegram / Signal by number" only works if the person uses that app and allows it.
- The calendar log reads repeating events once (their first date); calendars not shown in the
  Mac Calendar app need an .ics export. Matching needs an email or a full name — a first name
  alone is ignored on purpose.
- Business card text recognition needs macOS (Arabic needs a recent macOS); handwriting and
  stylised logos are often misread, so the result is always shown for checking.
- Name matching across languages uses consonants, so a few names share a key (Mohammed / Mahmoud,
  Hassan / Hussain); a name alone never merges two cards without you.
- Spelling suggestions for the other language cover common Arabic names and Gulf family names;
  others are typed by hand.
- Clean Up knows the numbering rules of about 25 countries; numbers from others are kept as written.
- Contact photos: iPhone HEIC needs converting to JPEG; photos are shrunk to 512 px.
- Cards in the Trash stay until you empty it; edits can be undone with Edit → Undo (history kept between launches).
- One photo per contact and no attachments or documents.
- Connections have no direction (no manager → report or parent → child); in vCards a connection
  goes out as a related name with its label, and a related name comes back as a connection only
  when exactly one card has that name.
- Groups are flat; no nesting.
- CSV exports keep up to 3 phones, 2 emails and 1 address per contact.
- Apple's instant-messaging handles (IMPP) don't survive import or export; profile links do.
- CSV export writes one Anniversary column and the other dates together in one "Other Dates" column.
- Arabic and English names sort as one list, so the A–Z letter headings mix.
- Every save rewrites the whole file; very large books (around 50,000 contacts) are untested.
- Daily backups are kept for 30 days, then deleted. The Trash keeps cards until you empty it.

## Not built yet (could be)

- Directed connections (manager → report, parent → child)
- Instant-messaging handles from Apple vCards (IMPP)

## Current limits of the newest features

- **No system-wide shortcut for Quick Find.** The toolkit can't register a global hotkey; Quick Find
  opens from the menu bar icon, or ⇧⌘O while the app is in front.
- **WhatsApp history only from chat exports.** WhatsApp keeps its data encrypted and offers no
  reader; each chat has to be exported (⋯ → Export Chat) and dropped on the window.
- **Messages, calls and Mail need Full Disk Access** for the app. The databases' layouts are
  Apple's and undocumented; if a macOS update changes them, that source shows as "couldn't be read"
  rather than giving wrong dates.
- **Calls and Messages only include what reaches the Mac:** iPhone calls appear when the iPhone and
  Mac share calls (Continuity), and SMS when Text Message Forwarding is on.
- **AI note tidying sends the note text** (and the person's name) to the OpenRouter model chosen in
  Settings, and only when Cloud AI is switched on. Everything else in the app stays offline.
- **Reminders are notifications from the app,** so they only appear while it's running (or when it
  next opens); there's no background helper.

## Done

- **Typo-tolerant search**, **anniversaries and other dates** (on the card and on Today),
  **prefix, middle name, suffix and phonetic names**, and **related people in vCards** (connections
  out, related names in), all round-tripping through Apple, Google and Outlook files. Your own
  lines (Spouse, Children, Diwaniya…), the timeline, Quick Find, encrypted backups, reminders and
  undo across restarts were already in, below.

- **Last talked** from Messages, calls, Mail, WhatsApp exports and Calendar; **People Not in
  Contacts**; **keep-in-touch reminders** with a Reach Out list; dated notes; How we met; birthday
  and reach-out notifications; **Quick Find** in the menu bar; **search filters** and **smart
  groups**; **Needs Attention**; **AI note tidying**; **merge chooser**; company suggestions and
  "make main number"; **encrypted backups and restore**; undo kept across restarts; and saving that
  stays quick with 10,000 contacts.

- **Undo and redo for every change**, a History window with restore points, **Select mode** for
  changing many people at once, **Bring Everything In** checklist, **Back Up To…** a drive,
  **where each detail came from**, a **timeline** kept apart from notes, exact re-imports by
  source ID, and name-only matches listed for review with a pre-selected answer.

- **Calendar log**, **business card images**, **one-click personal photo**, **notes clean-up**,
  **Deduplicate** (cards and repeats inside cards), and **contact buttons** for every app the card
  has a link or handle for.
- **Telegram, Facebook and Snapchat exports**, phones over Bluetooth / AirDrop (Import wizard), notes
  saying where details came from, and automatic clean-up during import.

- **Import from every source:** Mac/iPhone, other phones, Gmail, Outlook, Yahoo, iCloud, LinkedIn,
  spreadsheets (.csv/.xlsx) and notes, with matching and merging into existing cards.
- **Phone and email clean-up:** 00/+, country codes from clues on the card, typos, duplicates and
  wrong numbers.
- **Names in Arabic and English** on one card, matched and searched in both.

- **Six degrees of separation:** link people as friend, family, colleague… (⌘K); *How Are They
  Connected?* (⌘⇧K) finds the shortest chain between two people, up to six steps. Built offline,
  from your own contacts only.
