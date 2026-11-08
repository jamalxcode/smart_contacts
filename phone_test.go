package main

import (
	"strings"
	"testing"
)

func TestAnalyzePhone(t *testing.T) {
	cases := []struct {
		in     string
		hints  []string
		norm   string
		status int
	}{
		{"99998888", nil, "+96599998888", phOK},
		{"0096599998888", nil, "+96599998888", phOK},
		{"00 965 9999 8888", nil, "+96599998888", phOK},
		{"+00965 99998888", nil, "+96599998888", phOK},
		{"96599998888", nil, "+96599998888", phOK},
		{"099998888", nil, "+96599998888", phOK}, // extra 0 typed on a Kuwaiti number
		{"٩٩٩٩٨٨٨٨", nil, "+96599998888", phOK},
		{"22223333", nil, "+96522223333", phOK},
		{"0044 20 7946 0958", nil, "+442079460958", phOK},
		{"+44 (0)20 7946 0958", nil, "+442079460958", phOK},
		{"+966 050 123 4567", nil, "+966501234567", phOK}, // trunk 0 after the country code
		{"00971501234567", nil, "+971501234567", phOK},
		{"0501234567", []string{"966"}, "+966501234567", phOK}, // Saudi address on the card
		{"0501234567", []string{"971"}, "+971501234567", phOK},
		{"0501234567", nil, "0501234567", phUnsure}, // Saudi or UAE? ask
		{"07911 123456", []string{"44"}, "+447911123456", phOK},
		{"171", nil, "171", phService},
		{"1802555", nil, "1802555", phService},
		{"+965 1802555", nil, "1802555", phService},
		{"33334444", nil, "+96533334444", phSuspicious}, // no Kuwaiti number starts with 3
		{"+965 9999 888", nil, "+9659999888", phSuspicious},
		{"00000000", nil, "00000000", phInvalid},
		{"12345678", nil, "12345678", phInvalid},
		{"call office", nil, "", phInvalid},
		{"+1 415 555 0100 ext 22", nil, "+14155550100", phOK},
	}
	for _, tc := range cases {
		r := analyzePhone(tc.in, "965", tc.hints)
		if r.Norm != tc.norm || r.Status != tc.status {
			t.Errorf("%q (hints %v): got %q status %d (%s), want %q status %d", tc.in, tc.hints, r.Norm, r.Status, r.Reason, tc.norm, tc.status)
		}
	}
	if r := analyzePhone("0501234567", "965", nil); len(r.Options) < 2 {
		t.Errorf("unsure numbers should offer countries: %v", r.Options)
	}
}

func TestCardHints(t *testing.T) {
	c := &Contact{
		Addresses: []Address{{City: "Riyadh"}},
		Emails:    []Field{{"Work", "a@company.ae"}},
		Phones:    []Field{{"Mobile", "+44 7911 123456"}, {"Mobile", "99998888"}},
	}
	h := strings.Join(cardHints(c, "965"), ",")
	if h != "966,44,971" {
		t.Errorf("hints %s", h)
	}
}

