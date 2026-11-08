package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Real "last talked" history, read on this Mac and kept on this Mac. Messages (iMessage and SMS
// from the iPhone), call history (iPhone calls through Continuity and FaceTime) and Mail keep
// their own databases; macOS's sqlite3 reads them once Smart Contacts has Full Disk Access.
// WhatsApp can't be read directly, so its chat exports are read instead. Nothing is sent anywhere.

// Touch is one person-side of the history: a phone number, email or (WhatsApp) name, when they
// were last in touch over a channel, and how often.
type Touch struct {
	Key     string    `json:"key"` // "p:<last 8 digits>", "e:<email>" or "n:<folded name>"
	Show    string    `json:"show"`
	Channel string    `json:"channel"` // Messages, Calls, Mail, WhatsApp
	Last    time.Time `json:"last"`
	Count   int       `json:"count"`
	Mine    bool      `json:"mine,omitempty"` // I wrote or called (not only received)
}

// Activity is what's known about one contact.
type Activity struct {
	Last     time.Time            `json:"last"`
	Channel  string               `json:"channel"`
	Channels map[string]time.Time `json:"channels"`
	Counts   map[string]int       `json:"counts"`
}

// ActivityBook is activity.json: per contact, plus people talked to who aren't in the book.
type ActivityBook struct {
	Read     time.Time            `json:"read"`
	People   map[string]*Activity `json:"people"`
	Unknown  []Touch              `json:"unknown,omitempty"`
	Problems []string             `json:"problems,omitempty"` // sources that couldn't be read
	Touches  []Touch              `json:"touches,omitempty"`  // from Messages, Calls, Mail (replaced at each read)
	WhatsApp []Touch              `json:"whatsapp,omitempty"` // from chat exports (kept: each export is one-off)
}

func activityPath(dir string) string { return filepath.Join(dir, "activity.json") }

func LoadActivity(dir string) *ActivityBook {
	ab := &ActivityBook{People: map[string]*Activity{}}
	if b, err := os.ReadFile(activityPath(dir)); err == nil {
		_ = json.Unmarshal(b, ab)
	}
	if ab.People == nil {
		ab.People = map[string]*Activity{}
	}
	return ab
}

func (ab *ActivityBook) Save(dir string) error {
	b, err := json.Marshal(ab)
	if err != nil {
		return err
	}
	return writeFileAtomic(activityPath(dir), b, 0o600)
}

// ErrNoAccess means macOS refused to let the app read a database (Full Disk Access is off).
var ErrNoAccess = errors.New("no Full Disk Access")

// sqliteRows runs a read-only query on a copy of a database (Messages keeps its database open
// with a write-ahead log, so the copy includes the -wal and -shm files).
func sqliteRows(db, query string) ([][]string, error) {
	if _, err := os.Stat(db); err != nil {
		if os.IsPermission(err) {
			return nil, ErrNoAccess
		}
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "sc-read-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	cp := filepath.Join(tmp, "db")
	for _, suf := range []string{"", "-wal", "-shm"} {
		b, err := os.ReadFile(db + suf)
		if err != nil {
			if suf == "" {
				if os.IsPermission(err) || strings.Contains(err.Error(), "operation not permitted") {
					return nil, ErrNoAccess
				}
				return nil, err
			}
			continue
		}
		if err := os.WriteFile(cp+suf, b, 0o600); err != nil {
			return nil, err
		}
	}
	out, err := exec.Command("sqlite3", "-readonly", "-separator", "\x1f", cp, query).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	var rows [][]string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if l != "" {
			rows = append(rows, strings.Split(l, "\x1f"))
		}
	}
	return rows, nil
}

var appleEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// appleTime reads Apple's dates: seconds (or, in newer Messages, nanoseconds) since 2001.
func appleTime(s string) time.Time {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f <= 0 {
		return time.Time{}
	}
	if f > 1e12 {
		f /= 1e9
	}
	return appleEpoch.Add(time.Duration(f * float64(time.Second)))
}

func unixTime(s string) time.Time {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(f), 0)
}

