package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ---------- reading any file into cards ----------

// Batch is what one file (or the Mac Contacts app) brings in.
type Batch struct {
	Source  string   // suggested source name: "LinkedIn", "Google Contacts", "Notes"…
	Format  string   // what the file was recognised as, for the preview
	Cards   []Card   //
	Table   *Table   // set for spreadsheets, so the user can change the column matching
	Skipped int      // rows with nothing usable
	Notes   []string // anything the user should know
	// MatchOnly: the cards hold names only (Facebook, Snapchat). They enrich people already in
	// the book; unmatched ones are added only if the user ticks them.
	MatchOnly bool
}

// Table is a spreadsheet: a header row, data rows, and what each column means.
type Table struct {
	Header []string
	Rows   [][]string
	Cols   []ColMap
}

// ColMap says what a column holds. Kind is one of colKinds; Label refines phones, emails,
// links and addresses ("Mobile", "Work", "LinkedIn"…). Group ties Google's "Phone 1 - Type"
// to "Phone 1 - Value".
type ColMap struct {
	Kind  string
	Label string
	Group string
}

// colKinds are the choices offered in the column matcher.
var colKinds = []string{"skip", "full name", "first name", "middle name", "last name", "name (other language)",
	"nickname", "company", "job title", "phone", "email", "website", "address", "street", "city", "region",
	"postcode", "country", "birthday", "notes", "groups", "type of previous column"}

// ReadImport recognises a file by its name and content and turns it into cards.
func ReadImport(name string, data []byte, home string) (*Batch, error) {
	ext := strings.ToLower(filepath.Ext(name))
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	switch {
	case ext == ".vcf" || ext == ".vcard" || bytes.Contains(data[:min(len(data), 2048)], []byte("BEGIN:VCARD")):
		cards, err := ParseVCards(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return &Batch{Source: guessSource(base, "vCard"), Format: "vCard contacts file", Cards: cards}, nil
	case ext == ".xlsx":
		rows, err := readXLSX(data)
		if err != nil {
			return nil, fmt.Errorf("couldn't read that Excel file: %w", err)
		}
		return tableBatch(rows, base, "Excel spreadsheet", home), nil
	case ext == ".xls" || ext == ".numbers":
		return nil, errors.New("save the spreadsheet as .xlsx or .csv first (File → Export in Numbers or Excel)")
	case ext == ".json" || ext == ".js":
		return readSocialJSON(name, data)
	case ext == ".csv" || ext == ".tsv" || looksLikeTable(data):
		rows, err := readDelimited(data)
		if err != nil {
			return nil, err
		}
		return tableBatch(rows, base, "spreadsheet (CSV)", home), nil
	case ext == ".txt" || ext == ".md" || ext == ".text" || utf8.Valid(data):
		b := NotesBatch(string(data), home)
		b.Source = guessSource(base, "Notes")
		return b, nil
	}
	return nil, errors.New("this kind of file can't be imported; use .vcf, .csv, .xlsx or .txt")
}

func guessSource(base, fallback string) string {
	l := strings.ToLower(base)
	switch {
	case strings.Contains(l, "linkedin") || strings.Contains(l, "connections"):
		return "LinkedIn"
	case strings.Contains(l, "google") || strings.Contains(l, "gmail"):
		return "Google Contacts"
	case strings.Contains(l, "outlook") || strings.Contains(l, "hotmail"):
		return "Outlook"
	case strings.Contains(l, "icloud"):
		return "iCloud"
	case strings.Contains(l, "yahoo"):
		return "Yahoo"
	case strings.Contains(l, "android"):
		return "Android phone"
	case strings.Contains(l, "whatsapp"):
		return "WhatsApp"
	}
	if fallback == "vCard" {
		return "File: " + base
	}
	return fallback
}

func looksLikeTable(data []byte) bool {
	lines := strings.SplitN(string(data[:min(len(data), 4096)]), "\n", 4)
	if len(lines) < 2 {
		return false
	}
	return strings.Count(lines[0], ",") >= 2 && strings.Count(lines[1], ",") >= 2 ||
		strings.Count(lines[0], "\t") >= 2 || strings.Count(lines[0], ";") >= 2 && strings.Count(lines[1], ";") >= 2
}

func readDelimited(data []byte) ([][]string, error) {
	head := string(data[:min(len(data), 4096)])
	sep := ','
	if strings.Count(head, "\t") > strings.Count(head, ",") {
		sep = '\t'
	} else if strings.Count(head, ";") > strings.Count(head, ",") {
		sep = ';'
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = sep
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			if pe, ok := err.(*csv.ParseError); ok && pe.Err == csv.ErrFieldCount {
				continue
			}
			return nil, fmt.Errorf("couldn't read the spreadsheet: %w", err)
		}
		rows = append(rows, rec)
	}
	return rows, nil
}

