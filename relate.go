package main

import (
	"strings"
	"time"
)

// Related people in vCards (issue #10). On export a card carries its connections in this book and
// its Spouse / Children / Assistant… lines as Apple's "related names" (X-ABRELATEDNAMES with a
// label), plus a vCard 4 RELATED line holding the other card's ID so a re-import links the very
// same person. On import a related name becomes a connection only when it points at exactly one
// person in the book (by that ID, or by a name only one card has); otherwise it becomes one of the
// card's own lines ("Spouse: Fatima") so nothing is lost and no one is guessed at.

// Related is one related person named on a card.
type Related struct {
	Label string // Spouse, Friend, Colleague…
	Name  string // as written
	ID    string // the other card's ID, when known (urn:uuid on export/import)
}

// Apple's own labels: written as _$!<Spouse>!$_ the iPhone and Mac show them in their language.
var appleRelatedLabels = []string{"Father", "Mother", "Parent", "Brother", "Sister", "Child", "Friend",
	"Spouse", "Partner", "Assistant", "Manager", "Other"}
var appleDateLabels = []string{"Anniversary", "Other"}

// appleStdLabel writes a label the way Apple stores it: its own labels wrapped, others as typed.
func appleStdLabel(label string, std []string) string {
	l := strings.TrimSpace(label)
	if strings.EqualFold(l, "Children") {
		l = "Child"
	}
	for _, s := range std {
		if strings.EqualFold(s, l) {
			return "_$!<" + s + ">!$_"
		}
	}
	return escape(l)
}

// relatedTypes maps the app's labels to vCard 4 RELATED types (RFC 6350 §6.6.6) and back.
var relatedTypes = map[string]string{
	"friend": "friend", "family": "kin", "spouse": "spouse", "partner": "sweetheart", "colleague": "colleague",
	"classmate": "acquaintance", "neighbour": "neighbor", "neighbor": "neighbor", "business contact": "contact",
	"child": "child", "children": "child", "father": "parent", "mother": "parent", "parent": "parent",
	"brother": "sibling", "sister": "sibling", "assistant": "agent", "manager": "co-worker", "other": "contact",
}

func relatedType(label string) string {
	if t, ok := relatedTypes[strings.ToLower(strings.TrimSpace(label))]; ok {
		return t
	}
	return "contact"
}

// relatedLabel turns a vCard 4 RELATED type into a label.
func relatedLabel(t string) string {
	switch strings.ToLower(t) {
	case "kin":
		return "Family"
	case "co-worker", "colleague":
		return "Colleague"
	case "neighbor":
		return "Neighbour"
	case "sweetheart", "date", "crush":
		return "Partner"
	case "acquaintance", "met":
		return "Friend"
	case "contact", "":
		return "Other"
	case "agent":
		return "Assistant"
	case "sibling":
		return "Sibling"
	}
	return niceLabel(t, "")
}

// parseRelated reads a vCard 4 RELATED line: a uri (urn:uuid:…, or a link) or plain text.
func parseRelated(l vline) Related {
	r := Related{Label: "Other"}
	if t := l.params["TYPE"]; len(t) > 0 {
		r.Label = relatedLabel(t[0])
	}
	v := strings.TrimSpace(unescape(l.value))
	switch {
	case strings.HasPrefix(strings.ToLower(v), "urn:uuid:"):
		r.ID = v[len("urn:uuid:"):]
	case hasParam(l, "VALUE", "text"):
		r.Name = v
	default:
		r.Name = v // a uri we can't follow: kept as written
	}
	return r
}

// mergeRelated joins Apple's name line and the vCard 4 ID line written right after it for the same
// person (WriteVCardRelated writes both), so they count once and keep the file's order.
func mergeRelated(rs []Related) []Related {
	var out []Related
	for _, r := range rs {
		if r.Name == "" && r.ID != "" {
			paired := false
			for i := len(out) - 1; i >= 0; i-- { // the nearest name line of the same kind without an ID
				if out[i].ID == "" && out[i].Name != "" && relatedType(out[i].Label) == relatedType(r.Label) {
					out[i].ID, paired = r.ID, true
					break
				}
			}
			if !paired {
				out = append(out, r)
			}
			continue
		}
		if r.Name != "" || r.ID != "" {
			out = append(out, r)
		}
	}
	return out
}

