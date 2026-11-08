package main

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// menuLayout is the menu bar's titles and shape, without states.
func menuLayout(m *fyne.MainMenu) string {
	var b strings.Builder
	var walk func(items []*fyne.MenuItem, depth int)
	walk = func(items []*fyne.MenuItem, depth int) {
		for _, it := range items {
			fmt.Fprintf(&b, "%d:%s|%v\n", depth, it.Label, it.IsSeparator)
			if it.ChildMenu != nil {
				walk(it.ChildMenu.Items, depth+1)
			}
		}
	}
	for _, mm := range m.Items {
		b.WriteString("# " + mm.Label + "\n")
		walk(mm.Items, 0)
	}
	return b.String()
}

// The Mac menu bar is built once (rebuilding it crashed the app), so its titles and layout must
// not depend on which card is open, whether it's a favorite, or how many cards are clipped.
func TestMenuLayoutNeverChanges(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range []string{"Ali Hassan", "Sara Khan", "Omar Ali"} {
		f := strings.Fields(n)
		s.Put(&Contact{ID: fmt.Sprint("c", i), First: f[0], Last: f[1]})
	}
	cfg := &Config{NoCover: true}
	u := newUI(a, s, cfg)
	base := menuLayout(u.mainMenu())

	check := func(what string) {
		t.Helper()
		if got := menuLayout(u.mainMenu()); got != base {
			t.Errorf("menu layout changed when %s:\n%s\nwas:\n%s", what, got, base)
		}
	}
	u.selectedID = ""
	check("no card was open")
	u.selectedID = s.Contacts[0].ID
	check("a card was opened")
	s.Contacts[1].Favorite = true
	u.selectedID = s.Contacts[1].ID
	check("another card, a favorite, was opened")
	u.toggleClip(s.Contacts[0].ID)
	check("one card was clipped")
	u.toggleClip(s.Contacts[2].ID)
	check("two cards were clipped")
	s.Put(&Contact{ID: "c9", First: "New", Last: "Person", Groups: []string{"Golf"}})
	check("a card and a group were added")
}
