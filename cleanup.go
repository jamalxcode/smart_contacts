package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Kinds of Clean Up suggestion, in the order the screen shows them.
const (
	ckFix       = iota // a confident correction (00 → +, add +965, email typo…)
	ckDuplicate        // the same number or email twice on one card
	ckWrong            // a number or email that looks wrong; removing it is suggested
	ckUnsure           // a number whose country can't be told; the user picks
)

// CleanupChange is one suggestion. Apply makes it; for ckUnsure, choice is the picked option.
type CleanupChange struct {
	Contact *Contact
	Kind    int
	Field   string // "phone", "email", "name", "link"
	Old     string
	New     string   // empty for a removal
	Reason  string   // shown under the suggestion
	Options []string // ckUnsure: candidate numbers
	Default bool     // ticked when the screen opens
	apply   func(choice string)
}

// Apply makes the change (choice is only used by ckUnsure suggestions).
func (ch CleanupChange) Apply(choice string) { ch.apply(choice) }

// What renders the change on one line.
func (ch CleanupChange) What() string {
	if ch.New == "" {
		return "remove " + ch.Old
	}
	return ch.Old + "  →  " + ch.New
}

func setField(fs []Field, old, val string) {
	for i := range fs {
		if fs[i].Value == old {
			fs[i].Value = val
			return
		}
	}
}

func dropField(fs []Field, old string) []Field {
	for i := range fs {
		if fs[i].Value == old {
			return append(fs[:i], fs[i+1:]...)
		}
	}
	return fs
}

// dropLaterDup removes every field after the first whose key matches k. Matching by key
// (not by text) still works after another suggestion has reformatted one of the copies.
func dropLaterDup(fs []Field, k string, key func(string) string) []Field {
	out, seen := fs[:0], false
	for _, f := range fs {
		if key(f.Value) == k {
			if seen {
				continue
			}
			seen = true
		}
		out = append(out, f)
	}
	return out
}

