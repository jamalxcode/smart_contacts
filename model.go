package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Field is a labelled value: a phone number, an email address or a web link.
type Field struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Address is a postal address. Every part is optional.
type Address struct {
	Label    string `json:"label"`
	Street   string `json:"street,omitempty"`
	City     string `json:"city,omitempty"`
	Region   string `json:"region,omitempty"`
	Postcode string `json:"postcode,omitempty"`
	Country  string `json:"country,omitempty"`
}

// OneLine renders the address on a single line, skipping empty parts.
func (a Address) OneLine() string {
	parts := []string{}
	for _, p := range []string{a.Street, a.City, a.Region, a.Postcode, a.Country} {
		p = strings.TrimSpace(strings.ReplaceAll(p, "\n", ", "))
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// IsEmpty reports whether the address holds nothing worth keeping.
func (a Address) IsEmpty() bool { return a.OneLine() == "" }

// Contact is one person or organisation.
type Contact struct {
	ID            string    `json:"id"`
	First         string    `json:"first,omitempty"`
	Last          string    `json:"last,omitempty"`
	Nickname      string    `json:"nickname,omitempty"`
	Company       string    `json:"company,omitempty"`
	Title         string    `json:"title,omitempty"`
	Phones        []Field   `json:"phones,omitempty"`
	Emails        []Field   `json:"emails,omitempty"`
	URLs          []Field   `json:"urls,omitempty"`
	Addresses     []Address `json:"addresses,omitempty"`
	Birthday      string    `json:"birthday,omitempty"` // YYYY-MM-DD, or --MM-DD when the year is unknown
	Notes         string    `json:"notes,omitempty"`
	Groups        []string  `json:"groups,omitempty"`
	Favorite      bool      `json:"favorite,omitempty"`
	HasPhoto      bool      `json:"hasPhoto,omitempty"`
	Created       time.Time `json:"created"`
	Updated       time.Time `json:"updated"`
	LastContacted time.Time `json:"lastContacted,omitempty"`
}

// PhoneLabels, EmailLabels and URLLabels are the choices offered in the editor.
var (
	PhoneLabels   = []string{"Mobile", "Home", "Work", "Main", "WhatsApp", "Fax", "Other"}
	EmailLabels   = []string{"Home", "Work", "Other"}
	URLLabels     = []string{"Website", "LinkedIn", "Instagram", "X", "Other"}
	AddressLabels = []string{"Home", "Work", "Other"}
)

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewContact returns an empty contact with an ID and timestamps.
func NewContact() *Contact {
	now := time.Now()
	return &Contact{ID: newID(), Created: now, Updated: now}
}

// Clone returns a deep copy, so an editor can work on it without touching the original.
func (c *Contact) Clone() *Contact {
	n := *c
	n.Phones = append([]Field(nil), c.Phones...)
	n.Emails = append([]Field(nil), c.Emails...)
	n.URLs = append([]Field(nil), c.URLs...)
	n.Addresses = append([]Address(nil), c.Addresses...)
	n.Groups = append([]string(nil), c.Groups...)
	return &n
}

// FullName is "First Last" (either may be missing).
func (c *Contact) FullName() string {
	return strings.TrimSpace(strings.TrimSpace(c.First) + " " + strings.TrimSpace(c.Last))
}

// DisplayName is what the list shows: the name, else the company, else the first way to reach them.
func (c *Contact) DisplayName() string {
	if n := c.FullName(); n != "" {
		return n
	}
	if c.Nickname != "" {
		return c.Nickname
	}
	if c.Company != "" {
		return c.Company
	}
	if len(c.Emails) > 0 {
		return c.Emails[0].Value
	}
	if len(c.Phones) > 0 {
		return c.Phones[0].Value
	}
	return "No Name"
}

// IsCompany reports whether the card is an organisation rather than a person.
func (c *Contact) IsCompany() bool { return c.FullName() == "" && c.Company != "" }

// Subtitle is the second line in the list: job and company, else the first phone or email.
func (c *Contact) Subtitle() string {
	if c.IsCompany() {
		if len(c.Phones) > 0 {
			return c.Phones[0].Value
		}
		return ""
	}
	if c.Title != "" && c.Company != "" {
		return c.Title + " · " + c.Company
	}
	if c.Company != "" {
		return c.Company
	}
	if c.Title != "" {
		return c.Title
	}
	if len(c.Phones) > 0 {
		return c.Phones[0].Value
	}
	if len(c.Emails) > 0 {
		return c.Emails[0].Value
	}
	return ""
}

// Initials gives up to two letters for the avatar.
func (c *Contact) Initials() string {
	pick := func(s string) string {
		for _, r := range strings.TrimSpace(s) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return string(unicode.ToUpper(r))
			}
		}
		return ""
	}
	if c.FullName() != "" {
		f := pick(c.First)
		for _, r := range f {
			if !unicode.Is(unicode.Latin, r) && !unicode.Is(unicode.Greek, r) && !unicode.Is(unicode.Cyrillic, r) {
				return f
			}
		}
		i := f + pick(c.Last)
		if i != "" {
			return i
		}
	}
	if i := pick(c.DisplayName()); i != "" {
		return i
	}
	return "?"
}

