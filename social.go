package main

import (
	"net/url"
	"strings"
)

// Profile is a social or messaging account found on a card, from a link or a handle.
type Profile struct {
	Service string // "LinkedIn", "Telegram", "Instagram", "X", "Facebook", "Signal", "Snapchat", "TikTok", "GitHub", "Skype", "WhatsApp"
	Handle  string // username, if one can be read from the link
	URL     string // the profile page
}

// services maps link hosts (and labels for bare handles) to a service, with how to build its page.
var services = []struct {
	name   string
	hosts  []string // host + path prefix
	labels []string // labels that mean a bare value is a handle of this service
	page   string   // %s = handle
}{
	{"LinkedIn", []string{"linkedin.com/in/"}, []string{"linkedin"}, "https://www.linkedin.com/in/%s"},
	{"Telegram", []string{"t.me/", "telegram.me/", "telegram.dog/"}, []string{"telegram"}, "https://t.me/%s"},
	{"Instagram", []string{"instagram.com/", "instagr.am/"}, []string{"instagram", "insta"}, "https://www.instagram.com/%s"},
	{"X", []string{"x.com/", "twitter.com/"}, []string{"x", "twitter"}, "https://x.com/%s"},
	{"Facebook", []string{"facebook.com/", "fb.com/", "m.me/"}, []string{"facebook", "messenger"}, "https://www.facebook.com/%s"},
	{"Snapchat", []string{"snapchat.com/add/"}, []string{"snapchat", "snap"}, "https://www.snapchat.com/add/%s"},
	{"TikTok", []string{"tiktok.com/@"}, []string{"tiktok"}, "https://www.tiktok.com/@%s"},
	{"GitHub", []string{"github.com/"}, []string{"github"}, "https://github.com/%s"},
	{"Signal", []string{"signal.me/"}, []string{"signal"}, ""},
	{"WhatsApp", []string{"wa.me/", "api.whatsapp.com/"}, nil, ""},
	{"Skype", []string{"join.skype.com/"}, []string{"skype"}, ""},
}

// pages that are site sections, not people ("instagram.com/explore")
var notHandles = map[string]bool{"explore": true, "share": true, "home": true, "login": true, "pages": true, "groups": true,
	"company": true, "school": true, "jobs": true, "feed": true, "messages": true, "intent": true, "search": true, "hashtag": true,
	"p": true, "reel": true, "status": true, "watch": true, "profile.php": true}

// Profiles lists the accounts found in a card's links (one per service and handle).
func (c *Contact) Profiles() []Profile {
	var out []Profile
	seen := map[string]bool{}
	add := func(p Profile) {
		k := strings.ToLower(p.Service + "|" + p.Handle + "|" + p.URL)
		if !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	for _, f := range c.URLs {
		if p, ok := parseProfile(f.Label, f.Value); ok {
			add(p)
		}
	}
	return out
}

// parseProfile reads one link or handle.
func parseProfile(label, value string) (Profile, bool) {
	v := strings.TrimSpace(value)
	l := strings.ToLower(v)
	if strings.HasPrefix(l, "skype:") {
		h := strings.SplitN(v[6:], "?", 2)[0]
		return Profile{Service: "Skype", Handle: h}, h != ""
	}
	bare := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(l, "https://"), "http://"), "www.")
	for _, s := range services {
		for _, h := range s.hosts {
			b := bare
			if !strings.HasPrefix(b, h) { // mobile sites: m.facebook.com, mobile.twitter.com
				b = strings.TrimPrefix(strings.TrimPrefix(b, "m."), "mobile.")
				if !strings.HasPrefix(b, h) {
					continue
				}
			}
			rest := v[len(v)-len(b)+len(h):]
			rest = strings.SplitN(strings.SplitN(rest, "?", 2)[0], "#", 2)[0]
			handle := strings.Trim(strings.SplitN(strings.Trim(rest, "/"), "/", 2)[0], "@ ")
			if s.name == "Facebook" && strings.HasPrefix(strings.ToLower(rest), "profile.php") {
				if id := queryValue(v, "id"); id != "" {
					return Profile{Service: "Facebook", Handle: id, URL: v}, true
				}
			}
			if notHandles[strings.ToLower(handle)] {
				handle = ""
			}
			page := v
			if !strings.Contains(l, "://") {
				page = "https://" + v
			}
			return Profile{Service: s.name, Handle: handle, URL: page}, true
		}
	}
	// A bare handle under a service label: "@khalid" labelled Telegram.
	if !strings.Contains(v, "/") && !strings.Contains(v, ".") || strings.HasPrefix(v, "@") {
		h := strings.TrimPrefix(v, "@")
		for _, s := range services {
			for _, lab := range s.labels {
				if strings.EqualFold(strings.TrimSpace(label), lab) && h != "" && !strings.ContainsAny(h, " ") &&
					!strings.EqualFold(h, lab) && !strings.EqualFold(h, s.name) { // "snapchat" under Snapchat is not a handle
					p := Profile{Service: s.name, Handle: h}
					if s.page != "" {
						p.URL = strings.Replace(s.page, "%s", url.PathEscape(h), 1)
					}
					return p, true
				}
			}
		}
	}
	return Profile{}, false
}

