package main

import (
	"strings"
	"unicode"
)

// Names across languages. A contact can carry its name in two scripts: the main name (First/Last)
// and AltName, the same person's name in the other language (محمد المطيري / Mohammed Al-Mutairi).
//
// Matching uses a "skeleton": consonants only, with Arabic letters and their usual English
// spellings mapped to the same symbol, the article "Al-/El-/ال" dropped and doubled letters
// collapsed. Mohammed, Muhammad, Mohamed and محمد all become "mhmd".

// hasArabic reports whether s contains Arabic letters.
func hasArabic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Arabic, r) && unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

var arabicSkel = map[rune]string{
	'ب': "b", 'ت': "t", 'ط': "t", 'ث': "t", 'ج': "j", 'ح': "h", 'ه': "h", 'خ': "x",
	'د': "d", 'ذ': "z", 'ر': "r", 'ز': "z", 'س': "s", 'ص': "s", 'ش': "S", 'ض': "z",
	'ظ': "z", 'غ': "g", 'ف': "f", 'ق': "k", 'ك': "k", 'ک': "k", 'ل': "l", 'م': "m",
	'ن': "n", 'پ': "b", 'چ': "S", 'گ': "g", 'ڤ': "f",
	// vowels and weak letters carry no consonant: ا أ إ آ ء ع ئ ؤ و ي ى ی ة
}

var englishDigraphs = strings.NewReplacer(
	"kh", "x", "gh", "g", "sh", "S", "ch", "S", "th", "t", "dh", "z", "ph", "f",
	"ck", "k", "q", "k", "c", "k", "ou", "", "ee", "", "oo", "",
)

// wordSkeleton reduces one name word to its consonant skeleton.
func wordSkeleton(w string) string {
	w = strings.TrimSpace(w)
	var b strings.Builder
	if hasArabic(w) {
		w = arabicFold.Replace(w)
		w = strings.Replace(w, "عبدال", "عبد", 1) // عبدالرحمن = عبد الرحمن = Abdul Rahman
		rs := []rune(w)
		if len(rs) > 3 && rs[0] == 'ا' && rs[1] == 'ل' { // the article ال
			rs = rs[2:]
		}
		for _, r := range rs {
			b.WriteString(arabicSkel[r])
		}
	} else {
		w = strings.ToLower(fold(w))
		w = strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) })
		for _, p := range []string{"al-", "el-", "al ", "el "} {
			if strings.HasPrefix(w, p) {
				w = w[len(p):]
			}
		}
		if w == "al" || w == "el" {
			return ""
		}
		for _, p := range []string{"abdul", "abdel", "abdal"} { // Abdulrahman = Abdul Rahman
			if strings.HasPrefix(w, p) {
				w = "abd" + w[len(p):]
			}
		}
		w = englishDigraphs.Replace(w)
		for _, r := range w {
			if strings.ContainsRune("aeiouyw'-", r) || !unicode.IsLetter(r) {
				continue
			}
			b.WriteRune(r)
		}
	}
	// collapse doubled letters (Hassan = Hasan)
	out := []rune{}
	for _, r := range b.String() {
		if len(out) == 0 || out[len(out)-1] != r {
			out = append(out, r)
		}
	}
	// a final h is usually silent: Fatimah = Fatima = فاطمة, Abdullah = عبدالله
	if len(out) > 2 && out[len(out)-1] == 'h' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// NameSkeleton is the language-neutral key for a full name.
func NameSkeleton(name string) string {
	var parts []string
	for _, w := range strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '-' || r == '_' }) {
		if s := wordSkeleton(w); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "")
}

// nameSkeletons gives the skeletons of a contact's names in both languages.
func (c *Contact) nameSkeletons() []string {
	var out []string
	for _, n := range []string{c.FullName(), c.AltName} {
		if s := NameSkeleton(n); len(s) >= 3 {
			out = append(out, s)
		}
	}
	return out
}

// firstSkeleton is the skeleton of the first word of a name.
func firstSkeleton(name string) string {
	for _, w := range strings.Fields(strings.ReplaceAll(name, "-", " ")) {
		if s := wordSkeleton(w); s != "" {
			return s
		}
	}
	return ""
}

// sameNameAcrossScripts: the two cards' names agree in some language (full name, or one
// name contained in the other: "Khalid" and "خالد المطيري").
func sameNameAcrossScripts(a, b *Contact) bool {
	for _, x := range a.nameSkeletons() {
		for _, y := range b.nameSkeletons() {
			if x == y || strings.HasPrefix(x, y) || strings.HasPrefix(y, x) {
				return true
			}
		}
	}
	fa := []string{firstSkeleton(a.FullName()), firstSkeleton(a.AltName)}
	fb := []string{firstSkeleton(b.FullName()), firstSkeleton(b.AltName)}
	for _, x := range fa {
		for _, y := range fb {
			if x != "" && x == y && len(x) >= 2 {
				return true
			}
		}
	}
	return false
}

