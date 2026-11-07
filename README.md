# Smart Contacts

A fast, native Mac address book written in Go ([Fyne](https://fyne.io)). Your contacts stay on your
Mac, in one readable file, with daily backups and a 30-day trash. Arabic and English names search
the same way, and an optional AI reader turns a pasted email signature into a contact.

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

- **Three panes** — groups on the left, a searchable list, the contact card on the right.
- **Search that forgives** — names in any script, with or without accents or hamza (أحمد = احمد),
  part of a phone number with or without spaces (`9999 8888`, `٩٩٩٩`), company, notes, groups.
  ↑ ↓ move through results, Return edits.
- **Smart Add** (⌘⇧N) — paste an email signature, business-card text or a WhatsApp contact
  message. It's read privately on this Mac: a built-in reader picks out
  names, titles, companies, numbers (and whether they're mobile, work or fax), emails and links.
  (Optional Cloud AI can do it instead.) You always check the result before saving, and it warns
  if the person is already in your contacts.
- **Quick actions** — Call (FaceTime / iPhone), Message, WhatsApp, Email, open the address in
  Maps, copy any value. The app remembers when you last contacted someone.
- **Birthdays** — enter `1990-03-14`, `14/03/1990` or just `14/03`; the Birthdays list shows the
  next 30 days with the age they turn. 29 February is handled.
- **Groups** — tick groups on the Edit page; renaming or deleting a group never deletes people.
- **Import** — from the Mac's own Contacts app (one click, macOS asks permission once) or any
  `.vcf` file (Apple, Google, Outlook, Android; vCard 2.1, 3.0 and 4.0, photos included).
  Contacts you already have are spotted and skipped.
- **Export** — vCard or CSV (opens cleanly in Excel and Numbers, Arabic included), or send
  copy contacts into Apple's Contacts app (with an iCloud warning).
- **Find Duplicates** — cards sharing a phone, email or name, merged into the fullest card.
- **Clean Up** — writes every number one way (`+965 9999 8888`), fixes ALL-CAPS names, lower-cases
  emails, adds `https://` to links. You tick which changes to apply.
- **Safe by default** — atomic saves, a backup for each of the last 30 days, a 30-day trash.

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
| ⌘O | import a .vcf |
| ⌘, | settings |

## Building it yourself

Needs Go and the Xcode command-line tools (`xcode-select --install`):

```bash
git clone https://github.com/jamalxcode/smart_contacts.git
cd smart_contacts
./build.command      # tests, packages, installs to /Applications and opens it
```

Code map: `model.go` (contacts, search, phones, birthdays) · `store.go` (file, backups, trash,
photos) · `vcard.go` · `tools.go` (CSV, duplicates, clean-up) · `smart.go` (Smart Add) ·
`mac.go` (Mac Contacts) · `ui.go`, `editor.go`, `dialogs.go` (the window) · `model_test.go`.

See [LESSONS.md](LESSONS.md) for notes from building it.
