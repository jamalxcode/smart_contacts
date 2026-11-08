# Lessons learned

Notes from building Smart Contacts in one cloud session (2026-11-07). Builds on the lessons in
`office_lights/LESSONS.md`; only what was new is written here.

## 1. Building a Fyne Mac app without touching the Mac

- **The real compile happens on GitHub.** `.github/workflows/build.yml` runs on a `macos-15`
  runner: `go mod tidy`, `go vet`, tests, `fyne package --os darwin --release`, ad-hoc
  `codesign`, a smoke test of the packaged binary, then a rolling `latest` release. The Mac never
  needs Go, Xcode or a build script.
- **Install with curl, not a browser.** Files a browser downloads get the quarantine flag and an
  ad-hoc-signed app is then blocked by Gatekeeper. `install.sh` fetches the zip with `curl`, unzips
  with `ditto` into /Applications and clears any quarantine — one line in Terminal, also the update.
- **Add Info.plist keys after packaging.** `fyne package` has no field for privacy strings; the
  workflow adds `NSAppleEventsUsageDescription` / `NSContactsUsageDescription` with PlistBuddy
  *before* codesigning (signing after any change, or the signature is invalid).
- **You can compile and screenshot Fyne for real in the cloud** (better than the API stub used for
  office_lights): install `libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev xvfb`, build for
  Linux, run under `Xvfb`, drive it with `xdotool`, capture with ImageMagick `import`. Every
  screenshot in `docs/` was taken that way, and each one caught something.
- **When proxy.golang.org is blocked**, Go modules on github.com still fetch with `GOPROXY=direct`.
  Vanity paths (`fyne.io/*`, `golang.org/x/*`, `gopkg.in/*`) don't, so clone them from their GitHub
  mirrors at the right tag and point a `go.work` (kept outside the repo, `GOWORK=…`) at them with
  `replace`. The committed `go.mod` stays clean and CI resolves normally. Note `-mod=mod` is refused
  in workspace mode.
- `golang.org/x/tools/go/vcs` lives at tag `go/vcs/v0.1.0-deprecated` in the tools repo, and
  `fyne.io/systray` is pinned to a pseudo-version commit — check the dependency's go.mod.

