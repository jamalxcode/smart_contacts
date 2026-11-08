package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// What a change did, card by card and field by field. It's worked out from the undo record
// (each card as it was before) and the cards as they are now, so every command reports its
// changes the same way, whether it's one edit or a clean-up of hundreds of cards.

type FieldChange struct {
	Field, Before, After string
}

type CardChange struct {
	ID, Name string
	Kind     string // "added", "changed" or "deleted"
	Fields   []FieldChange
}

// cardFields is a card as labelled text, in the order the card shows it.
func cardFields(c *Contact) [][2]string {
	fields := func(fs []Field) string {
		var out []string
		for _, f := range fs {
			if f.Value != "" {
				out = append(out, strings.TrimSpace(fieldLabel(f.Label)+" "+f.Value))
			}
		}
		return strings.Join(out, "\n")
	}
	var addrs []string
	for _, a := range c.Addresses {
		var p []string
		for _, s := range []string{a.Street, a.City, a.Region, a.Postcode, a.Country} {
			if s != "" {
				p = append(p, s)
			}
		}
		if len(p) > 0 {
			addrs = append(addrs, strings.TrimSpace(fieldLabel(a.Label)+" "+strings.Join(p, ", ")))
		}
	}
	yes := func(b bool) string {
		if b {
			return "Yes"
		}
		return ""
	}
	keep := ""
	if c.KeepInTouch > 0 {
		keep = keepText(c.KeepInTouch)
	}
	return [][2]string{
		{"Name", strings.TrimSpace(c.First + " " + c.Last)},
		{"Other-language name", c.AltName},
		{"Nickname", c.Nickname},
		{"Company", c.Company},
		{"Job title", c.Title},
		{"Phones", fields(c.Phones)},
		{"Emails", fields(c.Emails)},
		{"Links", fields(c.URLs)},
		{"Addresses", strings.Join(addrs, "\n")},
		{"Birthday", c.Birthday},
		{"How we met", c.HowWeMet},
		{"Notes", c.Notes},
		{"Groups", strings.Join(c.Groups, ", ")},
		{"Favorite", yes(c.Favorite)},
		{"Keep in touch", keep},
		{"Photo", yes(c.HasPhoto)},
	}
}

// diffCards lists the fields that differ between two versions of a card.
func diffCards(a, b *Contact) []FieldChange {
	fa, fb := cardFields(a), cardFields(b)
	var out []FieldChange
	for i := range fa {
		if fa[i][1] != fb[i][1] {
			out = append(out, FieldChange{fa[i][0], fa[i][1], fb[i][1]})
		}
	}
	return out
}

// changesOf compares the cards an undo record saved with the cards as they are now.
func (s *Store) changesOf(st undoStep) []CardChange {
	var out []CardChange
	for id, old := range st.Before {
		now := s.Get(id)
		var was *Contact
		if old != nil {
			was = &Contact{}
			if json.Unmarshal(old, was) != nil {
				was = nil
			}
		}
		switch {
		case was == nil && now != nil:
			out = append(out, CardChange{ID: id, Name: now.DisplayName(), Kind: "added", Fields: diffCards(&Contact{}, now)})
		case was != nil && now == nil:
			out = append(out, CardChange{ID: id, Name: was.DisplayName(), Kind: "deleted"})
		case was != nil && now != nil:
			if f := diffCards(was, now); len(f) > 0 {
				out = append(out, CardChange{ID: id, Name: now.DisplayName(), Kind: "changed", Fields: f})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind // added, changed, deleted
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// LastChanges is what the most recent change did (nil if there's nothing to undo).
func (s *Store) LastChanges() []CardChange {
	if len(s.undo) == 0 {
		return nil
	}
	return s.changesOf(s.undo[len(s.undo)-1])
}

// UndoneChanges is what the most recent Undo did (nil if there's nothing to redo).
func (s *Store) UndoneChanges() []CardChange {
	if len(s.redo) == 0 {
		return nil
	}
	return s.changesOf(s.redo[len(s.redo)-1])
}

// UndoDepth counts the changes that can be undone, so callers can tell whether a save made one.
func (s *Store) UndoDepth() int { return len(s.undo) }

// summariseChanges is one line for a banner: "3 cards changed — 5 phone numbers, 2 names".
func summariseChanges(cs []CardChange) string {
	if len(cs) == 0 {
		return ""
	}
	var added, changed, deleted int
	fields := map[string]int{}
	var order []string
	for _, c := range cs {
		switch c.Kind {
		case "added":
			added++
		case "deleted":
			deleted++
		default:
			changed++
			for _, f := range c.Fields {
				if fields[f.Field] == 0 {
					order = append(order, f.Field)
				}
				fields[f.Field]++
			}
		}
	}
	var parts []string
	if added > 0 {
		parts = append(parts, fmt.Sprintf("%d %s added", added, pluralWords(added, "card", "cards")))
	}
	if changed > 0 {
		s := fmt.Sprintf("%d %s changed", changed, pluralWords(changed, "card", "cards"))
		if changed == 1 {
			for _, c := range cs {
				if c.Kind == "changed" {
					s = c.Name + " changed"
				}
			}
		}
		var fp []string
		for _, f := range order {
			if len(fp) == 3 {
				fp = append(fp, "…")
				break
			}
			fp = append(fp, strings.ToLower(f))
		}
		if len(fp) > 0 {
			s += ": " + strings.Join(fp, ", ")
		}
		parts = append(parts, s)
	}
	if deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d %s moved to the Trash", deleted, pluralWords(deleted, "card", "cards")))
	}
	return strings.Join(parts, " · ")
}

// StepSerial goes up by one each time a save records an undoable change.
func (s *Store) StepSerial() int { return s.stepSerial }