// ---------- spreadsheets ----------

type headerRule struct {
	re    *regexp.Regexp
	kind  string
	label string
}

// headerRules recognise column headers in English and Arabic (Google, Outlook, LinkedIn, Yahoo,
// and hand-made sheets). Earlier rules win.
var headerRules = func() []headerRule {
	r := func(pat, kind, label string) headerRule {
		return headerRule{regexp.MustCompile(`(?i)^\s*(?:` + pat + `)\s*$`), kind, label}
	}
	return []headerRule{
		r(`(phone|e-?mail|website|address|relation|im|event|custom field|organization) \d+ - (type|label)`, "type of previous column", ""),
		r(`(first|given) ?name|firstname|first|الاسم الاول|الاسم الأول`, "first name", ""),
		r(`(middle|additional) ?name`, "middle name", ""),
		r(`(last|family|sur) ?name|surname|lastname|last|اسم العائل[ةه]|العائل[ةه]|الكني[ةه]|اللقب`, "last name", ""),
		r(`(arabic|english) name|name \((arabic|english)\)|الاسم بالعربي[ةه]?|الاسم بالانجليزي[ةه]?|الاسم بالإنجليزي[ةه]?`, "name (other language)", ""),
		r(`nick ?name|الكني[ةه] الشخصي[ةه]|اللقب الشخصي`, "nickname", ""),
		r(`(full |display |contact )?name|contact|الاسم|الاسم الكامل|اسم|الاسم الثلاثي`, "full name", ""),
		r(`job ?title|position|role|organization( 1)? - title|organization title|المسمى( الوظيفي)?|الوظيف[ةه]|المنصب`, "job title", ""),
		r(`company|organi[sz]ation( 1)?( - name)?|organization name|employer|work ?place|الشرك[ةه]|جه[ةه] العمل|المؤسس[ةه]`, "company", ""),
		r(`(e-?mail|email address|e-mail address|mail)( ?\d+)?( - value| address)?|e-mail \d+ address|البريد( الالكتروني| الإلكتروني)?|ايميل|إيميل`, "email", ""),
		r(`(mobile|cell|cellular|iphone)( ?phone)?( ?\d+)?|جوال|موبايل|نقال|الجوال|الموبايل`, "phone", "Mobile"),
		r(`whats ?app|واتساب|واتس`, "phone", "WhatsApp"),
		r(`(business|work|office|company)( ?phone)?( ?\d+)?|business phone \d+|هاتف العمل|رقم العمل`, "phone", "Work"),
		r(`home( ?phone)?( ?\d+)?|هاتف المنزل|رقم المنزل`, "phone", "Home"),
		r(`(business |home |other )?fax|فاكس`, "phone", "Fax"),
		r(`(primary |other |main )?(phone|telephone|tel|number|phone number)( ?\d+)?( - value)?|هاتف|الهاتف|رقم|الرقم|رقم الهاتف|تلفون`, "phone", ""),
		r(`linked ?in|linkedin url|profile url|url`, "website", "LinkedIn"),
		r(`instagram|انستقرام|انستغرام`, "website", "Instagram"),
		r(`twitter|x handle|x\.com`, "website", "X"),
		r(`(web ?site|web ?page|homepage|web)( \d+ - value)?|الموقع`, "website", "Website"),
		r(`(business |home |other )?street( address)?|العنوان التفصيلي`, "street", ""),
		r(`(business |home |other )?city|المدين[ةه]`, "city", ""),
		r(`(business |home |other )?(state|region|province|governorate)|المنطق[ةه]|المحافظ[ةه]`, "region", ""),
		r(`(business |home |other )?(postal ?code|zip( code)?|post ?code)|الرمز البريدي`, "postcode", ""),
		r(`(business |home |other )?country( ?/ ?region)?|الدول[ةه]|البلد`, "country", ""),
		r(`(business |home |other )?address|address \d+ - formatted|العنوان`, "address", ""),
		r(`birthday|date of birth|birth ?date|dob|تاريخ الميلاد|الميلاد`, "birthday", ""),
		r(`notes?|comments?|remarks|ملاحظات|ملاحظ[ةه]`, "notes", ""),
		r(`group membership|labels|categories|groups?|tags?|المجموع[ةه]|التصنيف|الفئ[ةه]`, "groups", ""),
		r(`connected on`, "notes", "LinkedIn connection since"),
	}
}()