- **"Bounces in the Dock, never opens" was macOS quarantine, not the code.** Activity Monitor →
  select the process → View → Sample Process showed it frozen at `_dyld_start` (1 thread, 112 KB)
  and running from `/Volumes/VOLUME/…` (App Translocation of a browser/chat download). An ad-hoc
  signed, quarantined app can hang there on macOS 26 with no Gatekeeper dialog at all. Every later
  launch only re-activated that frozen copy, so each "new build" looked broken too. Fix: install
  with `install.sh` (curl doesn't quarantine) or `xattr -dr com.apple.quarantine`; the lasting fix
  is a Developer ID signature + notarisation. Sample the process *before* changing code.

## 2. Fyne 2.8

- **Arabic works**: Fyne falls back to system fonts per character and shapes Arabic correctly,
  word order included. For avatars, use one initial for non-Latin scripts (two joined Arabic
  letters look wrong).
- **Low-importance labels use the *disabled* colour**, which is almost invisible in dark mode. A
  three-method theme wrapper (`theme.go`) that returns macOS's secondary-label grey for
  `ColorNameDisabled` fixed every faint caption at once.
- `canvas.Image` has `CornerRadius` and `ImageFillCover` — a circular photo avatar is one object.
- `widget.SelectEntry` gives a label picker that also accepts a custom label ("Driver", "Office in SF").
- Container field order: `container.NewBorder(top, bottom, left, right, center...)` stores
  `center` *first* in `.Objects`, then the edges. List item updaters index into that.
- Setting `list.OnSelected = nil` around `list.Select(i)` lets you highlight without re-running
  the selection logic (same idea as the `loading` guard in office_lights).
- A literal U+FEFF in Go source is a compile error ("illegal byte order mark"); write `"\uFEFF"`.
  The file-writing tool turned the escape into the character, so grep for `\xef\xbb\xbf` after.

## 3. Contacts data

- **Apple vCards**: custom labels come as a group pair (`item1.TEL` + `item1.X-ABLabel`,
  sometimes wrapped as `_$!<Main>!$_`); a birthday with no year is written as year **1604**
  (`X-APPLE-OMIT-YEAR`). Both round-trip in `vcard.go`.
- vCard 2.1 (old phones) uses bare `TEL;CELL;PREF` params and quoted-printable values with soft
  line breaks; folded lines must be cut on a UTF-8 boundary when writing.
- **Search folding** is the feature people notice: lower-case, strip accents and Arabic harakat,
  unify أ/إ/آ→ا, ة→ه, ى→ي, Arabic-Indic digits → 0-9, and match phone queries on digits only.
- Phone normalisation needs a home country code; 8-digit Kuwaiti numbers with no prefix are the
  common case. Keep what the user typed; offer Clean Up to rewrite it, with a tick per change.
- Imported cards should keep their own date (`REV`), or a first import floods "Recently Added".
- Duplicates = union-find over shared normalised phone / lower-cased email / folded full name,
  **but a shared key alone is not proof**. The first version would have merged two different
  "Mohammed Ali"s, a whole family sharing a home landline, and an employee into the company's
  card (same main line), and import silently skipped those people. Rule now (`sameEnough`): a
  shared phone/email needs compatible names (same first name, one name inside the other, or one
  card nameless and not a company); a shared name alone only counts when one card has no number
  or email. Merge into the fullest card; the rest go to the Trash, never deleted.
- Check "is it already there?" with an index (`DupIndex`), not pairwise; pairwise was
  4 million comparisons for a 2,000-contact re-import.
- Short and service numbers (171, 112, Kuwait's 7-digit 180xxxx lines) must not get the country
  code: 8-digit countries (KW, BH, QA, OM) treat anything shorter as a service number.
- A native Mac menu shortcut fires before any text field sees the key: ⌘⌫ bound to "Delete
  Contact" would fire while typing in search. Check `Canvas().Focused()` in the handler.
- ⌘Q from the app menu skips `SetCloseIntercept`; save settings in `Lifecycle().SetOnStopped`.
- Cache folded search text per contact, invalidated by `Updated`; any code that edits a contact
  outside `Store.Put` (group rename/remove, clean-up) must bump `Updated`.
- AppleScript's default Apple Event timeout is 2 minutes; wrap big Contacts reads in
  `with timeout of 900 seconds`.
- GitHub's macOS runners have no OpenGL, so no Fyne window can open there ("NSGL: Failed to find
  a suitable pixel format") — the launch test is informational. Don't mistake it for the app's bug
  (I briefly did and downgraded Fyne 2.8.2 → 2.6.3; 2.6.3 is also what office_lights runs, so it
  stayed). Fyne 2.6 has no rounded images, so photos are cut to a circle PNG when loaded.

## 4. Smart Add

- The offline parser must skip sign-offs ("Best regards," was being read as a name) and read
  inline tags (`M: … | T: …`) per number, not per line.
- Test the parser with a realistic signature before trusting it; the AI path returns JSON that
  goes through the same normalisation, and on any error the app falls back to the local parser.
- The OpenRouter key sits in `config.json` (0600), never the Keychain — by request, because the
  Keychain keeps asking for the Mac password.

## 5. Privacy and connections (decided 2026-11-07)

- **Everything stays on this Mac.** No accounts, no sync, no network unless the user ticks Cloud AI
  (off by default). Data folder 700/600 with `.metadata_never_index`. Anything that could copy
  contacts off the Mac (Apple Contacts → iCloud, exports into iCloud Drive/Dropbox) asks first.
  Native `whatsapp://` and `maps://` links instead of web pages.
- **No online enrichment, by decision.** Automatic LinkedIn/social lookup by name, number or email
  was declined by the user ("keep it fully offline"); Google/X OSINT profiling of contacts is out
  of scope (it builds dossiers on private people). Profile links are added by hand.
- **Connections** (`graph.go`): symmetric user-recorded links plus implicit same-company links
  (company cards excluded, legal suffixes like K.S.C.P/W.L.L ignored). BFS for the shortest chain,
  capped at six steps; recorded links are listed before company links so they win ties. Links
  survive the Trash, follow merges (`RepointLinks`) and are dropped when the Trash is emptied.
- An Arabic name and a +965 number in one label get reordered by the bidi algorithm; put them in
  separate labels.

## 6. One source of truth (import, clean-up, names)

- **Import is a merge, not an append.** `PlanImport` sorts each incoming card into new / update an
  existing card / up to date / ask / repeated-in-file, shown as a preview; `ApplyImport` merges.
  Strong matches: shared phone, email or social profile link (a LinkedIn URL is an identity) with
  names that fit. Name-only matches merge only when exactly one person has that name and nothing
  contradicts it; otherwise the user picks. Re-importing the same file changes nothing (tested).
- **LinkedIn's Connections.csv starts with note lines**; find the header row instead of assuming
  row 1. Outlook's "Title" column is Mr./Dr. when "Job Title" exists. Google pairs "Phone 1 - Type"
  with "Phone 1 - Value" and packs several values as " ::: ". Excel stores typed phone numbers as
  numbers (9.6599998888E10). A small zip+XML reader handles .xlsx without a dependency.
- **Phone numbers need the country's rules**, not just digits: trunk zeros (UK 0, Saudi 0 after
  +966), Kuwait has no trunk 0 but people add one, 8-digit Kuwaiti numbers start with 2/4/5/6/9,
  services are 3–4 digits or 7 starting with 1. A local number with no country takes it from clues
  on the same card (address, other numbers, email TLD); otherwise ask, never guess.
- **Cross-script names:** a consonant skeleton (Mohammed = Muhammad = محمد = "mhmd"), dropping the
  article (Al-/El-/ال), "Abdul" joins (عبدالرحمن = Abdul Rahman), Gulf ض = dh, silent final ة/h.
  It is deliberately loose, so it only ever *proposes* name-only matches.
- Clean-up changes are applied by value/key, not by index: removing one field shifts the others.
- Test with the data folder you mean: one screenshot run without `SMART_CONTACTS_DIR` opened an
  empty default folder and looked like data loss. `--preview <file>` opens straight into an import.

## 7. Reach, calendar, cards (2026-11-08)

- **Buttons from data, not a fixed set:** `Contact.Actions` builds them from phones, emails and
  profile links (`parseProfile` reads URLs and bare "@handles" under a service label). Deep links:
  `tg://resolve?domain=` / `?phone=`, `https://ig.me/m/<user>`, `https://m.me/<user>`,
  `sgnl://signal.me/#p/+…`, `facetime://`, `msteams:/l/chat/0/0?users=`. Guesses ("by number")
  go under More.
- **A horizontal scroll container swallows the mouse wheel** in Fyne: the card stopped scrolling
  whenever the pointer was over the group chips. Use a wrapping GridWrap instead.
- **Calendar via AppleScript is fast only in bulk:** fetch `uid/summary/start date… of (a reference
  to every event whose …)` once per calendar, then loop locally; use ASCII 30/31 as separators
  (notes contain tabs and newlines). `«class isot»` gives an ISO date.
- **OCR without cgo:** `osascript -l JavaScript` can drive Vision (`VNRecognizeTextRequest`)
  synchronously; ask for ["ar-SA","en-US"] and fall back to English. HEIC → JPEG with `sips`.
- **Facebook exports are mis-encoded** (UTF-8 bytes as Latin-1 characters); re-encode runes ≤ 255 as
  bytes and keep the result if it is valid UTF-8.
- **Merging loses disagreements unless you write them down:** `noteDifferences` notes another
  source's company/birthday; LinkedIn's job is taken as current with "Previously: …". A Google row
  that only disagreed on company was first classed "up to date" — differences now count as changes.
- Rebuild before screenshotting: one round of screenshots showed a stale binary without the new
  sections.

## 8. Trust: undo, origins, timeline (2026-11-08)

- **Undo at the store, not per screen:** every `Store.Save` pushes the previous state with a label
  worked out by comparing (`describeChange`), so imports, clean-ups, merges and bulk actions are all
  undoable without each knowing about undo. Files that undo needs (photos, card images) are renamed
  to `.removed` rather than deleted. The first change to an empty book was lost until `lastSaved`
  was set on open.
- **Phone keys must survive formatting:** origins keyed on the last 9 digits broke between
  "6000 1234" and "+965 6000 1234"; the last 8 work for Kuwait and are distinct enough elsewhere.
- **Automatic text doesn't belong in notes:** import facts went into Notes and drowned what the user
  wrote. A separate timeline fixed it; Clean Up moves the old automatic lines across. A batch whose
  only news was a timeline entry was classed "up to date" and dropped — new entries count as changes.
- **Name-only matches are never silent:** they go into their own review list with a default and a
  confidence, so a wrong merge is a choice the user saw.
- **Clean-up can make things worse:** "add https://" turned "@khalid_kw" into "https://@khalid_kw".
  Links now go through `fixLink`, which turns a handle into its profile page and repairs the damage.
- **Fyne layout:** hiding a container inside a Border doesn't give its space back until the parent
  is refreshed; Accordion in 2.6 has no open callback (custom fold headers remember their state);
  menu shortcuts fire before a focused text entry, so ⌘Z must hand off to the entry while editing.

## 9. Updating from inside the app

- The app downloads and unpacks the new build itself (with `ditto`, which keeps the signature)
  *before* quitting, so a failed download leaves the running app alone. A detached helper
  (`Setsid`) waits for the app's process to end, moves the old .app aside, moves the new one in,
  restores the old one if that fails, and reopens it.
- Downloads made by the app's own HTTP client aren't quarantined, like curl's, so no Gatekeeper
  prompt. A translocated copy (opened from a quarantined zip) can't update itself.
- The release notes carry the commit message so the update dialog can say what changed.

## 10. Making a Fyne app feel like a Mac app (2026-11-08)

- **Theme:** Apple's NSColor values for light and dark (system blue accent, label/secondary/
  tertiary greys, separator), 13-pt text, 11-pt captions, 4/7 padding, 6-pt corner radius. The
  font stays Fyne's: macOS ships San Francisco only as a variable font with no separate bold file.
