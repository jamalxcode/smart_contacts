package main

import (
	"context"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
)

// Questions in plain English in the search field: "who do I know at Gulf Bank in marketing",
// "people I met at GITEX I haven't talked to in a year", "favorites in Kuwait without an email".
//
// The app turns the question into its search filters (company:, country:, talked:>1y, no:email…)
// on this Mac, and shows what it understood under the search field. With Cloud AI on, pressing
// Return asks the AI to do the turning — and only the question is sent, never any contact.

var (
	reQuestionStart = regexp.MustCompile(`^(who|whom|which|find|show|list|people|anyone|everyone|contacts|friends)\b`)
	reTalkedNot     = regexp.MustCompile(`(?:haven'?t|have not|not|never|didn'?t)\s+(?:talked|spoken|been in touch|contacted|called)(?:\s+(?:to|with))?(?:\s+(?:in|for|since))?\s+(?:(\d+)\s+|a\s+|an\s+|the\s+last\s+)?(day|week|month|year)s?`)
	reTalkedRecent  = regexp.MustCompile(`(?:talked|spoken|been in touch)(?:\s+(?:to|with))?\s+(?:this|in the last|in the past|last)\s+(?:(\d+)\s+)?(day|week|month|year)s?`)
	reNoField       = regexp.MustCompile(`(?:with no|without|missing|no)\s+(?:an?\s+)?(phone|number|email|address|photo|picture|birthday|note)s?`)
	reHasField      = regexp.MustCompile(`with\s+(?:an?\s+)?(phone|number|email|address|photo|picture|birthday)s?`)
	reGroupIn       = regexp.MustCompile(`\bin\s+(?:my\s+|the\s+)?([\p{L}\d][\p{L}\d &'-]*?)\s+group`)
	reAt            = regexp.MustCompile(`\b(?:works?\s+|working\s+)?at\s+([\p{L}\d][\p{L}\d &.'-]*?)(?:\s+(?:in|who|that|from|with|and|i)\b|$)`)
	reMet           = regexp.MustCompile(`\b(?:met|meet)\s+(?:at|in|during|through|via)\s+([\p{L}\d][\p{L}\d &.'-]*?)(?:\s+(?:who|that|and|i)\b|$)`)
	reIn            = regexp.MustCompile(`\bin\s+([\p{L}][\p{L} -]*?)(?:\s+(?:who|that|at|from|with|and|i)\b|$)`)
	reFrom          = regexp.MustCompile(`\b(?:from|on|via)\s+([\p{L}\d][\p{L}\d .'-]*?)(?:\s+(?:who|that|in|at|with|and|i)\b|$)`)
	askStop         = map[string]bool{"who": true, "whom": true, "which": true, "do": true, "does": true, "did": true, "i": true, "know": true,
		"people": true, "person": true, "the": true, "a": true, "an": true, "my": true, "me": true, "show": true, "find": true, "list": true,
		"anyone": true, "everyone": true, "all": true, "are": true, "is": true, "works": true, "work": true, "working": true,
		"with": true, "and": true, "of": true, "to": true, "contacts": true, "friends": true, "have": true, "has": true, "there": true,
		"any": true, "that": true, "whose": true, "on": true, "for": true, "ones": true, "someone": true, "somebody": true}
)

// looksLikeQuestion: a sentence rather than a few search words.
func looksLikeQuestion(q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if strings.HasSuffix(q, "?") {
		return true
	}
	return len(strings.Fields(q)) >= 3 && reQuestionStart.MatchString(q)
}