// HasGroup reports membership of a group (case-insensitive).
func (c *Contact) HasGroup(g string) bool {
	for _, x := range c.Groups {
		if strings.EqualFold(x, g) {
			return true
		}
	}
	return false
}

// Tidy trims whitespace, drops empty entries and duplicate groups. Call it before saving.
func (c *Contact) Tidy() {
	c.First = strings.TrimSpace(c.First)
	c.Last = strings.TrimSpace(c.Last)
	c.Nickname = strings.TrimSpace(c.Nickname)
	c.Company = strings.TrimSpace(c.Company)
	c.Title = strings.TrimSpace(c.Title)
	c.Birthday = strings.TrimSpace(c.Birthday)
	c.Notes = strings.TrimRight(c.Notes, " \n\t")
	clean := func(fs []Field, def string) []Field {
		out := []Field{}
		seen := map[string]bool{}
		for _, f := range fs {
			f.Value = strings.TrimSpace(f.Value)
			f.Label = strings.TrimSpace(f.Label)
			if f.Value == "" {
				continue
			}
			k := strings.ToLower(f.Value)
			if seen[k] {
				continue
			}
			seen[k] = true
			if f.Label == "" {
				f.Label = def
			}
			out = append(out, f)
		}
		return out
	}
	c.Phones = clean(c.Phones, "Mobile")
	c.Emails = clean(c.Emails, "Home")
	c.URLs = clean(c.URLs, "Website")
	addrs := []Address{}
	for _, a := range c.Addresses {
		if !a.IsEmpty() {
			if a.Label == "" {
				a.Label = "Home"
			}
			addrs = append(addrs, a)
		}
	}
	c.Addresses = addrs
	groups := []string{}
	for _, g := range c.Groups {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		dup := false
		for _, x := range groups {
			if strings.EqualFold(x, g) {
				dup = true
			}
		}
		if !dup {
			groups = append(groups, g)
		}
	}
	sort.Strings(groups)
	c.Groups = groups
}

// ---------- search ----------

var arabicFold = strings.NewReplacer(
	"أ", "ا", "إ", "ا", "آ", "ا", "ٱ", "ا", "ة", "ه", "ى", "ي", "ؤ", "و", "ئ", "ي", "ـ", "",
)

// fold lower-cases, strips accents and Arabic diacritics, unifies Arabic letter forms
// and turns Arabic-Indic digits into 0-9, so "Jose", "José", "احمد" and "أحمد" all match.
func fold(s string) string {
	s = norm.NFD.String(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r): // accents, Arabic harakat
			continue
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		default:
			b.WriteRune(r)
		}
	}
	return arabicFold.Replace(norm.NFC.String(b.String()))
}

