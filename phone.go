package main

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// country describes how numbers are written in one country.
type country struct {
	cc    string   // calling code without +
	name  string   // shown to the user
	trunk bool     // local numbers start with a 0 that is dropped after +cc (UK 020…, Saudi 05…)
	nsn   []int    // lengths of the number after the country code (without the trunk 0)
	tlds  []string // email/web domains that suggest the country
	words []string // address words that suggest the country (folded)
}

// countries Smart Contacts knows well; numbers from anywhere else are still accepted as written.
var countries = []country{
	{"965", "Kuwait", false, []int{8}, []string{".kw"}, []string{"kuwait", "الكويت", "salmiya", "hawally", "jahra", "farwaniya", "ahmadi", "mubarak al-kabeer", "حولي", "السالمية", "الجهراء", "الفروانية", "الاحمدي"}},
	{"966", "Saudi Arabia", true, []int{9}, []string{".sa"}, []string{"saudi", "ksa", "riyadh", "jeddah", "dammam", "khobar", "makkah", "mecca", "madinah", "السعوديه", "الرياض", "جده", "الدمام"}},
	{"971", "UAE", true, []int{8, 9}, []string{".ae"}, []string{"uae", "emirates", "dubai", "abu dhabi", "sharjah", "ajman", "الامارات", "دبي", "ابوظبي", "الشارقه"}},
	{"973", "Bahrain", false, []int{8}, []string{".bh"}, []string{"bahrain", "manama", "البحرين", "المنامه"}},
	{"974", "Qatar", false, []int{8}, []string{".qa"}, []string{"qatar", "doha", "قطر", "الدوحه"}},
	{"968", "Oman", false, []int{8}, []string{".om"}, []string{"oman", "muscat", "عمان", "مسقط"}},
	{"20", "Egypt", true, []int{9, 10}, []string{".eg"}, []string{"egypt", "cairo", "alexandria", "giza", "مصر", "القاهره", "الاسكندريه"}},
	{"962", "Jordan", true, []int{8, 9}, []string{".jo"}, []string{"jordan", "amman", "الاردن", "عمان الاردن"}},
	{"961", "Lebanon", true, []int{7, 8}, []string{".lb"}, []string{"lebanon", "beirut", "لبنان", "بيروت"}},
	{"963", "Syria", true, []int{9}, []string{".sy"}, []string{"syria", "damascus", "aleppo", "سوريا", "دمشق"}},
	{"964", "Iraq", true, []int{10}, []string{".iq"}, []string{"iraq", "baghdad", "basra", "erbil", "العراق", "بغداد", "البصره"}},
	{"98", "Iran", true, []int{10}, []string{".ir"}, []string{"iran", "tehran", "ايران", "طهران"}},
	{"90", "Turkey", true, []int{10}, []string{".tr"}, []string{"turkey", "turkiye", "istanbul", "ankara", "تركيا"}},
	{"91", "India", true, []int{10}, []string{".in"}, []string{"india", "mumbai", "delhi", "kerala", "bangalore", "chennai", "الهند"}},
	{"92", "Pakistan", true, []int{10}, []string{".pk"}, []string{"pakistan", "karachi", "lahore", "islamabad", "باكستان"}},
	{"880", "Bangladesh", true, []int{10}, []string{".bd"}, []string{"bangladesh", "dhaka", "بنغلاديش"}},
	{"94", "Sri Lanka", true, []int{9}, []string{".lk"}, []string{"sri lanka", "colombo", "سريلانكا"}},
	{"63", "Philippines", true, []int{10}, []string{".ph"}, []string{"philippines", "manila", "الفلبين"}},
	{"44", "United Kingdom", true, []int{10}, []string{".uk"}, []string{"united kingdom", "england", "london", "scotland", "manchester", "بريطانيا", "لندن"}},
	{"1", "USA / Canada", false, []int{10}, []string{".us", ".ca"}, []string{"usa", "united states", "america", "canada", "new york", "california", "toronto", "امريكا", "كندا"}},
	{"33", "France", true, []int{9}, []string{".fr"}, []string{"france", "paris", "فرنسا", "باريس"}},
	{"49", "Germany", true, []int{10, 11}, []string{".de"}, []string{"germany", "deutschland", "berlin", "munich", "المانيا"}},
	{"39", "Italy", false, []int{9, 10}, []string{".it"}, []string{"italy", "italia", "rome", "milan", "ايطاليا"}},
	{"34", "Spain", false, []int{9}, []string{".es"}, []string{"spain", "madrid", "barcelona", "اسبانيا"}},
	{"61", "Australia", true, []int{9}, []string{".au"}, []string{"australia", "sydney", "melbourne", "استراليا"}},
	{"86", "China", true, []int{11}, []string{".cn"}, []string{"china", "beijing", "shanghai", "الصين"}},
}

