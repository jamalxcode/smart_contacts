package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWhatsAppExport(t *testing.T) {
	ios := "[01/11/2026, 21:04:10] Jamal: Hi\n[02/11/2026, 9:15:03 PM] Khalid Al-Mutairi: Hello\nsecond line of a message\n[13/11/2026, 08:00:00] Khalid Al-Mutairi: ok\n"
	ts, chat, err := ReadWhatsAppExport("WhatsApp Chat - Khalid Al-Mutairi.txt", []byte(ios))
	if err != nil || chat != "Khalid Al-Mutairi" || len(ts) != 2 {
		t.Fatalf("%v %q %+v", err, chat, ts)
	}
	k := ts[1]
	if k.Show != "Khalid Al-Mutairi" || k.Count != 2 || k.Last.Day() != 13 || k.Last.Month() != 11 {
		t.Errorf("%+v", k)
	}
	android := "11/13/26, 8:01 PM - +965 9999 8888: hey\n11/14/26, 7:00 AM - Me: yo\n"
	ts, _, err = ReadWhatsAppExport("chat.txt", []byte(android))
	if err != nil || len(ts) != 2 || ts[0].Key != "p:99998888" || ts[0].Last.Month() != 11 || ts[0].Last.Day() != 13 || ts[0].Last.Hour() != 20 {
		t.Fatalf("%v %+v", err, ts)
	}
	if !IsWhatsAppExport("x.txt", []byte(ios)) || IsWhatsAppExport("x.txt", []byte("hello\nworld")) {
		t.Error("detection")
	}
}

func TestActivityApply(t *testing.T) {
	a := NewContact()
	a.First, a.Last = "Khalid", "Al-Mutairi"
	a.Phones = []Field{{Label: "Mobile", Value: "6000 1234"}}
	b := NewContact()
	b.First = "Sara"
	b.Emails = []Field{{Label: "Work", Value: "Sara@Example.com"}}
	now := time.Now()
	ab := &ActivityBook{}
	ab.Apply([]*Contact{a, b}, []Touch{
		{Key: "p:60001234", Channel: "Messages", Last: now.Add(-48 * time.Hour), Count: 10, Mine: true},
		{Key: "n:" + fold("Khalid Al-Mutairi"), Channel: "WhatsApp", Last: now.Add(-1 * time.Hour), Count: 3},
		{Key: "e:sara@example.com", Channel: "Mail", Last: now.Add(-24 * time.Hour), Count: 4, Mine: true},
		{Key: "p:55554444", Show: "+965 5555 4444", Channel: "Calls", Last: now, Count: 6, Mine: true},
		{Key: "e:news@shop.com", Channel: "Mail", Last: now, Count: 300}, // never written to: not suggested
	}, nil)
	if x := ab.People[a.ID]; x == nil || x.Channel != "WhatsApp" || x.Counts["Messages"] != 10 {
		t.Errorf("khalid %+v", x)
	}
	if x := ab.People[b.ID]; x == nil || x.Channel != "Mail" {
		t.Errorf("sara %+v", x)
	}
	if len(ab.Unknown) != 1 || ab.Unknown[0].Key != "p:55554444" {
		t.Errorf("unknown %+v", ab.Unknown)
	}
	if got := appleTime("7.3e17"); got.Year() < 2023 {
		t.Errorf("nanosecond date %v", got)
	}
}

