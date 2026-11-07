package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

// A card the way Apple Contacts writes it: grouped custom labels, folded lines, no-year birthday.
const appleCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nPRODID:-//Apple Inc.//macOS 15.0//EN\r\n" +
	"N:Al-Sabah;Fatima;;;\r\nFN:Fatima Al-Sabah\r\nORG:Gulf Bank;\r\nTITLE:Branch Manager\r\n" +
	"EMAIL;type=INTERNET;type=WORK;type=pref:fatima@example.com\r\n" +
	"TEL;type=CELL;type=VOICE;type=pref:+965 9999 1234\r\n" +
	"item1.TEL:2222 3333\r\nitem1.X-ABLabel:_$!<Main>!$_\r\n" +
	"item2.TEL:+1 (415) 555-0100\r\nitem2.X-ABLabel:Office in SF\r\n" +
	"item3.ADR;type=HOME;type=pref:;;Block 3\\, Street 10;Salmiya;;22000;Kuwait\r\n" +
	"item3.X-ABADR:kw\r\n" +
	"BDAY;X-APPLE-OMIT-YEAR=1604:1604-03-14\r\n" +
	"NOTE:Met at the GITEX booth\\nLikes long emails\r\n" +
	"PHOTO;ENCODING=b;TYPE=JPEG:/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHB\r\n" +
	" wcICQuJyAiLCMcHCg3KSwwMTQ0NB8nOT04MjwuMzQy\r\n" +
	"END:VCARD\r\n"

const arabicCard = "BEGIN:VCARD\nVERSION:3.0\nN:العنزي;أحمد;;;\nFN:أحمد العنزي\nTEL;TYPE=CELL:٩٩٩٩٨٨٨٨\nCATEGORIES:Family,starred\nEND:VCARD\n"

const v21Card = "BEGIN:VCARD\nVERSION:2.1\nN;CHARSET=UTF-8;ENCODING=QUOTED-PRINTABLE:=4D=C3=BC=6C=6C=65=72;J=C3=B6rg\nTEL;CELL;PREF:0049 171 1234567\nEND:VCARD\n"

func TestParseAppleCard(t *testing.T) {
	cards, err := ParseVCards(strings.NewReader(appleCard))
	if err != nil || len(cards) != 1 {
		t.Fatalf("got %d cards, err %v", len(cards), err)
	}
	c := cards[0].Contact
	if c.First != "Fatima" || c.Last != "Al-Sabah" || c.Company != "Gulf Bank" || c.Title != "Branch Manager" {
		t.Errorf("names wrong: %+v", c)
	}
	if len(c.Phones) != 3 || c.Phones[0].Label != "Mobile" || c.Phones[1].Label != "Main" || c.Phones[2].Label != "Office in SF" {
		t.Errorf("phones wrong: %+v", c.Phones)
	}
	if len(c.Addresses) != 1 || c.Addresses[0].Street != "Block 3, Street 10" || c.Addresses[0].City != "Salmiya" {
		t.Errorf("address wrong: %+v", c.Addresses)
	}
	if c.Birthday != "--03-14" {
		t.Errorf("birthday %q", c.Birthday)
	}
	if c.Notes != "Met at the GITEX booth\nLikes long emails" {
		t.Errorf("notes %q", c.Notes)
	}
	if len(cards[0].Photo) < 20 || cards[0].Photo[0] != 0xFF {
		t.Errorf("photo not decoded (%d bytes)", len(cards[0].Photo))
	}
}

func TestParseArabicAndOldCards(t *testing.T) {
	cards, _ := ParseVCards(strings.NewReader(arabicCard + v21Card))
	if len(cards) != 2 {
		t.Fatalf("got %d cards", len(cards))
	}
	a := cards[0].Contact
	if a.First != "أحمد" || a.Last != "العنزي" || !a.Favorite || !a.HasGroup("family") {
		t.Errorf("arabic card: %+v", a)
	}
	if NormalizePhone(a.Phones[0].Value, "965") != "+96599998888" {
		t.Errorf("arabic digits: %q", NormalizePhone(a.Phones[0].Value, "965"))
	}
	b := cards[1].Contact
	if b.First != "Jörg" || b.Last != "Müller" || b.Phones[0].Label != "Mobile" {
		t.Errorf("v2.1 card: %+v", b)
	}
}