// relationLineLabels are the card's own-line labels that name a person, written out as related:
// Apple's own, the connection labels, and family words.
var relationLineLabels = func() map[string]bool {
	m := map[string]bool{"children": true, "sibling": true, "son": true, "daughter": true, "wife": true, "husband": true}
	for _, l := range append(append([]string{}, appleRelatedLabels...), RelationLabels...) {
		if l != "Other" {
			m[strings.ToLower(l)] = true
		}
	}
	return m
}()

// RelatedFor lists who c is related to, for a vCard: its connections in the book (with their IDs)
// and its own lines that name a person (Spouse: Fatima), without saying anyone twice.
func (s *Store) RelatedFor(c *Contact) []Related {
	var out []Related
	seen := map[string]bool{}
	for _, l := range s.LinksOf(c.ID) {
		o := s.Get(l.other(c.ID))
		if o == nil {
			continue
		}
		out = append(out, Related{Label: l.Label, Name: o.DisplayName(), ID: o.ID})
		seen[fold(o.DisplayName())] = true
	}
	for _, f := range c.Custom {
		if !relationLineLabels[strings.ToLower(f.Label)] || seen[fold(f.Value)] {
			continue
		}
		for _, n := range splitNames(f.Value) { // "Children: Ali, Noor" → two related names
			if !seen[fold(n)] {
				seen[fold(n)] = true
				out = append(out, Related{Label: f.Label, Name: n})
			}
		}
	}
	return out
}

// splitNames splits "Ali, Noor and Sara" / "علي، نور" into names; a single name stays whole.
func splitNames(v string) []string {
	r := strings.NewReplacer("،", ",", " and ", ",", " & ", ",", " و ", ",")
	var out []string
	for _, p := range strings.Split(r.Replace(v), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// applyRelated links or records the related people of the cards just imported. final maps each
// incoming card to the contact it became (new, or the one it merged into); uid maps the IDs the
// file gave its cards to those contacts, so cards in one file can point at each other.
func applyRelated(s *Store, items []ImportItem, final map[*Contact]*Contact, uid map[string]*Contact) {
	byName := map[string][]*Contact{}
	for _, c := range s.Contacts {
		for _, n := range []string{c.FullName(), c.FormalName(), c.AltName} {
			if n != "" {
				k := fold(n)
				if len(byName[k]) == 0 || byName[k][len(byName[k])-1] != c {
					byName[k] = append(byName[k], c)
				}
			}
		}
	}
	for _, it := range items {
		c := final[it.Card.Contact]
		if c == nil || len(it.Card.Related) == 0 || s.Get(c.ID) == nil {
			continue
		}
		changed := false
		for _, r := range it.Card.Related {
			var other *Contact
			if r.ID != "" {
				other = s.Get(r.ID) // our own export
				if other == nil {
					other = uid[strings.ToLower(r.ID)]
				}
			}
			if other == nil && r.Name != "" {
				if m := byName[fold(r.Name)]; len(m) == 1 {
					other = m[0]
				}
			}
			if other != nil && other.ID != c.ID {
				if !linked(s, c.ID, other.ID) {
					s.AddLink(c.ID, other.ID, r.Label)
					c.AddEvent(time.Now(), "Connected to "+other.DisplayName()+" ("+r.Label+") from "+sourceOf(c), sourceOf(c))
					changed = true
				}
				continue
			}
			if r.Name == "" {
				continue // an ID we can't find and no name: nothing to keep
			}
			dup := false
			for _, f := range c.Custom {
				dup = dup || (strings.EqualFold(f.Label, r.Label) && fold(f.Value) == fold(r.Name))
			}
			if !dup {
				c.Custom = append(c.Custom, Field{Label: r.Label, Value: r.Name})
				changed = true
			}
		}
		if changed {
			c.Updated = time.Now()
			s.Put(c)
		}
	}
}

func linked(s *Store, a, b string) bool {
	for _, l := range s.LinksOf(a) {
		if l.has(b) {
			return true
		}
	}
	return false
}
