package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// CalEvent is one calendar entry, kept on this Mac so each contact's card can show the
// meetings they were in or mentioned in.
type CalEvent struct {
	UID       string     `json:"uid"`
	Title     string     `json:"title"`
	Start     time.Time  `json:"start"`
	AllDay    bool       `json:"allDay,omitempty"`
	Calendar  string     `json:"calendar,omitempty"`
	Location  string     `json:"location,omitempty"`
	Notes     string     `json:"notes,omitempty"` // first 2,000 characters, used for matching
	Attendees []Attendee `json:"attendees,omitempty"`

	text string // folded title + location + notes + attendee names, for matching
}

// Attendee is someone invited to an event.
type Attendee struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// Calendar is the stored calendar data: <data dir>/calendar.json (permission 0600).
type Calendar struct {
	Read   time.Time  `json:"read"`
	From   string     `json:"from"` // "Mac Calendar" or the .ics file name
	Events []CalEvent `json:"events"`
}

func calendarPath(dir string) string { return filepath.Join(dir, "calendar.json") }

// LoadCalendar reads the stored events (an empty calendar if there are none).
func LoadCalendar(dir string) *Calendar {
	c := &Calendar{}
	if b, err := os.ReadFile(calendarPath(dir)); err == nil {
		_ = json.Unmarshal(b, c)
	}
	return c
}

// Save writes the events.
func (c *Calendar) Save(dir string) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return writeFileAtomic(calendarPath(dir), b, 0o600)
}

// Forget deletes the stored calendar data.
func ForgetCalendar(dir string) { os.Remove(calendarPath(dir)) }

// Merge adds events, replacing ones with the same UID and start.
func (c *Calendar) Merge(evs []CalEvent, from string) {
	key := func(e CalEvent) string { return e.UID + "|" + e.Start.Format(time.RFC3339) + "|" + e.Title }
	idx := map[string]int{}
	for i, e := range c.Events {
		idx[key(e)] = i
	}
	for _, e := range evs {
		if i, ok := idx[key(e)]; ok {
			c.Events[i] = e
		} else {
			idx[key(e)] = len(c.Events)
			c.Events = append(c.Events, e)
		}
	}
	sort.Slice(c.Events, func(i, j int) bool { return c.Events[i].Start.After(c.Events[j].Start) })
	c.Read = time.Now()
	if c.From == "" || c.From == from {
		c.From = from
	} else if !strings.Contains(c.From, from) {
		c.From += ", " + from
	}
}

func (e *CalEvent) folded() string {
	if e.text == "" {
		parts := []string{e.Title, e.Location, e.Notes}
		for _, a := range e.Attendees {
			parts = append(parts, a.Name)
		}
		e.text = " " + fold(strings.Join(parts, " ")) + " "
	}
	return e.text
}

// Mention is an event linked to a contact, and how.
type Mention struct {
	Event *CalEvent
	How   string // "attendee" or "mentioned"
}

// EventsFor finds the events a contact took part in (their email among the attendees, or their
// full name as an attendee) or was mentioned in (their full name, in either language, in the
// title, place or notes). A first name alone is never enough: "Ali" is in too many meetings.
func (c *Calendar) EventsFor(ct *Contact) []Mention {
	emails := map[string]bool{}
	for _, e := range ct.Emails {
		emails[strings.ToLower(strings.TrimSpace(e.Value))] = true
	}
	var names, skels []string
	for _, n := range []string{ct.FullName(), ct.AltName} {
		n = strings.TrimSpace(n)
		if len(strings.Fields(n)) >= 2 {
			names = append(names, fold(n))
			skels = append(skels, NameSkeleton(n))
		}
	}
	if len(emails) == 0 && len(names) == 0 {
		return nil
	}
	var out []Mention
	for i := range c.Events {
		e := &c.Events[i]
		how := ""
		for _, a := range e.Attendees {
			if emails[strings.ToLower(a.Email)] {
				how = "attendee"
			}
			for _, n := range names {
				if a.Name != "" && fold(a.Name) == n {
					how = "attendee"
				}
			}
		}
		if how == "" {
			txt := e.folded()
			for _, n := range names {
				if strings.Contains(txt, " "+n+" ") || strings.Contains(txt, n) && len(n) > 8 {
					how = "mentioned"
				}
			}
			if how == "" && containsNameAcrossScripts(txt, skels) {
				how = "mentioned"
			}
		}
		if how != "" {
			out = append(out, Mention{e, how})
		}
	}
	return out
}