- **Fyne moves menu items labelled exactly "About" and "Settings…"** into the macOS app menu.
  "About Smart Contacts" stays where it is, so the label is just "About".
- **The split divider is painted with the shadow colour**; a theme override on the split container
  makes it a light gutter without touching anything else.
- **Any widget that handles a mouse event takes every click:** a right-click wrapper on list rows
  stopped rows from selecting until it passed left clicks back to the list.
- **Fyne dialogs ignore Esc and Return;** dialogs are tracked and the window's key handler
  dismisses or confirms the top one. Yes/No buttons were replaced with verbs (HIG).
- Menu shortcuts are caught before text fields, so Cut/Copy/Paste/Select All forward to the
  focused field; ⌘C with nothing focused copies the card.
- Missing glyphs: "↔" rendered as a coloured blob in Fyne's font; use words.

## 11. Big address books, history, reminders (2026-11-08)

- **Measure with a fake 10,000-card book** (`SCALE=1 go test -run TestScale`): search was fine
  (≤40 ms) but each save took 230 ms and every undo step held a full copy of the book. Saves now
  encode cards in parallel and compare per card; an undo step holds only the cards that changed.
  Save fell to ~40 ms.
- **Undo across restarts** is written on quit, tied to a hash of the exact contacts file; if the
  file changed since (restore, another copy), the old history is dropped rather than misapplied.
