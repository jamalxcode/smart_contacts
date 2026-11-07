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
	if got := strings.Join(phones, " | "); got != "+965 9999 8888 | +966 501 234 567" {
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