// ---------- spelling suggestions ----------

// nameDict pairs common Arabic names with their usual English spelling. Suggestions are only
// offered when every word of a name is known, and never applied without the user.
// Some names share a skeleton (Mohammed/Mahmoud, Hassan/Hussain); for those, only an exact listed
// spelling gives a suggestion, and a name alone never merges two cards without the user.
// nameVariants are other common English spellings; they are recognised but never suggested.
var nameVariants = [][2]string{
	{"محمد", "Muhammad"}, {"محمد", "Mohamed"}, {"محمد", "Mohammad"}, {"أحمد", "Ahmad"},
	{"يوسف", "Yusuf"}, {"يوسف", "Youssef"}, {"خالد", "Khaled"}, {"فاطمة", "Fatimah"}, {"نورة", "Nora"},
	{"عبدالله", "Abdallah"}, {"حسين", "Hussein"}, {"عمر", "Umar"}, {"مريم", "Mariam"}, {"عائشة", "Aysha"},
}

var nameDict = [][2]string{
	{"محمد", "Mohammed"}, {"أحمد", "Ahmed"}, {"محمود", "Mahmoud"}, {"مصطفى", "Mustafa"}, {"علي", "Ali"},
	{"حسن", "Hassan"}, {"حسين", "Hussain"}, {"عبدالله", "Abdullah"}, {"عبدالرحمن", "Abdulrahman"},
	{"عبدالعزيز", "Abdulaziz"}, {"عبدالرزاق", "Abdulrazzaq"}, {"عبداللطيف", "Abdullatif"}, {"عبدالمحسن", "Abdulmohsen"},
	{"عبدالوهاب", "Abdulwahab"}, {"عبدالكريم", "Abdulkarim"}, {"عبدالحميد", "Abdulhamid"}, {"عبدالمجيد", "Abdulmajeed"},
	{"عبدالسلام", "Abdulsalam"}, {"عبدالرحيم", "Abdulrahim"}, {"عبدالهادي", "Abdulhadi"}, {"عبدالناصر", "Abdulnasser"},
	{"خالد", "Khalid"}, {"فهد", "Fahad"}, {"فيصل", "Faisal"}, {"سعود", "Saud"}, {"سعد", "Saad"}, {"سالم", "Salem"},
	{"سلمان", "Salman"}, {"سليمان", "Sulaiman"}, {"صالح", "Saleh"}, {"ناصر", "Nasser"}, {"منصور", "Mansour"},
	{"يوسف", "Yousef"}, {"يعقوب", "Yaqoub"}, {"ابراهيم", "Ibrahim"}, {"إبراهيم", "Ibrahim"}, {"إسماعيل", "Ismail"},
	{"اسماعيل", "Ismail"}, {"عمر", "Omar"}, {"عثمان", "Othman"}, {"طلال", "Talal"}, {"بدر", "Bader"}, {"مشعل", "Mishal"},
	{"مشاري", "Mishari"}, {"نواف", "Nawaf"}, {"تركي", "Turki"}, {"جاسم", "Jassim"}, {"قاسم", "Qasim"}, {"حمد", "Hamad"},
	{"حمود", "Hamoud"}, {"راشد", "Rashid"}, {"ماجد", "Majed"}, {"وليد", "Waleed"}, {"خليل", "Khalil"}, {"جمال", "Jamal"},
	{"كمال", "Kamal"}, {"عادل", "Adel"}, {"عماد", "Emad"}, {"أنور", "Anwar"}, {"انور", "Anwar"}, {"مبارك", "Mubarak"},
	{"ضاري", "Dhari"}, {"ظاهر", "Dhaher"}, {"شاهين", "Shaheen"}, {"زياد", "Ziad"}, {"طارق", "Tariq"}, {"عيسى", "Eisa"},
	{"موسى", "Mousa"}, {"هاني", "Hani"}, {"هشام", "Hisham"}, {"وائل", "Wael"}, {"سامي", "Sami"}, {"رامي", "Rami"},
	{"مازن", "Mazen"}, {"نبيل", "Nabil"}, {"ياسر", "Yasser"}, {"بشار", "Bashar"}, {"غازي", "Ghazi"}, {"عبيد", "Obaid"},
	{"فاطمة", "Fatima"}, {"عائشة", "Aisha"}, {"مريم", "Maryam"}, {"نورة", "Noura"}, {"نور", "Noor"}, {"سارة", "Sara"},
	{"ساره", "Sara"}, {"هند", "Hind"}, {"منى", "Mona"}, {"ليلى", "Layla"}, {"زينب", "Zainab"}, {"خديجة", "Khadija"},
	{"أمل", "Amal"}, {"امل", "Amal"}, {"دانة", "Dana"}, {"شيخة", "Sheikha"}, {"لولوة", "Lulwa"}, {"حصة", "Hessa"},
	{"دلال", "Dalal"}, {"أسماء", "Asma"}, {"اسماء", "Asma"}, {"هيا", "Haya"}, {"رنا", "Rana"}, {"ريم", "Reem"},
	{"لطيفة", "Latifa"}, {"شهد", "Shahad"}, {"جنى", "Jana"}, {"رهف", "Rahaf"}, {"غدير", "Ghadeer"}, {"بشاير", "Bashayer"},
	{"العنزي", "Al-Enezi"}, {"المطيري", "Al-Mutairi"}, {"العجمي", "Al-Ajmi"}, {"الرشيدي", "Al-Rashidi"},
	{"الشمري", "Al-Shammari"}, {"الهاجري", "Al-Hajri"}, {"العتيبي", "Al-Otaibi"}, {"العازمي", "Al-Azmi"},
	{"الكندري", "Al-Kandari"}, {"العوضي", "Al-Awadhi"}, {"الملا", "Al-Mulla"}, {"الغانم", "Al-Ghanim"},
	{"الصالح", "Al-Saleh"}, {"البحر", "Al-Bahar"}, {"الخرافي", "Al-Kharafi"}, {"الرومي", "Al-Roumi"},
	{"الفضلي", "Al-Fadhli"}, {"الدوسري", "Al-Dosari"}, {"الحربي", "Al-Harbi"}, {"القحطاني", "Al-Qahtani"},
	{"الخالدي", "Al-Khaldi"}, {"المطوع", "Al-Mutawa"}, {"بهبهاني", "Behbehani"}, {"المرزوق", "Al-Marzouq"},
	{"الحمد", "Al-Hamad"}, {"العلي", "Al-Ali"}, {"دشتي", "Dashti"}, {"النجار", "Al-Najjar"}, {"الصباح", "Al-Sabah"},
	{"الرشيد", "Al-Rasheed"}, {"السعيد", "Al-Saeed"}, {"الشطي", "Al-Shatti"}, {"الفارس", "Al-Fares"},
	{"البدر", "Al-Bader"}, {"الصقر", "Al-Saqer"}, {"الغريب", "Al-Gharib"}, {"الجاسم", "Al-Jassim"},
	{"الحساوي", "Al-Hasawi"}, {"الدويسان", "Al-Duwaisan"}, {"الشرهان", "Al-Sharhan"}, {"حسن", "Hassan"},
	{"عبد", "Abdul"}, {"بن", "bin"}, {"بنت", "bint"}, {"أبو", "Abu"}, {"ابو", "Abu"}, {"أم", "Um"}, {"ام", "Um"},
}