// digitsOnly keeps 0-9 (after folding Arabic digits).
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range fold(s) {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SearchText is everything a search can hit, folded once.
func (c *Contact) SearchText() string {
	parts := []string{c.First, c.Last, c.Nickname, c.Company, c.Title, c.Notes, strings.Join(c.Groups, " ")}
	for _, f := range c.Emails {
		parts = append(parts, f.Value)
	}
	for _, f := range c.URLs {
		parts = append(parts, f.Value)
	}
	for _, a := range c.Addresses {
		parts = append(parts, a.OneLine())
	}
	s := fold(strings.Join(parts, " "))
	for _, f := range c.Phones {
		s += " " + digitsOnly(f.Value)
	}
	return s
}

// Matches reports whether every word of the query appears somewhere in the contact.
// Words made of digits are matched against phone numbers with spaces and dashes ignored.
func (c *Contact) Matches(query string) bool {
	q := strings.Fields(fold(query))
	if len(q) == 0 {
		return true
	}
	hay := c.SearchText()
	for _, w := range q {
		if d := digitsOnly(w); d != "" && len(d) == len(strings.Trim(w, "+-()")) {
			w = d
		}
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// SortKey orders contacts by first or last name; companies sort by company name.
func (c *Contact) SortKey(byLast bool) string {
	var k string
	switch {
	case c.IsCompany():
		k = c.Company
	case byLast && c.Last != "":
		k = c.Last + " " + c.First
	default:
		k = c.DisplayName()
	}
	return fold(k)
}

// SortContacts sorts in place.
func SortContacts(cs []*Contact, byLast bool) {
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i].SortKey(byLast), cs[j].SortKey(byLast)
		if a == b {
			return cs[i].ID < cs[j].ID
		}
		return a < b
	})
}

// IndexLetter is the letter heading a contact falls under ("#" for anything that isn't a letter).
func (c *Contact) IndexLetter(byLast bool) string {
	for _, r := range c.SortKey(byLast) {
		if unicode.IsLetter(r) {
			return string(unicode.ToUpper(r))
		}
		return "#"
	}
	return "#"
}

// ---------- phones ----------

// NormalizePhone turns a number into +<country><number> when it can tell the country,
// using defCC (e.g. "965") for local numbers. It returns digits with a leading + when known.
func NormalizePhone(s, defCC string) string {
	s = strings.TrimSpace(fold(s))
	plus := strings.HasPrefix(s, "+")
	d := digitsOnly(s)
	if d == "" {
		return ""
	}
	switch {
	case plus:
		return "+" + d
	case strings.HasPrefix(d, "00") && len(d) > 8:
		return "+" + d[2:]
	case defCC != "" && strings.HasPrefix(d, defCC) && len(d) > len(defCC)+6:
		return "+" + d
	case defCC != "":
		return "+" + defCC + strings.TrimLeft(d, "0")
	}
	return d
}

// FormatPhone prints a normalised number in a readable way: +965 9999 8888.
// Numbers from other countries keep the country code separate and the rest in groups.
func FormatPhone(s, defCC string) string {
	n := NormalizePhone(s, defCC)
	if !strings.HasPrefix(n, "+") {
		return strings.TrimSpace(s)
	}
	d := n[1:]
	cc := ""
	for _, code := range knownCountryCodes {
		if strings.HasPrefix(d, code) {
			cc = code
			break
		}
	}
	if cc == "" || len(d)-len(cc) < 6 {
		return n
	}
	rest := d[len(cc):]
	var groups []string
	switch {
	case cc == "1" && len(rest) == 10:
		groups = []string{rest[:3], rest[3:6], rest[6:]}
	case len(rest) == 8:
		groups = []string{rest[:4], rest[4:]}
	default:
		for len(rest) > 4 {
			groups = append(groups, rest[:3])
			rest = rest[3:]
		}
		groups = append(groups, rest)
	}
	return "+" + cc + " " + strings.Join(groups, " ")
}

// Longest first so "971" wins over "97".
var knownCountryCodes = []string{
	"965", "966", "971", "973", "974", "968", "962", "961", "963", "964", "967", "970", "972",
	"212", "213", "216", "218", "249", "351", "353", "358", "380", "420", "852", "880", "886",
	"20", "27", "30", "31", "32", "33", "34", "36", "39", "40", "41", "43", "44", "45", "46", "47", "48", "49",
	"51", "52", "53", "54", "55", "56", "57", "58", "60", "61", "62", "63", "64", "65", "66",
	"81", "82", "84", "86", "90", "91", "92", "93", "94", "95", "98",
	"1", "7",
}