func TestVCardRoundTrip(t *testing.T) {
	c := NewContact()
	c.First, c.Last, c.Company = "Sara", "Al Ali; Jr", "Acme, Inc"
	c.Phones = []Field{{"Mobile", "+965 5555 1111"}, {"Driver", "+965 6666 2222"}}
	c.Emails = []Field{{"Work", "sara@acme.example"}}
	c.URLs = []Field{{"LinkedIn", "https://linkedin.com/in/sara"}}
	c.Addresses = []Address{{Label: "Work", Street: "Tower 1\nFloor 20", City: "Kuwait City", Country: "Kuwait"}}
	c.Birthday = "1990-07-02"
	c.Notes = strings.Repeat("A long note with ümlauts and عربي, ", 10)
	c.Groups = []string{"Clients", "VIP"}
	photo := []byte{0xFF, 0xD8, 0xFF, 1, 2, 3, 4, 5}
	var b bytes.Buffer
	if err := WriteVCard(&b, c, photo); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(b.String(), "\r\n") {
		if len(line) > 75 {
			t.Fatalf("line longer than 75 bytes: %q", line)
		}
	}
	cards, _ := ParseVCards(&b)
	if len(cards) != 1 {
		t.Fatal("round trip lost the card")
	}
	d := cards[0].Contact
	if d.Last != c.Last || d.Company != c.Company || d.Birthday != c.Birthday || d.Notes != strings.TrimSpace(c.Notes) {
		t.Errorf("scalars changed:\n%+v\n%+v", c, d)
	}
	if len(d.Phones) != 2 || d.Phones[1].Label != "Driver" || d.URLs[0].Label != "LinkedIn" {
		t.Errorf("labels changed: %+v %+v", d.Phones, d.URLs)
	}
	if d.Addresses[0].Street != "Tower 1\nFloor 20" || len(d.Groups) != 2 || !bytes.Equal(cards[0].Photo, photo) {
		t.Errorf("address/groups/photo changed: %+v %+v", d.Addresses, d.Groups)
	}
	c.Birthday = "--12-25"
	b.Reset()
	_ = WriteVCard(&b, c, nil)
	cards, _ = ParseVCards(&b)
	if cards[0].Contact.Birthday != "--12-25" {
		t.Errorf("no-year birthday became %q", cards[0].Contact.Birthday)
	}
}

func TestSearch(t *testing.T) {
	c := NewContact()
	c.First, c.Last, c.Company = "أحمد", "José", "Zain"
	c.Phones = []Field{{"Mobile", "+965 9999-8888"}}
	for q, want := range map[string]bool{
		"احمد": true, "jose": true, "zain jose": true, "99998888": true, "9999 8888": true,
		"٩٩٩٩": true, "zain nobody": false, "+965": true, "": true,
	} {
		if got := c.Matches(q); got != want {
			t.Errorf("Matches(%q) = %v", q, got)
		}
	}
}

func TestPhones(t *testing.T) {
	cases := []struct{ in, norm, pretty string }{
		{"99998888", "+96599998888", "+965 9999 8888"},
		{"+965 9999 8888", "+96599998888", "+965 9999 8888"},
		{"0096599998888", "+96599998888", "+965 9999 8888"},
		{"96599998888", "+96599998888", "+965 9999 8888"},
		{"+1 (415) 555-0100", "+14155550100", "+1 415 555 0100"},
		{"+44 20 7946 0958", "+442079460958", "+44 207 946 0958"},
		{"+971 50 123 4567", "+971501234567", "+971 501 234 567"},
	}
	for _, tc := range cases {
		if n := NormalizePhone(tc.in, "965"); n != tc.norm {
			t.Errorf("NormalizePhone(%q) = %q, want %q", tc.in, n, tc.norm)
		}
		if p := FormatPhone(tc.in, "965"); p != tc.pretty {
			t.Errorf("FormatPhone(%q) = %q, want %q", tc.in, p, tc.pretty)
		}
	}
	if WhatsAppNumber("9999 8888", "965") != "96599998888" {
		t.Error("whatsapp number")
	}
}

func TestBirthdays(t *testing.T) {
	now := time.Date(2026, 11, 7, 15, 0, 0, 0, time.UTC)
	c := &Contact{Birthday: "1990-11-10"}
	if d, turns, ok := c.NextBirthday(now); !ok || d != 3 || turns != 36 {
		t.Errorf("got %d days, turns %d", d, turns)
	}
	c.Birthday = "--11-07"
	if d, turns, _ := c.NextBirthday(now); d != 0 || turns != 0 {
		t.Errorf("today: %d %d", d, turns)
	}
	c.Birthday = "2000-01-02"
	if d, turns, _ := c.NextBirthday(now); d != 56 || turns != 27 {
		t.Errorf("next year: %d %d", d, turns)
	}
	c.Birthday = "1996-02-29"
	if d, _, _ := c.NextBirthday(time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)); d != 27 {
		t.Errorf("leap day: %d", d)
	}
	for in, want := range map[string]string{"14/03/1988": "1988-03-14", "19880314": "1988-03-14", "--0314": "--03-14", "1604-03-14": "--03-14", "14/3": "--03-14", "14/03": "--03-14"} {
		if got, _ := CanonicalBirthday(in); got != want {
			t.Errorf("CanonicalBirthday(%q) = %q", in, got)
		}
	}
	if BirthdayText("--03-14") != "14 March" {
		t.Error(BirthdayText("--03-14"))
	}
}

