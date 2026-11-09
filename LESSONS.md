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

## 26. Importing is a conversation

Fourteen import choices on one sheet overwhelmed people. Import is now a wizard on a clipboard (`importwizard.go`):

- It asks where the contacts are, then sometimes one more question, then shows numbered steps ending in the button that imports. Back goes back a page.
- The questions and guides are plain data (`wizQuestion`, `wizAnswer`, `wizGuide`), so adding a source means adding an entry.
- Choose File opens in Downloads, because every guide sends the export there.
- The winged-cards funnel (`assets/import_funnel.png`, cut out of its white background) is the Import button under the business-card pocket.
- A 36 px, 144 dpi copy of the funnel is File → Import…'s menu icon. macOS sizes a menu image by its dpi, so a plain 256 px PNG would be huge.
- A dialog made of choices has no text fields, so it's marked `clipboard` by hand.

## 27. Menu icons match the tabs

The Card and View menu items carry the same pictures as their toolbar tabs and side tabs (`menuicons.go`).

- Fyne tints a *themed* menu icon with macOS's highlighted-text colour, which is white and vanishes in a light menu. So each icon is redrawn as a plain SVG in its tab's colour, deepened to read in both light and dark menus.
- Fyne's built-in icons include an invisible `fill="none"` background box. Recolouring it turns the icon into a solid square, so it's left alone.
- Icons are set once, with the menu bar. Favorite keeps its tick rather than a star that fills in.

## 28. One command list, and print you can read

- The right-click menu on a person has the same commands, names and icons as the toolbar and Card menu (`rowMenu`). Right-clicking a card opens it, as Mail does. On a clipped card, it acts on all the clipped cards. Its first line says which.
- The sticky note no longer has its own buttons. It counts the clips, says the tabs, the Card menu and right-click now act on them, and keeps Clip All / Unclip All / Take Clips Off.
- Words that sat on the leather in cream are now black on small paper labels: the tab column's headings (TO TIDY, GROUPS…) and the status line (`statusLabel`, refitted with `fitStatus` whenever the text changes). Tab names went up from 12 to 13.5 points.

## 29. A Trash that keeps cards until it's emptied

- Cards moved to the Trash stay until Empty Trash. The 30-day purge is gone.
- Moving one card to the Trash doesn't ask, as Finder doesn't: nothing is lost, and the banner has Undo. Moving several clipped cards asks once, naming them.
- Move to Trash, Show Trash (with Put Back and Put Back All) and Empty Trash are in the Card menu, a grey Trash tab after Check Messages, and the right-click menu. A trash button sits on the card after message.
- The Card menu shows the count ("Empty Trash (3)…") without rebuilding the menu. On a Mac, `setMenuTitle` changes just the NSMenuItem's title in place. Fyne's model keeps the plain title, so `refreshMenu`'s layout check still matches.

## 30. More useful every day

- **Follow-ups** (`followups.go`): a dated to-do on a card. They live inside Keep in Touch rather than adding a tab, appear on Today when due (Later / Done), and have a Follow-ups list. The editor keeps the store's follow-ups on save, like the timeline.
- **Coming up** on Today: matched calendar events in the next 72 hours.
- **Greetings** (`greetings.go`): who, occasion, message with {name}. Then one window per person opens WhatsApp or Messages with the message ready. The timeline note "<occasion> greeting sent (<year>)" warns against sending twice. (`AddEvent` drops exact repeats, which is why the year is in it.)
- **Company pages** (`company.go`): the job line gets a › when others share the company.
- **Tidy one by one** (`tidysession.go`): a queue of cards needing work, skips remembered for the session.
- **Own lines**: `Contact.Custom`, a growable label/value list, searchable.
- **Questions** (`ask.go`): regex rules turn a question into the existing filter syntax. Cloud AI, on Return, sees only the question. `TestQuestionToQuery` covers the patterns.
- **Today is worked out once** per data change (and every five minutes for the clock), not three times per refresh. The tidy check invalidates it when it finishes.
- **Drawn textures** take about 0.15 s, so they aren't cached to disk.
- **No real window on GitHub's Mac**: it has no OpenGL. `TestEveryWindowOpens` drives every flow and window with Fyne's test driver on every build instead. It catches Go-side crashes, not macOS menu bugs.