// PlanCleanup looks at every contact and proposes fixes. Nothing changes until Apply.
func PlanCleanup(cs []*Contact, home string) []CleanupChange {
	var out []CleanupChange
	for _, c := range cs {
		c := c
		hints := cardHints(c, home)
		phoneKey := func(v string) string { return analyzePhone(v, home, hints).Norm }

		// values in the wrong field: an email typed as a phone, a number typed as an email
		for _, p := range c.Phones {
			if m := reEmailInText.FindString(p.Value); m != "" && digitsOnly(p.Value) == digitsOnly(m) {
				v, em := p.Value, strings.ToLower(m)
				out = append(out, CleanupChange{Contact: c, Kind: ckFix, Field: "phone", Old: v, New: em,
					Reason: "an email in the phone field — move it to Email", Default: true,
					apply: func(string) {
						c.Phones = dropField(c.Phones, v)
						c.Emails = append(c.Emails, Field{Label: "Other", Value: em})
					}})
			}
		}
		for _, e := range c.Emails {
			if !strings.Contains(e.Value, "@") && len(digitsOnly(e.Value)) >= 7 {
				v := e.Value
				out = append(out, CleanupChange{Contact: c, Kind: ckFix, Field: "email", Old: v, New: v,
					Reason: "a phone number in the email field — move it to Phone", Default: true,
					apply: func(string) {
						c.Emails = dropField(c.Emails, v)
						c.Phones = append(c.Phones, Field{Label: "Mobile", Value: FormatPhone(v, home)})
					}})
			}
		}

		// phones
		seen := map[string]string{}
		for _, p := range c.Phones {
			if reEmailInText.MatchString(p.Value) {
				continue
			}
			v := p.Value
			r := analyzePhone(v, home, hints)
			switch r.Status {
			case phOK:
				pretty := FormatPhone(r.Norm, home)
				if pretty != v {
					why := "written in one international format"
					switch {
					case strings.HasPrefix(digitsOnly(v), "00"):
						why = "00 replaced with +"
					case r.Reason != "":
						why = r.Reason
					case !strings.Contains(v, "+"):
						why = "country code added (" + countryName(r.Norm) + ")"
					}
					out = append(out, CleanupChange{Contact: c, Kind: ckFix, Field: "phone", Old: v, New: pretty,
						Reason: why, Default: true, apply: func(string) { setField(c.Phones, v, pretty) }})
				}
			case phInvalid, phSuspicious:
				out = append(out, CleanupChange{Contact: c, Kind: ckWrong, Field: "phone", Old: v,
					Reason: "looks wrong: " + r.Reason, Default: r.Status == phInvalid,
					apply: func(string) { c.Phones = dropField(c.Phones, v) }})
			case phUnsure:
				opts := append([]string(nil), r.Options...)
				out = append(out, CleanupChange{Contact: c, Kind: ckUnsure, Field: "phone", Old: v, Options: opts,
					Reason: r.Reason,
					apply: func(choice string) {
						if choice != "" {
							setField(c.Phones, v, FormatPhone(choice, home))
						}
					}})
			}
			k := r.Norm
			if k == "" || r.Status == phInvalid {
				continue
			}
			if first, ok := seen[k]; ok {
				out = append(out, CleanupChange{Contact: c, Kind: ckDuplicate, Field: "phone", Old: v,
					Reason: "same number as " + first, Default: true,
					apply: func(string) { c.Phones = dropLaterDup(c.Phones, k, phoneKey) }})
			} else {
				seen[k] = v
			}
		}

		// emails
		seenE := map[string]string{}
		emailKey := func(v string) string { f, _, _ := fixEmail(v); return f }
		for _, e := range c.Emails {
			if !strings.Contains(e.Value, "@") && len(digitsOnly(e.Value)) >= 7 {
				continue // handled above (moved to Phone)
			}
			v := e.Value
			fixed, why, valid := fixEmail(v)
			switch {
			case !valid:
				out = append(out, CleanupChange{Contact: c, Kind: ckWrong, Field: "email", Old: v,
					Reason: "not a valid email address", Default: !strings.Contains(v, "@"),
					apply: func(string) { c.Emails = dropField(c.Emails, v) }})
			case fixed != v:
				out = append(out, CleanupChange{Contact: c, Kind: ckFix, Field: "email", Old: v, New: fixed,
					Reason: why, Default: true, apply: func(string) { setField(c.Emails, v, fixed) }})
			}
			if !valid {
				continue
			}
			if first, ok := seenE[fixed]; ok {
				out = append(out, CleanupChange{Contact: c, Kind: ckDuplicate, Field: "email", Old: v,
					Reason: "same email as " + first, Default: true,
					apply: func(string) { c.Emails = dropLaterDup(c.Emails, fixed, emailKey) }})
			} else {
				seenE[fixed] = v
			}
		}

		// repeated links (same profile or page) and addresses on one card
		seenU := map[string]string{}
		urlKey := func(v string) string { // the label matters: "@khalid_kw" under Telegram = t.me/khalid_kw
			for _, f := range c.URLs {
				if f.Value == v {
					return linkKey(f.Label, v)
				}
			}
			return linkKey("", v)
		}
		for _, f := range c.URLs {
			k := linkKey(f.Label, f.Value)
			if k == "" {
				continue
			}
			if first, ok := seenU[k]; ok {
				kk := k
				out = append(out, CleanupChange{Contact: c, Kind: ckDuplicate, Field: "link", Old: f.Value,
					Reason: "same link as " + first, Default: true,
					apply: func(string) { c.URLs = dropLaterDup(c.URLs, kk, urlKey) }})
			} else {
				seenU[k] = f.Value
			}
		}
		seenA := map[string]bool{}
		for _, a := range c.Addresses {
			k := fold(a.OneLine())
			if seenA[k] {
				kk := k
				out = append(out, CleanupChange{Contact: c, Kind: ckDuplicate, Field: "address", Old: a.OneLine(),
					Reason: "same address listed twice", Default: true,
					apply: func(string) { c.Addresses = dropLaterAddr(c.Addresses, kk) }})
			}
			seenA[k] = true
		}

		// notes: repeats, blank-line runs, leftover HTML, lines that only repeat a number or email
		if tidy, why, onlyRepeats := TidyNotes(c); tidy != c.Notes {
			old, n := c.Notes, tidy
			kind := ckFix
			if onlyRepeats {
				kind = ckDuplicate
			}
			out = append(out, CleanupChange{Contact: c, Kind: kind, Field: "notes", Old: "notes", New: "tidied notes",
				Reason: why, Default: true, apply: func(string) {
					if c.Notes == old {
						c.Notes = n
					}
				}})
		}

		// names typed ALL CAPS / all lower case
		for _, fld := range []*string{&c.First, &c.Last} {
			fld := fld
			if n := fixCase(*fld); n != *fld {
				old := *fld
				out = append(out, CleanupChange{Contact: c, Kind: ckFix, Field: "name", Old: old, New: n,
					Reason: "capitals tidied", Default: true, apply: func(string) {
						if *fld == old {
							*fld = n
						}
					}})
			}
		}

		// links without https://
		for _, u := range c.URLs {
			v := strings.TrimSpace(u.Value)
			if v != "" && !strings.Contains(v, "://") {
				old, n := u.Value, "https://"+v
				out = append(out, CleanupChange{Contact: c, Kind: ckFix, Field: "link", Old: old, New: n,
					Reason: "https:// added", Default: true, apply: func(string) { setField(c.URLs, old, n) }})
			}
		}
	}
	return out
}

