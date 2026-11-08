package main

import (
	"archive/zip"
	"io"
	"strings"
	"testing"
	"time"
)

func TestUndoRedo(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	a := NewContact()
	a.First = "Ali"
	s.Put(a)
	_ = s.Save()
	if s.CanUndo() != "Added Ali" {
		t.Fatalf("undo label %q", s.CanUndo())
	}
	b := a.Clone()
	b.Phones = []Field{{"Mobile", "99998888"}}
	s.Put(b)
	_ = s.Save()
	c := s.Get(a.ID)
	c.LastContacted = time.Now() // not worth an undo step
	_ = s.Save()
	if got := s.History(); len(got) != 2 || got[0].Label != "Changed Ali" {
		t.Fatalf("history %+v", got)
	}
	if l, err := s.Undo(); err != nil || l != "Changed Ali" || len(s.Get(a.ID).Phones) != 0 {
		t.Fatalf("undo: %v %q %+v", err, l, s.Get(a.ID))
	}
	if l, _ := s.Redo(); l != "Changed Ali" || len(s.Get(a.ID).Phones) != 1 {
		t.Fatalf("redo: %q", l)
	}
	// undoing a whole import (many people) in one step
	var cards []Card
	for _, n := range []string{"X One", "Y Two", "Z Three"} {
		c := NewContact()
		setName(c, n)
		cards = append(cards, Card{Contact: c})
	}
	ApplyImport(s, PlanImport(s, cards, "Test", "965"), nil, "965")
	_ = s.Save()
	if s.CanUndo() != "Added 3 people" || len(s.Contacts) != 4 {
		t.Fatalf("%q %d", s.CanUndo(), len(s.Contacts))
	}
	_, _ = s.Undo()
	if len(s.Contacts) != 1 {
		t.Errorf("import not undone: %d", len(s.Contacts))
	}
	// a merge of duplicates can be undone too
	d := NewContact()
	d.First, d.Phones = "Ali", []Field{{"Mobile", "99998888"}}
	s.Put(d)
	_ = s.Save()
	keep := s.Get(a.ID)
	Merge(keep, []*Contact{d}, "965")
	s.Delete(d.ID)
	s.Put(keep)
	_ = s.Save()
	if len(s.Contacts) != 1 {
		t.Fatal("merge setup")
	}
	if err := s.UndoTo(0); err != nil || len(s.Contacts) != 2 {
		t.Errorf("merge undo: %v %d", err, len(s.Contacts))
	}
	if pts := s.RestorePoints(); len(pts) == 0 {
		t.Error("expected the daily backup as a restore point")
	}
}

func TestPhotoRemovalUndo(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	c := NewContact()
	c.First = "P"
	s.Put(c)
	_ = s.SetPhoto(c, []byte{1, 2, 3})
	_ = s.Save()
	_ = s.SetPhoto(c, nil)
	_ = s.Save()
	if s.Photo(c) != nil {
		t.Error("removed photo shows")
	}
	_, _ = s.Undo()
	if got := s.Photo(s.Get(c.ID)); len(got) != 3 {
		t.Error("undo should bring the photo back")
	}
}