func countryByCC(cc string) *country {
	for i := range countries {
		if countries[i].cc == cc {
			return &countries[i]
		}
	}
	return nil
}

// splitCC finds the calling code at the start of an international number (longest match first).
func splitCC(d string) (string, string) {
	for _, n := range []int{3, 2, 1} {
		if len(d) > n {
			if countryByCC(d[:n]) != nil {
				return d[:n], d[n:]
			}
		}
	}
	// any other country libphonenumber knows (country codes are prefix-free, so the first match is it)
	for _, n := range []int{1, 2, 3} {
		if len(d) > n {
			if cc, err := strconv.Atoi(d[:n]); err == nil && phonenumbers.GetRegionCodeForCountryCode(cc) != "ZZ" {
				return d[:n], d[n:]
			}
		}
	}
	return "", d
}

// validNumber asks libphonenumber whether "+<digits>" is a real, assignable number.
func validNumber(e164 string) bool {
	n, err := phonenumbers.Parse(e164, "")
	return err == nil && phonenumbers.IsValidNumber(n)
}

// PhoneType says what kind of line a normalised number is: "mobile", "landline", "toll-free",
// "service" or "" when unknown.
func PhoneType(norm string) string {
	if !strings.HasPrefix(norm, "+") {
		return ""
	}
	n, err := phonenumbers.Parse(norm, "")
	if err != nil {
		return ""
	}
	switch phonenumbers.GetNumberType(n) {
	case phonenumbers.MOBILE:
		return "mobile"
	case phonenumbers.FIXED_LINE:
		return "landline"
	case phonenumbers.TOLL_FREE, phonenumbers.SHARED_COST, phonenumbers.PREMIUM_RATE:
		return "toll-free"
	case phonenumbers.UAN, phonenumbers.VOIP:
		return "service"
	}
	return ""
}

// invalidReason explains why libphonenumber rejects a number.
func invalidReason(e164 string, c *country) string {
	n, err := phonenumbers.Parse(e164, "")
	name := "its country"
	if c != nil {
		name = c.name
	}
	if err != nil {
		return "not a phone number"
	}
	switch phonenumbers.IsPossibleNumberWithReason(n) {
	case phonenumbers.TOO_SHORT:
		return "too short for " + name
	case phonenumbers.TOO_LONG:
		return "too long for " + name
	case phonenumbers.INVALID_LENGTH:
		return "wrong length for " + name
	}
	return "not a number that exists in " + name
}

// fits: the number is valid in this country (checked by libphonenumber).
func (c *country) fits(nsn string) bool {
	if c.cc == "965" && !kuwaitValid(nsn) {
		return false
	}
	return validNumber("+" + c.cc + nsn)
}

// kuwaitValid: Kuwaiti numbers are 8 digits starting with 2 (landline) or 4, 5, 6, 9 (mobile).
func kuwaitValid(nsn string) bool {
	return len(nsn) == 8 && strings.ContainsRune("24569", rune(nsn[0]))
}

// kuwaitService: 3–4 digit codes (112, 171, 1888) and 7-digit lines starting with 1 (1802555).
func kuwaitService(nsn string) bool {
	return (len(nsn) >= 3 && len(nsn) <= 4) || (len(nsn) == 7 && nsn[0] == '1')
}

// Phone analysis results.
const (
	phOK         = iota // Norm is a confident +<cc><number>
	phService           // a short or service number (171, 112, 1802555): keep as dialled
	phUnsure            // can't tell the country; Options holds the candidates
	phSuspicious        // probably mistyped (wrong length or prefix for its country)
	phInvalid           // not a phone number at all (no digits, 00000000, 12345678…)
)

type phoneResult struct {
	Norm    string   // best normalised form ("+965…" when confident, digits otherwise)
	Status  int      //
	Reason  string   // why, for the Clean Up list
	Options []string // for phUnsure: candidate "+cc…" numbers
}

var reExtension = regexp.MustCompile(`(?i)\s*(?:ext\.?|extension|x|#)\s*\d{1,6}\s*$`)