// QuestionToQuery turns a plain-English question into the app's search filters. sources are
// the names cards came from ("LinkedIn"…), to tell "from LinkedIn" from "from Boubyan".
func QuestionToQuery(q string, sources []string) string {
	s := " " + strings.ToLower(strings.Trim(strings.TrimSpace(q), "?.!")) + " "
	var out []string
	quote := func(v string) string {
		v = strings.TrimSpace(v)
		if strings.Contains(v, " ") {
			return `"` + v + `"`
		}
		return v
	}
	unit := map[string]string{"day": "d", "week": "w", "month": "m", "year": "y"}
	take := func(re *regexp.Regexp, f func(m []string)) {
		for {
			loc := re.FindStringSubmatchIndex(s)
			if loc == nil {
				return
			}
			m := make([]string, len(loc)/2)
			for i := range m {
				if loc[2*i] >= 0 {
					m[i] = s[loc[2*i]:loc[2*i+1]]
				}
			}
			f(m)
			s = s[:loc[0]] + " " + s[loc[1]:]
		}
	}
	take(reTalkedNot, func(m []string) {
		n := m[1]
		if n == "" {
			n = "1"
		}
		out = append(out, "talked:>"+n+unit[m[2]])
	})
	take(reTalkedRecent, func(m []string) {
		n := m[1]
		if n == "" {
			n = "1"
		}
		out = append(out, "talked:<"+n+unit[m[2]])
	})
	field := func(f string) string {
		switch f {
		case "number":
			return "phone"
		case "picture":
			return "photo"
		}
		return f
	}
	take(reNoField, func(m []string) { out = append(out, "no:"+field(m[1])) })
	take(reHasField, func(m []string) { out = append(out, "has:"+field(m[1])) })
	if strings.Contains(s, "favorite") || strings.Contains(s, "favourite") {
		out = append(out, "is:favorite")
		s = strings.NewReplacer("favorites", " ", "favourites", " ", "favorite", " ", "favourite", " ").Replace(s)
	}
	if strings.Contains(s, "birthday") {
		out = append(out, "has:birthday")
		s = strings.NewReplacer("birthdays", " ", "birthday", " ", "this month", " ", "this week", " ").Replace(s)
	}
	for _, p := range []string{" due ", " should call ", " need to call ", " reach out "} {
		if strings.Contains(s, p) {
			out = append(out, "is:due")
			s = strings.Replace(s, p, " ", 1)
		}
	}
	take(reGroupIn, func(m []string) { out = append(out, "group:"+quote(m[1])) })
	take(reMet, func(m []string) { out = append(out, quote(m[1])) }) // the words of how you met
	take(reFrom, func(m []string) {
		for _, src := range sources {
			if fold(src) == fold(strings.TrimSpace(m[1])) || strings.HasPrefix(fold(src), fold(strings.TrimSpace(m[1]))) {
				out = append(out, "source:"+quote(src))
				return
			}
		}
		out = append(out, "company:"+quote(m[1]))
	})
	take(reAt, func(m []string) { out = append(out, "company:"+quote(m[1])) })
	take(reIn, func(m []string) {
		if countryCodeFor(strings.TrimSpace(m[1])) != "" {
			out = append(out, "country:"+quote(m[1]))
		} else {
			out = append(out, quote(m[1])) // a city, a department, a job — matched as words
		}
	})
	for _, w := range strings.Fields(s) { // anything left that isn't a little word
		w = strings.Trim(w, ",;:'\"")
		if w == "" || askStop[w] || strings.HasPrefix(w, "i'") || w == "haven't" || w == "talked" {
			continue
		}
		if len(w) > 4 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") { // engineers → engineer
			w = strings.TrimSuffix(w, "s")
		}
		out = append(out, w)
	}
	return strings.Join(out, " ")
}

const askPrompt = `You turn a question about someone's address book into a search query for their contacts app.
Use only these filters, separated by spaces, and plain words for anything else:
company:<name>  title:<words>  group:<name>  source:<where the card came from, e.g. linkedin>  city:<city>  country:<country>
has:<phone|email|photo|birthday|address|link|note|card|reminder|met>  no:<same list>
talked:<30d (in touch in the last 30 days; d w m y)  talked:>1y (not in touch for a year)
is:favorite  is:due (time to get in touch)
Put values with spaces in double quotes. Reply with the query only, nothing else.`

// askAI turns the question into a query with the AI. Only the question is sent.
func (u *ui) askAI(question string) {
	q := question
	u.flash("Asking the AI to read your question (only the question is sent)…")
	go func() {
		out, err := aiChat(context.Background(), u.cfg.OpenRouterKey, u.cfg.AIModel, askPrompt, q, false)
		fyne.Do(func() {
			if err != nil {
				showErr(err, u.win)
				return
			}
			out = strings.TrimSpace(strings.Trim(strings.TrimSpace(out), "`"))
			if u.search.Text != q { // they've typed something else since
				return
			}
			u.askFor, u.askQuery = q, out
			u.refreshList()
		})
	}()
}

// searchQuery is what the list is filtered by: the search text, or what a question means.
func (u *ui) searchQuery() string {
	q := u.search.Text
	if u.askHint != nil {
		u.askHint.Hide()
	}
	if !looksLikeQuestion(q) {
		return q
	}
	meant := QuestionToQuery(q, u.store.SourceNames())
	if u.askFor == q && u.askQuery != "" {
		meant = u.askQuery
	}
	if u.askHint != nil {
		how := "Understood as: " + meant
		if u.cloudAI() && u.askFor != q {
			how += "   ·   Return asks the AI"
		}
		u.askHint.SetText(how)
		u.askHint.Show()
	}
	return meant
}