// mapHeader works out each column's meaning from its header.
func mapHeader(header []string) []ColMap {
	cols := make([]ColMap, len(header))
	hasJobTitle := false
	for _, h := range header {
		if regexp.MustCompile(`(?i)job ?title`).MatchString(h) {
			hasJobTitle = true
		}
	}
	group := regexp.MustCompile(`(?i)^(.*?) ?(\d+) - `)
	for i, h := range header {
		hl := strings.TrimSpace(h)
		cols[i] = ColMap{Kind: "skip"}
		if hasJobTitle && strings.EqualFold(hl, "title") { // Outlook: "Title" is Mr./Dr.
			continue
		}
		for _, rule := range headerRules {
			if rule.re.MatchString(hl) {
				cols[i] = ColMap{Kind: rule.kind, Label: rule.label}
				break
			}
		}
		if m := group.FindStringSubmatch(hl); m != nil {
			cols[i].Group = strings.ToLower(m[1] + m[2])
		}
		l := strings.ToLower(hl)
		if cols[i].Label == "" {
			switch {
			case strings.HasPrefix(l, "business") || strings.HasPrefix(l, "work"):
				cols[i].Label = "Work"
			case strings.HasPrefix(l, "home"):
				cols[i].Label = "Home"
			case strings.HasPrefix(l, "other"):
				cols[i].Label = "Other"
			}
		}
	}
	return cols
}

// guessColumns maps a sheet without a header row by looking at the values.
func guessColumns(rows [][]string, width int) []ColMap {
	cols := make([]ColMap, width)
	nameSet := false
	for c := 0; c < width; c++ {
		emails, phones, text, n := 0, 0, 0, 0
		for _, r := range rows[:min(len(rows), 50)] {
			if c >= len(r) || strings.TrimSpace(r[c]) == "" {
				continue
			}
			v := r[c]
			n++
			switch {
			case reEmail.MatchString(v):
				emails++
			case len(digitsOnly(v)) >= 7 && len(digitsOnly(v)) <= 15 && !hasLetters(v):
				phones++
			case hasLetters(v):
				text++
			}
		}
		cols[c] = ColMap{Kind: "skip"}
		switch {
		case n == 0:
		case emails*2 > n:
			cols[c].Kind = "email"
		case phones*2 > n:
			cols[c].Kind = "phone"
		case text*2 > n && !nameSet:
			cols[c].Kind = "full name"
			nameSet = true
		case text*2 > n:
			cols[c].Kind = "notes"
		}
	}
	return cols
}

func isHeaderRow(row []string) bool {
	known := 0
	for _, h := range mapHeader(row) {
		if h.Kind != "skip" {
			known++
		}
	}
	for _, v := range row {
		if reEmail.MatchString(v) || len(digitsOnly(v)) >= 7 {
			return false
		}
	}
	return known >= 1
}

