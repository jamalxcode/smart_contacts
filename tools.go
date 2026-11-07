package main

import (
	"encoding/csv"
	"io"
	"sort"
	"strings"
	"unicode"
)

// ---------- CSV export ----------

// WriteCSV writes a spreadsheet-friendly export (Numbers, Excel, Google Sheets).
func WriteCSV(w io.Writer, cs []*Contact) error {
	// A UTF-8 byte-order mark makes Excel read Arabic names correctly.
	if _, err := io.WriteString(w, "\uFEFF"); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	head := []string{"First Name", "Last Name", "Nickname", "Company", "Job Title",
		"Phone 1 Label", "Phone 1", "Phone 2 Label", "Phone 2", "Phone 3 Label", "Phone 3",
		"Email 1", "Email 2", "Website", "Address", "Birthday", "Groups", "Favorite", "Notes"}
	if err := cw.Write(head); err != nil {
		return err
	}
	get := func(fs []Field, i int) (string, string) {
		if i < len(fs) {
			return fs[i].Label, fs[i].Value
		}
		return "", ""
	}
	for _, c := range cs {
		row := []string{c.First, c.Last, c.Nickname, c.Company, c.Title}
		for i := 0; i < 3; i++ {
			l, v := get(c.Phones, i)
			row = append(row, l, v)
		}
		_, e1 := get(c.Emails, 0)
		_, e2 := get(c.Emails, 1)
		_, u := get(c.URLs, 0)
		addr := ""
		if len(c.Addresses) > 0 {
			addr = c.Addresses[0].OneLine()
		}
		fav := ""
		if c.Favorite {
			fav = "yes"
		}
		row = append(row, e1, e2, u, addr, c.Birthday, strings.Join(c.Groups, "; "), fav, c.Notes)
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// ---------- duplicates ----------

// DupeGroup is a set of contacts that look like the same person, and why.
type DupeGroup struct {
	Contacts []*Contact
	Reason   string
}

// FindDuplicates groups contacts sharing a phone number, an email address or the exact same name.
func FindDuplicates(cs []*Contact, defCC string) []DupeGroup {
	parent := make([]int, len(cs))
	reason := make([]string, len(cs))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int, why string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
			if reason[ra] == "" {
				reason[ra] = why
			}
		}
	}
	first := map[string]int{}
	link := func(key string, i int, why string) {
		if key == "" {
			return
		}
		if j, ok := first[key]; ok {
			union(j, i, why)
		} else {
			first[key] = i
		}
	}
	for i, c := range cs {
		for _, p := range c.Phones {
			n := NormalizePhone(p.Value, defCC)
			if len(digitsOnly(n)) >= 7 {
				link("p:"+n, i, "same phone number")
			}
		}
		for _, e := range c.Emails {
			link("e:"+strings.ToLower(strings.TrimSpace(e.Value)), i, "same email")
		}
		if n := fold(c.FullName()); len([]rune(n)) >= 3 {
			link("n:"+n, i, "same name")
		}
	}
	groups := map[int][]*Contact{}
	for i, c := range cs {
		r := find(i)
		groups[r] = append(groups[r], c)
	}
	var out []DupeGroup
	for r, g := range groups {
		if len(g) > 1 {
			out = append(out, DupeGroup{Contacts: g, Reason: reason[r]})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return fold(out[i].Contacts[0].DisplayName()) < fold(out[j].Contacts[0].DisplayName())
	})
	return out
}