// containsNameAcrossScripts looks for a run of words in text whose skeleton equals a full name's
// skeleton, so "Lunch with محمد المطيري" is found for Mohammed Al-Mutairi.
func containsNameAcrossScripts(text string, skels []string) bool {
	if len(skels) == 0 {
		return false
	}
	words := strings.Fields(strings.Map(func(r rune) rune {
		if strings.ContainsRune(",.;:()[]/|", r) {
			return ' '
		}
		return r
	}, text))
	for n := 2; n <= 4; n++ {
		for i := 0; i+n <= len(words); i++ {
			s := NameSkeleton(strings.Join(words[i:i+n], " "))
			if len(s) < 4 {
				continue
			}
			for _, k := range skels {
				if s == k {
					return true
				}
			}
		}
	}
	return false
}

// LastMet is the most recent past event the contact attended (or was mentioned in).
func LastMet(ms []Mention, now time.Time) time.Time {
	for _, m := range ms { // events are newest first
		if !m.Event.Start.After(now) {
			return m.Event.Start
		}
	}
	return time.Time{}
}

// ---------- reading the Mac Calendar app ----------

const fieldSep, recordSep = "\x1f", "\x1e"

// ReadMacCalendar asks the Calendar app for events from daysBack ago to daysAhead from now.
// It covers every calendar shown in the app (iCloud, Google, Exchange…). Repeating events
// appear once, at their first date in the range or before it.
func ReadMacCalendar(daysBack, daysAhead int) ([]CalEvent, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("this only works on a Mac")
	}
	script := fmt.Sprintf(`on clean(v)
	if v is missing value then return ""
	try
		return v as text
	on error
		return ""
	end try
end clean
set d1 to (current date) - (%d * days)
set d2 to (current date) + (%d * days)
set FS to (ASCII character 31)
set RS to (ASCII character 30)
set out to {}
with timeout of 1800 seconds
	tell application "Calendar"
		repeat with cal in calendars
			set cn to name of cal
			try
				set evRef to a reference to (every event of cal whose start date ≥ d1 and start date ≤ d2)
				set uids to uid of evRef
				set sums to summary of evRef
				set sds to start date of evRef
				set locs to location of evRef
				set descs to description of evRef
				set alls to allday event of evRef
				set attE to {}
				set attN to {}
				try
					set attE to email of attendees of evRef
					set attN to display name of attendees of evRef
				end try
			on error
				set uids to {}
			end try
			repeat with i from 1 to count of uids
				set att to ""
				try
					set es to item i of attE
					set ns to item i of attN
					repeat with j from 1 to count of es
						set att to att & my clean(item j of es) & "|" & my clean(item j of ns) & ";"
					end repeat
				end try
				set ad to "0"
				try
					if item i of alls then set ad to "1"
				end try
				set end of out to my clean(item i of uids) & FS & my clean(item i of sums) & FS & ((item i of sds) as «class isot» as string) & FS & ad & FS & cn & FS & my clean(item i of locs) & FS & att & FS & my clean(item i of descs)
			end repeat
		end repeat
	end tell
end timeout
set AppleScript's text item delimiters to RS
return out as text`, daysBack, daysAhead)
	cmd := exec.Command("osascript", "-e", script)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		switch {
		case strings.Contains(msg, "-1743") || strings.Contains(strings.ToLower(msg), "not authorized"):
			return nil, errors.New("macOS blocked access to Calendar. Open System Settings → Privacy & Security → Automation, " +
				"turn on Calendar under Smart Contacts, then try again")
		case strings.Contains(msg, "-1712"):
			return nil, errors.New("the Calendar app took too long; try a shorter period")
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("couldn't read the Calendar app: %s", msg)
	}
	return parseCalendarOutput(out.String()), nil
}