// tableBatch recognises a spreadsheet's layout (skipping LinkedIn's note lines above the header).
func tableBatch(rows [][]string, base, format, home string) *Batch {
	b := &Batch{Source: guessSource(base, "Spreadsheet"), Format: format}
	start := -1
	for i, r := range rows[:min(len(rows), 10)] {
		if isHeaderRow(r) && len(strings.Join(r, "")) > 0 {
			start = i
			break
		}
	}
	t := &Table{}
	if start >= 0 {
		t.Header = rows[start]
		t.Rows = rows[start+1:]
		t.Cols = mapHeader(t.Header)
	} else {
		width := 0
		for _, r := range rows {
			width = max(width, len(r))
		}
		t.Header = make([]string, width)
		for i := range t.Header {
			t.Header[i] = fmt.Sprintf("Column %d", i+1)
		}
		t.Rows = rows
		t.Cols = guessColumns(rows, width)
		b.Notes = append(b.Notes, "No header row — columns were guessed from their contents; check them below.")
	}
	h := strings.ToLower(strings.Join(t.Header, "|"))
	switch {
	case strings.Contains(h, "connected on") && strings.Contains(h, "url"):
		b.Format, b.Source = "LinkedIn connections", "LinkedIn"
	case strings.Contains(h, "phone 1 - value") || strings.Contains(h, "group membership") || strings.Contains(h, "given name"):
		b.Format = "Google Contacts export"
		if b.Source == "Spreadsheet" {
			b.Source = "Google Contacts"
		}
	case strings.Contains(h, "e-mail address") && strings.Contains(h, "business phone"):
		b.Format = "Outlook export"
		if b.Source == "Spreadsheet" {
			b.Source = "Outlook"
		}
	}
	b.Table = t
	b.Cards, b.Skipped = t.Cards(home)
	return b
}

// Cards turns the table rows into contacts using the current column matching.
func (t *Table) Cards(home string) ([]Card, int) {
	var cards []Card
	skipped := 0
	for _, row := range t.Rows {
		c := NewContact()
		typeOf := map[string]string{} // Google: "phone1" → "Mobile"
		for i, cm := range t.Cols {
			if cm.Kind == "type of previous column" && i < len(row) && cm.Group != "" {
				typeOf[cm.Group] = strings.Trim(strings.TrimPrefix(strings.TrimSpace(row[i]), "*"), " ")
			}
		}
		addr := Address{}
		var fullNames []string
		for i, cm := range t.Cols {
			if i >= len(row) {
				continue
			}
			v := strings.TrimSpace(row[i])
			if v == "" {
				continue
			}
			label := cm.Label
			if tl := typeOf[cm.Group]; tl != "" {
				label = niceLabel(tl, "")
			}
			switch cm.Kind {
			case "first name":
				c.First = strings.TrimSpace(c.First + " " + v)
			case "middle name":
				c.First = strings.TrimSpace(c.First + " " + v)
			case "last name":
				c.Last = v
			case "full name":
				fullNames = append(fullNames, v)
			case "name (other language)":
				c.AltName = v
			case "nickname":
				c.Nickname = v
			case "company":
				c.Company = v
			case "job title":
				c.Title = v
			case "phone":
				for _, p := range splitMulti(v) {
					if label == "" {
						label = "Mobile"
					}
					c.Phones = append(c.Phones, Field{Label: label, Value: p})
				}
			case "email":
				for _, e := range splitMulti(v) {
					if label == "" {
						label = "Other"
					}
					c.Emails = append(c.Emails, Field{Label: label, Value: e})
				}
			case "website":
				if label == "" {
					label = "Website"
				}
				if strings.Contains(strings.ToLower(v), "linkedin.com") {
					label = "LinkedIn"
				}
				c.URLs = append(c.URLs, Field{Label: label, Value: v})
			case "address":
				addr.Street = strings.TrimSpace(addr.Street + "\n" + v)
				if addr.Label == "" {
					addr.Label = label
				}
			case "street":
				addr.Street = strings.TrimSpace(addr.Street + "\n" + v)
				if addr.Label == "" {
					addr.Label = label
				}
			case "city":
				addr.City = v
			case "region":
				addr.Region = v
			case "postcode":
				addr.Postcode = v
			case "country":
				addr.Country = v
			case "birthday":
				if bd, ok := CanonicalBirthday(v); ok {
					c.Birthday = bd
				}
			case "notes":
				if label != "" {
					v = label + " " + v
				}
				c.Notes = strings.TrimSpace(c.Notes + "\n" + v)
			case "groups":
				for _, g := range regexp.MustCompile(`:::|;|,|\|`).Split(v, -1) {
					g = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(g), "*"))
					if g != "" && !strings.EqualFold(g, "myContacts") && !strings.EqualFold(g, "starred") {
						c.Groups = append(c.Groups, g)
					}
				}
			}
		}
		for i, fn := range fullNames {
			switch {
			case i == 0 && c.First == "" && c.Last == "":
				setName(c, fn)
			case c.AltName == "" && fold(fn) != fold(c.FullName()):
				c.AltName = fn // a second name column: usually the other language
			}
		}
		if !addr.IsEmpty() {
			c.Addresses = append(c.Addresses, addr)
		}
		c.Tidy()
		if c.DisplayName() == "No Name" || (c.FullName() == "" && c.Company == "" && len(c.Phones)+len(c.Emails) == 0) {
			skipped++
			continue
		}
		cards = append(cards, Card{Contact: c})
	}
	return cards, skipped
}

