# Known issues and open work

What's left to do on Smart Contacts. Each item is also a GitHub issue (the number links to it).
When you fix one, close the issue and move the line to *Done recently* with the version.

## Known issues

| # | Issue | State |
|---|---|---|
| [#1](https://github.com/jamalxcode/smart_contacts/issues/1) | Import window took ~3 s to appear on the Mac | Fix shipped in 1.0.55 (sheet moves instead of full refresh; wizard prebuilt). Not reproducible on Linux — waiting for the owner to confirm on the Mac. |
| [#2](https://github.com/jamalxcode/smart_contacts/issues/2) | Import guides follow vendor menus that change | Google, iCloud, Outlook, Samsung, LinkedIn checked 2026-11-08. Facebook, Snapchat, Telegram not walked through signed in. |
| [#3](https://github.com/jamalxcode/smart_contacts/issues/3) | No automated test of the real Mac window | CI has no OpenGL; cloud sessions can't click the GLFW window. Mac-only behaviour is checked by hand. |
| [#4](https://github.com/jamalxcode/smart_contacts/issues/4) | Fyne darwin crashes when the menu bar is rebuilt | Worked around: menu built once, labels never change (DESIGN.md §3). Re-check on each Fyne upgrade. |

## Open work

| # | Idea | Notes |
|---|---|---|
| [#5](https://github.com/jamalxcode/smart_contacts/issues/5) | Developer ID signing and notarisation | So a browser-downloaded zip opens; today use the curl install. |

## Small technical debt

- Import answers are found by label (`u.importGuide(label)`); labels must stay unique except the
  repeated "Yes…" / "No, or I'm not sure". The flow test skips repeated labels.
- `importui.go` still holds file picking, preview and Receive from a Phone; the old source list
  was removed in favour of the wizard.

## Won't do (by decision)

See [LIMITATIONS.md](LIMITATIONS.md): online lookups or profiling of people, cloud sync, reading
messages to rank people, automatic merging, sending messages without the user's press, two-way
sync with Mac Contacts (the app stands on its own).

## Done recently

| Version | What |
|---|---|
| 1.0.59 | The Bluetooth guide and the waiting window say whether Bluetooth Sharing (and Bluetooth) is on, live (#6) |
| 1.0.58 | Typo-tolerant search (#7); anniversaries and other dates (#8); prefix, middle name, suffix, phonetic (#9); related people in vCards (#10); merging keeps own lines, dates and follow-ups |
| 1.0.57 | Builds pinned (go.sum committed, no `go mod tidy` in CI, fyne tool pinned); dialogs.go and filofax.go split by feature |
| 1.0.55 | Import opens at once (#1 fix); LinkedIn guide uses the larger archive |
| 1.0.54 | The step that opens a page carries the link |
| 1.0.53 | Guide buttons pinned in sight; every guide checked against vendor pages |
| 1.0.52 | Guides: intro, numbered steps with "You should see…", checks, fixes; Bluetooth Sharing first |
| 1.0.51 and before | Today page, follow-ups, greetings, company pages, tidy one by one, own fields, questions in search, Trash kept until emptied, card-tab toolbar, clipboard/stamp windows, menu icons, Import wizard, static menu (crash fix), fat Filofax icon |