// parseCalendarOutput reads the AppleScript output (fields split by 0x1f, events by 0x1e).
func parseCalendarOutput(s string) []CalEvent {
	var evs []CalEvent
	for _, rec := range strings.Split(strings.TrimRight(s, "\n"), recordSep) {
		f := strings.Split(rec, fieldSep)
		if len(f) < 8 {
			continue
		}
		start, err := time.ParseInLocation("2006-01-02T15:04:05", strings.TrimSpace(f[2]), time.Local)
		if err != nil {
			continue
		}
		e := CalEvent{UID: f[0], Title: f[1], Start: start, AllDay: f[3] == "1", Calendar: f[4], Location: f[5], Notes: clip(f[7], 2000)}
		for _, a := range strings.Split(f[6], ";") {
			em, name, _ := strings.Cut(a, "|")
			em = strings.TrimPrefix(strings.TrimSpace(em), "mailto:")
			if em != "" || strings.TrimSpace(name) != "" {
				e.Attendees = append(e.Attendees, Attendee{Name: strings.TrimSpace(name), Email: em})
			}
		}
		evs = append(evs, e)
	}
	return evs
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n]
}

// ---------- .ics files (Google Calendar, Outlook exports) ----------

// ParseICS reads the events in an iCalendar file.
func ParseICS(r io.Reader) ([]CalEvent, error) {
	lines, err := unfold(r)
	if err != nil {
		return nil, err
	}
	var evs []CalEvent
	var cur *CalEvent
	for _, raw := range lines {
		l, ok := parseLine(raw)
		if !ok {
			continue
		}
		switch {
		case l.name == "BEGIN" && strings.EqualFold(l.value, "VEVENT"):
			cur = &CalEvent{}
		case l.name == "END" && strings.EqualFold(l.value, "VEVENT"):
			if cur != nil && !cur.Start.IsZero() {
				evs = append(evs, *cur)
			}
			cur = nil
		case cur == nil:
		case l.name == "UID":
			cur.UID = l.value
		case l.name == "SUMMARY":
			cur.Title = unescape(l.value)
		case l.name == "LOCATION":
			cur.Location = unescape(l.value)
		case l.name == "DESCRIPTION":
			cur.Notes = clip(unescape(l.value), 2000)
		case l.name == "DTSTART":
			cur.Start, cur.AllDay = parseICSTime(l)
		case l.name == "ATTENDEE" || l.name == "ORGANIZER":
			a := Attendee{Email: strings.TrimPrefix(strings.TrimPrefix(l.value, "mailto:"), "MAILTO:")}
			if cn := l.params["CN"]; len(cn) > 0 {
				a.Name = strings.Trim(cn[0], `"`)
			}
			if a.Email == a.Name {
				a.Name = ""
			}
			cur.Attendees = append(cur.Attendees, a)
		}
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].Start.After(evs[j].Start) })
	return evs, nil
}

func parseICSTime(l vline) (time.Time, bool) {
	v := strings.TrimSpace(l.value)
	if len(v) == 8 { // VALUE=DATE
		t, err := time.ParseInLocation("20060102", v, time.Local)
		return t, err == nil
	}
	if strings.HasSuffix(v, "Z") {
		t, _ := time.Parse("20060102T150405Z", v)
		return t.Local(), false
	}
	loc := time.Local
	if tz := l.params["TZID"]; len(tz) > 0 {
		if z, err := time.LoadLocation(strings.Trim(tz[0], `"`)); err == nil {
			loc = z
		}
	}
	t, _ := time.ParseInLocation("20060102T150405", v, loc)
	return t, false
}