// splitMulti splits a cell holding several values ("9999 8888 / 2222 3333", Google's " ::: ").
func splitMulti(v string) []string {
	parts := regexp.MustCompile(`\s*(?::::|\n|;| / | \| )\s*`).Split(v, -1)
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------- notes and pasted text ----------

// NotesBatch reads free text holding several people: blocks separated by blank lines, or a
// list with one person per line ("Ali Hassan 9999 8888"). Each part goes through the same
// reader as Smart Add; only parts with a number or an email become contacts.
func NotesBatch(text, home string) *Batch {
	b := &Batch{Source: "Notes", Format: "notes / text"}
	text = strings.ReplaceAll(text, "\r", "")
	blocks := regexp.MustCompile(`\n\s*\n`).Split(text, -1)
	var parts []string
	for _, blk := range blocks {
		lines := nonEmpty(strings.Split(blk, "\n"))
		withContact := 0
		for _, l := range lines {
			if rePhone.MatchString(l) || reEmail.MatchString(l) {
				withContact++
			}
		}
		// every line has its own number/email and a name → a one-per-line list
		if len(lines) > 1 && withContact == len(lines) && linesHaveNames(lines) {
			parts = append(parts, lines...)
		} else if len(lines) > 0 {
			parts = append(parts, strings.Join(lines, "\n"))
		}
	}
	for _, p := range parts {
		c := ParseLocal(p, home)
		if len(c.Phones)+len(c.Emails) == 0 {
			b.Skipped++
			continue
		}
		cards := Card{Contact: c}
		b.Cards = append(b.Cards, cards)
	}
	return b
}

func linesHaveNames(lines []string) bool {
	for _, l := range lines {
		rest := rePhone.ReplaceAllString(reEmail.ReplaceAllString(l, ""), "")
		if !hasLetters(rest) {
			return false
		}
	}
	return true
}

// ---------- Excel .xlsx (first sheet) ----------

func readXLSX(data []byte) ([][]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	read := func(name string) ([]byte, error) {
		f := files[name]
		if f == nil {
			return nil, fmt.Errorf("%s missing", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, 200<<20))
	}
	var shared []string
	if b, err := read("xl/sharedStrings.xml"); err == nil {
		var sst struct {
			SI []struct {
				T string `xml:"t"`
				R []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"si"`
		}
		if err := xml.Unmarshal(b, &sst); err != nil {
			return nil, err
		}
		for _, si := range sst.SI {
			s := si.T
			for _, r := range si.R {
				s += r.T
			}
			shared = append(shared, s)
		}
	}
	sheet := "xl/worksheets/sheet1.xml"
	if files[sheet] == nil {
		var names []string
		for n := range files {
			if strings.HasPrefix(n, "xl/worksheets/sheet") && strings.HasSuffix(n, ".xml") {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		if len(names) == 0 {
			return nil, errors.New("no sheets found")
		}
		sheet = names[0]
	}
	b, err := read(sheet)
	if err != nil {
		return nil, err
	}
	var ws struct {
		Rows []struct {
			C []struct {
				R  string `xml:"r,attr"`
				T  string `xml:"t,attr"`
				V  string `xml:"v"`
				IS struct {
					T string `xml:"t"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(b, &ws); err != nil {
		return nil, err
	}
	var rows [][]string
	for _, r := range ws.Rows {
		var row []string
		for _, c := range r.C {
			col := colIndex(c.R)
			for len(row) < col {
				row = append(row, "")
			}
			v := c.V
			switch c.T {
			case "s":
				if i, err := strconv.Atoi(c.V); err == nil && i < len(shared) {
					v = shared[i]
				}
			case "inlineStr":
				v = c.IS.T
			case "", "n":
				// phone numbers typed into Excel become numbers, sometimes as 9.6599998888E10
				if f, err := strconv.ParseFloat(v, 64); err == nil && strings.ContainsAny(v, "Ee.") && f == float64(int64(f)) {
					v = strconv.FormatInt(int64(f), 10)
				}
			}
			row = append(row, v)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// colIndex turns "C7" into 2.
func colIndex(ref string) int {
	n := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		n = n*26 + int(r-'A'+1)
	}
	if n == 0 {
		return 0
	}
	return n - 1
}

// ---------- planning an import ----------

// What happens to each incoming card.
const (
	piNew      = iota // a person not in the book: add
	piMerge           // the same person as an existing card: add what's new to it
	piUpToDate        // the same person and nothing new to add
	piAsk             // maybe the same person (name only): the user decides
	piInFile          // repeats an earlier card of the same file: folded into it
)

// ImportItem is one incoming card and the plan for it.
type ImportItem struct {
	Card       Card
	Kind       int
	Match      *Contact   // piMerge / piUpToDate / piInFile: the card it goes into
	Candidates []*Contact // piAsk: who it might be
	Adds       []string   // what merging would add ("+965 9999 8888", "LinkedIn link"…)
	Why        string     // how the match was found
}

// PlanImport decides, for every incoming card, whether it is new, the same person as an existing
// card (matched by phone or email, with names that fit — in any language), or only possibly the
// same person (matched by name alone), which the user decides.
func PlanImport(s *Store, cards []Card, source, home string) []ImportItem {
	idx := NewDupIndex(s.Contacts, home)
	byName := map[string][]*Contact{}
	for _, c := range s.Contacts {
		for _, k := range c.nameSkeletons() {
			byName[k] = append(byName[k], c)
		}
	}
	var items []ImportItem
	fileIdx := NewDupIndex(nil, home)
	fileNames := map[string]*Contact{}
	for _, card := range cards {
		in := card.Contact
		in.Tidy()
		in.Sources = uniqueFold(append(in.Sources, source))
		it := ImportItem{Card: card}
		if m := strongMatch(idx, in); m != nil {
			it.Match, it.Why = m, "same phone, email or profile link"
			it.Adds = mergeAdds(m, in, home)
			it.Kind = piMerge
			if len(it.Adds) == 0 {
				it.Kind = piUpToDate
			}
		} else if m := strongMatch(fileIdx, in); m != nil {
			it.Kind, it.Match, it.Why = piInFile, m, "repeats a card earlier in this file"
		} else if m := fileNames[NameSkeleton(in.FullName())]; m != nil && NameSkeleton(in.FullName()) != "" && !reachable(in) {
			it.Kind, it.Match, it.Why = piInFile, m, "repeats a card earlier in this file"
		} else {
			var cands []*Contact
			seen := map[*Contact]bool{}
			for _, k := range in.nameSkeletons() {
				for _, c := range byName[k] {
					if !seen[c] {
						seen[c] = true
						cands = append(cands, c)
					}
				}
			}
			switch {
			case len(cands) == 1 && (!reachable(in) || !reachable(cands[0]) || fold(in.FullName()) == fold(cands[0].FullName())):
				// one person with this name, and nothing contradicts it
				it.Kind, it.Match, it.Why = piMerge, cands[0], "same name"
				switch {
				case hasArabic(in.FullName()) != hasArabic(cands[0].FullName()) || (cands[0].AltName != "" && fold(in.FullName()) == fold(cands[0].AltName) && hasArabic(in.FullName()) != hasArabic(cands[0].FullName())):
					it.Why = "same name in another language"
				case fold(in.FullName()) != fold(cands[0].FullName()):
					it.Why = "same name, spelled differently"
				}
				it.Adds = mergeAdds(cands[0], in, home)
				if len(it.Adds) == 0 {
					it.Kind = piUpToDate
				}
				if reachable(in) && reachable(cands[0]) {
					it.Kind, it.Candidates = piAsk, cands // both have different numbers: ask
				}
			case len(cands) > 0:
				it.Kind, it.Candidates, it.Why = piAsk, cands, "same name as someone you have"
			default:
				it.Kind = piNew
				fileIdx.Add(in)
				if k := NameSkeleton(in.FullName()); k != "" {
					fileNames[k] = in
				}
			}
		}
		items = append(items, it)
	}
	return items
}

// strongMatch: a shared phone or email with names that fit.
func strongMatch(idx *DupIndex, c *Contact) *Contact {
	for _, k := range dupKeys(c, idx.defCC) {
		if k.nameOnly {
			continue
		}
		for _, o := range idx.by[k.key] {
			if o.ID != c.ID && namesCompatible(o, c) {
				return o
			}
		}
	}
	return nil
}

// mergeAdds lists what merging in would add to existing, for the preview.
func mergeAdds(existing, in *Contact, home string) []string {
	trial := existing.Clone()
	inc := in.Clone()
	noteDifferences(trial, inc)
	Merge(trial, []*Contact{inc}, home)
	var adds []string
	if existing.Company != "" && trial.Company != existing.Company {
		adds = append(adds, "company → "+trial.Company)
	}
	if existing.Title != "" && trial.Title != existing.Title {
		adds = append(adds, "job title → "+trial.Title)
	}
	if trial.AltName != existing.AltName && trial.AltName != "" {
		adds = append(adds, "name "+trial.AltName)
	}
	for _, f := range []struct{ name, before, after string }{
		{"company", existing.Company, trial.Company}, {"job title", existing.Title, trial.Title},
		{"birthday", existing.Birthday, trial.Birthday}, {"name", existing.FullName(), trial.FullName()},
	} {
		if f.before == "" && f.after != "" {
			adds = append(adds, f.name+" "+f.after)
		}
	}
	newOnes := func(after, before []Field) []Field {
		had := map[string]bool{}
		for _, f := range before {
			had[strings.ToLower(f.Value)] = true
		}
		var out []Field
		for _, f := range after {
			if !had[strings.ToLower(f.Value)] {
				out = append(out, f)
			}
		}
		return out
	}
	for _, p := range newOnes(trial.Phones, existing.Phones) {
		adds = append(adds, p.Value)
	}
	for _, e := range newOnes(trial.Emails, existing.Emails) {
		adds = append(adds, e.Value)
	}
	for _, u := range newOnes(trial.URLs, existing.URLs) {
		adds = append(adds, u.Label+" link")
	}
	if len(trial.Addresses) > len(existing.Addresses) {
		adds = append(adds, "address")
	}
	if trial.Notes != existing.Notes {
		adds = append(adds, "notes")
	}
	return adds
}

// ApplyImport carries out the plan. choice[i] overrides item i: "merge:<id>", "new" or "skip".
// It returns the contacts that were added or changed.
func ApplyImport(s *Store, items []ImportItem, choice map[int]string, home string) []*Contact {
	var touched []*Contact
	newCard := map[*Contact]*Contact{} // incoming card → the contact it became
	for i, it := range items {
		in := it.Card.Contact
		kind, target := it.Kind, it.Match
		if ch, ok := choice[i]; ok {
			switch {
			case ch == "skip":
				continue
			case ch == "new":
				kind = piNew
			case strings.HasPrefix(ch, "merge:"):
				kind, target = piMerge, s.Get(strings.TrimPrefix(ch, "merge:"))
			}
		}
		switch kind {
		case piNew:
			s.Put(in)
			if len(it.Card.Photo) > 0 && CanDecodeImage(it.Card.Photo) {
				_ = s.SetPhoto(in, ShrinkPhoto(it.Card.Photo, 512))
			}
			newCard[in] = in
			touched = append(touched, in)
		case piInFile:
			if t := newCard[target]; t != nil {
				target = t
			}
			fallthrough
		case piMerge:
			if target == nil || s.Get(target.ID) == nil {
				continue
			}
			if !target.HasPhoto && len(it.Card.Photo) > 0 && CanDecodeImage(it.Card.Photo) {
				_ = s.SetPhoto(target, ShrinkPhoto(it.Card.Photo, 512))
			}
			noteDifferences(target, in)
			Merge(target, []*Contact{in}, home)
			s.Put(target)
			newCard[in] = target
			touched = append(touched, target)
		case piUpToDate:
			if target != nil {
				before := len(target.Sources)
				target.Sources = uniqueFold(append(target.Sources, in.Sources...))
				if len(target.Sources) != before {
					s.Put(target)
				}
			}
		}
	}
	return touched
}

// SourceNames lists every source in use, most common first.
func (s *Store) SourceNames() []string {
	count := map[string]int{}
	name := map[string]string{}
	for _, c := range s.Contacts {
		for _, src := range c.Sources {
			k := fold(src)
			count[k]++
			if name[k] == "" {
				name[k] = src
			}
		}
	}
	var out []string
	for k := range count {
		out = append(out, name[k])
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := count[fold(out[i])], count[fold(out[j])]
		if a != b {
			return a > b
		}
		return out[i] < out[j]
	})
	return out
}

var _ = json.Valid

// noteDifferences keeps information that merging would otherwise drop: when a source gives a
// different company, job title or birthday than the card has, the card keeps its own value and
// the other one is written in the notes ("LinkedIn: Head of Partnerships at Zain Group").
// LinkedIn is usually the most current for work, so its job replaces the card's, and the old job
// goes into the notes instead.
func noteDifferences(target, in *Contact) {
	src := "Also listed"
	if len(in.Sources) > 0 {
		src = in.Sources[0]
	}
	diff := func(a, b string) bool { return a != "" && b != "" && fold(a) != fold(b) }
	var lines []string
	jobOf := func(title, company string) string {
		switch {
		case title != "" && company != "":
			return title + " at " + company
		case title != "":
			return title
		}
		return company
	}
	if diff(target.Company, in.Company) || diff(target.Title, in.Title) {
		if strings.EqualFold(src, "LinkedIn") {
			old := jobOf(target.Title, target.Company)
			target.Company, target.Title = in.Company, in.Title
			if in.Title == "" {
				target.Title = ""
			}
			lines = append(lines, "Previously: "+old)
		} else {
			lines = append(lines, src+": "+jobOf(in.Title, in.Company))
		}
	}
	if diff(target.Birthday, in.Birthday) {
		lines = append(lines, src+" gives the birthday as "+BirthdayText(in.Birthday))
	}
	for _, l := range lines {
		if !strings.Contains(target.Notes, l) {
			target.Notes = strings.TrimSpace(target.Notes + "\n" + l)
		}
	}
}
