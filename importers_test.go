package main

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

const googleCSV = "Name,Given Name,Additional Name,Family Name,Nickname,Birthday,Notes,Group Membership,E-mail 1 - Type,E-mail 1 - Value,Phone 1 - Type,Phone 1 - Value,Phone 2 - Type,Phone 2 - Value,Organization 1 - Name,Organization 1 - Title,Website 1 - Type,Website 1 - Value\n" +
	"Fatima Al-Sabah,Fatima,,Al-Sabah,,1988-11-10,Met at GITEX,* myContacts ::: Clients,* Work,fatima@gulfbank.example,Mobile,+965 9999 1234,Work,2222 3333,Gulf Bank,Branch Manager,,\n" +
	"Sam Brown,Sam,,Brown,,,,* myContacts,,,Mobile,+1 415 555 0100 ::: +1 415 555 0101,,,,,Profile,https://example.com/sam\n"

const outlookCSV = "First Name,Middle Name,Last Name,Title,Suffix,E-mail Address,E-mail 2 Address,Mobile Phone,Home Phone,Business Phone,Company,Job Title,Business Street,Business City,Business Country/Region,Birthday,Notes,Categories\n" +
	"Chidi,,Okafor,Mr.,,chidi@northwind.example,,,,+44 20 7946 0958,Northwind Labs,CTO,1 King St,London,United Kingdom,,,Work\n"

const linkedinCSV = "Notes:\n\"When exporting your connection data, you may notice that some of the email addresses are missing.\"\n\n" +
	"First Name,Last Name,URL,Email Address,Company,Position,Connected On\n" +
	"Khalid,Al Mutairi,https://www.linkedin.com/in/khalidm,,Boubyan Bank,Senior Software Engineer,12 Mar 2021\n" +
	"Layla,Haddad,https://www.linkedin.com/in/laylah,layla.haddad@zain.example,Zain Group,Head of Partnerships,01 Jan 2024\n" +
	"New,Person,https://www.linkedin.com/in/newp,,Acme,Engineer,05 May 2025\n"

const arabicSheet = "الاسم,الجوال,البريد الالكتروني,الشركة,الاسم بالانجليزي\n" +
	"نورة الرشيد,0096550507070,noura@example.com,,Noura Al-Rasheed\n" +
	"علي حسن,65551212,,,\n"

func TestReadGoogleCSV(t *testing.T) {
	b, err := ReadImport("contacts.csv", []byte(googleCSV), "965")
	if err != nil || len(b.Cards) != 2 {
		t.Fatalf("%v %+v", err, b)
	}
	if b.Format != "Google Contacts export" || b.Source != "Google Contacts" {
		t.Errorf("format %q source %q", b.Format, b.Source)
	}
	f := b.Cards[0].Contact
	if f.First != "Fatima" || f.Company != "Gulf Bank" || f.Title != "Branch Manager" || f.Birthday != "1988-11-10" ||
		len(f.Phones) != 2 || f.Phones[1].Label != "Work" || !f.HasGroup("Clients") || len(f.Emails) != 1 {
		t.Errorf("fatima: %+v", f)
	}
	if s := b.Cards[1].Contact; len(s.Phones) != 2 {
		t.Errorf("':::' should split numbers: %+v", s.Phones)
	}
}

func TestReadOutlookCSV(t *testing.T) {
	b, _ := ReadImport("outlook.csv", []byte(outlookCSV), "965")
	c := b.Cards[0].Contact
	if b.Source != "Outlook" || c.Title != "CTO" || c.Phones[0].Label != "Work" || c.Addresses[0].City != "London" {
		t.Errorf("%q %+v", b.Source, c)
	}
}

func TestReadLinkedIn(t *testing.T) {
	b, _ := ReadImport("Connections.csv", []byte(linkedinCSV), "965")
	if b.Source != "LinkedIn" || len(b.Cards) != 3 {
		t.Fatalf("%q %d", b.Source, len(b.Cards))
	}
	k := b.Cards[0].Contact
	if k.Last != "Al Mutairi" || k.URLs[0].Label != "LinkedIn" || !strings.Contains(k.Notes, "LinkedIn connection since 12 Mar 2021") {
		t.Errorf("%+v", k)
	}
}

func TestReadArabicSheet(t *testing.T) {
	b, _ := ReadImport("ارقام.csv", []byte(arabicSheet), "965")
	if len(b.Cards) != 2 {
		t.Fatalf("%d cards", len(b.Cards))
	}
	n := b.Cards[0].Contact
	if n.First != "نورة" || n.Last != "الرشيد" || n.AltName != "Noura Al-Rasheed" || len(n.Phones) != 1 || len(n.Emails) != 1 {
		t.Errorf("%+v", n)
	}
}

