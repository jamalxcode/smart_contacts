package main

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

const sampleICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
	"BEGIN:VEVENT\r\nUID:a1\r\nDTSTART:20261020T070000Z\r\nSUMMARY:Project review\r\n" +
	"ATTENDEE;CN=\"Khalid Al-Mutairi\";PARTSTAT=ACCEPTED:mailto:khalid.m@example.com\r\n" +
	"ORGANIZER;CN=Me:mailto:me@example.com\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:a2\r\nDTSTART;TZID=Asia/Kuwait:20261101T130000\r\nSUMMARY:Lunch with محمد المطيري\r\nLOCATION:Avenues\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:a3\r\nDTSTART;VALUE=DATE:20261225\r\nSUMMARY:Ali's birthday party\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:a4\r\nDTSTART:20260901T090000Z\r\nSUMMARY:Call\r\nDESCRIPTION:Follow up with Fatima Al-Sabah\\nabout the loan\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestICSAndMatching(t *testing.T) {
	evs, err := ParseICS(strings.NewReader(sampleICS))
	if err != nil || len(evs) != 4 {
		t.Fatalf("%v %d", err, len(evs))
	}
	if !evs[0].AllDay || evs[0].Title != "Ali's birthday party" {
		t.Errorf("newest first, all-day: %+v", evs[0])
	}
	if evs[1].Start.UTC().Hour() != 10 { // 13:00 Kuwait = 10:00 UTC
		t.Errorf("TZID: %v", evs[1].Start.UTC())
	}
	cal := &Calendar{}
	cal.Merge(evs, "test.ics")
	cal.Merge(evs, "test.ics") // reading twice doesn't double up
	if len(cal.Events) != 4 {
		t.Errorf("merge doubled: %d", len(cal.Events))
	}

	khalid := &Contact{First: "Khalid", Last: "Al-Mutairi", Emails: []Field{{"Work", "khalid.m@example.com"}}}
	ms := cal.EventsFor(khalid)
	if len(ms) != 1 || ms[0].How != "attendee" {
		t.Errorf("khalid: %+v", ms)
	}
	mo := &Contact{First: "Mohammed", Last: "Al-Mutairi"}
	if ms := cal.EventsFor(mo); len(ms) != 1 || ms[0].How != "mentioned" || ms[0].Event.Location != "Avenues" {
		t.Errorf("Arabic mention of an English-named contact: %+v", ms)
	}
	fatima := &Contact{First: "فاطمة", Last: "الصباح", AltName: "Fatima Al-Sabah"}
	if ms := cal.EventsFor(fatima); len(ms) != 1 {
		t.Errorf("mention in notes via the other-language name: %+v", ms)
	}
	ali := &Contact{First: "Ali"} // first name only: never matched
	if ms := cal.EventsFor(ali); len(ms) != 0 {
		t.Errorf("first name alone must not match: %+v", ms)
	}
	now := time.Date(2026, 11, 8, 6, 0, 0, 0, time.UTC)
	if lm := LastMet(cal.EventsFor(khalid), now); lm.IsZero() || lm.Month() != 10 {
		t.Errorf("last met %v", lm)
	}
}

func TestParseCalendarOutput(t *testing.T) {
	out := "u1\x1fBoard meeting\x1f2026-11-03T09:30:00\x1f0\x1fWork\x1fHQ\x1fkhalid.m@example.com|Khalid;mailto:me@x.com|Me;\x1fagenda\nline 2" +
		"\x1eu2\x1fHoliday\x1f2026-12-01T00:00:00\x1f1\x1fHome\x1f\x1f\x1f\n"
	evs := parseCalendarOutput(out)
	if len(evs) != 2 || evs[0].Title != "Board meeting" || len(evs[0].Attendees) != 2 || evs[0].Attendees[1].Email != "me@x.com" ||
		evs[0].Notes != "agenda\nline 2" || !evs[1].AllDay || evs[0].Start.Minute() != 30 {
		t.Errorf("%+v", evs)
	}
}

func TestCalendarSaveLoad(t *testing.T) {
	dir := t.TempDir()
	c := &Calendar{}
	c.Merge([]CalEvent{{UID: "x", Title: "T", Start: time.Now()}}, "Mac Calendar")
	if err := c.Save(dir); err != nil {
		t.Fatal(err)
	}
	if got := LoadCalendar(dir); len(got.Events) != 1 || got.From != "Mac Calendar" {
		t.Errorf("%+v", got)
	}
	ForgetCalendar(dir)
	if len(LoadCalendar(dir).Events) != 0 {
		t.Error("forget")
	}
}

func TestCardImages(t *testing.T) {
	s, _ := OpenStore(t.TempDir())
	c := NewContact()
	c.First = "Card"
	s.Put(c)
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 60, 40))
	_ = png.Encode(&buf, img)
	if err := s.AddCardImage(c, buf.Bytes(), "added this contact"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCardImage(c, []byte("not an image"), ""); err == nil {
		t.Error("junk should be refused")
	}
	if len(c.CardImages) != 1 {
		t.Fatalf("%+v", c.CardImages)
	}
	path := s.CardImagePath(c.CardImages[0].File)
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("card file: %v %v", err, fi)
	}
	_ = s.Save()
	s2, _ := OpenStore(s.Dir())
	if got := s2.Get(c.ID); len(got.CardImages) != 1 {
		t.Error("card images must persist")
	}
	other := NewContact()
	other.First = "Other"
	s.Put(other)
	Merge(other, []*Contact{c}, "965")
	if len(other.CardImages) != 1 {
		t.Error("merge keeps the card image")
	}
	s.Delete(c.ID)
	s.EmptyTrash()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("emptying the trash deletes the card image file")
	}
	if !IsImageFile("IMG_0042.HEIC") || IsImageFile("contacts.csv") {
		t.Error("IsImageFile")
	}
	if _, err := ReadCardText(buf.Bytes()); err == nil && runtime.GOOS != "darwin" {
		t.Error("OCR needs macOS")
	}
}
