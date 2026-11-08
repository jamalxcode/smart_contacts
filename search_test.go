package main

import (
	"testing"
	"time"
)

func TestSearchFilters(t *testing.T) {
	a := NewContact()
	a.First, a.Last, a.Company = "Khalid", "Al-Mutairi", "Boubyan Bank"
	a.Phones = []Field{{Label: "Mobile", Value: "+965 6000 1234"}}
	a.Groups = []string{"Work"}
	a.Sources = []string{"LinkedIn"}
	b := NewContact()
	b.First, b.Company = "Giulia", "Gulf Bank"
	b.Phones = []Field{{Label: "Mobile", Value: "+39 333 123 4567"}}
	b.Emails = []Field{{Label: "Work", Value: "g@x.it"}}
	b.KeepInTouch = 30
	last := func(c *Contact) time.Time {
		if c == a {
			return time.Now().Add(-5 * 24 * time.Hour)
		}
		return time.Time{}
	}
	cases := map[string][]*Contact{
		"company:boubyan":         {a},
		`company:"gulf bank"`:     {b},
		"bank no:email":           {a},
		"has:email":               {b},
		"code:965":                {a},
		"country:kuwait":          {a},
		"-group:work":             {b},
		"source:linkedin khalid":  {a},
		"talked:<2w":              {a},
		"talked:>1y":              {b},
		"has:reminder":            {b},
		"is:due":                  {b},
		"company:bank group:work": {a},
	}
	for q, want := range cases {
		sq := parseQuery(q)
		var got []*Contact
		for _, c := range []*Contact{a, b} {
			if sq.matches(c, last, func(c *Contact) bool { return c.KeepInTouch > 0 }) {
				got = append(got, c)
			}
		}
		if len(got) != len(want) || (len(got) == 1 && got[0] != want[0]) {
			names := []string{}
			for _, g := range got {
				names = append(names, g.First)
			}
			t.Errorf("%q matched %v", q, names)
		}
	}
	if sq := parseQuery("http://x.com"); len(sq.filters) != 0 {
		t.Error("a URL isn't a filter")
	}
}
