package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestEveryWindowOpens drives the app the way a person does — opens cards, writes on one,
// turns one over, clips some, opens every kind of window and closes it, switches look and back —
// and fails on any crash. It runs on every build (on GitHub's Mac too, where the real window
// can't open for want of graphics), so a slip in one of these flows is caught before release.
func TestEveryWindowOpens(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i, n := range []string{"Ali Hassan", "Sara Khan", "Omar Ali", "Mona Saleh"} {
		f := strings.Fields(n)
		c := &Contact{ID: fmt.Sprint("c", i), First: f[0], Last: f[1], Company: "Gulf Bank", Title: "Manager",
			Phones: []Field{{Label: "Mobile", Value: fmt.Sprintf("+965 9999 000%d", i)}}, Groups: []string{"Clients"},
			Birthday: now.AddDate(-30, 0, 2).Format("2006-01-02"), Created: now}
		c.Custom = []Field{{Label: "Diwaniya", Value: "Tuesday"}}
		c.FollowUps = []FollowUp{{ID: "f" + fmt.Sprint(i), Text: "Send the proposal", Due: dayOf(now), Created: now}}
		s.Put(c)
	}
	s.Put(&Contact{ID: "x", First: "No", Last: "Number", Created: now})
	cfg := &Config{NoCover: true, CountryCode: "965"}
	u := newUI(a, s, cfg)
	base := menuLayout(u.mainMenu())

	closeAll := func(what string) {
		t.Helper()
		for len(keyDialogs) > 0 {
			keyDialogs[len(keyDialogs)-1].d.Hide()
		}
		if u.modalLayer != nil {
			u.modalLayer.Objects = nil
		}
		if got := menuLayout(u.mainMenu()); got != base {
			t.Errorf("menu layout changed after %s", what)
		}
	}
	step := func(what string, f func()) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("%s crashed: %v", what, r)
			}
		}()
		f()
		u.refreshMenu()
		closeAll(what)
	}

	for _, g := range []string{grpToday, grpAll, grpFavorites, grpBirthdays, grpFollowUps, grpRecent, grpIncomplete, "g:Clients"} {
		g := g
		step("showing "+g, func() { u.showGroup(g) })
	}
	step("opening a card", func() { u.selectedID = "c0"; u.showDetail() })
	step("opening another card", func() { u.selectedID = "c1"; u.showDetail() })
	step("writing on a card", func() { u.writeCard(nil); u.finishWriting() })
	step("turning a card over", func() { u.turnCard(s.Get("c1")) })
	step("Add Note", u.cmdNote)
	step("Favorite", u.cmdFavorite)
	step("Follow Up", u.cmdFollowUp)
	step("Groups", u.cmdGroups)
	step("Connections", u.cmdConnections)
	step("Tidy Up", u.cmdTidy)
	step("Tidy one by one", u.tidySession)
	step("Find Duplicates", u.cmdDuplicates)
	step("Greetings", u.cmdGreetings)
	step("a greeting", func() { u.greetingRun(s.Contacts[:2], "Eid al-Fitr", "Eid Mubarak, {name}!", 0, 0) })
	step("company page", func() { u.companyDialog("Gulf Bank") })
	step("follow-up done", func() { u.finishFollowUp(s.Get("c2"), "f2") })
	step("follow-up later", func() { u.snoozeFollowUp(s.Get("c3"), "f3", 7) })
	step("clipping cards", func() { u.toggleClip("c0"); u.toggleClip("c1") })
	step("Groups on clipped cards", u.cmdGroups)
	step("right-click menu", func() { _ = u.rowMenu("c0") })
	step("Move to Trash", func() { u.toggleSelectMode(); u.selectedID = "x"; u.showDetail(); u.cmdTrash() })
	step("Trash window", u.trash)
	step("Empty Trash", u.cmdEmptyTrash)
	step("Import wizard", u.importHub)
	step("About", u.about)
	step("Settings", u.settings)
	step("a question in search", func() { u.search.SetText("who do I know at Gulf Bank?") })
	step("clearing the search", func() { u.search.SetText("") })
	step("plain look", u.toggleLook)
	step("organiser look", u.toggleLook)
	_ = fyne.CurrentApp()
}