// analyzePhone works out what a typed number really is. home is the user's country code;
// hints are clues from the same card (address, other numbers, email domains) as calling codes.
func analyzePhone(raw, home string, hints []string) phoneResult {
	s := strings.TrimSpace(fold(raw))
	s = reExtension.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "(0)", "") // "+44 (0)20 …"
	d := digitsOnly(s)
	plus := strings.HasPrefix(strings.TrimLeft(s, " ("), "+")
	switch {
	case d == "":
		return phoneResult{Status: phInvalid, Reason: "no digits"}
	case len(d) >= 6 && strings.Count(d, d[:1]) == len(d):
		return phoneResult{Norm: d, Status: phInvalid, Reason: "the same digit repeated"}
	case strings.Contains("01234567890123", d) && len(d) >= 7:
		return phoneResult{Norm: d, Status: phInvalid, Reason: "a counting sequence, not a number"}
	case len(d) > 15:
		return phoneResult{Norm: d, Status: phInvalid, Reason: "too many digits for any phone number"}
	}

	international := plus
	if strings.HasPrefix(d, "00") { // 00 is the international prefix, the same as +
		d = strings.TrimLeft(d, "0")
		international = true
	}
	if international {
		d = strings.TrimLeft(d, "0") // "+00965…" or "+0965…"
		cc, rest := splitCC(d)
		c := countryByCC(cc)
		if c == nil {
			if cc != "" && validNumber("+"+d) {
				return phoneResult{Norm: "+" + d, Status: phOK}
			}
			if cc != "" && len(rest) > 0 && rest[0] == '0' && validNumber("+"+cc+rest[1:]) { // "+33 06…"
				return phoneResult{Norm: "+" + cc + rest[1:], Status: phOK, Reason: "extra 0 removed"}
			}
			if cc == "" {
				return phoneResult{Norm: "+" + d, Status: phSuspicious, Reason: "unknown country code"}
			}
			return phoneResult{Norm: "+" + d, Status: phSuspicious, Reason: invalidReason("+"+d, nil)}
		}
		if c.trunk && strings.HasPrefix(rest, "0") { // "+966 05…": drop the local 0
			rest = rest[1:]
		}
		if c.fits(rest) {
			return phoneResult{Norm: "+" + cc + rest, Status: phOK}
		}
		if cc == "965" && kuwaitService(rest) {
			return phoneResult{Norm: rest, Status: phService, Reason: "Kuwait service number"}
		}
		why := invalidReason("+"+cc+rest, c)
		if cc == "965" && len(rest) == 8 && !kuwaitValid(rest) {
			why = "Kuwaiti numbers start with 2, 4, 5, 6 or 9"
		}
		return phoneResult{Norm: "+" + cc + rest, Status: phSuspicious, Reason: why}
	}

	// A local number: written without + or 00.
	hc := countryByCC(home)
	if home != "" && strings.HasPrefix(d, home) && hc != nil && hc.fits(d[len(home):]) {
		return phoneResult{Norm: "+" + d, Status: phOK} // "96599998888"
	}
	if home == "965" && len(d) == 7 && !kuwaitService(d) {
		return phoneResult{Norm: "+965" + d, Status: phSuspicious, Reason: "Kuwaiti numbers have 8 digits"}
	}
	if len(d) < localDigits(home) && !strings.HasPrefix(d, "0") {
		return phoneResult{Norm: d, Status: phService, Reason: "short or service number"}
	}
	// Which countries could this be, written the local way?
	type cand struct {
		c   *country
		nsn string
	}
	var cands []cand
	for i := range countries {
		c := &countries[i]
		nsn := d
		if strings.HasPrefix(d, "0") {
			if !c.trunk && c.cc != home {
				continue
			}
			nsn = d[1:] // the local trunk 0 (or an extra 0 someone typed on a Kuwaiti number)
		} else if c.trunk {
			continue // in trunk countries a local number starts with 0
		}
		if c.fits(nsn) {
			cands = append(cands, cand{c, nsn})
		}
	}
	// The home country wins whenever it fits.
	for _, k := range cands {
		if k.c.cc == home {
			r := phoneResult{Norm: "+" + home + k.nsn, Status: phOK}
			if strings.HasPrefix(d, "0") {
				r.Reason = "extra 0 removed"
			}
			return r
		}
	}
	// Then a country the card itself points to.
	for _, h := range hints {
		for _, k := range cands {
			if k.c.cc == h {
				return phoneResult{Norm: "+" + h + k.nsn, Status: phOK, Reason: k.c.name + " number"}
			}
		}
	}
	if hc != nil && !strings.HasPrefix(d, "0") && len(d) == 8 && home == "965" {
		return phoneResult{Norm: "+965" + d, Status: phSuspicious, Reason: "Kuwaiti numbers start with 2, 4, 5, 6 or 9"}
	}
	if len(cands) == 0 {
		if hc == nil && home != "" && !strings.HasPrefix(d, "0") { // a home country not in the table
			return phoneResult{Norm: "+" + home + d, Status: phOK}
		}
		if hc != nil && hc.trunk && strings.HasPrefix(d, "0") && len(d) >= 8 {
			return phoneResult{Norm: "+" + home + d[1:], Status: phOK}
		}
		return phoneResult{Norm: d, Status: phSuspicious, Reason: "doesn't look like a number from any known country"}
	}
	r := phoneResult{Norm: d, Status: phUnsure, Reason: "couldn't tell which country"}
	for _, k := range cands {
		r.Options = append(r.Options, "+"+k.c.cc+k.nsn)
	}
	return r
}

