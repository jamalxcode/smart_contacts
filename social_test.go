package main

import (
	"strings"
	"testing"
)

func TestParseProfiles(t *testing.T) {
	cases := []struct{ label, value, service, handle string }{
		{"LinkedIn", "https://www.linkedin.com/in/khalidm/", "LinkedIn", "khalidm"},
		{"Website", "linkedin.com/in/laylah?trk=x", "LinkedIn", "laylah"},
		{"Other", "https://t.me/khalid_kw", "Telegram", "khalid_kw"},
		{"Telegram", "@khalid_kw", "Telegram", "khalid_kw"},
		{"Instagram", "instagram.com/noura.r", "Instagram", "noura.r"},
		{"X", "https://twitter.com/jamal", "X", "jamal"},
		{"X", "@jamal", "X", "jamal"},
		{"Facebook", "https://www.facebook.com/profile.php?id=12345", "Facebook", "12345"},
		{"Facebook", "https://m.me/ali.hassan", "Facebook", "ali.hassan"},
		{"Snapchat", "snapchat", "", ""}, // a label with no handle is not a profile
		{"TikTok", "https://www.tiktok.com/@dana", "TikTok", "dana"},
		{"Other", "https://github.com/jamalxcode", "GitHub", "jamalxcode"},
		{"Other", "skype:ali.h?call", "Skype", "ali.h"},
		{"Website", "https://sala.company", "", ""},
		{"Instagram", "https://www.instagram.com/explore/", "Instagram", ""},
	}
	for _, tc := range cases {
		p, ok := parseProfile(tc.label, tc.value)
		if tc.service == "" {
			if ok {
				t.Errorf("%q should not be a profile: %+v", tc.value, p)
			}
			continue
		}
		if !ok || p.Service != tc.service || p.Handle != tc.handle {
			t.Errorf("%q (%s): got %+v", tc.value, tc.label, p)
		}
	}
}

func TestActionsFollowTheCard(t *testing.T) {
	titles := func(as []Action, more bool) string {
		var s []string
		for _, a := range as {
			if a.More == more {
				s = append(s, a.Title)
			}
		}
		return strings.Join(s, ",")
	}
	c := &Contact{Phones: []Field{{"Work", "22223333"}, {"Mobile", "99998888"}}, Emails: []Field{{"Work", "k@boubyan.example"}},
		URLs: []Field{{"LinkedIn", "https://www.linkedin.com/in/khalidm"}, {"Telegram", "@khalid_kw"}, {"Instagram", "instagram.com/khalid"}}}
	as := c.Actions("965")
	if got := titles(as, false); got != "Call,Message,WhatsApp,Email,FaceTime,LinkedIn,Telegram,Instagram" {
		t.Errorf("main: %s", got)
	}
	if got := titles(as, true); got != "Signal (by number),FaceTime Audio,Teams chat" {
		t.Errorf("more: %s", got)
	}
	if as[0].Link != "tel:+96599998888" {
		t.Errorf("call should use the mobile: %s", as[0].Link)
	}
	for _, a := range as {
		if a.Title == "Telegram" && a.Link != "tg://resolve?domain=khalid_kw" {
			t.Errorf("telegram link %s", a.Link)
		}
		if a.Title == "Instagram" && a.Link != "https://ig.me/m/khalid" {
			t.Errorf("instagram link %s", a.Link)
		}
	}
	emailOnly := &Contact{Emails: []Field{{"Home", "a@gmail.com"}}}
	if got := titles(emailOnly.Actions("965"), false); got != "Email,FaceTime" {
		t.Errorf("email only: %s", got)
	}
	service := &Contact{Phones: []Field{{"Main", "171"}}}
	if got := titles(service.Actions("965"), false) + "|" + titles(service.Actions("965"), true); got != "Call,Message,FaceTime|" {
		t.Errorf("service number: %s", got)
	}
}
