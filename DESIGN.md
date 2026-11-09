# Design decisions

Why Smart Contacts is built the way it is, and the rules the code relies on. Read this before
changing menus, windows, commands, storage or the release. LESSONS.md tells the story of how each
rule was learned; this file states the rule.

## 1. Product rules (not up for change without the owner)

| Rule | Why | Where it shows |
|---|---|---|
| **Contacts never leave this Mac.** One JSON file plus photos in `~/Library/Application Support/Smart Contacts` (`SMART_CONTACTS_DIR` overrides it for tests). No cloud sync, no accounts. | Privacy is the product. | `store.go`, LIMITATIONS.md |
| **The repo holds code only, never contacts.** Test data is invented (`testdata/`). | The repo is public. | — |
| **No Keychain.** Keys (OpenRouter) live in `config.json`, mode 0600. | Keychain prompts for the password again and again. | `config.go` |
| **Cloud AI sees only what the user typed** (a question, a note being tidied), never the address book. | Same as above. | `ask.go`, `notesai.go` |
| **No online lookups of people** (no OSINT, reverse lookup, scraping, profiling). | Declined by the owner; see LIMITATIONS.md. | — |
| **The app stands on its own.** Mac Contacts is only a one-time import you start; nothing syncs back or in the background. | The owner asked for it. | `mac.go`, importwizard.go "iPhone → Yes" |
| **Times are Kuwait time**; default country code +965 (Settings → Your country code). | Owner is in Kuwait. | `config.go`, `phone.go` |
| **Nothing is lost silently.** Atomic saves, a backup per day (30 days) and before every import/clean-up, Undo/Redo kept across launches, a Trash kept until emptied. | Contacts are irreplaceable. | `store.go`, `historyui.go` |
| **Every merge and name-only match is reviewed** by the user; nothing is merged automatically. | Wrong merges destroy data. | `mergeui.go`, import preview |
| **Messages are only prepared, never sent** by the app (greetings open WhatsApp/Messages ready to send). | Sending on someone's behalf needs their press. | `greetings.go` |

## 2. The look: a photo-real Filofax

- The default look is an open cognac-leather organiser on a desk (`filofax.go`): sidebar groups
  are coloured index tabs on the left page, letters are tabs on the right edge, cards are ruled
  paper. View → Filofax Look switches to plain Mac panes (`ClassicLook`); every feature must work
  in both.
- **Everything is drawn in code** (leather, wood, paper grain, stamps) — no image assets except
  the icon and the import funnel (`assets/`). Textures are generated once and kept in memory.
- **Text on paper is black ink** (`cardBlack`, `inkTextTheme`), never Fyne's grey captions, which
  disappear on cream paper.
- **The toolbar is the Card menu as card tabs** standing above the book (`toolbar.go`), the
  **sidebar is the View menu** (`viewcmds.go`). Same order, same names, same pictures
  (`menuicons.go`). Change one, change all three (menu, toolbar, right-click `rowMenu`).
- **Windows are things on the desk** (`paperdialog.go`):
  - a window that asks for input is a **clipboard** (wood board, steel clip, ruled paper);
  - a window that only informs or confirms is a **postage stamp** (perforated edge, postmark);
  - `d.clipboard = true` forces a clipboard (the Import wizard asks questions without inputs);
  - in the plain look, or while the cover is closed, they fall back to Fyne's own dialogs.
  - Use `newConfirm`, `newCustom`, `newCustomWithoutButtons`, `newForm`/`showForm`,
    `newInformation`, `newError`, `showCustomConfirm` — never Fyne's `dialog.*` directly.
  - Radios and checkboxes inside paper windows need `inkChoices` or they're invisible.
- **Animations move, they don't refresh.** A container `Refresh()` redraws everything inside it;
  animate by moving/resizing (`sheetPlacer.Layout`). See LESSONS §32.

## 3. Menus (macOS crash rules)

- **The menu bar is built once** (`u.menu` in `setupMenuBar`). Fyne's darwin code over-releases
  separators when a menu is rebuilt, and the app crashes on the next card click.
  `refreshMenu` only changes `Action`, `Disabled` and `Checked`. Labels and layout never change —
  `TestMenuLayoutNeverChanges` and the menu check inside `TestEveryWindowOpens` enforce it.
- A title that must change (Trash counts) is changed natively in place via cgo
  `setMenuTitle(prefix, title)` (`window_darwin.go`; no-op elsewhere). Fyne's model keeps the
  plain title so the layout check still matches.
- **Never title an item "About"** — Fyne hijacks it into the app menu and it works only sometimes.
  Ours is Help → "About Smart Contacts…".
- Menu icons are recoloured plain SVGs (`inkIcon`): Fyne tints themed icons white. Keep
  `fill="none"` boxes as they are or icons render as squares.

## 4. Commands

- **One command rule:** every Card command acts on the **clipped cards if any, otherwise the open
  card** (`targets()`, `targetText()` in `cardcmds.go`). The toolbar shows "acting on …".
