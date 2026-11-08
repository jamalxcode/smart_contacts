package main

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"
)

// makeBook builds n plausible contacts (Arabic and English names, Kuwaiti and foreign numbers).
func makeBook(n int) []*Contact {
	r := rand.New(rand.NewSource(1))
	firstEn := []string{"Mohammed", "Ahmad", "Khalid", "Fatima", "Sara", "Ali", "Hamad", "Noura", "Yousef", "Abdullah", "Maryam", "Omar", "John", "Priya", "Giulia"}
	lastEn := []string{"Al-Mutairi", "Al-Enezi", "Al-Sabah", "Al-Kandari", "Hassan", "Al-Ali", "Smith", "Sharma", "Rossi", "Al-Rashidi", "Al-Ajmi", "Bin Salem"}
	firstAr := []string{"محمد", "أحمد", "خالد", "فاطمة", "سارة", "علي", "حمد", "نورة"}
	lastAr := []string{"المطيري", "العنزي", "الصباح", "الكندري", "الرشيدي", "العجمي"}
	cos := []string{"Boubyan Bank", "Gulf Bank", "KOC", "Zain", "Agility", "NBK", "Alghanim", ""}
	var out []*Contact
	for i := 0; i < n; i++ {
		c := NewContact()
		if i%5 == 0 {
			c.First, c.Last = firstAr[r.Intn(len(firstAr))], lastAr[r.Intn(len(lastAr))]
		} else {
			c.First, c.Last = firstEn[r.Intn(len(firstEn))], lastEn[r.Intn(len(lastEn))]
		}
		c.Company = cos[r.Intn(len(cos))]
		c.Phones = []Field{{Label: "Mobile", Value: fmt.Sprintf("+965 %d%03d %04d", 5+r.Intn(5), r.Intn(1000), r.Intn(10000))}}
		if i%3 == 0 {
			c.Emails = []Field{{Label: "Work", Value: fmt.Sprintf("user%d@example.com", i)}}
		}
		if i%7 == 0 {
			c.Groups = []string{"Work"}
		}
		c.Sources = []string{"LinkedIn"}
		c.Notes = "Met at an event. Some notes about this person."
		c.AddEvent(time.Now().Add(-time.Duration(r.Intn(1000))*24*time.Hour), "Connected on LinkedIn", "LinkedIn")
		out = append(out, c)
	}
	return out
}

func TestScale(t *testing.T) {
	if os.Getenv("SCALE") == "" {
		t.Skip("set SCALE=1 to run")
	}
	n := 10000
	dir := t.TempDir()
	s, _ := OpenStore(dir)
	s.Contacts = makeBook(n)
	tm := func(name string, f func()) time.Duration {
		st := time.Now()
		f()
		d := time.Since(st)
		t.Logf("%-28s %v", name, d.Round(time.Millisecond))
		return d
	}
	tm("first save", func() { _ = s.Save() })
	for i := 0; i < 5; i++ {
		c := s.Contacts[i*100]
		tm("save after one edit", func() {
			c.Notes += " x"
			s.Put(c)
			_ = s.Save()
		})
	}
	tm("reopen", func() { s, _ = OpenStore(dir) })
	tm("undo", func() { _, _ = s.Undo() })
	for _, q := range []string{"", "a", "ahm", "mohammed al", "6555", "محمد"} {
		q := q
		tm("search "+q, func() {
			k := 0
			for _, c := range s.Contacts {
				if c.Matches(q) {
					k++
				}
			}
		})
	}
	tm("sort", func() { SortContacts(append([]*Contact{}, s.Contacts...), false) })
	tm("groups+sources", func() { _ = s.Groups(); _ = s.SourceNames() })
	tm("find duplicates", func() { _ = FindDuplicates(s.Contacts, "965") })
	tm("plan cleanup", func() { _ = PlanCleanup(s.Contacts, "965") })
	tm("graph + degrees", func() { g := NewGraph(s, true); _ = g.Degrees(s.Contacts[0].ID, 3) })
	var m runtimeMem
	t.Logf("undo steps held: %d, approx bytes %d", len(s.undo), m.undoBytes(s))
}

type runtimeMem struct{}

func (runtimeMem) undoBytes(s *Store) int {
	n := 0
	for _, u := range s.undo {
		n += u.size()
	}
	return n
}