// WhatsAppNumber is the digits-only international form wa.me wants.
func WhatsAppNumber(s, defCC string) string {
	return strings.TrimPrefix(NormalizePhone(s, defCC), "+")
}

// ---------- birthdays ----------

// ParseBirthday accepts YYYY-MM-DD, YYYYMMDD, --MM-DD, --MMDD, DD/MM/YYYY and DD.MM.YYYY.
// Apple writes 1604 for an unknown year; that becomes "no year".
// It returns month, day, year (0 = unknown) and ok.
func ParseBirthday(s string) (month, day, year int, ok bool) {
	s = strings.TrimSpace(fold(s))
	if s == "" {
		return 0, 0, 0, false
	}
	if i := strings.IndexAny(s, "T "); i > 0 {
		s = s[:i]
	}
	try := func(layout string, hasYear bool) bool {
		t, err := time.Parse(layout, s)
		if err != nil {
			return false
		}
		month, day = int(t.Month()), t.Day()
		if hasYear && t.Year() != 1604 && t.Year() > 1800 {
			year = t.Year()
		}
		return true
	}
	if strings.HasPrefix(s, "--") {
		r := strings.TrimPrefix(s, "--")
		if len(r) == 4 && !strings.Contains(r, "-") {
			r = r[:2] + "-" + r[2:]
		}
		t, err := time.Parse("2006-01-02", "2000-"+r)
		if err != nil {
			return 0, 0, 0, false
		}
		return int(t.Month()), t.Day(), 0, true
	}
	if t, err := time.Parse("2/1", s); err == nil { // "14/3" — day and month only
		return int(t.Month()), t.Day(), 0, true
	}
	for _, l := range []string{"2006-01-02", "20060102", "02/01/2006", "2/1/2006", "02.01.2006", "2006/01/02"} {
		if try(l, true) {
			return month, day, year, true
		}
	}
	return 0, 0, 0, false
}

// CanonicalBirthday rewrites a birthday as YYYY-MM-DD or --MM-DD.
func CanonicalBirthday(s string) (string, bool) {
	m, d, y, ok := ParseBirthday(s)
	if !ok {
		return "", false
	}
	if y == 0 {
		return fmt.Sprintf("--%02d-%02d", m, d), true
	}
	return fmt.Sprintf("%04d-%02d-%02d", y, m, d), true
}

// NextBirthday returns how many days until the next birthday (0 = today) and the age they turn (0 if the year is unknown).
func (c *Contact) NextBirthday(now time.Time) (days, turns int, ok bool) {
	m, d, y, ok := ParseBirthday(c.Birthday)
	if !ok {
		return 0, 0, false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yr := now.Year()
	next := birthdayIn(yr, m, d)
	if next.Before(today) {
		yr++
		next = birthdayIn(yr, m, d)
	}
	days = int(next.Sub(today).Hours() / 24)
	if y > 0 {
		turns = yr - y
	}
	return days, turns, true
}

// birthdayIn handles 29 February in years that don't have it (celebrated on 28 February).
func birthdayIn(year, m, d int) time.Time {
	t := time.Date(year, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Month() != time.Month(m) {
		t = time.Date(year, time.Month(m), 28, 0, 0, 0, 0, time.UTC)
	}
	return t
}

// BirthdayText renders a birthday for people: "14 March 1988" or "14 March".
func BirthdayText(s string) string {
	m, d, y, ok := ParseBirthday(s)
	if !ok {
		return s
	}
	out := fmt.Sprintf("%d %s", d, time.Month(m).String())
	if y > 0 {
		out += fmt.Sprintf(" %d", y)
	}
	return out
}

// Ago renders a past time roughly: "today", "yesterday", "5 days ago", "3 months ago".
func Ago(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	days := int(now.Sub(t).Hours() / 24)
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days < 30:
		return fmt.Sprintf("%d days ago", days)
	case days < 60:
		return "a month ago"
	case days < 365:
		return fmt.Sprintf("%d months ago", days/30)
	case days < 730:
		return "a year ago"
	}
	return fmt.Sprintf("%d years ago", days/365)
}
