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
		"Email 1", "Email 2", "Website", "Address", "Birthday", "Groups", "Favorite", "Notes", "Name (other language)", "Sources"}
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
		row = append(row, e1, e2, u, addr, c.Birthday, strings.Join(c.Groups, "; "), fav, c.Notes, c.AltName, strings.Join(c.Sources, "; "))
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

type dupKey struct {
	key, why string
	nameOnly bool
}

// dupKeys lists what can tie a contact to another card: each phone (7+ digits, normalised),
// each email, and the full name.
func dupKeys(c *Contact, defCC string) []dupKey {
	var ks []dupKey
	for _, p := range c.Phones {
		if n := NormalizePhone(p.Value, defCC); len(digitsOnly(n)) >= 7 {
			ks = append(ks, dupKey{"p:" + n, "same phone number", false})
		}
	}
	for _, e := range c.Emails {
		if v := strings.ToLower(strings.TrimSpace(e.Value)); v != "" {
			ks = append(ks, dupKey{"e:" + v, "same email", false})
		}
	}
	for _, u := range c.URLs { // a LinkedIn / Instagram / X profile identifies the person
		if k := profileKey(u.Value); k != "" {
			ks = append(ks, dupKey{"u:" + k, "same profile link", false})
		}
	}
	for _, s := range c.nameSkeletons() { // "Mohammed Al-Mutairi" = "محمد المطيري"
		ks = append(ks, dupKey{"n:" + s, "same name", true})
	}
	return ks
}

func reachable(c *Contact) bool { return len(c.Phones)+len(c.Emails) > 0 }

// profileKey turns a social profile link into a stable key: "linkedin.com/in/khalidm".
func profileKey(u string) string {
	l := strings.ToLower(strings.TrimSpace(u))
	for _, p := range []string{"https://", "http://", "www.", "m.", "mobile."} {
		l = strings.TrimPrefix(l, p)
	}
	l = strings.TrimRight(strings.SplitN(strings.SplitN(l, "?", 2)[0], "#", 2)[0], "/")
	for _, site := range []string{"linkedin.com/in/", "instagram.com/", "x.com/", "twitter.com/", "facebook.com/", "github.com/"} {
		if strings.HasPrefix(l, site) && len(l) > len(site) {
			return l
		}
	}
	return ""
}

// namesCompatible reports whether two cards could be the same person: either has no name,
// one name contains the other ("Khalid" / "Khalid Al-Mutairi"), or the first names match.
// "Ali Hassan" and "Noor Hassan" sharing the family's home phone are not the same person.
func namesCompatible(a, b *Contact) bool {
	na, nb := fold(a.FullName()), fold(b.FullName())
	ca, cb := fold(a.Company), fold(b.Company)
	switch {
	case na == "" && nb == "": // two company cards (or two bare numbers)
		return ca == "" || cb == "" || ca == cb
	case na == "" || nb == "":
		// A bare card (just a number or email) can be anyone; a company card is not a person,
		// even when an employee lists the company's main line.
		bare := a
		if nb == "" {
			bare = b
		}
		return fold(bare.Company) == "" // a nickname-only card ("Abu Ali") may well be them
	}
	if na == nb || strings.Contains(na, nb) || strings.Contains(nb, na) || sameNameAcrossScripts(a, b) {
		return true
	}
	return strings.Fields(na)[0] == strings.Fields(nb)[0]
}

// sameEnough decides whether a shared key really ties two cards together.
// A shared phone or email needs compatible names. A shared name alone only counts when
// one card has no phone and no email; two "Mohammed Ali"s with different numbers are
// two people.
func sameEnough(a, b *Contact, k dupKey) bool {
	if k.nameOnly {
		return !(reachable(a) && reachable(b))
	}
	return namesCompatible(a, b)
}

// FindDuplicates groups contacts that look like the same person (see sameEnough).
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
	owners := map[string][]int{}
	for i, c := range cs {
		for _, k := range dupKeys(c, defCC) {
			for _, j := range owners[k.key] {
				if sameEnough(cs[j], c, k) {
					union(j, i, k.why)
					break
				}
			}
			owners[k.key] = append(owners[k.key], i)
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

// DupIndex answers "is this card already in the address book?" in one lookup per key,
// so importing 2,000 cards into 2,000 contacts doesn't compare every pair.
type DupIndex struct {
	defCC string
	by    map[string][]*Contact
}

// NewDupIndex indexes cs.
func NewDupIndex(cs []*Contact, defCC string) *DupIndex {
	x := &DupIndex{defCC: defCC, by: map[string][]*Contact{}}
	for _, c := range cs {
		x.Add(c)
	}
	return x
}

// Add puts c in the index.
func (x *DupIndex) Add(c *Contact) {
	for _, k := range dupKeys(c, x.defCC) {
		x.by[k.key] = append(x.by[k.key], c)
	}
}

// Find returns an indexed contact that is the same person as c, or nil.
func (x *DupIndex) Find(c *Contact) *Contact {
	for _, k := range dupKeys(c, x.defCC) {
		for _, o := range x.by[k.key] {
			if o.ID != c.ID && sameEnough(o, c, k) {
				return o
			}
		}
	}
	return nil
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
		// a name in the other language becomes the second name instead of being lost
		for _, n := range []string{o.FullName(), o.AltName} {
			if n == "" || fold(n) == fold(keep.FullName()) || fold(n) == fold(keep.AltName) {
				continue
			}
			if keep.AltName == "" && hasArabic(n) != hasArabic(keep.FullName()) {
				keep.AltName = n
			}
		}
		keep.Sources = append(keep.Sources, o.Sources...)
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