var (
	dictAr2En = map[string]string{}   // folded Arabic → English
	dictEn2Ar = map[string]string{}   // lower-case English spelling → Arabic
	dictSkel  = map[string][]string{} // skeleton → Arabic spellings (for English variants)
)

func init() {
	for i, p := range append(append([][2]string{}, nameDict...), nameVariants...) {
		ar, en := p[0], p[1]
		if _, ok := dictAr2En[fold(ar)]; !ok && i < len(nameDict) {
			dictAr2En[fold(ar)] = en
		}
		dictEn2Ar[strings.ToLower(en)] = ar
		k := wordSkeleton(en)
		found := false
		for _, x := range dictSkel[k] {
			found = found || fold(x) == fold(ar)
		}
		if !found {
			dictSkel[k] = append(dictSkel[k], ar)
		}
	}
}

// SuggestOtherName proposes the name in the other script (Arabic → English or English → Arabic)
// when every word is a known name. It returns "" when it can't be sure.
func SuggestOtherName(name string) string {
	words := strings.Fields(name)
	if len(words) == 0 {
		return ""
	}
	var out []string
	if hasArabic(name) {
		for _, w := range words {
			en, ok := dictAr2En[fold(w)]
			if !ok && strings.HasPrefix(fold(w), "عبد") && len([]rune(w)) > 4 { // عبدال… written as one word
				if rest, ok2 := dictAr2En[fold("ال"+strings.TrimPrefix(strings.TrimPrefix(fold(w), "عبدال"), "عبد"))]; ok2 {
					en, ok = "Abdul"+strings.ToLower(strings.TrimPrefix(rest, "Al-")), true
				}
			}
			if !ok {
				return ""
			}
			out = append(out, en)
		}
		return strings.Join(out, " ")
	}
	for _, w := range words {
		l := strings.ToLower(strings.Trim(w, ".,"))
		if ar, ok := dictEn2Ar[l]; ok {
			out = append(out, ar)
			continue
		}
		if strings.HasPrefix(l, "al-") || strings.HasPrefix(l, "el-") { // Al-Mutairi spelled differently
			if c := dictSkel[wordSkeleton(l)]; len(c) == 1 && strings.HasPrefix(c[0], "ال") {
				out = append(out, c[0])
				continue
			}
			return ""
		}
		if c := dictSkel[wordSkeleton(l)]; len(c) == 1 {
			out = append(out, c[0])
			continue
		}
		return ""
	}
	return strings.Join(out, " ")
}