func TestHeaderlessSheet(t *testing.T) {
	b, _ := ReadImport("list.csv", []byte("Ali Hassan,65551212,ali@x.com\nSara Ali,99990000,sara@x.com\n"), "965")
	if len(b.Cards) != 2 || b.Cards[0].Contact.First != "Ali" || len(b.Cards[0].Contact.Emails) != 1 || len(b.Notes) == 0 {
		t.Errorf("%+v", b)
	}
}

func TestReadXLSX(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, _ := z.Create("xl/sharedStrings.xml")
	w.Write([]byte(`<sst><si><t>Name</t></si><si><t>Mobile</t></si><si><t>Mona Saleh</t></si></sst>`))
	w, _ = z.Create("xl/worksheets/sheet1.xml")
	w.Write([]byte(`<worksheet><sheetData><row><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>` +
		`<row><c r="A2" t="s"><v>2</v></c><c r="B2"><v>9.6555551111E10</v></c></row></sheetData></worksheet>`))
	z.Close()
	b, err := ReadImport("phones.xlsx", buf.Bytes(), "965")
	if err != nil || len(b.Cards) != 1 {
		t.Fatalf("%v %+v", err, b)
	}
	c := b.Cards[0].Contact
	if c.First != "Mona" || NormalizePhone(c.Phones[0].Value, "965") != "+96555551111" {
		t.Errorf("%+v", c)
	}
}

func TestNotesImport(t *testing.T) {
	text := `Ali Hassan 6555 1212
Sara Ali 9999 0000
Mona Saleh mona@example.com

Dr. Yousef Al-Kandari
Dentist
Dar Al Shifa Hospital
T: 1802555

just a note without contact details`
	b := NotesBatch(text, "965")
	if len(b.Cards) != 4 || b.Skipped != 1 {
		t.Fatalf("%d cards, %d skipped", len(b.Cards), b.Skipped)
	}
	if b.Cards[0].Contact.First != "Ali" || b.Cards[3].Contact.Company == "" {
		t.Errorf("%+v / %+v", b.Cards[0].Contact, b.Cards[3].Contact)
	}
}

func TestSocialJSON(t *testing.T) {
	b, err := ReadImport("friends.json", []byte(`{"friends_v2":[{"name":"A B","timestamp":1}]}`), "965")
	if err != nil || !b.MatchOnly || b.Source != "Facebook" {
		t.Errorf("%v %+v", err, b)
	}
}

func TestPlanAndApplyImport(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	khalid := NewContact()
	khalid.First, khalid.Last = "خالد", "المطيري"
	khalid.Phones = []Field{{"Mobile", "+965 6000 1234"}}
	s.Put(khalid)
	layla := NewContact()
	layla.First, layla.Last, layla.Company = "Layla", "Haddad", "Zain Group"
	layla.Emails = []Field{{"Work", "layla.haddad@zain.example"}}
	layla.URLs = []Field{{"LinkedIn", "https://www.linkedin.com/in/laylah"}}
	layla.Title = "Head of Partnerships"
	s.Put(layla)
	mo1, mo2 := NewContact(), NewContact()
	mo1.First, mo1.Last, mo1.Phones = "Mohammed", "Ali", []Field{{"Mobile", "99990001"}}
	mo2.First, mo2.Last, mo2.Phones = "Mohammed", "Ali", []Field{{"Mobile", "99990002"}}
	s.Put(mo1)
	s.Put(mo2)

	b, _ := ReadImport("Connections.csv", []byte(linkedinCSV+"Mohammed,Ali,https://www.linkedin.com/in/moali,,Acme,Sales,01 Jan 2020\n"+
		"New,Person,https://www.linkedin.com/in/newp,,Acme,Engineer,05 May 2025\n"), "965")
	items := PlanImport(s, b.Cards, b.Source, "965")
	kinds := []int{}
	for _, it := range items {
		kinds = append(kinds, it.Kind)
	}
	// Khalid (Arabic card, name match across languages) → merge; Layla → up to date (only "since" note is new → merge);
	// New Person → new; Mohammed Ali (two of them) → ask; repeated New Person → folded into the first
	want := []int{piMerge, piMerge, piNew, piAsk, piInFile}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("plan kinds %v, want %v\n%+v", kinds, want, items)
		}
	}
	if items[0].Why != "same name in another language" || !strings.Contains(strings.Join(items[0].Adds, ","), "LinkedIn link") {
		t.Errorf("khalid: %+v", items[0])
	}
	touched := ApplyImport(s, items, map[int]string{3: "merge:" + mo1.ID}, "965")
	if len(touched) != 5 {
		t.Errorf("touched %d", len(touched))
	}
	if khalid.AltName != "Khalid Al Mutairi" || khalid.Company != "Boubyan Bank" || !khalid.HasSource("LinkedIn") || len(khalid.URLs) != 1 {
		t.Errorf("khalid after import: %+v", khalid)
	}
	if mo1.Company != "Acme" || mo2.Company != "" {
		t.Error("the chosen Mohammed Ali should get the details, the other not")
	}
	if len(s.Contacts) != 5 {
		t.Errorf("%d contacts, want 5 (one new person, not two)", len(s.Contacts))
	}
	if src := s.SourceNames(); len(src) != 1 || src[0] != "LinkedIn" {
		t.Errorf("sources %v", src)
	}
	// importing the same file again changes nothing
	again := PlanImport(s, mustRead(t, linkedinCSV).Cards, "LinkedIn", "965")
	for _, it := range again {
		if it.Kind != piUpToDate {
			t.Errorf("re-import: %+v", it)
		}
	}
}