func queryValue(raw, key string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Query().Get(key)
	}
	return ""
}

// Action is one way to reach a contact: a button (or a More-menu item) that opens a link.
type Action struct {
	Title string
	Kind  string // icon: "call", "message", "whatsapp", "chat", "video", "email", "profile"
	Link  string
	More  bool // offered in the More menu (it may not work: we can't know they use that app)
}

// Actions lists how this contact can be reached, from what the card actually holds.
func (c *Contact) Actions(cc string) []Action {
	var out []Action
	// the best phone: a mobile one first
	var phone *Field // the best number to call or message: a real mobile, else one labelled Mobile, else the first
	score := func(f Field) int {
		s := 0
		if PhoneType(NormalizePhone(f.Value, cc)) == "mobile" {
			s += 2
		}
		if strings.EqualFold(f.Label, "mobile") {
			s++
		}
		return s
	}
	for i := range c.Phones {
		if phone == nil || score(c.Phones[i]) > score(*phone) {
			phone = &c.Phones[i]
		}
	}
	whatsapp := false
	if phone != nil {
		n := NormalizePhone(phone.Value, cc)
		out = append(out, Action{"Call", "call", "tel:" + n, false}, Action{"Message", "message", "sms:" + n, false})
		if CanWhatsApp(phone.Value, cc) {
			out = append(out, Action{"WhatsApp", "whatsapp", "whatsapp://send?phone=" + WhatsAppNumber(phone.Value, cc), false})
			whatsapp = true
		}
	}
	if len(c.Emails) > 0 {
		out = append(out, Action{"Email", "email", "mailto:" + c.Emails[0].Value, false})
	}
	if phone != nil {
		out = append(out, Action{"FaceTime", "video", "facetime://" + NormalizePhone(phone.Value, cc), false})
	} else if len(c.Emails) > 0 {
		out = append(out, Action{"FaceTime", "video", "facetime://" + c.Emails[0].Value, false})
	}

	has := map[string]bool{}
	for _, p := range c.Profiles() {
		has[p.Service] = true
		switch p.Service {
		case "LinkedIn":
			out = append(out, Action{"LinkedIn", "profile", p.URL, false}) // the profile page has the Message button
		case "Telegram":
			if p.Handle != "" {
				out = append(out, Action{"Telegram", "chat", "tg://resolve?domain=" + url.QueryEscape(p.Handle), false})
			} else {
				out = append(out, Action{"Telegram", "chat", p.URL, false})
			}
		case "Instagram":
			if p.Handle != "" {
				out = append(out, Action{"Instagram", "chat", "https://ig.me/m/" + url.PathEscape(p.Handle), false})
			} else {
				out = append(out, Action{"Instagram", "profile", p.URL, false})
			}
		case "Facebook":
			if p.Handle != "" {
				out = append(out, Action{"Messenger", "chat", "https://m.me/" + url.PathEscape(p.Handle), false})
			} else {
				out = append(out, Action{"Facebook", "profile", p.URL, false})
			}
		case "X":
			out = append(out, Action{"X", "profile", p.URL, false})
		case "Snapchat", "TikTok", "GitHub":
			out = append(out, Action{p.Service, "profile", p.URL, false})
		case "Signal":
			out = append(out, Action{"Signal", "chat", strings.Replace(p.URL, "https://signal.me/", "sgnl://signal.me/", 1), false})
		case "Skype":
			out = append(out, Action{"Skype", "video", "skype:" + p.Handle + "?chat", false})
		case "WhatsApp":
			if !whatsapp {
				out = append(out, Action{"WhatsApp", "whatsapp", p.URL, false})
			}
		}
	}
	// Number- and email-based apps we can't know they use: offered under More.
	if phone != nil && CanWhatsApp(phone.Value, cc) {
		n := NormalizePhone(phone.Value, cc)
		if !has["Telegram"] {
			out = append(out, Action{"Telegram (by number)", "chat", "tg://resolve?phone=" + strings.TrimPrefix(n, "+"), true})
		}
		if !has["Signal"] {
			out = append(out, Action{"Signal (by number)", "chat", "sgnl://signal.me/#p/" + n, true})
		}
		out = append(out, Action{"FaceTime Audio", "call", "facetime-audio://" + n, true})
	}
	for _, e := range c.Emails {
		dom := strings.ToLower(e.Value[strings.LastIndex(e.Value, "@")+1:])
		if dom != "gmail.com" && dom != "yahoo.com" && dom != "icloud.com" && dom != "hotmail.com" {
			out = append(out, Action{"Teams chat", "chat", "msteams:/l/chat/0/0?users=" + url.QueryEscape(e.Value), true})
			break
		}
	}
	return out
}