// The database readers run against small copies of the real schemas, where sqlite3 exists
// (always on a Mac, including the build machine).
func TestReadLocalDatabases(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not installed")
	}
	home := t.TempDir()
	mk := func(rel, sql string) {
		p := filepath.Join(home, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		if out, err := exec.Command("sqlite3", p, sql).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v %s", rel, err, out)
		}
	}
	mk("Library/Messages/chat.db", `CREATE TABLE handle(ROWID INTEGER PRIMARY KEY, id TEXT);
CREATE TABLE message(ROWID INTEGER PRIMARY KEY, handle_id INTEGER, date INTEGER, is_from_me INTEGER);
INSERT INTO handle VALUES(1,'+96560001234');
INSERT INTO message VALUES(1,1,780000000000000000,1),(2,1,781000000000000000,0);`)
	mk("Library/Application Support/CallHistoryDB/CallHistory.storedata", `CREATE TABLE ZCALLRECORD(Z_PK INTEGER PRIMARY KEY, ZADDRESS BLOB, ZDATE REAL, ZORIGINATED INTEGER);
INSERT INTO ZCALLRECORD VALUES(1,CAST('+965 6000 1234' AS BLOB),781500000.5,1);`)
	mk("Library/Mail/V10/MailData/Envelope Index", `CREATE TABLE addresses(ROWID INTEGER PRIMARY KEY, address TEXT, comment TEXT);
CREATE TABLE mailboxes(ROWID INTEGER PRIMARY KEY, url TEXT);
CREATE TABLE messages(ROWID INTEGER PRIMARY KEY, sender INTEGER, date_sent INTEGER, date_received INTEGER, mailbox INTEGER);
CREATE TABLE recipients(ROWID INTEGER PRIMARY KEY, message INTEGER, address INTEGER, type INTEGER);
INSERT INTO addresses VALUES(1,'me@x.com',''),(2,'Sara@Example.com','');
INSERT INTO mailboxes VALUES(1,'imap://me/INBOX'),(2,'imap://me/Sent%20Messages');
INSERT INTO messages VALUES(1,2,1762000000,1762000100,1),(2,1,1762500000,1762500000,2);
INSERT INTO recipients VALUES(1,1,1,0),(2,2,2,0);`)
	m, err := ReadMessages(home)
	if err != nil || len(m) != 1 || m[0].Key != "p:60001234" || m[0].Count != 2 || !m[0].Mine || m[0].Last.Year() != 2025 {
		t.Errorf("messages %v %+v", err, m)
	}
	c, err := ReadCalls(home)
	if err != nil || len(c) != 1 || c[0].Key != "p:60001234" || c[0].Last.Year() != 2025 {
		t.Errorf("calls %v %+v", err, c)
	}
	ml, err := ReadMail(home)
	var sent *Touch
	for i := range ml {
		if ml[i].Mine && ml[i].Key == "e:sara@example.com" {
			sent = &ml[i]
		}
	}
	if err != nil || sent == nil || sent.Last.Unix() != 1762500000 {
		t.Errorf("mail %v %+v", err, ml)
	}
}

func TestJumbledNotes(t *testing.T) {
	bad := `MET AT GITEX DUBAI 2025 ||| works boubyan\nphone 6000 1234 ||| ~~~ prefers whatsapp ~~~ works boubyan ||| <br> son: Ahmad`
	if ok, why := JumbledNote(bad); !ok || len(why) < 2 {
		t.Errorf("should be jumbled: %v", why)
	}
	good := "Met at GITEX 2025. Works at Boubyan Bank.\nPrefers WhatsApp over calls."
	if ok, why := JumbledNote(good); ok {
		t.Errorf("tidy note flagged: %v", why)
	}
	lost := LostFacts("call +965 6000 1234 or a@b.com on 12/03/2025", "call 6000 1234 on 12/03/2025")
	if len(lost) != 1 || lost[0] != "a@b.com" {
		t.Errorf("lost %v", lost)
	}
}

func TestTidyNoteAI(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Met in Milan at Food Expo.\n- Wants a Kuwait distributor\n- Phone: +39 333 123 4567\n- Prefers email\n- Husband: Marco"}}]}`))
	}))
	defer srv.Close()
	old := aiEndpoint
	aiEndpoint = srv.URL
	defer func() { aiEndpoint = old }()
	in := "MET IN MILAN AT FOOD EXPO ||| wants kuwait distributor\\nphone +39 333 123 4567 ||| <br> husband: Marco"
	out, err := TidyNoteAI(context.Background(), "k", "m", in, "Giulia Rossi")
	if err != nil || !strings.Contains(out, "Husband: Marco") || !strings.Contains(got, "Giulia Rossi") || !strings.Contains(got, "Keep EVERY fact") {
		t.Fatalf("%v %q", err, out)
	}
	if lost := LostFacts(in, out); len(lost) != 0 {
		t.Errorf("lost %v", lost)
	}
}