- The Card menu, toolbar tabs and right-click menu call the same `cmd*` functions. Add a command
  once in `cardcmds.go`, then list it in `mainMenu` (menus.go), `toolbar.go`, `rowMenu`
  (macui.go) and `cardMenuIcons` (menuicons.go).
- Card menu order: Today · Add Note, Favorite, Keep in Touch ▸, Groups, Connections · Tidy Up,
  Find Duplicates, Share ▸, Send Greetings · Clip Cards · Check Messages · Move to Trash, Show
  Trash, Empty Trash.

## 5. Data model

- `Contact` (`model.go`): names in two scripts (First/Last + AltName), phones/emails/links as
  labelled `Field`s, `Custom` (the user's own label/value lines), `FollowUps`, `Timeline`
  (what happened, kept apart from Notes), `Origins` (where each detail came from), `HowWeMet`,
  groups, links to other people (`graph.go`), photo and business-card images (files beside the
  JSON). `Clone` must copy every slice — add new slices there.
- **Names:** `FullName` (first + last) is what matching, duplicates and sorting use; `FormalName`
  (prefix, first, middle, last, suffix) is what the card heading, copy-as-text and vCard FN show.
  Never put prefix or middle into `FullName`, or cards stop matching their other copies.
- `Dates` are labelled like phones and written like `Birthday` (`YYYY-MM-DD` or `--MM-DD`);
  `nextYearly` serves both.
- `Merge` must carry every list (`Custom`, `Dates`, `FollowUps` were once dropped there); a new list
  field goes into `Clone`, `Tidy`, `Merge`, `cardFields` and the vCard/CSV writers and readers.
- `searchText` decides what search finds; add new text fields there. `wordsForFuzzy` is the smaller
  set typos are forgiven against (names, nickname, company, email names).
- Phones are stored as written and shown normalised; `phone.go` knows ~25 countries.
- The Trash is a list of `TrashItem`s in the store (the card, when it was moved), kept until
  Empty Trash. Daily backups are pruned after 30 days.
- The store saves atomically (temp file + rename). Never write the JSON any other way.

## 6. Features and where they live

| Feature | Files | Notes |
|---|---|---|
| Today page (first tab) | `today.go` | memoized in `u.todayMemo`, cleared by `invalidate` and when the tidy check ends |
| Follow-ups | `followups.go` | inside Keep in Touch; due ones on Today; own list `grpFollowUps` |
| Greetings | `greetings.go` | one window per person, opens WhatsApp/Messages; timeline note stops repeats per year |
| Company pages | `company.go` | the › after the company on a card |
| Tidy one by one | `tidysession.go` | queue of cards needing work; skips remembered for the session |
| Questions in search | `ask.go` | regex rules → filter syntax; Return sends only the question to AI |
| Typo-tolerant search | `fuzzy.go` | second pass only when the exact search finds no one; list title says so |
| Related people in vCards | `relate.go`, `vcard.go` | X-ABRELATEDNAMES + RELATED urn:uuid; linked on import only when one card fits |
| Trash | `store.go`, `trashui.go` (`trash`, `cmdEmptyTrash`) | |
| Import wizard | `importwizard.go`, `importui.go`, `importers.go`, `socialimport.go`, `vcard.go` | see §7 |
| Clean Up / duplicates | `cleanup.go`, `tools.go`, `mergeui.go`, `names.go` | |
| Last talked | `activity.go`, `activityui.go` | read from Messages/calls on this Mac only |
| Updates | `updater.go`, `updateui.go` | from the rolling `latest` GitHub release |

## 7. Import wizard

- **A conversation, not a wall of choices:** `wizQuestion` → `wizAnswer` → either another question
  (`next`) or a `wizGuide`. The tree is built in `importQuestions()`.
- **Every guide has the same shape:** `intro`, `steps []wizStep{do, see}` ("You should see…"),
  `check` (Check before you continue), `trouble` (If it doesn't work), `expect`, optional
  `link`/`linkTo`, and a `finish` (Choose File / Import Now / Start Waiting / Paste Text / none).
- **Buttons the steps mention are pinned at the bottom**, outside the scroll; steps say "at the
  bottom of this window", never "below". The step whose text contains `linkTo` also shows the link.
- **One copy of the instructions.** Other places open a guide with `u.importGuide(label)` (Bring
  Everything In does). Labels must stay unique except "Yes…"/"No, or I'm not sure".
- **The wizard is built once** (`prewarmImport`, ~2 s after start) and reused; `start()` resets it.
- **Vendor steps go stale.** When changing a guide, check the vendor's current help page and note
  the date in the commit. Last full review: 2026-11-08 (Google, iCloud, Outlook, Samsung, LinkedIn).
- Every import shows a preview first (matching, enriching, name-only review); a backup is saved
  before anything is written.

## 8. Testing

- `go test ./...` runs everything; `TestEveryWindowOpens` (flows_test.go) drives every flow and
  window with Fyne's test driver, opens every import guide, and checks the menu layout never
  changes. Add a `step(...)` for every new window.
- **GitHub's Mac runners have no OpenGL**, so the real window never opens in CI. The packaged
  binary gets a smoke test only.
- **Real screenshots come from Linux**: build for Linux, run under `Xvfb :9`, drive with
  `xdotool`, capture with ImageMagick `import`. Use `SMART_CONTACTS_DIR` with invented data.
- macOS-only behaviour (menus, cgo, permissions, Bluetooth) can't be tested in the cloud;
  computer-use clicks from a cloud session don't reach the GLFW window. Ask the owner to check.

## 9. Build and release

- **Everything is pinned:** `go.mod` + `go.sum` are committed, Go comes from `go.mod`, the packager
  is `fyne.io/tools/cmd/fyne@v1.26.1`. CI never runs `go mod tidy`; upgrade on purpose, in a
  commit of its own (LESSONS §34).
- Push to `main` → `.github/workflows/build.yml` on `macos-15`: vet, tests, `fyne package`,
  Info.plist privacy strings added with PlistBuddy, ad-hoc `codesign`, smoke test, then the rolling
  `latest` release named "Smart Contacts 1.0.N" (N = run number).
- Install/update: `curl -fsSL https://raw.githubusercontent.com/jamalxcode/smart_contacts/main/install.sh | bash`
  or Help → Check for Updates. A browser-downloaded zip is quarantined and may hang (LESSONS §1).
- Commit as `jamal xcode <j.mutawa@gmail.com>`. After pushing, watch the run
  (`gh run watch <id> --exit-status`) and confirm the release name.
- Icon: `MAKE_ICON=Icon.png go test -run TestMakeIcon` (drawn in `icon_test.go`).

## 10. Checklist for a change

1. Does it keep contacts on the Mac and send nothing new to the cloud?
2. Does it work in the Filofax look **and** the plain look?
3. New command → `cardcmds.go` + menu + toolbar + right-click + icon, same name everywhere.
4. New window → a paper window helper, and a `step` in `TestEveryWindowOpens`.
5. Menu labels unchanged after it runs (the test checks).
6. New `Contact` field → `Clone`, `searchText`, `Tidy`, `Merge`, `cardFields`, vCard and CSV if it maps.
7. Help text (`helpui.go`), README, LESSONS (what you learned), ISSUES (what's left).
8. Screenshot it on the test screen; push; watch CI; check the release.

## Code map

| File | What it holds |
|---|---|
| `main.go` | start-up; where data lives |
| `ui.go` | the main `ui` struct, sidebar groups, list, detail, refresh |
| `filofax.go` | the organiser look: theme and fonts, open cover, pages, rings, layout, status line |
| `textures.go` | leather, paper, chrome, pen, brass and felt, drawn in code |
| `tabs.go` | coloured index tabs and the A–Z letter tabs |
| `closedbook.go` | the closed cover, opening the book, the strap |
| `paperdialog.go` | clipboard and stamp windows, paper buttons |
| `toolbar.go`, `cardcmds.go` | Card commands and their tab toolbar |
| `viewcmds.go` | View menu ↔ sidebar, sort, organiser footer |
| `menus.go` | the menu bar (built once), `refreshMenu`, About |
| `settingsui.go`, `helpui.go` | Settings; Help and privacy text |
| `trashui.go`, `groupsui.go` | Trash; new group and the group menu |
| `smartaddui.go`, `exportui.go`, `cleanupui.go` | Smart Add and business-card windows; import from Mac, export; Clean Up and duplicates windows |
| `menuicons.go`, `icons.go`, `flags.go` | icons and flags |
| `macui.go`, `window_darwin.go`, `window_other.go` | Mac behaviour, right-click menu, native window/menu calls |
| `readcard.go`, `livecard.go`, `ruled.go`, `editor.go` | reading and writing a card |
| `today.go`, `followups.go`, `greetings.go`, `company.go`, `tidysession.go`, `ask.go` | daily-use features |
| `bulkui.go`, `clips.go` | clipping several cards |
| `pageturn.go`, `desk.go`, `feedbackui.go` | page turns, desk behaviour, change feedback with Undo |
| `model.go`, `search.go`, `fuzzy.go`, `names.go`, `phone.go`, `social.go`, `origins.go`, `timeline.go`, `graph.go`, `relate.go` | the data model and its rules |
| `store.go`, `config.go`, `backupcrypt.go`, `changes.go`, `historyui.go` | saving, settings, backups, undo |
| `importwizard.go`, `importui.go`, `importers.go`, `socialimport.go`, `vcard.go`, `mac.go`, `guideui.go` | bringing contacts in |
| `cleanup.go`, `tools.go`, `mergeui.go` | clean-up, export, duplicates |
| `smart.go`, `cards.go`, `notesai.go` | Smart Add, business cards, AI note tidying |
| `activity.go`, `activityui.go`, `calendar.go`, `calendarui.go` | last talked, reminders, calendar log |
| `connections.go`, `photosui.go`, `filterui.go`, `quickfind.go` | pickers, photos, filters, menu-bar Quick Find |
| `updater.go`, `updateui.go`, `crashlog.go` | updates and crash log |
| `theme.go` | Mac system colours, light/dark |
| `*_test.go` | tests; `flows_test.go` is the whole-app walk-through |