func TestDuplicatesAndMerge(t *testing.T) {
	a := &Contact{ID: "a", First: "Ali", Phones: []Field{{"Mobile", "9999 8888"}}, Groups: []string{"Work"}}
	b := &Contact{ID: "b", First: "Ali", Last: "Hassan", Phones: []Field{{"Mobile", "+96599998888"}}, Emails: []Field{{"Work", "ali@x.com"}}}
	c := &Contact{ID: "c", First: "Noor", Emails: []Field{{"Home", "ALI@x.com"}}}
	d := &Contact{ID: "d", First: "Someone", Last: "Else"}
	g := FindDuplicates([]*Contact{a, b, c, d}, "965")
	if len(g) != 1 || len(g[0].Contacts) != 3 {
		t.Fatalf("groups: %+v", g)
	}
	Merge(a, []*Contact{b, c}, "965")
	if a.Last != "Hassan" || len(a.Phones) != 1 || len(a.Emails) != 1 || !a.HasGroup("work") {
		t.Errorf("merge: %+v", a)
	}
}

func TestCleanup(t *testing.T) {
	c := &Contact{First: "MOHAMMED", Last: "al-rashid", Phones: []Field{{"Mobile", "99998888"}},
		Emails: []Field{{"Work", "Bob@Example.COM"}}, URLs: []Field{{"Website", "sala.company"}}}
	ch := PlanCleanup([]*Contact{c}, "965")
	if len(ch) != 5 {
		t.Fatalf("expected 5 changes, got %d: %+v", len(ch), ch)
	}
	for _, x := range ch {
		x.Apply()
	}
	if c.First != "Mohammed" || c.Last != "Al-Rashid" || c.Phones[0].Value != "+965 9999 8888" || c.Emails[0].Value != "bob@example.com" || c.URLs[0].Value != "https://sala.company" {
		t.Errorf("after cleanup: %+v", c)
	}
	if fixCase("McDonald") != "McDonald" {
		t.Error("mixed case must be left alone")
	}
}

func TestParseLocal(t *testing.T) {
	sig := `Best regards,
Khalid Al Mutairi
Senior Software Engineer
Boubyan Bank K.S.C.P
M: +965 6000 1234 | T: 2232 5000
khalid.m@example.com
www.example.com.kw
linkedin.com/in/khalidm`
	c := ParseLocal(sig, "965")
	if c.First != "Khalid" || c.Last != "Al Mutairi" {
		t.Errorf("name: %q / %q", c.First, c.Last)
	}
	if c.Title != "Senior Software Engineer" || c.Company != "Boubyan Bank K.S.C.P" {
		t.Errorf("title/company: %q / %q", c.Title, c.Company)
	}
	if len(c.Phones) != 2 || c.Phones[0].Value != "+965 6000 1234" || c.Phones[1].Label != "Work" {
		t.Errorf("phones: %+v", c.Phones)
	}
	if len(c.Emails) != 1 || len(c.URLs) != 2 || c.URLs[1].Label != "LinkedIn" {
		t.Errorf("emails/urls: %+v %+v", c.Emails, c.URLs)
	}
}

func TestDecodeAI(t *testing.T) {
	c, conf, err := decodeAIContact("```json\n{\"first\":\"Mona\",\"last\":\"Saleh\",\"phones\":[{\"label\":\"Mobile\",\"value\":\"55551111\"}],\"birthday\":\"1985-1-2\",\"confident\":true}\n```", "965")
	if err != nil || !conf || c.First != "Mona" || c.Phones[0].Value != "+965 5555 1111" {
		t.Errorf("%v %+v", err, c)
	}
}

func TestStore(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := NewContact()
	c.First = "Test"
	s.Put(c)
	_ = s.SetPhoto(c, []byte{1, 2, 3})
	s.AddGroup("Empty group")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s.Delete(c.ID)
	_ = s.Save()
	s2, _ := OpenStore(dir)
	if len(s2.Contacts) != 0 || len(s2.Trash) != 1 || len(s2.Groups()) != 1 {
		t.Fatalf("reload: %d contacts, %d trash, %v", len(s2.Contacts), len(s2.Trash), s2.Groups())
	}
	s2.Restore(c.ID)
	if got := s2.Photo(s2.Get(c.ID)); len(got) != 3 {
		t.Error("photo lost")
	}
	if _, err := os.Stat(dir + "/backups"); err != nil {
		t.Error("no backup folder")
	}
	cfg := LoadConfig(dir)
	cfg.OpenRouterKey = "sk-test"
	_ = cfg.Save(dir)
	if LoadConfig(dir).OpenRouterKey != "sk-test" || LoadConfig(dir).CountryCode != "965" {
		t.Error("config round trip")
	}
	if fi, _ := os.Stat(configPath(dir)); fi.Mode().Perm() != 0o600 {
		t.Errorf("config permissions %v", fi.Mode().Perm())
	}
}

func TestCSV(t *testing.T) {
	var b bytes.Buffer
	c := &Contact{First: "A", Last: "B", Phones: []Field{{"Mobile", "1"}}, Groups: []string{"x", "y"}}
	if err := WriteCSV(&b, []*Contact{c}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.String(), "\uFEFFFirst Name") || !strings.Contains(b.String(), "x; y") {
		t.Error(b.String())
	}
}