func TestFixEmail(t *testing.T) {
	for in, want := range map[string]string{
		"mailto:Ali@Gmial.com": "ali@gmail.com",
		" ali @ gmail.com ":    "ali@gmail.com",
		"<ali@hotmail.con>":    "ali@hotmail.com",
		"ali@yaho.com.":        "ali@yahoo.com",
		"ali@company.com.kw":   "ali@company.com.kw",
		"Ali@@outlok.com":      "ali@outlook.com",
	} {
		got, _, ok := fixEmail(in)
		if got != want || !ok {
			t.Errorf("fixEmail(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"ali.gmail.com", "ali@", "@gmail.com", "ali@gmail"} {
		if _, _, ok := fixEmail(bad); ok {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestCleanupPlan(t *testing.T) {
	c := &Contact{First: "Sara",
		Addresses: []Address{{Country: "Saudi Arabia"}},
		Phones: []Field{
			{"Mobile", "0096599998888"},   // 00 → +
			{"Home", "99998888"},          // the same number again
			{"Work", "0501234567"},        // local Saudi number (address says Saudi)
			{"Other", "00000000"},         // junk
			{"Other", "sara@example.com"}, // email in the phone field
		},
		Emails: []Field{{"Work", "Sara@Gmial.com"}, {"Home", "sara@gmail.com"}, {"Other", "not an email"}},
	}
	ch := PlanCleanup([]*Contact{c}, "965")
	kinds := map[int]int{}
	for _, x := range ch {
		kinds[x.Kind]++
		if x.Default {
			x.Apply("")
		}
	}
	if kinds[ckDuplicate] != 2 || kinds[ckWrong] < 2 {
		t.Errorf("kinds: %v\n%+v", kinds, ch)
	}
	phones := []string{}
	for _, p := range c.Phones {
		phones = append(phones, p.Value)
	}
	if got := strings.Join(phones, " | "); got != "+965 9999 8888 | +966 50 123 4567" {
		t.Errorf("phones after clean-up: %s", got)
	}
	emails := []string{}
	for _, e := range c.Emails {
		emails = append(emails, e.Value)
	}
	if got := strings.Join(emails, " | "); got != "sara@gmail.com | sara@example.com" {
		t.Errorf("emails after clean-up: %s", got)
	}
}

func TestCleanupUnsureNeedsChoice(t *testing.T) {
	c := &Contact{First: "X", Phones: []Field{{"Mobile", "0501234567"}}}
	ch := PlanCleanup([]*Contact{c}, "965")
	if len(ch) != 1 || ch[0].Kind != ckUnsure || ch[0].Default {
		t.Fatalf("%+v", ch)
	}
	ch[0].Apply("")
	if c.Phones[0].Value != "0501234567" {
		t.Error("no choice = leave as is")
	}
	ch[0].Apply(ch[0].Options[0])
	if !strings.HasPrefix(c.Phones[0].Value, "+9") {
		t.Errorf("chosen: %s", c.Phones[0].Value)
	}
}

func TestTidyNotes(t *testing.T) {
	c := &Contact{Phones: []Field{{"Mobile", "+965 6000 1234"}}, Emails: []Field{{"Work", "k@x.com"}},
		Notes: "Met at GITEX 2025.  \n\nMet at GITEX 2025.\nLinkedIn connection since 12 Mar 2021\nLinkedIn connection since 12 Mar 2021\n\n\n\nMobile: +965 6000 1234\nk@x.com\nPrefers Telegram &amp; WhatsApp<br>Coffee at 4pm\\nCall after 5"}
	got, why, only := TidyNotes(c)
	want := "Met at GITEX 2025.\n\nLinkedIn connection since 12 Mar 2021\n\nPrefers Telegram & WhatsApp\nCoffee at 4pm\nCall after 5"
	if got != want || only {
		t.Errorf("got %q (%s, only=%v)\nwant %q", got, why, only, want)
	}
	d := &Contact{Notes: "Same line\nsame line\nOther"}
	if got, _, only := TidyNotes(d); got != "Same line\nOther" || !only {
		t.Errorf("repeat-only: %q %v", got, only)
	}
	clean := &Contact{Notes: "All fine\n\nSecond paragraph"}
	if got, _, _ := TidyNotes(clean); got != clean.Notes {
		t.Errorf("clean notes changed: %q", got)
	}
}

func TestRepeatedLinksAndAddresses(t *testing.T) {
	c := &Contact{First: "K", URLs: []Field{{"LinkedIn", "https://www.linkedin.com/in/khalidm/"}, {"Website", "linkedin.com/in/khalidm"},
		{"Telegram", "@khalid_kw"}, {"Other", "https://t.me/khalid_kw"}, {"Website", "https://sala.company"}, {"Website", "sala.company/"}},
		Addresses: []Address{{Label: "Home", City: "Salmiya"}, {Label: "Work", City: "salmiya"}}}
	n := 0
	for _, ch := range PlanCleanup([]*Contact{c}, "965") {
		if ch.Kind == ckDuplicate {
			ch.Apply("")
			n++
		}
	}
	if n != 4 || len(c.URLs) != 3 || len(c.Addresses) != 1 {
		t.Errorf("%d repeats; urls %+v; addresses %+v", n, c.URLs, c.Addresses)
	}
}

func TestPhoneTypes(t *testing.T) {
	for n, want := range map[string]string{"+96599998888": "mobile", "+96522223333": "landline", "+447911123456": "mobile", "+442079460958": "landline", "171": ""} {
		if got := PhoneType(n); got != want {
			t.Errorf("PhoneType(%s) = %q, want %q", n, got, want)
		}
	}
	if CanWhatsApp("22223333", "965") || !CanWhatsApp("99998888", "965") {
		t.Error("WhatsApp only for mobiles")
	}
	// numbers from countries outside the built-in table are now validated too
	if r := analyzePhone("+351 912 345 678", "965", nil); r.Status != phOK || r.Norm != "+351912345678" {
		t.Errorf("Portugal: %+v", r)
	}
	if r := analyzePhone("+351 912", "965", nil); r.Status != phSuspicious {
		t.Errorf("short foreign number should be flagged: %+v", r)
	}
}

func TestFixLink(t *testing.T) {
	for _, tc := range []struct{ label, in, want string }{
		{"Telegram", "@khalid_kw", "https://t.me/khalid_kw"},
		{"Telegram", "https://@khalid_kw", "https://t.me/khalid_kw"},
		{"X", "https://@khalidkw", "https://x.com/khalidkw"},
		{"Homepage", "example.com", "https://example.com"},
		{"LinkedIn", "https://www.linkedin.com/in/khalidm", ""},
		{"Other", "somehandle", ""},
	} {
		got, _ := fixLink(tc.label, tc.in)
		if got != tc.want {
			t.Errorf("fixLink(%q, %q) = %q, want %q", tc.label, tc.in, got, tc.want)
		}
	}
}

func TestSwappedNames(t *testing.T) {
	c := NewContact()
	c.First, c.Last = "Al-Kandari", "Dr. Yousef"
	found := false
	for _, ch := range PlanCleanup([]*Contact{c}, "965") {
		if ch.Reason == "first and last name look swapped" {
			found = ch.Default
			ch.apply("")
		}
	}
	if !found || c.First != "Dr. Yousef" || c.Last != "Al-Kandari" {
		t.Errorf("got %q %q", c.First, c.Last)
	}
	d := NewContact()
	d.First, d.Last = "Fatima", "Al-Sabah"
	for _, ch := range PlanCleanup([]*Contact{d}, "965") {
		if ch.Reason == "first and last name look swapped" {
			t.Error("a normal name must not be swapped")
		}
	}
}