// linkKey compares links: a social profile by account, any other page without https/www/slash.
func linkKey(label, v string) string {
	if p, ok := parseProfile(label, v); ok && p.Handle != "" {
		return strings.ToLower(p.Service + ":" + p.Handle)
	}
	l := strings.ToLower(strings.TrimSpace(v))
	for _, p := range []string{"https://", "http://", "www."} {
		l = strings.TrimPrefix(l, p)
	}
	return strings.TrimRight(l, "/")
}

func dropLaterAddr(as []Address, k string) []Address {
	out, seen := as[:0], false
	for _, a := range as {
		if fold(a.OneLine()) == k {
			if seen {
				continue
			}
			seen = true
		}
		out = append(out, a)
	}
	return out
}

var (
	reHTMLBreak = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</li>`)
	reHTMLTag   = regexp.MustCompile(`<[^>]{1,200}>`)
	htmlEntity  = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"", "&#39;", "'", "&apos;", "'", "&nbsp;", " ")
	reNoteLabel = regexp.MustCompile(`(?i)^\s*(mobile|mob|phone|tel|cell|work|home|whatsapp|m|t|p|e|email|e-mail|mail|جوال|هاتف|ايميل)\s*[:.\-]?\s*`)
)

// TidyNotes cleans a contact's notes: literal "\n" codes and HTML from exports become real text,
// trailing spaces and runs of blank lines go, repeated lines (common after merging sources) are
// kept once, and lines that only repeat a phone number or email already on the card are removed.
// It returns the tidy notes, a short description, and whether only repeats were removed.
func TidyNotes(c *Contact) (string, string, bool) {
	orig := c.Notes
	s := strings.ReplaceAll(orig, "\r\n", "\n")
	var whys []string
	onlyRepeats := true
	if strings.Contains(s, `\n`) {
		s = strings.ReplaceAll(s, `\n`, "\n")
		whys = append(whys, "turned \\n codes into line breaks")
		onlyRepeats = false
	}
	if reHTMLTag.MatchString(s) || strings.Contains(s, "&amp;") || strings.Contains(s, "&nbsp;") || strings.Contains(s, "&quot;") {
		s = reHTMLBreak.ReplaceAllString(s, "\n")
		s = reHTMLTag.ReplaceAllString(s, "")
		s = htmlEntity.Replace(s)
		whys = append(whys, "removed HTML")
		onlyRepeats = false
	}
	phones := map[string]bool{}
	for _, p := range c.Phones {
		if d := digitsOnly(NormalizePhone(p.Value, "")); len(d) >= 7 {
			phones[d[len(d)-7:]] = true
		}
	}
	emails := map[string]bool{}
	for _, e := range c.Emails {
		emails[strings.ToLower(strings.TrimSpace(e.Value))] = true
	}
	var lines []string
	seen := map[string]bool{}
	repeats, echoes, blank := 0, 0, false
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if len(lines) > 0 && !blank {
				lines = append(lines, "")
			}
			blank = true
			continue
		}
		k := fold(strings.Join(strings.Fields(line), " "))
		if seen[k] {
			repeats++
			continue
		}
		seen[k] = true
		// a line that is just "Mobile: +965 6000 1234" or an email already on the card
		rest := strings.TrimSpace(reNoteLabel.ReplaceAllString(line, ""))
		if d := digitsOnly(rest); len(d) >= 7 && !hasLetters(rest) && phones[d[len(d)-7:]] {
			echoes++
			continue
		}
		if emails[strings.ToLower(rest)] {
			echoes++
			continue
		}
		lines = append(lines, line)
		blank = false
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	out := strings.Join(lines, "\n")
	if repeats > 0 {
		whys = append(whys, plural(repeats, "repeated line")+" removed")
	}
	if echoes > 0 {
		whys = append(whys, plural(echoes, "line")+" that only repeat a number or email on the card")
		onlyRepeats = false
	}
	if out != orig && len(whys) == 0 {
		whys = append(whys, "spacing tidied")
		onlyRepeats = false
	}
	return out, strings.Join(whys, ", "), onlyRepeats && repeats > 0
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