- **The Mac's own history is readable** with `/usr/bin/sqlite3` once the app has Full Disk Access:
  `chat.db` (handle/message, dates in ns since 2001), `CallHistory.storedata` (ZCALLRECORD, s since
  2001), Mail's `Envelope Index` (messages/addresses/recipients/mailboxes, Unix seconds). Copy the
  database with its -wal/-shm first. Tests build tiny databases with the same layout.
- **Suggest only real conversations:** "People Not in Contacts" counts only people you wrote to or
  called, several times — otherwise every newsletter sender shows up.
- **AI rewrites need a safety net:** numbers, emails and dates in the original are checked against
  the rewrite, and one that lost any isn't pre-ticked.
- Fyne's system tray is a menu only (no text field), and there's no global hotkey API; Quick Find
  is a small window opened from that menu.

## 12. The Filofax look (2026-11-08)

- **Photo-real without image files:** leather is value noise (pebbles + creases + dye variation +
  vignette), paper is a cream rectangle with a stretched grain overlay, rules and the margin are
  canvas lines laid out for the page's height, holes are circles showing the leather colour, rings
  are a shaded ellipse with a specular band. Everything is generated once at start-up.
- **A nested ThemeOverride must wrap the current theme,** not a fresh default one: the card's
  override used the old theme and its text vanished on the paper.