func TestOrigins(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	k := NewContact()
	k.First, k.Last = "Khalid", "Al-Mutairi"
	k.Phones = []Field{{"Mobile", "60001234"}}
	StampNew(k, "Mac Contacts", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	s.Put(k)
	b, _ := ReadImport("Connections.csv", []byte("First Name,Last Name,URL,Email Address,Company,Position,Connected On\nKhalid,Al-Mutairi,https://www.linkedin.com/in/khalidm,k@x.com,Boubyan Bank,Engineer,12 Mar 2021\n"), "965")
	ApplyImport(s, PlanImport(s, b.Cards, "LinkedIn", "965"), map[int]string{0: "merge:" + k.ID}, "965")
	if o := k.OriginText(phoneKey("+965 6000 1234")); !strings.HasPrefix(o, "Mac Contacts") {
		t.Errorf("phone origin %q (reformatting must not lose it)", o)
	}
	if o := k.OriginText("e:k@x.com"); !strings.HasPrefix(o, "LinkedIn") {
		t.Errorf("email origin %q", o)
	}
	if o := k.OriginText("company"); !strings.HasPrefix(o, "LinkedIn") {
		t.Errorf("company origin %q", o)
	}
	ed := k.Clone()
	ed.Phones = append(ed.Phones, Field{"Work", "22223333"})
	StampChanges(k, ed, "You", time.Now())
	if !strings.HasPrefix(ed.OriginText(phoneKey("22223333")), "You") || !strings.HasPrefix(ed.OriginText(phoneKey("60001234")), "Mac Contacts") {
		t.Errorf("edit origins: %+v", ed.Origins)
	}
}

func TestExactReimport(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	card := "BEGIN:VCARD\nVERSION:3.0\nN:Rashid;Omar;;;\nTEL:99112233\nX-ABUID:ABC-123:ABPerson\nEND:VCARD\n"
	b, _ := ReadImport("mac.vcf", []byte(card), "965")
	ApplyImport(s, PlanImport(s, b.Cards, "Mac Contacts", "965"), nil, "965")
	if len(s.Contacts) != 1 || s.Contacts[0].SourceIDs[0] != "Mac Contacts|ABC-123" {
		t.Fatalf("%+v", s.Contacts)
	}
	// on the phone the person was renamed and got a new number: the ID still finds them
	changed := "BEGIN:VCARD\nVERSION:3.0\nN:Al-Rashid;Omar;;;\nTEL:66554433\nX-ABUID:ABC-123:ABPerson\nEND:VCARD\n"
	b, _ = ReadImport("mac.vcf", []byte(changed), "965")
	items := PlanImport(s, b.Cards, "Mac Contacts", "965")
	if items[0].Kind != piMerge || items[0].Why != "the same Mac Contacts card as last time" {
		t.Fatalf("%+v", items[0])
	}
	ApplyImport(s, items, nil, "965")
	if len(s.Contacts) != 1 || len(s.Contacts[0].Phones) != 2 {
		t.Errorf("exact re-import should update, not add: %+v", s.Contacts)
	}
	// re-importing our own vCard export matches by our own ID
	var buf strings.Builder
	_ = WriteVCard(&buf, s.Contacts[0], nil)
	b, _ = ReadImport("export.vcf", []byte(buf.String()), "965")
	if it := PlanImport(s, b.Cards, "File: export", "965")[0]; it.Kind != piUpToDate || it.Match != s.Contacts[0] {
		t.Errorf("own export: %+v", it)
	}
}

func TestOldAutoNotesMoveToTimeline(t *testing.T) {
	c := &Contact{First: "K", Notes: "Met at GITEX.\nLinkedIn connection since 12 Mar 2021\nPreviously: Branch Manager at Gulf Bank\nGoogle Contacts: Analyst at X\nCall after 5"}
	var moved bool
	for _, ch := range PlanCleanup([]*Contact{c}, "965") {
		if ch.New == "timeline" {
			ch.Apply("")
			moved = true
		}
	}
	if !moved || c.Notes != "Met at GITEX.\nCall after 5" || len(c.Timeline) != 3 {
		t.Fatalf("notes %q timeline %+v", c.Notes, c.Timeline)
	}
	if c.Timeline[0].Text != "Connected on LinkedIn" || c.Timeline[0].At.Year() != 2021 { // dated first; undated after
		t.Errorf("%+v", c.Timeline)
	}
}

func TestBackupZipLeavesKeyOut(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenStore(dir)
	c := NewContact()
	c.First = "Z"
	s.Put(c)
	_ = s.Save()
	cfg := LoadConfig(dir)
	cfg.OpenRouterKey = "sk-secret"
	_ = cfg.Save(dir)
	out := t.TempDir() + "/b.zip"
	if err := zipDataDir(dir, out); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		if f.Name == "config.json" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			if strings.Contains(string(b), "sk-secret") {
				t.Error("the key must not be in the backup")
			}
		}
	}
	if !names["contacts.json"] || !names["config.json"] {
		t.Errorf("zip contents %v", names)
	}
}
