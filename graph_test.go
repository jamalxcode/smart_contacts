package main

import (
	"strings"
	"testing"
)

func person(s *Store, first, company string) *Contact {
	c := NewContact()
	c.First, c.Company = first, company
	s.Put(c)
	return c
}

func chainText(p []Step) string {
	var b []string
	for _, st := range p {
		if st.Via != "" {
			b = append(b, "("+st.Via+")")
		}
		b = append(b, st.Contact.First)
	}
	return strings.Join(b, " ")
}

func TestSixDegrees(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	ali := person(s, "Ali", "")
	fatima := person(s, "Fatima", "Gulf Bank K.S.C.P")
	khalid := person(s, "Khalid", "Gulf Bank")
	noor := person(s, "Noor", "")
	lonely := person(s, "Lonely", "")
	_ = person(s, "Switchboard", "") // filled below as a company card
	s.Contacts[len(s.Contacts)-1].First, s.Contacts[len(s.Contacts)-1].Company = "", "Gulf Bank"

	s.AddLink(ali.ID, fatima.ID, "Friend")
	s.AddLink(khalid.ID, noor.ID, "Family")

	g := NewGraph(s, true)
	if got := chainText(g.Path(ali.ID, noor.ID, MaxDegrees)); got != "Ali (friend) Fatima (colleague at Gulf Bank K.S.C.P) Khalid (family) Noor" {
		t.Errorf("chain: %s", got)
	}
	if g.Path(ali.ID, lonely.ID, MaxDegrees) != nil {
		t.Error("lonely should not be reachable")
	}
	if d := g.Degrees(ali.ID, MaxDegrees); d[0] != 1 || d[1] != 1 || d[2] != 1 {
		t.Errorf("degrees from Ali: %v", d)
	}
	// without company links Ali can't reach Noor
	if NewGraph(s, false).Path(ali.ID, noor.ID, MaxDegrees) != nil {
		t.Error("company links should be optional")
	}
	if len(g.Colleagues(fatima)) != 1 {
		t.Errorf("colleagues: %v (company cards don't count)", g.Colleagues(fatima))
	}
}

func TestSixStepLimit(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	var cs []*Contact
	for i := 0; i < 9; i++ {
		cs = append(cs, person(s, string(rune('A'+i)), ""))
		if i > 0 {
			s.AddLink(cs[i-1].ID, cs[i].ID, "Friend")
		}
	}
	g := NewGraph(s, false)
	if p := g.Path(cs[0].ID, cs[6].ID, MaxDegrees); len(p) != 7 {
		t.Errorf("6 steps should be found, got %d people", len(p))
	}
	if g.Path(cs[0].ID, cs[7].ID, MaxDegrees) != nil {
		t.Error("7 steps is beyond six degrees")
	}
}

func TestLinksFollowTrashAndMerge(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	a, b, c := person(s, "A", ""), person(s, "B", ""), person(s, "C", "")
	if s.AddLink(a.ID, a.ID, "Friend") {
		t.Error("no self links")
	}
	s.AddLink(a.ID, b.ID, "Friend")
	s.AddLink(b.ID, a.ID, "Family") // relabels, doesn't duplicate
	if len(s.Links) != 1 || s.Links[0].Label != "Family" {
		t.Fatalf("links: %+v", s.Links)
	}
	s.Delete(b.ID)
	if len(s.LinksOf(a.ID)) != 0 {
		t.Error("trashed contacts aren't shown as connections")
	}
	s.Restore(b.ID)
	if len(s.LinksOf(a.ID)) != 1 {
		t.Error("restoring brings the link back")
	}
	s.AddLink(c.ID, b.ID, "Colleague")
	s.RepointLinks(b.ID, c.ID) // B merged into C
	if l := s.LinksOf(a.ID); len(l) != 1 || l[0].other(a.ID) != c.ID {
		t.Errorf("merge should move links: %+v", s.Links)
	}
	if len(s.LinksOf(c.ID)) != 1 {
		t.Error("the B–C link must not become a C–C self link")
	}
	s.Delete(c.ID)
	s.EmptyTrash()
	if len(s.Links) != 0 {
		t.Error("emptying the trash drops links")
	}
	_ = s.Save()
	s2, _ := OpenStore(s.Dir())
	s2.AddLink(a.ID, b.ID, "Friend")
	_ = s2.Save()
	s3, _ := OpenStore(s.Dir())
	if len(s3.Links) != 1 {
		t.Error("links must persist")
	}
}
