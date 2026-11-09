package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// Issues #8, #9, #10: name parts, other dates and related people survive vCards and CSVs.

const appleFullCard = "BEGIN:VCARD\r\nVERSION:3.0\r\n" +
	"N:Al-Kandari;Yousef;Ahmed;Dr.;PhD\r\nFN:Dr. Yousef Ahmed Al-Kandari\\, PhD\r\n" +
	"X-PHONETIC-FIRST-NAME:Yoo-sef\r\nX-PHONETIC-LAST-NAME:Al-Kan-da-ri\r\n" +
	"TEL;type=CELL:+965 9999 1111\r\n" +
	"item1.X-ABDATE;type=pref:2012-04-19\r\nitem1.X-ABLabel:_$!<Anniversary>!$_\r\n" +
	"item2.X-ABDATE:2020-09-01\r\nitem2.X-ABLabel:Started at the clinic\r\n" +
	"item3.X-ABRELATEDNAMES;type=pref:Mariam\r\nitem3.X-ABLabel:_$!<Spouse>!$_\r\n" +
	"item4.X-ABRELATEDNAMES:Hamad Al-Ali\r\nitem4.X-ABLabel:Partner at the clinic\r\n" +
	"END:VCARD\r\n"

func TestVCardNamePartsDatesRelated(t *testing.T) {
	cards, err := ParseVCards(strings.NewReader(appleFullCard))
	if err != nil || len(cards) != 1 {
		t.Fatalf("%v %d", err, len(cards))
	}
	c := cards[0].Contact
	if c.First != "Yousef" || c.Middle != "Ahmed" || c.Last != "Al-Kandari" || c.Prefix != "Dr." || c.Suffix != "PhD" {
		t.Errorf("name parts: %+v", c)
	}
	if c.FullName() != "Yousef Al-Kandari" {
		t.Errorf("FullName must stay first + last for matching: %q", c.FullName())
	}
	if c.FormalName() != "Dr. Yousef Ahmed Al-Kandari, PhD" {
		t.Errorf("FormalName: %q", c.FormalName())
	}
	if c.PhoneticName() != "Yoo-sef Al-Kan-da-ri" {
		t.Errorf("phonetic: %q", c.PhoneticName())
	}
	if len(c.Dates) != 2 || c.Dates[0] != (Field{"Anniversary", "2012-04-19"}) || c.Dates[1] != (Field{"Started at the clinic", "2020-09-01"}) {
		t.Errorf("dates: %+v", c.Dates)
	}
	rel := cards[0].Related
	if len(rel) != 2 || rel[0] != (Related{"Spouse", "Mariam", ""}) || rel[1].Name != "Hamad Al-Ali" || rel[1].Label != "Partner at the clinic" {
		t.Errorf("related: %+v", rel)
	}

	// and back out again
	var b bytes.Buffer
	if err := WriteVCardRelated(&b, c, nil, []Related{{"Spouse", "Mariam", ""}, {"Colleague", "Hamad Al-Ali", "id-hamad"}}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"N:Al-Kandari;Yousef;Ahmed;Dr.;PhD", "FN:Dr. Yousef Ahmed Al-Kandari\\, PhD",
		"X-PHONETIC-FIRST-NAME:Yoo-sef", ".X-ABDATE:2012-04-19", ".X-ABLabel:_$!<Anniversary>!$_",
		".X-ABRELATEDNAMES:Mariam", ".X-ABLabel:_$!<Spouse>!$_", "RELATED;TYPE=colleague:urn:uuid:id-hamad"} {
		if !strings.Contains(out, want) {
			t.Errorf("vCard should contain %q:\n%s", want, out)
		}
	}
	back, _ := ParseVCards(&b)
	bc := back[0].Contact
	if bc.FormalName() != c.FormalName() || bc.PhoneticName() != c.PhoneticName() || len(bc.Dates) != 2 {
		t.Errorf("round trip lost something: %+v", bc)
	}
	if r := back[0].Related; len(r) != 2 || r[1] != (Related{"Colleague", "Hamad Al-Ali", "id-hamad"}) {
		t.Errorf("round trip related (name and ID paired): %+v", r)
	}

	// a date without a year goes out the Apple way and comes back without one
	y := &Contact{ID: "y", First: "Noura", Dates: []Field{{"Anniversary", "--05-01"}}}
	b.Reset()
	_ = WriteVCard(&b, y, nil)
	if !strings.Contains(b.String(), "X-ABDATE;X-APPLE-OMIT-YEAR=1604:1604-05-01") {
		t.Errorf("no-year date: %s", b.String())
	}
	back, _ = ParseVCards(&b)
	if d := back[0].Contact.Dates; len(d) != 1 || d[0].Value != "--05-01" {
		t.Errorf("no-year date back: %+v", d)
	}

	// vCard 4 ANNIVERSARY and RELATED with text
	v4 := "BEGIN:VCARD\nVERSION:4.0\nFN:Sara\nN:;Sara;;;\nANNIVERSARY:20150501\nRELATED;TYPE=friend;VALUE=text:Layla Haddad\nEND:VCARD\n"
	back, _ = ParseVCards(strings.NewReader(v4))
	if d := back[0].Contact.Dates; len(d) != 1 || d[0] != (Field{"Anniversary", "2015-05-01"}) {
		t.Errorf("ANNIVERSARY: %+v", d)
	}
	if r := back[0].Related; len(r) != 1 || r[0] != (Related{"Friend", "Layla Haddad", ""}) {
		t.Errorf("RELATED text: %+v", r)
	}
}

