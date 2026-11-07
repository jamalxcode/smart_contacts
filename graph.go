package main

import (
	"strings"
	"time"
)

// Link is a relationship between two contacts ("friend", "family", "colleague"…).
// Links are symmetric: A–B is the same link as B–A. Contacts in the Trash keep their links,
// so restoring someone brings their connections back; links go when the Trash is emptied.
type Link struct {
	A       string    `json:"a"`
	B       string    `json:"b"`
	Label   string    `json:"label"`
	Created time.Time `json:"created"`
}

// RelationLabels are the choices offered; any other word can be typed.
var RelationLabels = []string{"Friend", "Family", "Spouse", "Colleague", "Classmate", "Neighbour", "Business contact", "Other"}

// MaxDegrees is the "six degrees of separation" limit for chains.
const MaxDegrees = 6

func (l Link) other(id string) string {
	if l.A == id {
		return l.B
	}
	return l.A
}

func (l Link) has(id string) bool { return l.A == id || l.B == id }

// AddLink records (or relabels) a relationship. It returns false for a link to oneself.
func (s *Store) AddLink(a, b, label string) bool {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "Friend"
	}
	if a == "" || b == "" || a == b {
		return false
	}
	for i, l := range s.Links {
		if l.has(a) && l.has(b) {
			s.Links[i].Label = label
			return true
		}
	}
	s.Links = append(s.Links, Link{A: a, B: b, Label: label, Created: time.Now()})
	return true
}

// RemoveLink deletes the relationship between a and b.
func (s *Store) RemoveLink(a, b string) {
	keep := s.Links[:0]
	for _, l := range s.Links {
		if !(l.has(a) && l.has(b)) {
			keep = append(keep, l)
		}
	}
	s.Links = keep
}

// LinksOf lists c's recorded relationships to contacts that are in the address book (not trashed).
func (s *Store) LinksOf(id string) []Link {
	var out []Link
	for _, l := range s.Links {
		if l.has(id) && s.Get(l.other(id)) != nil {
			out = append(out, l)
		}
	}
	return out
}

// RepointLinks moves every link of from onto to (used when merging duplicates).
func (s *Store) RepointLinks(from, to string) {
	var moved []Link
	keep := s.Links[:0]
	for _, l := range s.Links {
		if l.has(from) {
			moved = append(moved, l)
		} else {
			keep = append(keep, l)
		}
	}
	s.Links = keep
	for _, l := range moved {
		if o := l.other(from); o != to {
			s.AddLink(to, o, l.Label)
		}
	}
}

// dropDeadLinks removes links to contacts that are neither in the book nor in the Trash.
func (s *Store) dropDeadLinks() {
	alive := map[string]bool{}
	for _, c := range s.Contacts {
		alive[c.ID] = true
	}
	for _, t := range s.Trash {
		alive[t.Contact.ID] = true
	}
	keep := s.Links[:0]
	for _, l := range s.Links {
		if alive[l.A] && alive[l.B] {
			keep = append(keep, l)
		}
	}
	s.Links = keep
}

// ---------- the graph ----------

// Graph answers "how are these two people connected?" over the recorded links and,
// optionally, over shared employers (people at the same company count as colleagues).
type Graph struct {
	s         *Store
	byID      map[string]*Contact
	adj       map[string][]edge
	companies map[string][]string // folded company → member IDs
	useCo     bool
}

type edge struct {
	to    string
	label string
}

// Step is one link in a chain: the person reached, and how ("friend", "colleague at Zain").
// The first step of a chain has an empty Via.
type Step struct {
	Contact *Contact
	Via     string
}

// NewGraph builds the graph from the store's current contacts.
func NewGraph(s *Store, useCompany bool) *Graph {
	g := &Graph{s: s, byID: map[string]*Contact{}, adj: map[string][]edge{}, companies: map[string][]string{}, useCo: useCompany}
	for _, c := range s.Contacts {
		g.byID[c.ID] = c
	}
	for _, l := range s.Links {
		if g.byID[l.A] == nil || g.byID[l.B] == nil {
			continue
		}
		g.adj[l.A] = append(g.adj[l.A], edge{l.B, strings.ToLower(l.Label)})
		g.adj[l.B] = append(g.adj[l.B], edge{l.A, strings.ToLower(l.Label)})
	}
	if useCompany {
		for _, c := range s.Contacts {
			if k := companyKey(c); k != "" {
				g.companies[k] = append(g.companies[k], c.ID)
			}
		}
	}
	return g
}

// companyKey ignores company cards themselves (Gulf Bank's switchboard isn't a colleague).
func companyKey(c *Contact) string {
	if c.IsCompany() {
		return ""
	}
	k := fold(c.Company)
	for _, suffix := range []string{" k.s.c.p", " k.s.c", " w.l.l", " llc", " ltd", " inc", " co.", ","} {
		k = strings.TrimSuffix(k, suffix)
	}
	return strings.TrimSpace(k)
}

func (g *Graph) neighbours(id string) []edge {
	out := append([]edge(nil), g.adj[id]...)
	if g.useCo {
		c := g.byID[id]
		if k := companyKey(c); k != "" {
			for _, o := range g.companies[k] {
				if o != id {
					out = append(out, edge{o, "colleague at " + c.Company})
				}
			}
		}
	}
	return out
}

// Path finds the shortest chain from a to b of at most max steps (nil if there is none).
// Recorded relationships are preferred over shared-company links when both are as short.
func (g *Graph) Path(a, b string, max int) []Step {
	if g.byID[a] == nil || g.byID[b] == nil {
		return nil
	}
	if a == b {
		return []Step{{Contact: g.byID[a]}}
	}
	type prev struct {
		from, via string
	}
	seen := map[string]prev{a: {}}
	frontier := []string{a}
	for depth := 0; depth < max && len(frontier) > 0; depth++ {
		var next []string
		for _, id := range frontier {
			for _, e := range g.neighbours(id) { // recorded links come first in the list
				if _, ok := seen[e.to]; ok {
					continue
				}
				seen[e.to] = prev{id, e.label}
				if e.to == b {
					var chain []Step
					for cur := b; cur != a; cur = seen[cur].from {
						chain = append([]Step{{Contact: g.byID[cur], Via: seen[cur].via}}, chain...)
					}
					return append([]Step{{Contact: g.byID[a]}}, chain...)
				}
				next = append(next, e.to)
			}
		}
		frontier = next
	}
	return nil
}

// Degrees counts how many people are exactly 1, 2, … max steps from id.
func (g *Graph) Degrees(id string, max int) []int {
	counts := make([]int, max)
	if g.byID[id] == nil {
		return counts
	}
	seen := map[string]bool{id: true}
	frontier := []string{id}
	for depth := 0; depth < max && len(frontier) > 0; depth++ {
		var next []string
		for _, cur := range frontier {
			for _, e := range g.neighbours(cur) {
				if !seen[e.to] {
					seen[e.to] = true
					next = append(next, e.to)
				}
			}
		}
		counts[depth] = len(next)
		frontier = next
	}
	return counts
}

// Colleagues lists other people at the same company as c.
func (g *Graph) Colleagues(c *Contact) []*Contact {
	var out []*Contact
	for _, id := range g.companies[companyKey(c)] {
		if id != c.ID && companyKey(c) != "" {
			out = append(out, g.byID[id])
		}
	}
	return out
}
