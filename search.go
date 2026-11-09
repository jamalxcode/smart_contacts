package main

import (
	"strconv"
	"strings"
	"time"
)

// Search filters. Plain words search everything as before; words with a colon filter:
//
//	company:boubyan  title:engineer  group:family  from:linkedin (or source:)  city:salmiya  country:kuwait
//	has:email  no:phone  (phone, email, photo, birthday, address, link, note, card, reminder, met)
//	code:965          numbers with that country code
//	talked:<30d       in touch in the last 30 days (d, w, m, y);  talked:>1y  not for a year
//	is:favorite  is:due (time to reach out)  is:arabic (has an Arabic name)
//
// A minus in front reverses a filter: -group:work. Several filters must all match.
type searchQuery struct {
	text    string
	filters []searchFilter
	fuzzy   bool // forgive typos in the words (second pass when nothing matched exactly)
}

type searchFilter struct {
	key, val string
	neg      bool
}

var filterKeys = map[string]bool{"company": true, "title": true, "group": true, "source": true, "city": true,
	"country": true, "has": true, "no": true, "code": true, "talked": true, "is": true, "label": true, "from": true}

func parseQuery(q string) searchQuery {
	var sq searchQuery
	var words []string
	for _, w := range splitQuery(q) {
		neg := strings.HasPrefix(w, "-") && len(w) > 1
		ww := strings.TrimPrefix(w, "-")
		if k, v, ok := strings.Cut(ww, ":"); ok && filterKeys[strings.ToLower(k)] && v != "" {
			k = strings.ToLower(k)
			if k == "from" { // from:linkedin — where the card came from
				k = "source"
			}
			sq.filters = append(sq.filters, searchFilter{k, strings.Trim(v, `"`), neg})
			continue
		}
		words = append(words, w)
	}
	sq.text = strings.Join(words, " ")
	return sq
}

// splitQuery splits on spaces but keeps quoted values together: company:"gulf bank".
func splitQuery(q string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range q {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ' ' && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		if r != '"' {
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func (sq searchQuery) empty() bool { return sq.text == "" && len(sq.filters) == 0 }

// lastTalkedFunc gives when a contact was last in touch (from the UI's activity data).
type lastTalkedFunc func(*Contact) time.Time

// matches applies the query to a contact.
func (sq searchQuery) matches(c *Contact, last lastTalkedFunc, due func(*Contact) bool) bool {
	if sq.text != "" {
		if sq.fuzzy && !c.MatchesFuzzy(sq.text) || !sq.fuzzy && !c.Matches(sq.text) {
			return false
		}
	}
	for _, f := range sq.filters {
		if f.test(c, last, due) == f.neg {
			return false
		}
	}
	return true
}

func foldContains(s, sub string) bool { return strings.Contains(fold(s), fold(sub)) }

func (f searchFilter) test(c *Contact, last lastTalkedFunc, due func(*Contact) bool) bool {
	v := f.val
	switch f.key {
	case "company":
		return foldContains(c.Company, v)
	case "title":
		return foldContains(c.Title, v)
	case "group":
		for _, g := range c.Groups {
			if foldContains(g, v) {
				return true
			}
		}
	case "source":
		for _, s := range c.Sources {
			if foldContains(s, v) {
				return true
			}
		}
	case "label":
		for _, p := range append(append([]Field{}, c.Phones...), c.Emails...) {
			if foldContains(p.Label, v) {
				return true
			}
		}
	case "city", "country":
		for _, a := range c.Addresses {
			if (f.key == "city" && foldContains(a.City, v)) || (f.key == "country" && foldContains(a.Country, v)) {
				return true
			}
		}
		if f.key == "country" { // a country name also matches its phone code: country:kuwait
			if cc := countryCodeFor(v); cc != "" {
				return hasCode(c, cc)
			}
		}
	case "code":
		return hasCode(c, strings.TrimPrefix(v, "+"))
	case "has", "no":
		h := hasField(c, strings.ToLower(v), last)
		if f.key == "no" {
			return !h
		}
		return h
	case "is":
		switch strings.ToLower(v) {
		case "favorite", "favourite", "fav":
			return c.Favorite
		case "due":
			return due != nil && due(c)
		case "arabic":
			return hasArabic(c.FullName()) || hasArabic(c.AltName)
		case "company":
			return c.IsCompany()
		}
	case "talked":
		if last == nil {
			return false
		}
		lt := last(c)
		op := v[:1]
		d, ok := parseSpan(strings.TrimLeft(v, "<>"))
		if !ok {
			return false
		}
		age := time.Since(lt)
		if op == ">" {
			return lt.IsZero() || age > d
		}
		return !lt.IsZero() && age <= d
	}
	return false
}

func hasField(c *Contact, what string, last lastTalkedFunc) bool {
	switch what {
	case "phone", "number":
		return len(c.Phones) > 0
	case "email", "mail":
		return len(c.Emails) > 0
	case "photo", "picture":
		return c.HasPhoto
	case "birthday":
		return c.Birthday != ""
	case "address":
		return len(c.Addresses) > 0
	case "link", "links", "social":
		return len(c.URLs) > 0
	case "note", "notes":
		return strings.TrimSpace(c.Notes) != ""
	case "card", "businesscard":
		return len(c.CardImages) > 0
	case "reminder", "keep":
		return c.KeepInTouch > 0
	case "group", "groups":
		return len(c.Groups) > 0
	case "met", "history":
		return last != nil && !last(c).IsZero()
	case "arabic", "othername":
		return c.AltName != ""
	}
	return false
}

func hasCode(c *Contact, cc string) bool {
	for _, p := range c.Phones {
		if strings.HasPrefix(strings.TrimPrefix(NormalizePhone(p.Value, ""), "+"), cc) && strings.HasPrefix(NormalizePhone(p.Value, ""), "+") {
			return true
		}
	}
	return false
}

// parseSpan reads 30d, 2w, 6m, 1y.
func parseSpan(s string) (time.Duration, bool) {
	if len(s) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n < 0 {
		return 0, false
	}
	day := 24 * time.Hour
	switch s[len(s)-1] {
	case 'd':
		return time.Duration(n) * day, true
	case 'w':
		return time.Duration(n) * 7 * day, true
	case 'm':
		return time.Duration(n) * 30 * day, true
	case 'y':
		return time.Duration(n) * 365 * day, true
	}
	return 0, false
}

// countryCodeFor finds a phone code by country name ("kuwait" → "965").
func countryCodeFor(name string) string {
	n := fold(name)
	for _, ct := range countries {
		if fold(ct.name) == n {
			return ct.cc
		}
		for _, w := range ct.words {
			if w == n {
				return ct.cc
			}
		}
	}
	return ""
}