// touchKey turns a phone number or email into a match key.
func touchKey(v string) (string, string) {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(v, "mailto:"), "tel:"))
	if strings.Contains(v, "@") {
		return "e:" + strings.ToLower(v), strings.ToLower(v)
	}
	if k := phoneKey(v); k != "" && len(digitsOnly(v)) >= 7 {
		return k, v
	}
	return "", ""
}

func countOf(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }

// ReadMessages: ~/Library/Messages/chat.db — handle.id is the number or email.
func ReadMessages(home string) ([]Touch, error) {
	rows, err := sqliteRows(filepath.Join(home, "Library/Messages/chat.db"),
		`SELECT h.id, MAX(m.date), COUNT(*), SUM(m.is_from_me) FROM message m JOIN handle h ON m.handle_id = h.ROWID GROUP BY h.id;`)
	if err != nil {
		return nil, err
	}
	var out []Touch
	for _, r := range rows {
		if len(r) < 4 {
			continue
		}
		k, show := touchKey(r[0])
		if k == "" {
			continue
		}
		out = append(out, Touch{Key: k, Show: show, Channel: "Messages", Last: appleTime(r[1]), Count: countOf(r[2]), Mine: countOf(r[3]) > 0})
	}
	return out, nil
}

// ReadCalls: the call history database (phone calls through the iPhone, and FaceTime).
func ReadCalls(home string) ([]Touch, error) {
	rows, err := sqliteRows(filepath.Join(home, "Library/Application Support/CallHistoryDB/CallHistory.storedata"),
		`SELECT CAST(ZADDRESS AS TEXT), MAX(ZDATE), COUNT(*), SUM(ZORIGINATED) FROM ZCALLRECORD WHERE ZADDRESS IS NOT NULL GROUP BY CAST(ZADDRESS AS TEXT);`)
	if err != nil {
		return nil, err
	}
	var out []Touch
	for _, r := range rows {
		if len(r) < 4 {
			continue
		}
		k, show := touchKey(r[0])
		if k == "" {
			continue
		}
		out = append(out, Touch{Key: k, Show: show, Channel: "Calls", Last: appleTime(r[1]), Count: countOf(r[2]), Mine: countOf(r[3]) > 0})
	}
	return out, nil
}

// ReadMail: Mail's "Envelope Index". People who wrote to me (sender), and people I wrote to
// (recipients of messages in a Sent mailbox).
func ReadMail(home string) ([]Touch, error) {
	dbs, _ := filepath.Glob(filepath.Join(home, "Library/Mail/V*/MailData/Envelope Index"))
	if len(dbs) == 0 {
		if _, err := os.ReadDir(filepath.Join(home, "Library/Mail")); err != nil && os.IsPermission(err) {
			return nil, ErrNoAccess
		}
		return nil, errors.New("Mail isn't set up on this Mac")
	}
	sort.Strings(dbs)
	db := dbs[len(dbs)-1] // newest Mail version
	q := `SELECT a.address, MAX(m.date_received), COUNT(*), 0 FROM messages m JOIN addresses a ON m.sender = a.ROWID GROUP BY a.address;
SELECT a.address, MAX(COALESCE(m.date_sent, m.date_received)), COUNT(*), 1 FROM recipients r JOIN messages m ON r.message = m.ROWID JOIN addresses a ON r.address = a.ROWID JOIN mailboxes b ON m.mailbox = b.ROWID WHERE b.url LIKE '%Sent%' GROUP BY a.address;`
	rows, err := sqliteRows(db, q)
	if err != nil {
		return nil, err
	}
	var out []Touch
	for _, r := range rows {
		if len(r) < 4 {
			continue
		}
		k, show := touchKey(r[0])
		if k == "" || !strings.HasPrefix(k, "e:") {
			continue
		}
		out = append(out, Touch{Key: k, Show: show, Channel: "Mail", Last: unixTime(r[1]), Count: countOf(r[2]), Mine: r[3] == "1"})
	}
	return out, nil
}