## 31. Import guides people can follow

- **One shape for every guide** (`wizGuide` in `importwizard.go`): what it does (`intro`), numbered steps each with "You should see…" (`wizStep{do, see}`), "Check before you continue" (`check`), "If it doesn't work" (`trouble`), what you'll have (`expect`). A step with no visible result is where people get lost.
- **A step that says "press X" must have X in sight.** The guide's buttons are pinned under the steps, outside the scroll, and the steps say "at the bottom of this window" — never "below". The step that opens a page also carries the link itself.
- **Put the blocking setting first.** macOS refuses Bluetooth files until Bluetooth Sharing is on *and* set to Accept and Save; the phone just says "refused". That is now steps 1–3, before anything is sent.
- **Vendor menus move.** Check each path against the vendor's current help page before shipping, and again when a user says "there's no …". Seen in one week: Google's Export is under ⋮ More actions; iCloud is Select → Select All Contacts, then Actions → Export vCard; LinkedIn took Connections out of "Want something in particular?" — it's only in the larger archive now (up to 24 hours); Facebook's download moved into Accounts Center → Export your information.
- **Only one copy of the instructions.** Bring Everything In kept its own (stale) wording. It now opens the wizard at that source's guide (`u.importGuide(label)`), and `TestEveryWindowOpens` opens every guide that way and checks its title appears.
- Prefer vCard to CSV where both exist: it keeps photos and notes.

## 32. Speed

- **Never refresh a whole window to animate it.** The paper window's slide-in called `overlay.Refresh()` every frame; a container Refresh re-draws every label and picture inside. Now only the sheet is moved (`sheetPlacer.Layout`).
- **Build big windows once.** The Import wizard is built about two seconds after start, while idle (`prewarmImport`), kept in `u.importWiz`, and reset to its first question on each open.
- A user-reported delay that doesn't show on the Linux test screen (3 s on the Mac vs 0.3 s here) needs a measurement on the Mac; see ISSUES.md.

## 33. Writing it down

- `DESIGN.md` holds the decisions and the rules the code relies on (static menus, one command rule, paper windows); `ISSUES.md` and GitHub issues hold what's open. Ideas that used to sit at the end of this file moved there.

## 34. Pin everything the build pulls in

- **`go.sum` is committed and CI never runs `go mod tidy`.** Before, every push re-resolved the
  libraries, so a new upstream release could change the app (or bring back the menu crash, §4 of
  DESIGN) with no commit here. Now `go.mod`/`go.sum` decide; CI runs `go mod verify`. Go comes
  from `go-version-file: go.mod`, not "stable".
- **`fyne@latest` was not the latest.** `fyne.io/tools` has a stray tag `v1.26.1` (April 2025,
  Fyne 2.6.0) that outranks the real current release `v1.7.4` by number, so `@latest` always
  installed it. Every working build used it, so it is pinned (`@v1.26.1`). Move to 1.7.x together
  with a Fyne upgrade, not on its own.
