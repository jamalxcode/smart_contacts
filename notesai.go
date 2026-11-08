package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Tidying jumbled notes with AI. Notes pasted from exports and old phones are often one long
// line of fragments, codes and repeats. JumbledNote finds them on this Mac; only with Cloud AI on,
// and after the user agrees, are those notes sent to the AI model to be rewritten readably. Every
// rewrite is shown before anything changes, and one that seems to have dropped a number, email or
// date is not ticked.

var (
	reLiteralNL = regexp.MustCompile(`\\n|\\r|\\t`)
	reHTML      = regexp.MustCompile(`(?i)</?[a-z][^>]{0,40}>|&(nbsp|amp|lt|gt|quot|#\d+);`)
	reSeparator = regexp.MustCompile(`\s*(\|\||\||;;|~{2,}|/{3,}|-{3,}|={3,}|\*{3,})\s*`)
	reCaps      = regexp.MustCompile(`\b[A-Z]{4,}(\s+[A-Z]{3,}){2,}\b`)
	reNumber    = regexp.MustCompile(`\+?\d[\d\s\-().]{5,}\d`)
	reNoteEmail = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)
	reDate      = regexp.MustCompile(`\b\d{1,4}[/.-]\d{1,2}[/.-]\d{1,4}\b`)
)

// JumbledNote says whether a note looks jumbled, and why.
func JumbledNote(s string) (bool, []string) {
	s = strings.TrimSpace(s)
	if len([]rune(s)) < 40 {
		return false, nil
	}
	var why []string
	score := 0
	if reLiteralNL.MatchString(s) {
		score += 2
		why = append(why, "export codes (\\n)")
	}
	if reHTML.MatchString(s) {
		score += 2
		why = append(why, "web code")
	}
	if n := len(reSeparator.FindAllString(s, -1)); n >= 3 {
		score++
		why = append(why, "run-together fragments")
	}
	longest := 0
	for _, l := range strings.Split(s, "\n") {
		if r := len([]rune(l)); r > longest {
			longest = r
		}
	}
	if longest > 220 && strings.Count(s, ". ")+strings.Count(s, "، ") < 2 {
		score++
		why = append(why, "one long line")
	}
	if reCaps.MatchString(s) {
		score++
		why = append(why, "shouting capitals")
	}
	// the same fragment repeated
	seen := map[string]int{}
	for _, part := range regexp.MustCompile(`[\n|;,.]+`).Split(s, -1) {
		if p := fold(strings.TrimSpace(part)); len(p) > 8 {
			seen[p]++
		}
	}
	for _, n := range seen {
		if n > 1 {
			score++
			why = append(why, "repeated text")
			break
		}
	}
	if strings.Count(s, "  ") > 4 {
		score++
		why = append(why, "odd spacing")
	}
	return score >= 2, why
}

const notesPrompt = `You tidy notes in a private address book. Rewrite the note the user sends so it reads clearly.
Rules:
- Keep EVERY fact exactly: names, phone numbers, emails, dates, places, amounts, account or ID numbers. Copy numbers digit for digit.
- Do not add, guess, explain or summarise away anything. If something is unclear, keep it as written.
- Keep the language of each part: Arabic stays Arabic, English stays English. Do not translate.
- Fix spacing, line breaks and punctuation; put related facts together; use short lines, or "- " bullets for lists.
- Remove exact repeats and export leftovers (HTML tags, \n codes, separators like ||| or ~~~).
- Reply with only the rewritten note: no title, no quotes, no commentary.`

// TidyNoteAI asks the model for a readable version of a note.
func TidyNoteAI(ctx context.Context, key, model, note, name string) (string, error) {
	out, err := aiChat(ctx, key, model, notesPrompt, "Note about "+name+":\n\n"+note, false)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(strings.Trim(strings.TrimSpace(out), "`"))
	return out, nil
}

// LostFacts lists numbers, emails and dates in the original that the rewrite no longer has.
func LostFacts(before, after string) []string {
	var lost []string
	for _, n := range reNumber.FindAllString(before, -1) {
		d := digitsOnly(n)
		if len(d) > 8 { // "+965 6000 1234" is kept if "6000 1234" is there
			d = d[len(d)-8:]
		}
		if len(d) >= 5 && !strings.Contains(digitsOnly(after), d) {
			lost = append(lost, strings.TrimSpace(n))
		}
	}
	for _, e := range reNoteEmail.FindAllString(before, -1) {
		if !strings.Contains(strings.ToLower(after), strings.ToLower(e)) {
			lost = append(lost, e)
		}
	}
	for _, d := range reDate.FindAllString(before, -1) {
		if !strings.Contains(after, d) && !strings.Contains(digitsOnly(after), digitsOnly(d)) {
			lost = append(lost, d)
		}
	}
	return lost
}

// ---------- screens ----------

type noteJob struct {
	c      *Contact
	before string
	after  string
	err    error
	lost   []string
}

