package main

import "strings"

// Typo-tolerant search (issue #7). Search is exact first; only when an exact search finds no one
// does the list try again letting each word be a typo or two away from a word in a name, nickname,
// company or email. Each card's words are folded once and cached, so the second pass over 10,000
// cards stays well under a frame.

// typoAllowance is how many slips a query word may have: none under 4 letters (too many short
// names are one letter apart), 1 up to 7 letters, 2 from 8.
func typoAllowance(w string) int {
	switch n := len([]rune(w)); {
	case n < 4:
		return 0
	case n < 8:
		return 1
	default:
		return 2
	}
}

// fuzzyWords are the folded words a typo may hit: names in both languages, nickname, company and
// the part of each email before the @. Notes and addresses are left out: too much text to forgive.
func (c *Contact) fuzzyWords() []string {
	c.fillCache()
	return c.cacheWords
}

func (c *Contact) wordsForFuzzy() []string {
	parts := []string{c.First, c.Last, c.AltName, c.Nickname, c.Company}
	for _, e := range c.Emails {
		local, _, _ := strings.Cut(e.Value, "@")
		parts = append(parts, local)
	}
	seen := map[string]bool{}
	var out []string
	for _, w := range strings.FieldsFunc(fold(strings.Join(parts, " ")), func(r rune) bool {
		return r == ' ' || r == '-' || r == '.' || r == '_' || r == '\'' || r == ','
	}) {
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// MatchesFuzzy is Matches with typos forgiven: every query word must appear exactly, as before,
// or be within its allowance of the start of one of the card's words.
func (c *Contact) MatchesFuzzy(query string) bool {
	q := strings.Fields(fold(query))
	if len(q) == 0 {
		return true
	}
	hay := c.SearchText()
	words := c.fuzzyWords()
	for _, w := range q {
		if d := digitsOnly(w); d != "" && len(d) == len(strings.Trim(w, "+-()")) {
			if !strings.Contains(hay, d) { // numbers aren't guessed at
				return false
			}
			continue
		}
		if strings.Contains(hay, w) || c.matchesNameSkeleton(w) {
			continue
		}
		k := typoAllowance(w)
		if k == 0 {
			return false
		}
		hit := false
		for _, cw := range words {
			if prefixDistance(w, cw, k) <= k {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

// prefixDistance is the smallest number of edits (insert, delete, change, or swap two letters
// side by side) that turns q into some start of word: "mohamad" → "mohammed" is 1, "khlaid" →
// "khalid" is 1. It gives up early and returns k+1 once no start of word can be within k.
func prefixDistance(q, word string, k int) int {
	a, b := []rune(q), []rune(word)
	if len(b) < len(a)-k { // the word is too short to be within k
		return k + 1
	}
	// rows i-2, i-1 and i of the edit table: q[:i] against word[:j]
	prev2 := make([]int, len(b)+1)
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				v = min(v, prev2[j-2]+1)
			}
			cur[j] = v
			rowMin = min(rowMin, v)
		}
		if rowMin > k {
			return k + 1
		}
		prev2, prev, cur = prev, cur, prev2
	}
	best := k + 1
	for _, v := range prev { // q in full against any start of the word
		best = min(best, v)
	}
	return best
}
