package main

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// TimelineEntry is something that happened with a contact: connected on LinkedIn, imported from a
// source, a job change, a business card scanned, cards merged. It keeps these out of the notes,
// which stay the user's own words.
type TimelineEntry struct {
	At     time.Time `json:"at"`
	Text   string    `json:"text"`
	Source string    `json:"src,omitempty"`
}

// AddEvent adds a timeline entry unless the same text is already there.
func (c *Contact) AddEvent(at time.Time, text, source string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	for _, e := range c.Timeline {
		if strings.EqualFold(e.Text, text) {
			return
		}
	}
	c.Timeline = append(c.Timeline, TimelineEntry{At: at, Text: text, Source: source})
	sort.SliceStable(c.Timeline, func(i, j int) bool { return c.Timeline[i].At.After(c.Timeline[j].At) })
}

// parseLooseDate reads the dates exports use: "12 Mar 2021", "2021-03-12", "Mar 2021", "2019".
func parseLooseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, l := range []string{"02 Jan 2006", "2 Jan 2006", "2006-01-02", "02/01/2006", "Jan 2006", "January 2006", "2006"} {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// autoNoteLine matches lines earlier versions wrote into notes automatically.
var autoNoteLine = regexp.MustCompile(`^(?:LinkedIn connection since .+|Facebook friend(?: since .+)?|Snapchat friend(?: since .+)?|Telegram contact(?: since .+)?|Previously: .+|.+ gives the birthday as .+|(?:LinkedIn|Google Contacts|Outlook|Mac Contacts|iCloud|Yahoo|Spreadsheet|Telegram|Facebook|Snapchat|File: [^:]+): .+)$`)

// splitAutoNotes separates automatic lines from the user's own notes.
func splitAutoNotes(notes string) (own string, auto []string) {
	var keep []string
	for _, l := range strings.Split(notes, "\n") {
		if autoNoteLine.MatchString(strings.TrimSpace(l)) {
			auto = append(auto, strings.TrimSpace(l))
			continue
		}
		keep = append(keep, l)
	}
	own = strings.TrimSpace(regexp.MustCompile(`\n{3,}`).ReplaceAllString(strings.Join(keep, "\n"), "\n\n"))
	return own, auto
}

// autoLineEvent turns an old automatic note line into a timeline entry.
func autoLineEvent(line string) TimelineEntry {
	e := TimelineEntry{Text: line}
	for _, p := range []string{"LinkedIn connection since ", "Facebook friend since ", "Snapchat friend since ", "Telegram contact since "} {
		if strings.HasPrefix(line, p) {
			if t, ok := parseLooseDate(strings.TrimPrefix(line, p)); ok {
				e.At = t
			}
			e.Text = map[string]string{"LinkedIn connection since ": "Connected on LinkedIn", "Facebook friend since ": "Became Facebook friends",
				"Snapchat friend since ": "Became Snapchat friends", "Telegram contact since ": "Added on Telegram"}[p]
			e.Source = strings.Fields(p)[0]
		}
	}
	if strings.HasPrefix(line, "Previously: ") {
		e.Text = "Previous job: " + strings.TrimPrefix(line, "Previously: ")
	}
	return e
}

// nowTime exists so screens and tests share one clock.
func nowTime() time.Time { return time.Now() }