func TestCSVNamePartsAndDates(t *testing.T) {
	google := "Name Prefix,Given Name,Additional Name,Family Name,Name Suffix,Phonetic First Name,Event 1 - Type,Event 1 - Value,Phone 1 - Type,Phone 1 - Value\n" +
		"Eng.,Khalid,Saad,Al-Mutairi,Jr.,Kha-lid,Anniversary,2014-02-14,Mobile,+965 6000 1234\n"
	b, err := ReadImport("contacts.csv", []byte(google), "965")
	if err != nil || len(b.Cards) != 1 {
		t.Fatalf("%v %+v", err, b)
	}
	c := b.Cards[0].Contact
	if c.Prefix != "Eng." || c.First != "Khalid" || c.Middle != "Saad" || c.Last != "Al-Mutairi" || c.Suffix != "Jr." || c.PhoneticFirst != "Kha-lid" {
		t.Errorf("Google name parts: %+v", c)
	}
	if len(c.Dates) != 1 || c.Dates[0] != (Field{"Anniversary", "2014-02-14"}) {
		t.Errorf("Google event: %+v", c.Dates)
	}

	outlook := "Title,First Name,Middle Name,Last Name,Suffix,Job Title,Mobile Phone,Anniversary\n" +
		"Dr.,Fatima,,Al-Sabah,,Branch Manager,+965 5555 2222,3/5/2010\n"
	b, _ = ReadImport("outlook.csv", []byte(outlook), "965")
	c = b.Cards[0].Contact
	if c.Prefix != "Dr." || c.Title != "Branch Manager" || len(c.Dates) != 1 || c.Dates[0].Label != "Anniversary" {
		t.Errorf("Outlook: prefix %q title %q dates %+v", c.Prefix, c.Title, c.Dates)
	}

	// the app's own CSV comes back whole
	orig := &Contact{ID: "x", Prefix: "Dr.", First: "Yousef", Middle: "Ahmed", Last: "Al-Kandari", Suffix: "PhD",
		PhoneticFirst: "Yoo-sef", PhoneticMiddle: "Ah-med", PhoneticLast: "Al-Kan-da-ri",
		Phones: []Field{{"Mobile", "+965 9999 1111"}},
		Dates:  []Field{{"Anniversary", "2012-04-19"}, {"Graduated", "2005-06-30"}, {"Other", "--03-08"}}}
	var buf bytes.Buffer
	if err := WriteCSV(&buf, []*Contact{orig}); err != nil {
		t.Fatal(err)
	}
	b, _ = ReadImport("export.csv", buf.Bytes(), "965")
	c = b.Cards[0].Contact
	if c.FormalName() != orig.FormalName() || c.PhoneticName() != orig.PhoneticName() {
		t.Errorf("own CSV names: %q / %q", c.FormalName(), c.PhoneticName())
	}
	if len(c.Dates) != 3 || c.Dates[1] != (Field{"Graduated", "2005-06-30"}) || c.Dates[2] != (Field{"Other", "--03-08"}) {
		t.Errorf("own CSV dates: %+v", c.Dates)
	}
}

func TestUpcomingDates(t *testing.T) {
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	c := &Contact{Dates: []Field{{"Anniversary", "2012-04-19"}, {"Other", "--04-15"}, {"Far", "2001-08-01"}, {"Bad", "soon"}}}
	got := c.UpcomingDates(now, 7)
	if len(got) != 2 || got[0].Label != "Other" || got[0].Days != 0 || got[1].Days != 4 || got[1].Years != 14 {
		t.Errorf("%+v", got)
	}
	c.Tidy() // a date it can't read is kept as written (the editor refuses to save it)
	if len(c.Dates) != 4 {
		t.Errorf("Tidy dropped a date: %+v", c.Dates)
	}
}

