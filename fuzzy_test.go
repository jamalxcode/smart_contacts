package main

import (
	"fmt"
	"testing"
	"time"
)

func TestPrefixDistance(t *testing.T) {
	cases := []struct {
		q, word string
		want    int
	}{
		{"khalid", "khalid", 0},
		{"khal", "khalid", 0},   // a word still being typed
		{"khlaid", "khalid", 1}, // two letters swapped
		{"khaild", "khalid", 1},
		{"khald", "khalid", 1}, // a letter missed
		{"mohamed", "mohammed", 1},
		{"mohamad", "mohammed", 2}, // two slips: left to the name skeleton, which knows this spelling
		{"mutari", "mutairi", 1},
		{"boubian", "boubyan", 1},
		{"engeneer", "engineer", 1},
		{"fatma", "fatima", 1},
		{"xyzzy", "khalid", 2}, // over the limit of 1: stops at k+1
	}
	for _, c := range cases {
		if got := prefixDistance(c.q, c.word, 1); min(got, 2) != min(c.want, 2) {
			t.Errorf("prefixDistance(%q, %q) = %d, want %d", c.q, c.word, got, c.want)
		}
	}
	if d := prefixDistance("abdulrahmn", "abdulrahman", 2); d != 1 {
		t.Errorf("long word with one slip: %d", d)
	}
	if d := prefixDistance("محمد", "محمود", 1); d != 1 {
		t.Errorf("Arabic: محمد → محمود should be 1, got %d", d)
	}
}

func TestFuzzySearch(t *testing.T) {
	khalid := &Contact{ID: "1", First: "Khalid", Last: "Al-Mutairi", Company: "Boubyan Bank", Title: "Engineer",
		Emails: []Field{{"Work", "k.mutairi@boubyan.example"}}}
	mohammed := &Contact{ID: "2", First: "Mohammed", Last: "Al-Sabah", AltName: "محمد الصباح"}
	ali := &Contact{ID: "3", First: "Ali", Last: "Hassan", Phones: []Field{{"Mobile", "+965 6555 1212"}}}

	finds := []struct {
		q string
		c *Contact
	}{
		{"Khlaid", khalid},
		{"khalid al-mutari", khalid},
		{"boubian", khalid},
		{"mutairy", khalid},
		{"Mohamad Sabah", mohammed},
		{"الصبح", mohammed}, // a letter dropped in Arabic
		{"Hasan", ali},      // already matched by the name skeleton
	}
	for _, f := range finds {
		if !f.c.MatchesFuzzy(f.q) {
			t.Errorf("%q should find %s %s", f.q, f.c.First, f.c.Last)
		}
	}

	misses := []struct {
		q string
		c *Contact
	}{
		{"Ala", ali},             // under 4 letters: no guessing
		{"Fahad", khalid},        // a different name
		{"6555 1213", ali},       // numbers are never guessed at
		{"Khalid Noura", khalid}, // every word must match
		{"engeneer", khalid},     // job titles are found exactly, not guessed (too much text)
	}
	for _, m := range misses {
		if m.c.MatchesFuzzy(m.q) {
			t.Errorf("%q must not find %s %s", m.q, m.c.First, m.c.Last)
		}
	}

	// Exact search is unchanged: typos don't match there.
	if khalid.Matches("boubian") {
		t.Error("plain Matches must stay exact")
	}
	sq := parseQuery("boubian title:engineer")
	if sq.matches(khalid, nil, nil) {
		t.Error("exact pass must not forgive typos")
	}
	sq.fuzzy = true
	if !sq.matches(khalid, nil, nil) {
		t.Error("fuzzy pass should forgive the typo and keep the filter")
	}
}

// 10,000 cards searched with a typo stays fast (the fallback runs on every keystroke that finds
// nothing exactly).
func TestFuzzySearchSpeed(t *testing.T) {
	first := []string{"Khalid", "Mohammed", "Fatima", "Noura", "Yousef", "Sara", "Hamad", "Layla", "Ahmed", "Dana"}
	last := []string{"Al-Mutairi", "Al-Sabah", "Al-Kandari", "Haddad", "Schmidt", "Rossi", "Okafor", "Al-Ali", "Hassan", "Al-Azmi"}
	var cs []*Contact
	for i := 0; i < 10000; i++ {
		cs = append(cs, &Contact{ID: fmt.Sprint(i), First: first[i%10], Last: last[(i/10)%10],
			Company: fmt.Sprintf("Company %d", i%300), Emails: []Field{{"Work", fmt.Sprintf("p%d@example.com", i)}},
			Notes: "met at a conference, follow up about the proposal next quarter"})
	}
	for _, c := range cs {
		c.fillCache()
	}
	start := time.Now()
	n := 0
	for _, c := range cs {
		if c.MatchesFuzzy("Mohamad Al-Kandri") {
			n++
		}
	}
	took := time.Since(start)
	if n != 100 {
		t.Errorf("found %d, want the 100 Mohammed Al-Kandaris", n)
	}
	if took > 150*time.Millisecond {
		t.Errorf("fuzzy search over 10,000 cards took %v", took)
	}
	t.Logf("fuzzy search over 10,000 cards: %v", took)
}