// ---------- WhatsApp chat exports ----------

// A chat export line: "[08/11/2026, 07:12:45] Khalid: hi" (iPhone) or
// "08/11/2026, 07:12 - Khalid: hi" (Android), with 12- or 24-hour times.
var waLine = regexp.MustCompile(`^\x{200e}?\[?(\d{1,2})[./-](\d{1,2})[./-](\d{2,4}),?\s+(\d{1,2}):(\d{2})(?::(\d{2}))?\s*([AaPp]\.?[Mm]\.?)?\]?\s*(?:-\s*)?([^:]{1,80}):\s`)

// IsWhatsAppExport tells a chat export from other text files.
func IsWhatsAppExport(name string, data []byte) bool {
	if strings.HasSuffix(strings.ToLower(name), ".zip") {
		return bytes.Contains(data, []byte("_chat.txt")) || strings.Contains(strings.ToLower(name), "whatsapp")
	}
	n := 0
	sc := bufio.NewScanner(bytes.NewReader(data))
	for i := 0; sc.Scan() && i < 20; i++ {
		if waLine.MatchString(sc.Text()) {
			n++
		}
	}
	return n >= 2
}

// ReadWhatsAppExport reads a chat (.txt, or the .zip WhatsApp makes) and returns each sender's
// last message. The chat's own name (from the file name) is the other person in a one-to-one chat.
func ReadWhatsAppExport(name string, data []byte) ([]Touch, string, error) {
	text := data
	if strings.HasSuffix(strings.ToLower(name), ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, "", err
		}
		found := false
		for _, f := range zr.File {
			if strings.HasSuffix(strings.ToLower(f.Name), ".txt") {
				rc, err := f.Open()
				if err != nil {
					return nil, "", err
				}
				text, _ = io.ReadAll(io.LimitReader(rc, 200<<20))
				rc.Close()
				found = true
				break
			}
		}
		if !found {
			return nil, "", errors.New("this zip has no WhatsApp chat in it")
		}
	}
	chat := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)), " (1)")
	chat = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(chat, "WhatsApp Chat - "), "WhatsApp Chat with "))

	type agg struct {
		last  time.Time
		count int
	}
	var lines [][]string
	sc := bufio.NewScanner(bytes.NewReader(text))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if m := waLine.FindStringSubmatch(sc.Text()); m != nil {
			lines = append(lines, m)
		}
	}
	if len(lines) == 0 {
		return nil, "", errors.New("this doesn't look like a WhatsApp chat export")
	}
	// Day/month order: decided by any first number over 12 (day first) or second over 12 (month first).
	dayFirst := true
	for _, m := range lines {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		if b > 12 {
			dayFirst = false
			break
		}
		if a > 12 {
			break
		}
	}
	by := map[string]*agg{}
	var order []string
	for _, m := range lines {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		if y < 100 {
			y += 2000
		}
		day, mon := a, b
		if !dayFirst {
			day, mon = b, a
		}
		h, _ := strconv.Atoi(m[4])
		mi, _ := strconv.Atoi(m[5])
		if ap := strings.ToLower(strings.ReplaceAll(m[7], ".", "")); ap == "pm" && h < 12 {
			h += 12
		} else if ap == "am" && h == 12 {
			h = 0
		}
		t := time.Date(y, time.Month(mon), day, h, mi, 0, 0, time.Local)
		who := strings.TrimSpace(strings.Trim(m[8], "‎‪‬~ "))
		if who == "" {
			continue
		}
		g := by[who]
		if g == nil {
			g = &agg{}
			by[who] = g
			order = append(order, who)
		}
		g.count++
		if t.After(g.last) {
			g.last = t
		}
	}
	var out []Touch
	for _, who := range order {
		g := by[who]
		k, show := touchKey(who)
		if k == "" {
			k, show = "n:"+fold(who), who
		}
		out = append(out, Touch{Key: k, Show: show, Channel: "WhatsApp", Last: g.last, Count: g.count})
	}
	return out, chat, nil
}