func mustRead(t *testing.T, csv string) *Batch {
	b, err := ReadImport("Connections.csv", []byte(csv), "965")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSocialExports(t *testing.T) {
	tg := `{"personal_information":{},"contacts":{"about":"x","list":[{"first_name":"Hamad","last_name":"Al-Ali","phone_number":"+96566112233","date":"2021-03-04T10:00:00"},{"first_name":"","last_name":"","phone_number":""}]}}`
	b, err := ReadImport("result.json", []byte(tg), "965")
	if err != nil || b.Source != "Telegram" || len(b.Cards) != 1 || b.Skipped != 1 || b.MatchOnly {
		t.Fatalf("%v %+v", err, b)
	}
	if c := b.Cards[0].Contact; c.Last != "Al-Ali" || len(c.Phones) != 1 || c.Notes != "Telegram contact since 4 Mar 2021" {
		t.Errorf("%+v", c)
	}
	// Facebook writes UTF-8 bytes as Latin-1 characters
	fb := `{"friends_v2":[{"name":"Khalid Al Mutairi","timestamp":1500000000},{"name":"Ù\u0086Ù\u0088Ø±Ø© Ø§Ù\u0084Ø±Ø´Ù\u008aØ¯","timestamp":1600000000}]}`
	b, err = ReadImport("your_friends.json", []byte(fb), "965")
	if err != nil || !b.MatchOnly || len(b.Cards) != 2 {
		t.Fatalf("%v %+v", err, b)
	}
	if n := b.Cards[1].Contact; n.First != "نورة" || n.Last != "الرشيد" {
		t.Errorf("Facebook Arabic name: %q %q", n.First, n.Last)
	}
	sc := `{"Friends":[{"Username":"dana.k","Display Name":"Dana K","Creation Timestamp":"2019-05-01 10:00:00 UTC"}]}`
	b, _ = ReadImport("friends.json", []byte(sc), "965")
	if b.Source != "Snapchat" || b.Cards[0].Contact.URLs[0].Label != "Snapchat" {
		t.Errorf("%+v", b)
	}
	for _, bad := range []string{`{"relationships_following":[{"string_list_data":[{"value":"x"}]}]}`, `window.YTD.following.part0 = [{"following":{"accountId":"1"}}]`} {
		if _, err := ReadImport("following.json", []byte(bad), "965"); err == nil {
			t.Errorf("should explain why it can't be imported: %s", bad)
		}
	}
}

func TestImportNotesDifferences(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	f := NewContact()
	f.First, f.Last, f.Company, f.Title = "Fatima", "Al-Sabah", "Gulf Bank", "Branch Manager"
	f.Phones = []Field{{"Mobile", "99991234"}}
	s.Put(f)
	b, _ := ReadImport("Connections.csv", []byte("First Name,Last Name,URL,Email Address,Company,Position,Connected On\nFatima,Al-Sabah,https://www.linkedin.com/in/fatimas,,KIB,Regional Director,03 Feb 2022\n"), "965")
	items := PlanImport(s, b.Cards, "LinkedIn", "965")
	ApplyImport(s, items, nil, "965")
	if f.Company != "KIB" || f.Title != "Regional Director" || !strings.Contains(f.Notes, "Previously: Branch Manager at Gulf Bank") ||
		!strings.Contains(f.Notes, "LinkedIn connection since 03 Feb 2022") {
		t.Errorf("LinkedIn job should be current, old job noted: %+v", f)
	}
	g, _ := ReadImport("google.csv", []byte("Name,Mobile,Company\nFatima Al-Sabah,99991234,Some Other Co\n"), "965")
	ApplyImport(s, PlanImport(s, g.Cards, "Google Contacts", "965"), nil, "965")
	if f.Company != "KIB" || !strings.Contains(f.Notes, "Google Contacts: Some Other Co") {
		t.Errorf("other sources keep the card's value and note theirs: %+v", f)
	}
	// importing the same thing again adds no repeated notes
	ApplyImport(s, PlanImport(s, g.Cards, "Google Contacts", "965"), nil, "965")
	if strings.Count(f.Notes, "Some Other Co") != 1 {
		t.Errorf("repeated note: %q", f.Notes)
	}
}
