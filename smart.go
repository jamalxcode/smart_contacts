package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Smart Add turns pasted text (an email signature, a business card photo's text, a WhatsApp
// "contact card" message, a LinkedIn blurb) into a contact. With an OpenRouter key it asks an AI
// model; without one, or if the call fails, a local parser picks out what it safely can.

var (
	reEmail = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	reURL   = regexp.MustCompile(`(?i)\b(?:https?://|www\.)[^\s<>"]+|\b[a-z0-9\-]+\.(?:com|net|org|io|co|me|app|dev|company|ai|kw|ae|sa|uk)(?:/[^\s<>"]*)?\b`)
	rePhone = regexp.MustCompile(`(?:\+|00)?[\d٠-٩۰-۹][\d٠-٩۰-۹ ()\-.]{5,}[\d٠-٩۰-۹]`)
	reLabel = regexp.MustCompile(`(?i)^\s*(tel|phone|mob(?:ile)?|cell|m|t|p|w|work|office|fax|f|email|e-?mail|e|web|website|w|address|addr)\s*[.:]\s*`)
	// rePhoneTag finds a label written just before a number on the same line ("M: … | T: …").
	rePhoneTag = regexp.MustCompile(`(?i)\b(tel|phone|mob(?:ile)?|cell|m|t|p|w|work|office|direct|fax|f)\s*[.:]?\s*$`)
	reSepTags  = regexp.MustCompile(`(?i)\b(tel|phone|mob(?:ile)?|cell|m|t|p|w|work|office|direct|fax|f)\s*[.:]\s*(\||$)`)
	reSignOff  = regexp.MustCompile(`(?i)^(best|kind|warm)?\s*(regards|wishes)|^(thanks|thank you|cheers|sincerely|sent from my|get outlook)|^(مع التحية|تحياتي|مع خالص|شكرا)`)
)

var titleWords = []string{"manager", "director", "engineer", "ceo", "cto", "cfo", "coo", "founder", "president",
	"head", "lead", "officer", "consultant", "developer", "designer", "partner", "owner", "teacher",
	"professor", "dr.", "doctor", "lawyer", "attorney", "accountant", "analyst", "specialist",
	"coordinator", "assistant", "executive", "supervisor", "architect", "agent", "advisor", "sales",
	"مدير", "مهندس", "رئيس", "مستشار", "محامي", "دكتور", "طبيب", "مدرس", "استاذ", "أستاذ", "مؤسس"}

var companyWords = []string{"inc", "ltd", "llc", "co.", "company", "corp", "group", "bank", "w.l.l", "wll",
	"k.s.c", "ksc", "holding", "trading", "est.", "university", "school", "hospital", "clinic",
	"شركة", "مؤسسة", "بنك", "مجموعة", "جامعة", "مستشفى"}

func containsAny(s string, words []string) bool {
	l := strings.ToLower(s)
	for _, w := range words {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}

// ParseLocal extracts emails, phones and links reliably, and guesses name, title and company
// from the remaining lines. It never invents anything.
func ParseLocal(text, defCC string) *Contact {
	c := NewContact()
	var rest []string
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		label := ""
		if m := reLabel.FindStringSubmatch(line); m != nil {
			label = strings.ToLower(m[1])
			line = strings.TrimSpace(line[len(m[0]):])
		}
		used := line
		for _, e := range reEmail.FindAllString(line, -1) {
			c.Emails = append(c.Emails, Field{Label: "Work", Value: e})
			used = strings.Replace(used, e, " ", 1)
		}
		for _, u := range reURL.FindAllString(used, -1) {
			if strings.Contains(u, "@") {
				continue
			}
			lab := "Website"
			switch l := strings.ToLower(u); {
			case strings.Contains(l, "linkedin."):
				lab = "LinkedIn"
			case strings.Contains(l, "instagram."):
				lab = "Instagram"
			case strings.Contains(l, "x.com/") || strings.Contains(l, "twitter."):
				lab = "X"
			}
			c.URLs = append(c.URLs, Field{Label: lab, Value: u})
			used = strings.Replace(used, u, " ", 1)
		}
		if reSignOff.MatchString(line) {
			continue
		}
		for _, loc := range rePhone.FindAllStringIndex(used, -1) {
			p := used[loc[0]:loc[1]]
			if n := len(digitsOnly(p)); n < 7 || n > 15 {
				continue
			}
			lab := "Mobile"
			tag := label
			if m := rePhoneTag.FindStringSubmatch(used[:loc[0]]); m != nil {
				tag = strings.ToLower(m[1])
			}
			switch tag {
			case "tel", "t", "phone", "p", "work", "office", "direct", "w":
				lab = "Work"
			case "fax", "f":
				lab = "Fax"
			}
			c.Phones = append(c.Phones, Field{Label: lab, Value: FormatPhone(p, defCC)})
		}
		for _, p := range rePhone.FindAllString(used, -1) {
			if n := len(digitsOnly(p)); n >= 7 && n <= 15 {
				used = strings.Replace(used, p, " ", 1)
			}
		}
		used = reSepTags.ReplaceAllString(used, " ")
		used = strings.Trim(used, " |,;·•-–—")
		if label == "address" || label == "addr" {
			c.Addresses = append(c.Addresses, Address{Label: "Work", Street: used})
			continue
		}
		if hasLetters(used) && len([]rune(used)) <= 80 {
			rest = append(rest, used)
		}
	}
	for _, l := range rest {
		switch {
		case c.Title == "" && containsAny(l, titleWords):
			c.Title = l
		case c.Company == "" && containsAny(l, companyWords):
			c.Company = l
		case c.FullName() == "" && looksLikeName(l):
			setName(c, l)
		case c.Company == "" && c.FullName() != "" && c.Title != "":
			c.Company = l
		default:
			if c.Notes != "" {
				c.Notes += "\n"
			}
			c.Notes += l
		}
	}
	c.Tidy()
	return c
}

func hasLetters(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func looksLikeName(s string) bool {
	w := strings.Fields(s)
	if len(w) == 0 || len(w) > 5 {
		return false
	}
	for _, r := range s {
		if unicode.IsDigit(r) || r == '@' || r == '/' {
			return false
		}
	}
	return true
}

// setName splits "First Last" (the last word is the family name; Arabic names keep
// "bin/bint/al/abdul" with what follows).
func setName(c *Contact, s string) {
	w := strings.Fields(s)
	if len(w) == 1 {
		c.First = w[0]
		return
	}
	cut := len(w) - 1
	for cut > 1 {
		p := strings.ToLower(w[cut-1])
		if p == "al" || p == "el" || p == "bin" || p == "bint" || p == "ibn" || p == "abu" || p == "van" || p == "von" || p == "de" || p == "آل" || p == "بن" || p == "بنت" || p == "ابو" || p == "أبو" {
			cut--
			continue
		}
		break
	}
	c.First = strings.Join(w[:cut], " ")
	c.Last = strings.Join(w[cut:], " ")
}

// ---------- AI via OpenRouter ----------

// DefaultAIModel is used until the user picks another in Settings.
const DefaultAIModel = "anthropic/claude-haiku-4.5"

type aiContact struct {
	First     string  `json:"first"`
	Last      string  `json:"last"`
	Nickname  string  `json:"nickname"`
	Company   string  `json:"company"`
	Title     string  `json:"title"`
	Phones    []Field `json:"phones"`
	Emails    []Field `json:"emails"`
	URLs      []Field `json:"urls"`
	Address   string  `json:"address"`
	Birthday  string  `json:"birthday"`
	Notes     string  `json:"notes"`
	Confident bool    `json:"confident"`
}

const aiPrompt = `Extract ONE contact from the text the user pastes. Reply with only a JSON object:
{"first":"","last":"","nickname":"","company":"","title":"","phones":[{"label":"Mobile|Work|Home|Fax|WhatsApp|Other","value":""}],
"emails":[{"label":"Work|Home|Other","value":""}],"urls":[{"label":"Website|LinkedIn|Instagram|X|Other","value":""}],
"address":"","birthday":"YYYY-MM-DD or --MM-DD or empty","notes":"","confident":true}
Rules: copy names in the script they are written in (Arabic stays Arabic). Never invent values that are not in the text.
Phones: keep the digits exactly; add the country code +%s only if the number is clearly a local one. Put anything useful that has
no field (where you met, a referral, an assistant's name) in notes, briefly. Set confident=false if the text may hold several people.`

// ParseAI asks the model. It returns an error the UI can show; callers fall back to ParseLocal.
func ParseAI(ctx context.Context, key, model, text, defCC string) (*Contact, bool, error) {
	if strings.TrimSpace(key) == "" {
		return nil, false, errors.New("no OpenRouter key set")
	}
	if model == "" {
		model = DefaultAIModel
	}
	body, _ := json.Marshal(map[string]any{
		"model":           model,
		"temperature":     0,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": fmt.Sprintf(aiPrompt, defCC)},
			{"role": "user", "content": text},
		},
	})
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/jamalxcode/smart_contacts")
	req.Header.Set("X-Title", "Smart Contacts")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("couldn't reach OpenRouter: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != 200 {
		msg := string(raw)
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		return nil, false, fmt.Errorf("OpenRouter said %d: %s", resp.StatusCode, msg)
	}
	var out struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return nil, false, errors.New("OpenRouter sent an empty answer")
	}
	return decodeAIContact(out.Choices[0].Message.Content, defCC)
}

// decodeAIContact is separate so it can be tested without the network.
func decodeAIContact(content, defCC string) (*Contact, bool, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	if i, j := strings.Index(content, "{"), strings.LastIndex(content, "}"); i >= 0 && j > i {
		content = content[i : j+1]
	}
	var a aiContact
	if err := json.Unmarshal([]byte(content), &a); err != nil {
		return nil, false, fmt.Errorf("the AI answer wasn't readable: %v", err)
	}
	c := NewContact()
	c.First, c.Last, c.Nickname, c.Company, c.Title = a.First, a.Last, a.Nickname, a.Company, a.Title
	for _, p := range a.Phones {
		p.Value = FormatPhone(p.Value, defCC)
		c.Phones = append(c.Phones, p)
	}
	c.Emails, c.URLs, c.Notes = a.Emails, a.URLs, a.Notes
	if strings.TrimSpace(a.Address) != "" {
		c.Addresses = []Address{{Label: "Work", Street: a.Address}}
	}
	if b, ok := CanonicalBirthday(a.Birthday); ok {
		c.Birthday = b
	}
	c.Tidy()
	return c, a.Confident, nil
}