// Merge folds the others into keep: blanks are filled, lists combined without repeats,
// notes appended. keep's own values win where both have one.
func Merge(keep *Contact, others []*Contact, defCC string) {
	for _, o := range others {
		fill := func(dst *string, src string) {
			if strings.TrimSpace(*dst) == "" {
				*dst = src
			}
		}
		fill(&keep.First, o.First)
		fill(&keep.Last, o.Last)
		fill(&keep.Nickname, o.Nickname)
		fill(&keep.Company, o.Company)
		fill(&keep.Title, o.Title)
		fill(&keep.Birthday, o.Birthday)
		keep.Phones = mergeFields(keep.Phones, o.Phones, func(s string) string { return NormalizePhone(s, defCC) })
		keep.Emails = mergeFields(keep.Emails, o.Emails, strings.ToLower)
		keep.URLs = mergeFields(keep.URLs, o.URLs, strings.ToLower)
		for _, a := range o.Addresses {
			dup := false
			for _, b := range keep.Addresses {
				if fold(a.OneLine()) == fold(b.OneLine()) {
					dup = true
				}
			}
			if !dup {
				keep.Addresses = append(keep.Addresses, a)
			}
		}
		if n := strings.TrimSpace(o.Notes); n != "" && !strings.Contains(keep.Notes, n) {
			if keep.Notes != "" {
				keep.Notes += "\n\n"
			}
			keep.Notes += n
		}
		keep.Groups = append(keep.Groups, o.Groups...)
		keep.Favorite = keep.Favorite || o.Favorite
		if o.Created.Before(keep.Created) && !o.Created.IsZero() {
			keep.Created = o.Created
		}
		if o.LastContacted.After(keep.LastContacted) {
			keep.LastContacted = o.LastContacted
		}
	}
	keep.Tidy()
}

func mergeFields(a, b []Field, key func(string) string) []Field {
	seen := map[string]bool{}
	for _, f := range a {
		seen[key(f.Value)] = true
	}
	for _, f := range b {
		k := key(f.Value)
		if !seen[k] {
			seen[k] = true
			a = append(a, f)
		}
	}
	return a
}

// ---------- clean up ----------

// CleanupReport lists what Clean Up would change, so the user can see it before applying.
type CleanupChange struct {
	Contact *Contact
	What    string
	apply   func()
}

// PlanCleanup proposes: phones in one international format, names with sensible capitals
// (only when typed all upper or all lower case), emails in lower case, links with https.
func PlanCleanup(cs []*Contact, defCC string) []CleanupChange {
	var out []CleanupChange
	for _, c := range cs {
		c := c
		for i, p := range c.Phones {
			i := i
			f := FormatPhone(p.Value, defCC)
			if f != p.Value && len(digitsOnly(p.Value)) >= 7 {
				out = append(out, CleanupChange{c, p.Value + "  →  " + f, func() { c.Phones[i].Value = f }})
			}
		}
		for _, fld := range []*string{&c.First, &c.Last} {
			fld := fld
			if n := fixCase(*fld); n != *fld {
				out = append(out, CleanupChange{c, *fld + "  →  " + n, func() { *fld = n }})
			}
		}
		for i, e := range c.Emails {
			i := i
			if l := strings.ToLower(strings.TrimSpace(e.Value)); l != e.Value {
				out = append(out, CleanupChange{c, e.Value + "  →  " + l, func() { c.Emails[i].Value = l }})
			}
		}
		for i, u := range c.URLs {
			i := i
			v := strings.TrimSpace(u.Value)
			if v != "" && !strings.Contains(v, "://") {
				n := "https://" + v
				out = append(out, CleanupChange{c, v + "  →  " + n, func() { c.URLs[i].Value = n }})
			}
		}
	}
	return out
}

// Apply makes the change.
func (ch CleanupChange) Apply() { ch.apply() }

// fixCase capitalises names typed ALL CAPS or all lower case; mixed case is left alone
// (McDonald, al-Sabah, van der Berg are the person's own choice).
func fixCase(s string) string {
	letters, upper, lower := 0, 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) && unicode.Is(unicode.Latin, r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			} else if unicode.IsLower(r) {
				lower++
			}
		}
	}
	if letters < 2 || (upper != letters && lower != letters) {
		return s
	}
	rs := []rune(strings.ToLower(s))
	start := true
	for i, r := range rs {
		if start && unicode.IsLetter(r) {
			rs[i] = unicode.ToUpper(r)
		}
		start = r == ' ' || r == '-' || r == '\''
	}
	return string(rs)
}