// ---------- matching history to contacts ----------

// Apply matches touches to contacts (by phone, email or name) and rebuilds the book. Touches
// that match nobody are kept as suggestions when they look like real conversations.
func (ab *ActivityBook) Apply(cs []*Contact, touches []Touch, ignored map[string]bool) {
	byKey := map[string]*Contact{}
	for _, c := range cs {
		for _, p := range c.Phones {
			if k := phoneKey(p.Value); k != "" {
				byKey[k] = c
			}
		}
		for _, e := range c.Emails {
			byKey["e:"+strings.ToLower(strings.TrimSpace(e.Value))] = c
		}
		for _, n := range []string{c.DisplayName(), c.FullName(), c.AltName, c.Nickname} {
			if strings.TrimSpace(n) != "" {
				byKey["n:"+fold(n)] = c
			}
		}
	}
	ab.People = map[string]*Activity{}
	unknown := map[string]*Touch{}
	for _, t := range touches {
		if t.Last.IsZero() {
			continue
		}
		c := byKey[t.Key]
		if c == nil {
			if ignored[t.Key] || strings.HasPrefix(t.Key, "n:") || !t.Mine || t.Count < 3 {
				continue // only people I wrote to or called, more than in passing
			}
			if u := unknown[t.Key]; u == nil || t.Last.After(u.Last) {
				tt := t
				if u != nil {
					tt.Count += u.Count
				}
				unknown[t.Key] = &tt
			} else {
				u.Count += t.Count
			}
			continue
		}
		a := ab.People[c.ID]
		if a == nil {
			a = &Activity{Channels: map[string]time.Time{}, Counts: map[string]int{}}
			ab.People[c.ID] = a
		}
		a.Counts[t.Channel] += t.Count
		if t.Last.After(a.Channels[t.Channel]) {
			a.Channels[t.Channel] = t.Last
		}
		if t.Last.After(a.Last) {
			a.Last, a.Channel = t.Last, t.Channel
		}
	}
	ab.Unknown = ab.Unknown[:0]
	for _, u := range unknown {
		ab.Unknown = append(ab.Unknown, *u)
	}
	sort.Slice(ab.Unknown, func(i, j int) bool {
		if ab.Unknown[i].Count != ab.Unknown[j].Count {
			return ab.Unknown[i].Count > ab.Unknown[j].Count
		}
		return ab.Unknown[i].Key < ab.Unknown[j].Key
	})
	if len(ab.Unknown) > 200 {
		ab.Unknown = ab.Unknown[:200]
	}
}

// LastTalked is the latest of: the activity history, a meeting in the calendar, and a call or
// message started from the app.
func LastTalked(a *Activity, met, clicked time.Time) (time.Time, string) {
	var at time.Time
	how := ""
	if a != nil {
		at, how = a.Last, a.Channel
	}
	if met.After(at) {
		at, how = met, "Calendar"
	}
	if clicked.After(at) {
		at, how = clicked, "from the app"
	}
	return at, how
}

// AddWhatsApp keeps the newest record per person from a chat export (importing the same chat
// twice doesn't double the count).
func (ab *ActivityBook) AddWhatsApp(ts []Touch) {
	for _, t := range ts {
		found := false
		for i := range ab.WhatsApp {
			if ab.WhatsApp[i].Key == t.Key {
				found = true
				if t.Last.After(ab.WhatsApp[i].Last) {
					ab.WhatsApp[i].Last = t.Last
				}
				if t.Count > ab.WhatsApp[i].Count {
					ab.WhatsApp[i].Count = t.Count
				}
			}
		}
		if !found {
			ab.WhatsApp = append(ab.WhatsApp, t)
		}
	}
}

// All is every touch known.
func (ab *ActivityBook) All() []Touch {
	return append(append([]Touch{}, ab.Touches...), ab.WhatsApp...)
}