// Merging used to drop the merged card's own lines and follow-ups.
func TestMergeKeepsOwnLinesDatesFollowUpsAndNameParts(t *testing.T) {
	due := time.Date(2026, 12, 1, 0, 0, 0, 0, time.Local)
	keep := &Contact{ID: "a", First: "Ali", Custom: []Field{{"Diwaniya", "Thursdays"}}}
	other := &Contact{ID: "b", First: "Ali", Middle: "Hassan", Prefix: "Eng.",
		Custom:    []Field{{"Diwaniya", "thursdays"}, {"Spouse", "Noor"}},
		Dates:     []Field{{"Anniversary", "2011-11-11"}},
		FollowUps: []FollowUp{{ID: "f1", Text: "Send the proposal", Due: due}}}
	Merge(keep, []*Contact{other}, "965")
	if keep.Middle != "Hassan" || keep.Prefix != "Eng." {
		t.Errorf("name parts not merged: %+v", keep)
	}
	if len(keep.Custom) != 2 || keep.Custom[1] != (Field{"Spouse", "Noor"}) {
		t.Errorf("own lines: %+v", keep.Custom)
	}
	if len(keep.Dates) != 1 || len(keep.FollowUps) != 1 {
		t.Errorf("dates %+v follow-ups %+v", keep.Dates, keep.FollowUps)
	}
	Merge(keep, []*Contact{other}, "965") // merging the same again adds nothing
	if len(keep.Custom) != 2 || len(keep.Dates) != 1 || len(keep.FollowUps) != 1 {
		t.Errorf("repeated merge doubled things: %d %d %d", len(keep.Custom), len(keep.Dates), len(keep.FollowUps))
	}
}

func TestRelatedOnImportAndExport(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	mariam := &Contact{ID: "m", First: "Mariam", Last: "Al-Kandari"}
	ali1 := &Contact{ID: "a1", First: "Ali", Last: "Hassan"}
	ali2 := &Contact{ID: "a2", First: "Ali", Last: "Hassan", Company: "Zain"}
	for _, c := range []*Contact{mariam, ali1, ali2} {
		s.Put(c)
	}
	file := "BEGIN:VCARD\nVERSION:3.0\nN:Al-Kandari;Yousef;;;\nTEL:+965 9999 1111\n" +
		"item1.X-ABRELATEDNAMES:Mariam Al-Kandari\nitem1.X-ABLabel:_$!<Spouse>!$_\n" + // one card: a connection
		"item2.X-ABRELATEDNAMES:Ali Hassan\nitem2.X-ABLabel:_$!<Friend>!$_\n" + // two cards: kept as a line
		"item3.X-ABRELATEDNAMES:Omar\nitem3.X-ABLabel:_$!<Child>!$_\n" + // nobody: kept as a line
		"END:VCARD\n"
	b, err := ReadImport("yousef.vcf", []byte(file), "965")
	if err != nil {
		t.Fatal(err)
	}
	ApplyImport(s, PlanImport(s, b.Cards, b.Source, "965"), nil, "965")
	var y *Contact
	for _, c := range s.Contacts {
		if c.First == "Yousef" {
			y = c
		}
	}
	if y == nil {
		t.Fatal("Yousef not imported")
	}
	links := s.LinksOf(y.ID)
	if len(links) != 1 || !links[0].has("m") || links[0].Label != "Spouse" {
		t.Errorf("links: %+v", links)
	}
	if len(y.Custom) != 2 || y.Custom[0] != (Field{"Friend", "Ali Hassan"}) || y.Custom[1] != (Field{"Child", "Omar"}) {
		t.Errorf("unlinked names should be kept as lines: %+v", y.Custom)
	}

	// export: the connection (with its ID) and the lines, each once
	rel := s.RelatedFor(y)
	if len(rel) != 3 || rel[0] != (Related{"Spouse", "Mariam Al-Kandari", "m"}) {
		t.Errorf("RelatedFor: %+v", rel)
	}
	y.Custom = append(y.Custom, Field{"Spouse", "Mariam Al-Kandari"}) // the same person as the link
	if n := len(s.RelatedFor(y)); n != 3 {
		t.Errorf("a line naming a linked person is said once, got %d", n)
	}

	// re-importing our own export links by ID, and adds nothing twice
	var buf bytes.Buffer
	_ = WriteVCardRelated(&buf, y, nil, s.RelatedFor(y))
	b, _ = ReadImport("again.vcf", buf.Bytes(), "965")
	ApplyImport(s, PlanImport(s, b.Cards, b.Source, "965"), nil, "965")
	if len(s.LinksOf(y.ID)) != 1 {
		t.Errorf("re-import doubled links: %+v", s.LinksOf(y.ID))
	}
	if got := s.Get(y.ID); len(got.Custom) != 3 {
		t.Errorf("re-import doubled lines: %+v", got.Custom)
	}

	if names := splitNames("Ali, Noor and Sara"); len(names) != 3 {
		t.Errorf("splitNames: %q", names)
	}
	if names := splitNames("علي، نور"); len(names) != 2 {
		t.Errorf("splitNames Arabic: %q", names)
	}
}