// tidyNotesAI finds jumbled notes among cs (or takes them all when force is set, for a single
// card) and offers AI rewrites.
func (u *ui) tidyNotesAI(cs []*Contact, force bool) {
	var jobs []*noteJob
	for _, c := range cs {
		n := strings.TrimSpace(c.Notes)
		if n == "" {
			continue
		}
		if ok, _ := JumbledNote(n); ok || force {
			// the free, local tidy first (repeats, HTML, blank runs)
			if t, _, _ := TidyNotes(c); strings.TrimSpace(t) != "" {
				n = t
			}
			jobs = append(jobs, &noteJob{c: c, before: c.Notes})
			jobs[len(jobs)-1].after = n
		}
	}
	if len(jobs) == 0 {
		showInfo("Notes look fine", "No jumbled notes found. (Card → Tidy Up rewrites one card's note on request: choose that card and “Jumbled notes”.)", u.win)
		return
	}
	if !u.cloudAI() {
		showInfo("Cloud AI is off", fmt.Sprintf("%d %s jumbled. Tidying them with AI sends the note text to OpenRouter, so it's off unless you allow it: "+
			"Settings → Cloud AI, add your OpenRouter key and tick “Allow…”.\n\nWithout AI, Card → Tidy Up already removes repeats, web code and blank lines on this Mac.",
			len(jobs), pluralWords(len(jobs), "note looks", "notes look")), u.win)
		return
	}
	what := fmt.Sprintf("%d %s", len(jobs), pluralWords(len(jobs), "note", "notes"))
	u.confirmCB("Tidy "+what+" with AI?", "The text of "+what+" (and each person's name, for context) will be sent to "+u.cfg.AIModel+
		" through OpenRouter to be rewritten. Nothing else leaves this Mac. You'll see every rewrite before anything changes.", "Send to AI", false, func(ok bool) {
		if ok {
			u.runNoteJobs(jobs)
		}
	}, u.win)
}

func (u *ui) runNoteJobs(jobs []*noteJob) {
	bar := widget.NewProgressBar()
	status := widget.NewLabel("Rewriting notes…")
	ctx, cancel := context.WithCancel(context.Background())
	prog := newCustom("Tidying Notes", "Stop", container.NewVBox(status, bar), u.win)
	prog.SetOnClosed(cancel)
	prog.Resize(fyne.NewSize(420, 150))
	prog.Show()
	key, model := u.cfg.OpenRouterKey, u.cfg.AIModel
	go func() {
		var wg sync.WaitGroup
		var mu sync.Mutex
		done := 0
		sem := make(chan struct{}, 3) // a few at a time
		for _, j := range jobs {
			j := j
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				if ctx.Err() != nil {
					return
				}
				out, err := TidyNoteAI(ctx, key, model, j.after, j.c.DisplayName())
				mu.Lock()
				if err != nil {
					j.err = err
				} else {
					j.after = out
					j.lost = LostFacts(j.before, out)
				}
				done++
				n := done
				mu.Unlock()
				fyne.Do(func() {
					bar.SetValue(float64(n) / float64(len(jobs)))
					status.SetText(fmt.Sprintf("Rewritten %d of %d…", n, len(jobs)))
				})
			}()
		}
		wg.Wait()
		fyne.Do(func() {
			stopped := ctx.Err() != nil
			prog.Hide()
			if stopped {
				return
			}
			u.reviewNoteJobs(jobs)
		})
	}()
}

// reviewNoteJobs shows each note before and after; ticked ones are applied (undoable).
func (u *ui) reviewNoteJobs(jobs []*noteJob) {
	box := container.NewVBox()
	ticks := map[*noteJob]*widget.Check{}
	failed := 0
	for _, j := range jobs {
		if j.err != nil {
			failed++
			continue
		}
		if strings.TrimSpace(j.after) == strings.TrimSpace(j.before) {
			continue
		}
		chk := widget.NewCheck(j.c.DisplayName(), nil)
		chk.SetChecked(len(j.lost) == 0)
		ticks[j] = chk
		before := caption(j.before)
		before.Wrapping = fyne.TextWrapWord
		after := widget.NewLabel(j.after)
		after.Wrapping = fyne.TextWrapWord
		after.Selectable = true
		box.Add(chk)
		box.Add(container.NewGridWithColumns(2,
			container.NewVBox(caption("Before"), before),
			container.NewVBox(caption("After"), after)))
		if len(j.lost) > 0 {
			warn := widget.NewLabel("⚠ The rewrite seems to have lost: " + strings.Join(j.lost, ", ") + " — not ticked; check before applying.")
			warn.Importance = widget.WarningImportance
			warn.Wrapping = fyne.TextWrapWord
			box.Add(warn)
		}
		box.Add(widget.NewSeparator())
	}
	if len(ticks) == 0 {
		msg := "The AI had nothing to change."
		if failed > 0 {
			msg = fmt.Sprintf("%d %s couldn't be rewritten: %v", failed, pluralWords(failed, "note", "notes"), firstErr(jobs))
		}
		showInfo("Tidy Notes", msg, u.win)
		return
	}
	head := widget.NewLabel("Tick the rewrites to keep. You can undo this afterwards (⌘Z).")
	if failed > 0 {
		head.SetText(head.Text + fmt.Sprintf(" %d couldn't be rewritten (%v).", failed, firstErr(jobs)))
	}
	head.Wrapping = fyne.TextWrapWord
	d := newConfirm("Review Tidied Notes", "Apply", "Cancel", container.NewBorder(head, nil, nil, nil, container.NewVScroll(box)), func(ok bool) {
		if !ok {
			return
		}
		n := 0
		for j, chk := range ticks {
			if chk.Checked && u.store.Get(j.c.ID) != nil && j.c.Notes == j.before {
				j.c.Notes = j.after
				j.c.AddEvent(nowTime(), "Note tidied with AI", "Smart Contacts")
				u.store.Put(j.c)
				n++
			}
		}
		if n > 0 {
			u.save()
			u.flash(fmt.Sprintf("Tidied %d %s (⌘Z to undo)", n, pluralWords(n, "note", "notes")))
		}
	}, u.win)
	d.SetConfirmImportance(widget.HighImportance)
	d.Resize(fyne.NewSize(900, 640))
	d.Show()
}

func firstErr(jobs []*noteJob) error {
	for _, j := range jobs {
		if j.err != nil {
			return j.err
		}
	}
	return nil
}
