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

## 5. Ideas not done yet

- Two-way sync with Mac Contacts (today: import, and "Send to Mac Contacts" which opens a .vcf).
- A menu-bar quick search.
- Private backup of `contacts.json` to a separate *private* GitHub repo (never this public one).
- Notarised signing if it is ever shared beyond one Mac.
