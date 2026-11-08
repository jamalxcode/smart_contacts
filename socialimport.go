package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Social network exports. Telegram's export holds contacts with phone numbers, so they import
// like any address book. Facebook and Snapchat list friends by name only: those are matched to
// people already in the book (adding a note and the profile) and only become new cards if the
// user ticks them. Instagram and X list only usernames or account numbers, which can't be matched.

// readSocialJSON recognises a social network export.
func readSocialJSON(name string, data []byte) (*Batch, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		if strings.Contains(string(data[:min(len(data), 200)]), "window.YTD") {
			return nil, errors.New("this is an X (Twitter) archive. It lists only account numbers, so it can't be matched to your contacts")
		}
		return nil, fmt.Errorf("couldn't read that file: %v", err)
	}
	switch {
	case probe["contacts"] != nil && (probe["personal_information"] != nil || probe["chats"] != nil || probe["frequent_contacts"] != nil || strings.Contains(string(probe["contacts"]), "phone_number")):
		return readTelegram(probe["contacts"])
	case probe["friends_v2"] != nil || probe["friends"] != nil:
		raw := probe["friends_v2"]
		if raw == nil {
			raw = probe["friends"]
		}
		return readFacebook(raw)
	case probe["Friends"] != nil:
		return readSnapchat(probe["Friends"])
	case probe["relationships_following"] != nil || probe["relationships_followers"] != nil || strings.Contains(string(data[:min(len(data), 4096)]), "string_list_data"):
		return nil, errors.New("this is an Instagram export. It lists only usernames — no names, numbers or emails — so it can't be matched to your contacts. " +
			"Add Instagram links to a contact by hand (Edit → Web & Social)")
	}
	return nil, errors.New("this JSON file isn't a contacts export Smart Contacts knows (Telegram, Facebook or Snapchat); export contacts as .vcf or .csv")
}

// Telegram Desktop → Settings → Advanced → Export Telegram data → Contacts list (JSON).
func readTelegram(raw json.RawMessage) (*Batch, error) {
	var tg struct {
		List []struct {
			First string `json:"first_name"`
			Last  string `json:"last_name"`
			Phone string `json:"phone_number"`
			Date  string `json:"date"`
		} `json:"list"`
	}
	if err := json.Unmarshal(raw, &tg); err != nil {
		return nil, fmt.Errorf("couldn't read the Telegram contacts: %v", err)
	}
	b := &Batch{Source: "Telegram", Format: "Telegram contacts export"}
	for _, p := range tg.List {
		c := NewContact()
		c.First, c.Last = strings.TrimSpace(p.First), strings.TrimSpace(p.Last)
		if p.Phone != "" {
			c.Phones = []Field{{Label: "Mobile", Value: p.Phone}}
		}
		c.Notes = "Telegram contact"
		if t, err := time.Parse("2006-01-02T15:04:05", p.Date); err == nil {
			c.Notes += " since " + t.Format("2 Jan 2006")
		}
		c.Tidy()
		if c.FullName() == "" && len(c.Phones) == 0 {
			b.Skipped++
			continue
		}
		b.Cards = append(b.Cards, Card{Contact: c})
	}
	return b, nil
}

// Facebook → Settings → Download your information → Friends (JSON).
func readFacebook(raw json.RawMessage) (*Batch, error) {
	var friends []struct {
		Name      string `json:"name"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := json.Unmarshal(raw, &friends); err != nil {
		return nil, fmt.Errorf("couldn't read the Facebook friends: %v", err)
	}
	b := &Batch{Source: "Facebook", Format: "Facebook friends (names only)", MatchOnly: true}
	b.Notes = append(b.Notes, "Facebook lists friends by name only, so they're matched to people you already have. Unmatched names are not added unless you tick them.")
	for _, f := range friends {
		name := fixFacebookText(f.Name)
		if strings.TrimSpace(name) == "" {
			b.Skipped++
			continue
		}
		c := NewContact()
		setName(c, name)
		c.Notes = "Facebook friend"
		if f.Timestamp > 0 {
			c.Notes += " since " + time.Unix(f.Timestamp, 0).Format("Jan 2006")
		}
		c.Tidy()
		b.Cards = append(b.Cards, Card{Contact: c})
	}
	return b, nil
}

// Snapchat → Settings → My Data → friends.json.
func readSnapchat(raw json.RawMessage) (*Batch, error) {
	var friends []map[string]string
	if err := json.Unmarshal(raw, &friends); err != nil {
		return nil, fmt.Errorf("couldn't read the Snapchat friends: %v", err)
	}
	b := &Batch{Source: "Snapchat", Format: "Snapchat friends (names only)", MatchOnly: true}
	b.Notes = append(b.Notes, "Snapchat lists friends by display name, so they're matched to people you already have (adding their Snapchat). Unmatched names are not added unless you tick them.")
	for _, f := range friends {
		name, user := strings.TrimSpace(f["Display Name"]), strings.TrimSpace(f["Username"])
		if name == "" {
			b.Skipped++
			continue
		}
		c := NewContact()
		setName(c, name)
		if user != "" {
			c.URLs = []Field{{Label: "Snapchat", Value: "https://www.snapchat.com/add/" + user}}
		}
		c.Notes = "Snapchat friend"
		if ts := f["Creation Timestamp"]; len(ts) >= 4 {
			c.Notes += " since " + ts[:4]
		}
		c.Tidy()
		b.Cards = append(b.Cards, Card{Contact: c})
	}
	return b, nil
}

// fixFacebookText repairs Facebook's export encoding, which writes UTF-8 bytes as separate
// characters ("Ù\u0085Ø­" instead of "مح").
func fixFacebookText(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 255 {
			return s // already proper text
		}
		b = append(b, byte(r))
	}
	if utf8.Valid(b) {
		return string(b)
	}
	return s
}