- Index tabs and A–Z tabs are small custom widgets; the selected tab sticks out further, like a
  real divider pulled forward.

## 13. Following a real photo (2026-11-08)

- **Fyne can't draw colour emoji,** so flag "emoji" are 48×36 PNGs rasterised with rsvg-convert
  from flag-icons (MIT) and embedded; the country comes from libphonenumber's region for the number.
- **A serif without Arabic breaks Arabic text** (Fyne doesn't fall back between fonts). The theme
  keeps the app font for everything and gives the serif only to text that asks for it with the
  otherwise unused Monospace style — and never to text containing Arabic.
- **Splitting "AB" by bytes broke "ع"** (two bytes in UTF-8): split by runes.
- Leather corners are rounded with small desk-coloured corner masks, since an image can't have a
  corner radius; stitching is a row of short lines laid out for the current size.

## 14. Updates that survive a busy GitHub (2026-11-08)

- "Download failed: 504 Gateway Timeout" came from GitHub's file-storage server, not from a missing file:
  the same file downloaded fine a minute later, and through the API link at the same moment.
- The updater now alternates between the two links on every attempt (pauses of 0, 3 and 8 s), gives each attempt 20 s to start answering and 30 s of silence at most, resumes with a Range request, and has a Cancel button. A 504 can take a minute to arrive, so without those limits the old version looked stuck.
  request, with GitHub's API asset link tried first (the plain release link kept failing), then the other link. Only if all of that fails does it say "try again in a few
  minutes" and show the Terminal line. install.sh does the same with curl --retry and -C -.
- The build used to delete and recreate the "latest" release, leaving a moment with nothing to
  download. It now replaces the file in place.

## 15. Opening the book without leaving the screen (2026-11-08)

- Fyne can't move a window or read the screen size, so opening the book grew the window off the right
  edge. window_darwin.go reaches the real Mac window (Fyne's RunNative gives the NSWindow) and animates
  it: the right edge stays put so the cover seems to swing open to the left, then the frame is pulled
  inside the screen's visible area (below the menu bar, above the Dock) and shrunk if the screen is small.
- The same helper gives the title bar the dark grey of the "desk" behind the book, so no white shows.
  Other systems just resize (window_other.go).

## 16. Saying what a command acts on, and what it did (2026-11-08)

- Commands were ambiguous: some used the ticked cards, some the open card, Clean Up always everything.
  The Card menu now has three headed sections (This Card — name / N Selected Cards / All N Cards) and
  every title names its target, as Finder's "Move 3 Items to Trash" does. The menu bar is rebuilt
  when the open card or the selection changes (skipped when nothing would differ).
- Feedback comes from one place: every save that records an undo step compares the step's "before"
  cards with the cards now (changes.go), so any command, old or new, gets the same banner, the
  Show Changes list and blue dots without its own code. A command's flash text becomes the banner title.
- Fyne pads window content with the theme background by default; SetPadded(false) removes the
  cream frame round the dark grey.

## 17. Making the organiser explain itself (2026-11-08)

- Rule followed: everything you can see should do what that object does in real life; anything
  with no real-world counterpart is turned into one. Pen = new card, page corner = next/previous
  letter, pocket = business cards, strap = close, paper clips = choosing several cards, sticky
  note = what to do with the clipped cards, rubber stamp + pencil note = what just changed.
- Ruled fields without rebuilding the editor: the card's theme hides Fyne's field box until a
  field has focus, and a transparent overlay (ruled.go) walks the card's containers and draws a
  line under every Entry/Select at its real position. It re-lays out whenever the card resizes.
- Page turns use Canvas.Capture(): a picture of the old page is laid over the new one and
  squeezed towards the rings (forward) or the outer edge (back) in 340 ms.
- The stamp is drawn once with x/image/opentype (the cover's serif), its ink knocked back with the
  same value noise as the leather, and tilted 8° with x/image/draw's bilinear transform.
- Hints on the book are drawn objects, not pop-ups: a pop-up would take the click meant for the
  object under it.

## 18. Never rebuild the Mac menu bar

Opening one card and then another made the app vanish. Since 1.0.33 the Card menu named the open card, so every card change threw the menu bar away and built a new one. Fyne's Mac menu code releases each divider one time too many, so a rebuilt menu bar eventually touches freed memory and the app dies (a Go exit, so macOS writes no crash report). It never showed on the Linux test screen, where Fyne draws its own menus.

- The menu bar is now built once. `refreshMenu` only updates each item's action, dimmed state and tick; the Mac reads those afresh every time a menu opens.
- Titles and layout are fixed: "This Card", "Clipped Cards", "Whole Address Book", "Delete…". Counts and names live on screen instead (the sticky note, the banner, the confirmation).
- `TestMenuLayoutNeverChanges` fails if a title depends on which card is open, what is clipped, or the groups.
- `menu.Refresh()` and `SetMainMenu` rebuild too, so don't call them.
- The app now writes `~/Library/Logs/Smart Contacts/crash.log` (and `app.log`) if it ever quits unexpectedly. The previous crash is kept as `crash-previous.log`. Neither contains contacts.

## 19. The icon

`Icon.png` is drawn in code by `TestMakeIcon` (`MAKE_ICON=Icon.png go test -run TestMakeIcon .`), with the same leather, brass and serif fonts as the cover, so it matches the closed organiser. macOS 26 puts any icon that doesn't fill Apple's rounded square onto a grey tile, so the icon fills the whole square (`squircle`): a fat organiser on dark felt, with cards out of the top, a deep page block and coloured tabs down the fore-edge. After an update, `touch` and `lsregister` make Finder and the Dock show the new icon instead of a cached one.

## 20. A card is read, then written on

A form showing every empty field hides the few lines that matter. A card that is always editable can be changed by one stray keystroke. So a card now has two states.

- **Reading** (`readcard.go`) shows only what's written: the name, job and company in the serif, the ways to reach them, then one ruled `inkLine` per number, email, address, link and birthday. Clicking a line offers what it's for; double-clicking writes on that field. One faint "+ add …" line lists what's missing.
- **Writing** is the old editable card, with Done instead of Save. `showDetail` leaves a card that's being written on alone.
- Fyne gives Esc only to the field that has the keyboard, never to the window. So the card's fields are a `cardEntry` that passes Esc to the card. Any type check for "is the user typing" must use `typing()`, or ⌘⌫ would delete the card while you type on it.
- The front stays short; Turn Over shows the back (notes, how you met, keeping in touch, history), with a squeeze animation like a card turning on its axis.
- One way to do each thing in the organiser: the pen makes a new card, the pocket takes business cards, and the funnel in the search field opens the filter. Sources are in the filter form and `from:` searches, not tabs.
- Hints go away once you've used the thing they explain (`cfg.Learned`). The status line then shows counts instead: contacts, cards to tidy, people to reach out to.

## 21. Ten commands and one rule

Twenty-five Card commands came down to ten by having one rule for what they act on: the clipped cards if any are clipped, otherwise the open card (`targets()` in `cardcmds.go`). Edit → Delete already worked that way.

- Commands that did the same job at three scopes became one command. Tidy Up and Find Duplicates ask for the scope in their window (this card, the clipped cards, or everyone).
- Pairs became one window: Groups (add and remove), Connections (connect, and how they're connected), Check Messages, Calls & Mail (update "last talked", then list the people not in your contacts).
- The toolbar above the organiser (`toolbar.go`) has the same commands in the same groups. Its left side says what they'll act on. It's refreshed with the menu (`refreshMenu` → `refreshToolbar`).

## 22. Menus and bars are the same thing twice

- The **Card menu** and the **toolbar** have the same ten commands in the same order and groups, with identical names. The menu adds "…" where a window opens. Toggles look the same in both: a tick in the menu, a lit button in the toolbar (Favorite, Clip Cards).
- The **View menu** and the **sidebar** (the organiser's tabs, or the plain look's list) have the same sections: the lists, To Tidy, Saved Searches, Groups, Sort, Also in the Book. The sidebar names each group and saved search. The menu can't, because its titles never change (section 18), so it uses Go to Group… and Go to Saved Search…. A list that isn't in the sidebar right now (no one missing a phone, say) is dimmed in the menu.
- When renaming anything, rename it in both places. "Upcoming Birthdays" is "Birthdays" everywhere.

## 23. Windows are things on the desk

- A window that wants something filled in is a **clipboard**: a wooden board, a steel clip, and a cream sheet whose fields are ruled lines. One that only tells you something or asks you to confirm is a **postage stamp**: perforated edges, an engraved skyline, a postmark with today's date. The toolbar's buttons are **coloured card tabs** standing up out of the book.
- Every window goes through `paperdialog.go`. Its `newConfirm`, `newCustom`, `newForm`, `newInformation` and `newError` have the same shape as Fyne's dialog functions. It picks clipboard or stamp by looking for inputs in the content (`hasInput`). In the plain look, or while the book is closed, it falls back to Fyne's own windows.
- The windows live on `u.modalLayer`, the top of the organiser's stack, with a dimmed desk under them that swallows clicks. Esc and Return work through the same `trackDialog` as before. A destructive confirm never answers Return.
- The paper hides input borders so fields read as ruled lines. Fyne draws tick boxes and radio buttons with that same colour, so `inkChoices` gives them back an ink outline.
- File pickers (open, save, choose folder) stay Fyne's own.

## 24. Don't call a menu item "About"

Fyne moves menu items titled exactly "About", "Settings…" or "Preferences…" into the macOS app menu. For About it takes over Apple's own item. That didn't always hold: About sometimes did nothing until the app was reopened. The Help item is now "About Smart Contacts…", which Fyne leaves alone. The app menu keeps Apple's standard About panel, which shows the name, version and icon from the app bundle.

## 25. The Today page

A real organiser opens on today. `grpToday` is the first tab. When it's open, the left page shows a diary page (`today.go`) instead of the list: the date in the serif, then Birthdays this week, Time to get in touch, Met recently (from the calendar log) and Cards to tidy.

- Each person has quick actions (call, message, WhatsApp, Done, Note, Review). Clicking the row opens their card.
- The list underneath still holds today's people, so keyboard walking and selection work. A search swaps the page back to the list.
- Today is first in the View menu (⌘0), the Card menu and the toolbar, so all three match the tabs.

## 26. Ideas not done yet

- Organisation / "how I know them" / "met at" fields the user fills in, feeding connections.
- A menu-bar quick search.
- Birthday reminders as Mac notifications.
- Encrypted backups (Back Up To… writes a plain .zip).
- Two-way sync with Mac Contacts (today: import, and "Copy to Apple Contacts" with a warning).
- Notarised signing (Apple Developer ID) so a downloaded zip opens with a double-click.
