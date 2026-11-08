package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// Card is a parsed vCard: the contact and its photo bytes, if any.
type Card struct {
	Contact *Contact
	Photo   []byte
}

type vline struct {
	group  string
	name   string
	params map[string][]string
	value  string
}

// ParseVCards reads every BEGIN:VCARD…END:VCARD block (versions 2.1, 3.0 and 4.0, as written
// by Apple Contacts, Google and Outlook).
func ParseVCards(r io.Reader) ([]Card, error) {
	lines, err := unfold(r)
	if err != nil {
		return nil, err
	}
	var out []Card
	var cur []vline
	in := false
	for _, raw := range lines {
		l, ok := parseLine(raw)
		if !ok {
			continue
		}
		switch {
		case l.name == "BEGIN" && strings.EqualFold(l.value, "VCARD"):
			in, cur = true, nil
		case l.name == "END" && strings.EqualFold(l.value, "VCARD"):
			if in {
				if c := buildCard(cur); c != nil {
					out = append(out, *c)
				}
			}
			in = false
		case in:
			cur = append(cur, l)
		}
	}
	return out, nil
}

// unfold joins continuation lines (a line starting with a space or tab continues the one before;
// in quoted-printable values a trailing "=" does too).
func unfold(r io.Reader) ([]string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	var out []string
	qpCont := false
	for sc.Scan() {
		t := strings.TrimRight(sc.Text(), "\r")
		t = strings.TrimPrefix(t, "\uFEFF")
		switch {
		case len(out) > 0 && qpCont:
			out[len(out)-1] = strings.TrimSuffix(out[len(out)-1], "=") + t
		case len(out) > 0 && (strings.HasPrefix(t, " ") || strings.HasPrefix(t, "\t")):
			out[len(out)-1] += t[1:]
		default:
			out = append(out, t)
		}
		last := out[len(out)-1]
		qpCont = strings.Contains(strings.ToUpper(last), "QUOTED-PRINTABLE") && strings.HasSuffix(last, "=")
	}
	return out, sc.Err()
}

func parseLine(s string) (vline, bool) {
	// The value starts at the first ':' that isn't inside a quoted parameter.
	inQ := false
	colon := -1
	for i, r := range s {
		if r == '"' {
			inQ = !inQ
		}
		if r == ':' && !inQ {
			colon = i
			break
		}
	}
	if colon < 0 {
		return vline{}, false
	}
	head, val := s[:colon], s[colon+1:]
	parts := splitUnquoted(head, ';')
	l := vline{params: map[string][]string{}, value: val}
	name := parts[0]
	if i := strings.LastIndex(name, "."); i >= 0 {
		l.group, name = strings.ToLower(name[:i]), name[i+1:]
	}
	l.name = strings.ToUpper(name)
	for _, p := range parts[1:] {
		k, v, ok := strings.Cut(p, "=")
		if !ok { // vCard 2.1 bare types: TEL;CELL;PREF:...
			k, v = "TYPE", p
		}
		k = strings.ToUpper(k)
		for _, x := range strings.Split(strings.Trim(v, `"`), ",") {
			if x != "" {
				l.params[k] = append(l.params[k], x)
			}
		}
	}
	if hasParam(l, "ENCODING", "QUOTED-PRINTABLE") {
		l.value = decodeQP(l.value)
	}
	return l, true
}

