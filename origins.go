package main

import (
	"strings"
	"time"
)

// Origin records where one detail of a card came from and when ("LinkedIn", 8 Nov 2026).
type Origin struct {
	Source string    `json:"src"`
	Added  time.Time `json:"at"`
}

// detailKeys lists a card's details under stable keys that survive reformatting:
// a phone by its last 8 digits, an email in lower case, a link by its account or page.
func detailKeys(c *Contact) map[string]string {
	keys := map[string]string{}
	for _, p := range c.Phones {
		if k := phoneKey(p.Value); k != "" {
			keys[k] = p.Value
		}
	}
	for _, e := range c.Emails {
		keys["e:"+strings.ToLower(strings.TrimSpace(e.Value))] = e.Value
	}
	for _, u := range c.URLs {
		if k := linkKey(u.Label, u.Value); k != "" {
			keys["u:"+k] = u.Value
		}
	}
	for _, a := range c.Addresses {
		keys["a:"+fold(a.OneLine())] = a.OneLine()
	}
	for name, v := range map[string]string{"name": c.FullName(), "altname": c.AltName, "company": c.Company, "title": c.Title, "birthday": c.Birthday} {
		if v != "" {
			keys[name] = fold(v)
		}
	}
	return keys
}

func phoneKey(v string) string {
	d := digitsOnly(v)
	if len(d) < 3 {
		return ""
	}
	if len(d) > 8 { // last 8 digits: the same number written locally or with +965 matches
		d = d[len(d)-8:]
	}
	return "p:" + d
}

// StampNew marks every detail without a recorded origin as coming from src.
func StampNew(c *Contact, src string, at time.Time) {
	if c.Origins == nil {
		c.Origins = map[string]Origin{}
	}
	for k := range detailKeys(c) {
		if _, ok := c.Origins[k]; !ok {
			c.Origins[k] = Origin{Source: src, Added: at}
		}
	}
}

// StampChanges marks details that are new or different in after (compared with before) as
// coming from src, and forgets origins of details that are gone.
func StampChanges(before, after *Contact, src string, at time.Time) {
	old := detailKeys(before)
	now := detailKeys(after)
	if after.Origins == nil {
		after.Origins = map[string]Origin{}
	}
	for k, v := range now {
		if ov, ok := old[k]; !ok || ov != v {
			after.Origins[k] = Origin{Source: src, Added: at}
		}
	}
	for k := range after.Origins {
		if _, ok := now[k]; !ok {
			delete(after.Origins, k)
		}
	}
}

// OriginText describes where a detail came from: "LinkedIn · 8 Nov 2026" ("" if unknown).
func (c *Contact) OriginText(key string) string {
	o, ok := c.Origins[key]
	if !ok || o.Source == "" {
		return ""
	}
	if o.Added.IsZero() {
		return o.Source
	}
	return o.Source + " · " + o.Added.Format("2 Jan 2006")
}
