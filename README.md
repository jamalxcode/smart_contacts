# Smart Contacts

A fast, native Mac address book written in Go ([Fyne](https://fyne.io)), built to be **the one place
all your contacts come together**: your Mac and iPhone, other phones, Gmail, Outlook, LinkedIn,
spreadsheets and notes are imported, matched and merged into a single, clean card per person.
Everything stays on your Mac, in one readable file, with backups and a 30-day trash. Names work in
Arabic and English, and numbers are tidied into one international format.

<p align="center"><img src="Icon.png" width="128" alt="Smart Contacts icon"></p>

![Smart Contacts](docs/screenshot.png)

## Install

Paste this into Terminal on your Mac (Apple Silicon):

```bash
curl -fsSL https://raw.githubusercontent.com/jamalxcode/smart_contacts/main/install.sh | bash
```

It downloads the latest build, puts **Smart Contacts.app** in Applications and opens it. Run the same
line again to update. (Downloading with `curl` avoids the "unidentified developer" warning a browser
download would trigger, because the app is ad-hoc signed rather than notarised.)

Every push to `main` is compiled, tested and packaged by GitHub on a real Mac
([build workflow](.github/workflows/build.yml)) and published as the
[latest release](https://github.com/jamalxcode/smart_contacts/releases/tag/latest).

## What it does

**Bring everything in (⌘I).** File → Import Contacts takes contacts from:

| Source | What to export |
|---|---|
| Mac & iPhone | nothing — read straight from the Mac's Contacts app (iPhone via iCloud) |
| Another phone | Android: Contacts → Export → .vcf · old iPhone: icloud.com → Export vCard |
| Gmail / Google | contacts.google.com → Export (Google CSV or vCard) |
| Outlook / Hotmail | outlook.com → People → Manage → Export contacts (CSV) |
| LinkedIn | Settings & Privacy → Data privacy → Get a copy of your data → Connections |
| Yahoo, iCloud, others | the account's export, as vCard or CSV |
| Spreadsheets | any .csv or .xlsx — columns matched automatically (English or Arabic headers), adjustable |
| Notes / text | a .txt or pasted text with several people (one per line, or blocks) |

Or drag files onto the window. Every import shows a preview first:

- **Matching:** people you already have are matched by phone, email, LinkedIn profile or name
  (across Arabic and English, and across spellings like Mohammed / Muhammad / محمد).
- **Enriching:** a matched card gets what's new (numbers, emails, job title, company, LinkedIn
  link, the name in the other language) instead of a duplicate being created.
- **You decide on doubtful matches:** two people with the same name are never merged by name
  alone without you.
- **Sources:** each card shows where its details came from, and the sidebar lists your sources.
- **Safety:** a backup is saved before every import, and Clean Up is offered straight afterwards.

**Clean Up (Tools).** Phones and emails are checked against what's known:

- **Phones:** 00 → +, +965 added to Kuwaiti numbers, stray zeros removed (+44 (0)20…,
  +966 05…, an extra 0 on a Kuwaiti number), and Arabic digits converted. A number with no
  obvious country (0501234567) takes its country from clues on the same card: address, other
  numbers, email domain. If nothing fits, you pick.
- **Emails:** typos fixed (gmial.com, .con), mailto: and stray spaces removed, and values in the
  wrong field moved (an email typed as a phone, or the other way round).
- **Duplicates:** the same number or email twice on one card is removed.
- **Wrong numbers:** 00000000, 12345678, too short or too long, or impossible Kuwaiti prefixes are
  flagged for removal. Clear junk is pre-ticked; numbers that might be real are not.
- **Safety:** 171, 112 and 7-digit 180… service lines are left alone, and a backup is saved
  before applying.

**Names in two languages.** Each card can hold the name in Arabic and English side by side
(محمد المطيري / Mohammed Al-Mutairi); *Suggest* fills the other spelling for common names. Search
finds people in either language.

**Everything else**

- **Three panes** — groups and sources on the left, a searchable list, the contact card on the right.
- **Search that forgives** — any script, with or without accents or hamza (أحمد = احمد), Persian
  letters, part of a phone number with or without spaces (`9999 8888`, `٩٩٩٩`), company, notes.
- **Smart Add** (⌘⇧N) — paste one person's signature or business card; read on this Mac
  (optional Cloud AI). It warns if they're already in your contacts.
- **Quick actions** — Call, Message, WhatsApp, Email, Maps (the Mac's own apps); last contacted.
- **Birthdays** — `1990-03-14`, `14/03/1990` or `14/03`; next 30 days with the age they turn.
- **Groups** — tick on the Edit page; renaming or deleting a group never deletes people.
- **Connections (six degrees)** — link people (⌘K); *How Are They Connected?* (⌘⇧K) finds the
  shortest chain between two people, up to six steps; colleagues at one company are linked.
- **Find Duplicates** — merges cards that are the same person (names must fit, in any language).
- **Export** — vCard or CSV (both names and sources included), or copy to Apple Contacts.
- **Safe by default** — atomic saves, daily backups, backups before imports and clean-ups, Trash.

![Birthdays in light mode](docs/birthdays-light.png)

## Where your data lives

`~/Library/Application Support/Smart Contacts/`

| File | What |
|---|---|
| `contacts.json` | every contact (plain, readable JSON) |
| `photos/` | one small JPEG per contact photo |
| `backups/` | `contacts-YYYY-MM-DD.json`, last 30 days |
| `config.json` | settings, including the optional OpenRouter key (permission 600, not the Keychain) |

### Privacy: everything stays on this Mac

- The data folder is readable only by your Mac user (folders 700, files 600), is excluded from
  Spotlight, and is not in iCloud Drive. FileVault encrypts it on disk.
- The app makes **no network connections and has no accounts or sync**. Smart Add reads text
  locally. Cloud AI (OpenRouter) is **off by default** and only works if you tick
  *Allow Smart Add to send pasted text to OpenRouter* in Settings and add a key.
- Call, Message, WhatsApp and Maps open the Mac's own apps (FaceTime, Messages, WhatsApp, Maps).
- Copying contacts to Apple's Contacts app, or saving an export into iCloud Drive / Dropbox /
  Google Drive, asks first, because those sync off the Mac. Temporary .vcf files are deleted.
- **This repository holds only the code** — no contacts are ever committed here.

## Settings

- **Country code** (default `965`, Kuwait) — used to understand local numbers like `9999 8888`.
- **Sort by** first or last name.
- **Cloud AI** (off by default), with the **OpenRouter key and model** for Smart Add (default
  `anthropic/claude-haiku-4.5`; *Test AI* checks it).

## Keyboard

| Keys | Action |
|---|---|
| ⌘N | new contact |
| ⌘⇧N | Smart Add |
| ⌘F | search |
| ⌘E / Return | edit |
| ⌘S | save |
| ⌘⌫ | delete (to Trash) |
| ⌘⇧F | favourite |
| ⌘K | connect to someone |
| ⌘⇧K | how are they connected? |
| ⌘I | import contacts (all sources) |
| ⌘O | import a file |
| ⌘, | settings |

## Roadmap

Planned: organisation / "how I know them" / "met at" fields linked into connections, a menu-bar
quick search, birthday notifications, editing several contacts at once, and an encrypted private
backup.

Not planned, by design: online lookup or enrichment of contacts (searching Google, X or LinkedIn
for people, reverse lookup by phone or email), cloud sync, or accounts.

Known limits: the app is ad-hoc signed, so install it with the curl line (a browser-downloaded
zip is held by macOS); iPhone HEIC photos need exporting as JPEG first.

## Building it yourself

Needs Go and the Xcode command-line tools (`xcode-select --install`):

```bash
git clone https://github.com/jamalxcode/smart_contacts.git
cd smart_contacts
./build.command      # tests, packages, installs to /Applications and opens it
```

Code map: `model.go` (contacts, search, phones, birthdays) · `store.go` (file, backups, trash,
photos) · `vcard.go` · `tools.go` (CSV, duplicates, clean-up) · `smart.go` (Smart Add) ·
`importers.go` (vCard/CSV/Excel/notes readers, import planning) · `phone.go` (numbers by country, emails) · `cleanup.go` · `names.go` (Arabic ↔ English names) · `graph.go` (connections, six-degrees paths) · `mac.go` (Mac Contacts) · `ui.go`, `editor.go`, `dialogs.go` (the window) · `model_test.go`.

See [LIMITATIONS.md](LIMITATIONS.md) for what the app deliberately won't do and what's next, and
[LESSONS.md](LESSONS.md) for notes from building it.
