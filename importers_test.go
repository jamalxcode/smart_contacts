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
	_, err := ReadImport("friends.json", []byte(`{"friends_v2":[{"name":"A","timestamp":1}]}`), "965")
	if err == nil || !strings.Contains(err.Error(), "Facebook") {
		t.Errorf("%v", err)
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