// cardHints collects calling codes the card points to: its address, its other numbers, its email
// and web domains. The most specific clue comes first.
func cardHints(c *Contact, home string) []string {
	var out []string
	add := func(cc string) {
		for _, x := range out {
			if x == cc {
				return
			}
		}
		out = append(out, cc)
	}
	for _, a := range c.Addresses {
		txt := fold(a.Country + " " + a.City + " " + a.Region + " " + a.Street)
		for _, k := range countries {
			for _, w := range k.words {
				if strings.Contains(txt, fold(w)) {
					add(k.cc)
				}
			}
		}
	}
	for _, p := range c.Phones {
		s := strings.TrimSpace(fold(p.Value))
		d := digitsOnly(s)
		if strings.HasPrefix(s, "+") || strings.HasPrefix(d, "00") {
			if cc, _ := splitCC(strings.TrimLeft(d, "0")); cc != "" && cc != home {
				add(cc)
			}
		}
	}
	domains := []string{}
	for _, e := range c.Emails {
		if i := strings.LastIndex(e.Value, "@"); i >= 0 {
			domains = append(domains, strings.ToLower(e.Value[i:]))
		}
	}
	for _, u := range c.URLs {
		domains = append(domains, strings.ToLower(u.Value))
	}
	for _, dmn := range domains {
		for _, k := range countries {
			for _, t := range k.tlds {
				if strings.HasSuffix(strings.TrimRight(strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(dmn, "https://"), "http://"), "/", 2)[0], "."), t) {
					add(k.cc)
				}
			}
		}
	}
	return out
}

func countryName(norm string) string {
	if cc, _ := splitCC(strings.TrimPrefix(norm, "+")); cc != "" {
		if c := countryByCC(cc); c != nil {
			return c.name
		}
	}
	return ""
}

// ---------- emails ----------

var (
	reEmailValid = regexp.MustCompile(`^[a-z0-9._%+'\-]+@[a-z0-9\-]+(\.[a-z0-9\-]+)*\.[a-z]{2,}$`)
	domainTypos  = map[string]string{
		"gmial.com": "gmail.com", "gmai.com": "gmail.com", "gamil.com": "gmail.com", "gnail.com": "gmail.com",
		"gmail.co": "gmail.com", "gmaill.com": "gmail.com", "gmail.cm": "gmail.com", "g-mail.com": "gmail.com",
		"hotmial.com": "hotmail.com", "hotmai.com": "hotmail.com", "hotmil.com": "hotmail.com", "hotmail.co": "hotmail.com",
		"yaho.com": "yahoo.com", "yahooo.com": "yahoo.com", "yahoo.co": "yahoo.com",
		"outlok.com": "outlook.com", "outloo.com": "outlook.com", "outlook.co": "outlook.com",
		"iclod.com": "icloud.com", "icoud.com": "icloud.com", "icloud.co": "icloud.com",
		"live.co": "live.com", "msn.co": "msn.com",
	}
	reEmailInText = regexp.MustCompile(`(?i)[a-z0-9._%+'\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
)

// fixEmail tidies an email address. It returns the fixed address, what changed, and whether
// the result is a valid address at all.
func fixEmail(raw string) (fixed, why string, valid bool) {
	s := strings.TrimSpace(raw)
	var whys []string
	if strings.HasPrefix(strings.ToLower(s), "mailto:") {
		s = s[7:]
		whys = append(whys, "removed mailto:")
	}
	s = strings.Trim(s, " <>\"'()[],;:")
	s = strings.TrimRight(s, ".")
	if strings.ContainsAny(s, " \t") {
		s = strings.Join(strings.Fields(s), "")
		whys = append(whys, "removed spaces")
	}
	s = strings.ReplaceAll(s, "@@", "@")
	if l := strings.ToLower(s); l != s {
		s = l
		whys = append(whys, "lower case")
	}
	if i := strings.LastIndex(s, "@"); i > 0 {
		dom := s[i+1:]
		if fix, ok := domainTypos[dom]; ok {
			dom = fix
		} else if strings.HasSuffix(dom, ".con") || strings.HasSuffix(dom, ".cmo") || strings.HasSuffix(dom, ".comm") {
			dom = dom[:strings.LastIndex(dom, ".")] + ".com"
		}
		if dom != s[i+1:] {
			whys = append(whys, "fixed "+s[i+1:]+" → "+dom)
			s = s[:i+1] + dom
		}
	}
	return s, strings.Join(whys, ", "), reEmailValid.MatchString(s)
}

// localDigits is the shortest ordinary local number in a country. Kuwait, Bahrain, Qatar
// and Oman use 8 digits, so a 7-digit number there (1802555) is a service line.
func localDigits(defCC string) int {
	switch defCC {
	case "965", "973", "974", "968":
		return 8
	}
	return 7
}