- **Getting `go.sum` without the module proxy.** Cloud sessions may not reach proxy.golang.org.
  The exact `go.sum` was made by a throwaway branch whose workflow ran `go mod tidy` on GitHub and
  committed the result back (artifacts can't be downloaded from the session). To compile locally,
  a scratch go.mod with `replace` lines pointing each `golang.org/x/…`, `fyne.io/…` and `gopkg.in/…`
  module at its GitHub mirror, used with `GOPROXY=direct GOSUMDB=off -modfile=…`, builds and tests
  the app; it is never committed.

## 35. Split big files by feature, as pure moves

- `dialogs.go` (1,330 lines) became `menus.go`, `settingsui.go`, `helpui.go`, `trashui.go`,
  `groupsui.go`, `smartaddui.go`, `exportui.go`, `cleanupui.go`; `filofax.go` (1,690) gave
  `textures.go`, `tabs.go` and `closedbook.go`. Each function moved with the comment above it,
  `goimports` fixed imports, and a sorted line-by-line comparison proved nothing else changed.

## 36. Fields that come back whole

- **Middle names were glued onto first names** by both the vCard and CSV readers, and prefix and
  suffix were dropped, so "Dr. Yousef Ahmed Al-Kandari, PhD" came back as "Yousef Ahmed
  Al-Kandari". Each part now has its own field. `FullName` stays first + last on purpose: matching
  and duplicate finding depend on it, and a middle name on one copy must not hide the other.
- **Outlook's "Title" is Mr./Dr.**, not a job title, when a "Job Title" column exists. It used to be
  skipped; now it's the prefix.
- **Apple writes other dates and related names as labelled items** (`item1.X-ABDATE` +
  `item1.X-ABLabel:_$!<Anniversary>!$_`, `X-ABRELATEDNAMES`). Wrapped labels are Apple's own and
  are shown in the phone's language, so ours are written the same way.
- **A related name is not a person.** It becomes a connection only when exactly one card has that
  name (or the vCard carries the card's own ID); otherwise it's kept as a line on the card. Linking
  "Ali Hassan" to the wrong one of two would be a quiet wrong merge of relationships.
- **Merge dropped whole lists.** `Merge` never copied `Custom` lines or follow-ups from the card
  being merged in. Found while adding `Dates`; a test now merges every list twice.

## 37. Forgiving typos without slowing search

- Typos are forgiven only when the exact search finds nobody, and only against a small, cached
  set of words (names, nickname, company, email names), with an edit table that gives up as soon
  as a row is over the allowance. 10,000 cards take ~45 ms on the test machine; exact searches
  cost nothing extra.
- No guessing under 4 letters (Ali / Ala / Alia are different people) and never for numbers.

## 38. Say what the Mac will do, don't just warn

- Bluetooth Sharing is a per-user setting: `defaults -currentHost read com.apple.Bluetooth
  PrefKeyServicesEnabled` gives 1 (on), 0 (off), or "does not exist" (never switched on, so off) —
  the same check the macOS 15 security benchmarks use. Bluetooth's power is
  `ControllerPowerState` in /Library/Preferences/com.apple.Bluetooth. The guide and the waiting
  window read both and say so in red or green; the waiting window re-reads every 1.5 s, so flipping
  the switch shows at once. "Accept and Save" isn't readable this way.
- `--bt-status` prints the check, and CI runs it on the real Mac runner: the only place the
  `defaults` call itself can be tested.
- A status built on the UI thread sets itself directly; later checks from goroutines go through
  `fyne.Do` (Fyne 2.6 refuses UI changes from other goroutines).

## 39. Signing waits on one thing only

- Notarisation needs a paid Apple Developer account, which only the owner can hold. The pipeline
  is in place and switches itself on when the six `APPLE_*` secrets exist; without them it signs ad
  hoc exactly as before, so nothing breaks while waiting.
- Hardened runtime blocks Apple Events unless the app has the
  `com.apple.security.automation.apple-events` entitlement; Import from Mac Contacts and the
  Calendar Log need it. Sign the binary, then the bundle, never with `--deep`.
- macOS remembers privacy permissions by code signature. Ad-hoc builds have none that lasts, so
  updates can look like a new app; a Developer ID keeps them.

## 40. Esc, three bugs deep

The real-window test passed while its screenshots showed the Import wizard still open after Esc.
Looking at the pictures found three separate causes:

1. **Windows were registered for Esc only once.** `SetOnClosed` set the "tracked" flag, and
   registering itself calls `SetOnClosed`, so a window shown a second time (Import is built once and
   reused, first shown invisibly at start-up) never got Esc or Return again. `Show` now registers
   every time.
2. **A focused field keeps the key.** Fyne sends keys to the focused widget and only otherwise to
   the window, and its Entry ignores Esc. A wizard with no fields left the search field behind it
   focused, which took Esc to clear the search. Windows without fields now take the focus, every text
   field is a `cardEntry` that closes its window on Esc, and a test forbids plain Fyne entries.
3. **Fyne's `Container.Remove` doesn't redraw.** The closed window stayed painted until something
   else refreshed — a click on Close did, so Close seemed to work and Esc didn't. `Hide` refreshes.

The test now also compares the screenshots before and after each open/close and fails if they're the
same.