func splitUnquoted(s string, sep rune) []string {
	var out []string
	inQ := false
	start := 0
	for i, r := range s {
		if r == '"' {
			inQ = !inQ
		}
		if r == sep && !inQ {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func hasParam(l vline, key, val string) bool {
	for _, v := range l.params[key] {
		if strings.EqualFold(v, val) {
			return true
		}
	}
	return false
}

func decodeQP(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '=' && i+2 < len(s) {
			var x byte
			if _, err := fmt.Sscanf(s[i+1:i+3], "%02X", &x); err == nil {
				b = append(b, x)
				i += 2
				continue
			}
		}
		b = append(b, s[i])
	}
	return string(b)
}

// splitValue splits a structured value on unescaped sep and unescapes each part.
func splitValue(v string, sep byte) []string {
	var out []string
	var cur strings.Builder
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '\\' && i+1 < len(v) {
			cur.WriteByte('\\')
			cur.WriteByte(v[i+1])
			i++
			continue
		}
		if c == sep {
			out = append(out, unescape(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	return append(out, unescape(cur.String()))
}

func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n', 'N':
				b.WriteByte('\n')
			default:
				b.WriteByte(s[i+1])
			}
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

var appleLabel = regexp.MustCompile(`^_\$!<(.*)>!\$_$`)

// labelFor turns vCard TYPE params (or an Apple X-ABLabel) into a friendly label.
func labelFor(l vline, custom map[string]string, kind string) string {
	if lab, ok := custom[l.group]; ok && l.group != "" {
		if m := appleLabel.FindStringSubmatch(lab); m != nil {
			lab = m[1]
		}
		return niceLabel(lab, kind)
	}
	for _, t := range l.params["TYPE"] {
		switch strings.ToLower(t) {
		case "cell", "mobile", "iphone":
			return "Mobile"
		case "home":
			return "Home"
		case "work":
			return "Work"
		case "main", "pref", "voice", "internet", "x400":
			continue
		case "fax":
			return "Fax"
		case "pager":
			return "Pager"
		case "other":
			return "Other"
		default:
			return niceLabel(t, kind)
		}
	}
	for _, t := range l.params["TYPE"] {
		if strings.EqualFold(t, "main") {
			return "Main"
		}
	}
	switch kind {
	case "tel":
		return "Mobile"
	case "url":
		return "Website"
	}
	return "Other"
}

func niceLabel(s, kind string) string {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "":
		return "Other"
	case "homepage", "home page":
		return "Website"
	case "iphone", "cell":
		return "Mobile"
	}
	if kind == "url" && strings.EqualFold(s, "home") {
		return "Website"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func buildCard(lines []vline) *Card {
	c := NewContact()
	var photo []byte
	custom := map[string]string{}
	for _, l := range lines {
		if l.name == "X-ABLABEL" {
			custom[l.group] = unescape(l.value)
		}
	}
	fn := ""
	var ext []string
	for _, l := range lines {
		switch l.name {
		case "N":
			p := splitValue(l.value, ';')
			if len(p) > 0 {
				c.Last = p[0]
			}
			if len(p) > 1 {
				c.First = p[1]
			}
			if len(p) > 2 && p[2] != "" {
				c.First = strings.TrimSpace(c.First + " " + p[2])
			}
		case "FN":
			fn = unescape(l.value)
		case "UID", "X-ABUID":
			if v := strings.TrimSpace(strings.TrimSuffix(unescape(l.value), ":ABPerson")); v != "" {
				ext = append(ext, v)
			}
		case "X-ALT-NAME":
			c.AltName = unescape(l.value)
		case "NICKNAME":
			c.Nickname = unescape(l.value)
		case "ORG":
			p := splitValue(l.value, ';')
			c.Company = strings.TrimSpace(strings.Join(nonEmpty(p), ", "))
		case "TITLE":
			c.Title = unescape(l.value)
		case "TEL":
			v := strings.TrimPrefix(unescape(l.value), "tel:")
			c.Phones = append(c.Phones, Field{Label: labelFor(l, custom, "tel"), Value: v})
		case "EMAIL":
			c.Emails = append(c.Emails, Field{Label: labelFor(l, custom, "email"), Value: unescape(l.value)})
		case "URL":
			c.URLs = append(c.URLs, Field{Label: labelFor(l, custom, "url"), Value: unescape(l.value)})
		case "X-SOCIALPROFILE":
			v := unescape(l.value)
			lab := "Other"
			if t := l.params["TYPE"]; len(t) > 0 {
				lab = niceLabel(t[0], "url")
			}
			if strings.HasPrefix(v, "http") {
				c.URLs = append(c.URLs, Field{Label: lab, Value: v})
			}
		case "ADR":
			p := splitValue(l.value, ';')
			for len(p) < 7 {
				p = append(p, "")
			}
			street := strings.TrimSpace(strings.Join(nonEmpty([]string{p[0], p[1], p[2]}), "\n"))
			c.Addresses = append(c.Addresses, Address{
				Label: labelFor(l, custom, "adr"), Street: street,
				City: p[3], Region: p[4], Postcode: p[5], Country: p[6],
			})
		case "BDAY":
			if b, ok := CanonicalBirthday(unescape(l.value)); ok {
				c.Birthday = b
			}
		case "NOTE":
			c.Notes = unescape(l.value)
		case "CATEGORIES":
			for _, g := range splitValue(l.value, ',') {
				if g = strings.TrimSpace(g); g != "" && !strings.EqualFold(g, "myContacts") && !strings.EqualFold(g, "starred") {
					c.Groups = append(c.Groups, g)
				} else if strings.EqualFold(g, "starred") {
					c.Favorite = true
				}
			}
		case "PHOTO":
			photo = decodePhoto(l)
		case "REV":
			for _, layout := range []string{"2006-01-02T15:04:05Z", "20060102T150405Z", "2006-01-02T15:04:05.000Z", "2006-01-02"} {
				if t, err := time.Parse(layout, l.value); err == nil {
					c.Updated, c.Created = t, t
					break
				}
			}
		}
	}
	for _, e := range ext {
		c.SourceIDs = append(c.SourceIDs, "|"+e) // the source name is filled in when imported
	}
	if c.FullName() == "" && fn != "" && fn != c.Company {
		c.First = fn
	}
	c.Tidy()
	if c.DisplayName() == "No Name" && len(c.Phones) == 0 && len(c.Emails) == 0 {
		return nil
	}
	return &Card{Contact: c, Photo: photo}
}

func nonEmpty(ss []string) []string {
	var out []string
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func decodePhoto(l vline) []byte {
	v := strings.TrimSpace(l.value)
	if strings.HasPrefix(v, "data:") { // vCard 4: data:image/jpeg;base64,....
		if i := strings.Index(v, ","); i >= 0 {
			v = v[i+1:]
		}
	} else if strings.HasPrefix(v, "http") {
		return nil
	}
	v = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, v)
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		b, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(v, "="))
		if err != nil {
			return nil
		}
	}
	return b
}

// ---------- writing ----------

func escape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`, ",", `\,`, ";", `\;`)
	return r.Replace(strings.ReplaceAll(s, "\r", ""))
}

// fold75 folds a content line at 75 bytes without splitting a UTF-8 character.
func fold75(s string) string {
	if len(s) <= 75 {
		return s + "\r\n"
	}
	var b strings.Builder
	limit := 75
	for len(s) > limit {
		cut := limit
		for cut > 0 && (s[cut]&0xC0) == 0x80 {
			cut--
		}
		b.WriteString(s[:cut])
		b.WriteString("\r\n ")
		s = s[cut:]
		limit = 74
	}
	b.WriteString(s)
	b.WriteString("\r\n")
	return b.String()
}

func vtype(label string) string {
	switch strings.ToLower(label) {
	case "mobile", "whatsapp":
		return "CELL"
	case "home":
		return "HOME"
	case "work":
		return "WORK"
	case "main":
		return "MAIN"
	case "fax":
		return "FAX"
	case "pager":
		return "PAGER"
	}
	return "OTHER"
}

// WriteVCard writes one vCard 3.0, the version Apple Contacts and Google both read.
func WriteVCard(w io.Writer, c *Contact, photo []byte) error {
	var b strings.Builder
	add := func(line string) { b.WriteString(fold75(line)) }
	add("BEGIN:VCARD")
	add("VERSION:3.0")
	add(fmt.Sprintf("N:%s;%s;;;", escape(c.Last), escape(c.First)))
	fn := c.FullName()
	if fn == "" {
		fn = c.DisplayName()
	}
	add("FN:" + escape(fn))
	if c.Nickname != "" {
		add("NICKNAME:" + escape(c.Nickname))
	}
	if c.AltName != "" {
		add("X-ALT-NAME:" + escape(c.AltName))
	}
	if c.Company != "" {
		add("ORG:" + escape(c.Company) + ";")
	}
	if c.IsCompany() {
		add("X-ABShowAs:COMPANY")
	}
	if c.Title != "" {
		add("TITLE:" + escape(c.Title))
	}
	item := 0
	custom := func(prop, label, value, std string) {
		if std != "OTHER" || strings.EqualFold(label, "other") {
			add(fmt.Sprintf("%s;TYPE=%s:%s", prop, std, value))
			return
		}
		item++
		add(fmt.Sprintf("item%d.%s:%s", item, prop, value))
		add(fmt.Sprintf("item%d.X-ABLabel:%s", item, escape(label)))
	}
	for _, p := range c.Phones {
		custom("TEL", p.Label, escape(p.Value), vtype(p.Label))
	}
	for _, e := range c.Emails {
		t := vtype(e.Label)
		if t == "CELL" || t == "MAIN" || t == "FAX" || t == "PAGER" {
			t = "OTHER"
		}
		custom("EMAIL;TYPE=INTERNET", e.Label, escape(e.Value), t)
	}
	for _, u := range c.URLs {
		item++
		add(fmt.Sprintf("item%d.URL:%s", item, escape(u.Value)))
		add(fmt.Sprintf("item%d.X-ABLabel:%s", item, escape(u.Label)))
	}
	for _, a := range c.Addresses {
		t := vtype(a.Label)
		if t != "HOME" && t != "WORK" {
			t = "OTHER"
		}
		add(fmt.Sprintf("ADR;TYPE=%s:;;%s;%s;%s;%s;%s", t, escape(a.Street), escape(a.City), escape(a.Region), escape(a.Postcode), escape(a.Country)))
	}
	if c.Birthday != "" {
		m, d, y, ok := ParseBirthday(c.Birthday)
		if ok {
			if y == 0 { // Apple's convention for "no year"
				add(fmt.Sprintf("BDAY;X-APPLE-OMIT-YEAR=1604:1604-%02d-%02d", m, d))
			} else {
				add(fmt.Sprintf("BDAY:%04d-%02d-%02d", y, m, d))
			}
		}
	}
	if c.Notes != "" {
		add("NOTE:" + escape(c.Notes))
	}
	if len(c.Groups) > 0 {
		gs := make([]string, len(c.Groups))
		for i, g := range c.Groups {
			gs[i] = escape(g)
		}
		add("CATEGORIES:" + strings.Join(gs, ","))
	}
	if len(photo) > 0 {
		t := "JPEG"
		if len(photo) > 4 && string(photo[1:4]) == "PNG" {
			t = "PNG"
		}
		add("PHOTO;ENCODING=b;TYPE=" + t + ":" + base64.StdEncoding.EncodeToString(photo))
	}
	add("UID:" + c.ID)
	add("REV:" + c.Updated.UTC().Format("2006-01-02T15:04:05Z"))
	add("END:VCARD")
	_, err := io.WriteString(w, b.String())
	return err
}
