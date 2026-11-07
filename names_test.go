package main

import "testing"

func TestNameSkeletons(t *testing.T) {
	same := [][2]string{
		{"Mohammed", "محمد"}, {"Muhammad", "محمد"}, {"Mohamed", "Mohammad"},
		{"Ahmed", "أحمد"}, {"Ahmad", "احمد"}, {"Khalid", "خالد"}, {"Khaled", "Khalid"},
		{"Fatima", "فاطمة"}, {"Fatimah", "فاطمه"}, {"Noura", "نورة"}, {"Nora", "Noura"},
		{"Yousef", "يوسف"}, {"Yusuf", "Youssef"}, {"Hassan", "حسن"}, {"Hasan", "Hassan"},
		{"Abdullah", "عبدالله"}, {"Abdul Rahman", "عبدالرحمن"}, {"Abdulrahman", "عبد الرحمن"},
		{"Al-Mutairi", "المطيري"}, {"Al Mutairi", "Mutairi"}, {"El-Mutairi", "Al-Mutairi"},
		{"Mohammed Al-Mutairi", "محمد المطيري"}, {"Dhari", "ضاري"}, {"Waleed", "وليد"},
	}
	for _, p := range same {
		if a, b := NameSkeleton(p[0]), NameSkeleton(p[1]); a != b {
			t.Errorf("%q (%s) should match %q (%s)", p[0], a, p[1], b)
		}
	}
	different := [][2]string{{"Ali", "Noura"}, {"Khalid", "Fahad"}, {"Sara", "Salem"}, {"Ali", "Omar"}}
	for _, p := range different {
		if NameSkeleton(p[0]) == NameSkeleton(p[1]) {
			t.Errorf("%q and %q must not match", p[0], p[1])
		}
	}
}

func TestCrossLanguageMatching(t *testing.T) {
	phone := &Contact{ID: "1", First: "خالد", Last: "المطيري", Phones: []Field{{"Mobile", "60001234"}}}
	linkedin := &Contact{ID: "2", First: "Khalid", Last: "Al Mutairi", Company: "Boubyan Bank"} // no number
	if NewDupIndex([]*Contact{phone}, "965").Find(linkedin) == nil {
		t.Error("LinkedIn's Khalid Al Mutairi should find the phone's خالد المطيري")
	}
	sameNumber := &Contact{ID: "3", First: "Khalid", Phones: []Field{{"Mobile", "+965 6000 1234"}}}
	if NewDupIndex([]*Contact{phone}, "965").Find(sameNumber) == nil {
		t.Error("a shared number with the same name in another script is the same person")
	}
	other := &Contact{ID: "4", First: "Fahad", Phones: []Field{{"Mobile", "+965 6000 1234"}}}
	if NewDupIndex([]*Contact{phone}, "965").Find(other) != nil {
		t.Error("a different first name sharing a number is a different person")
	}
	Merge(phone, []*Contact{linkedin}, "965")
	if phone.AltName != "Khalid Al Mutairi" || phone.Company != "Boubyan Bank" {
		t.Errorf("merge should keep both names: %+v", phone)
	}
	if !phone.Matches("khalid") || !phone.Matches("mutairi") || !phone.Matches("خالد") {
		t.Error("search should work in both languages")
	}
	plain := &Contact{First: "محمد", Last: "العنزي"}
	if !plain.Matches("mohammed") || !plain.Matches("Muhammad") || !plain.Matches("enezi") {
		t.Error("an English search should find an Arabic-only name")
	}
	if plain.Matches("fahad") {
		t.Error("no false match")
	}
}

func TestSuggestOtherName(t *testing.T) {
	for in, want := range map[string]string{
		"محمد المطيري":        "Mohammed Al-Mutairi",
		"فاطمة الصباح":        "Fatima Al-Sabah",
		"عبدالله العنزي":      "Abdullah Al-Enezi",
		"Mohammed Al-Mutairi": "محمد المطيري",
		"Muhammad Al-Mutairi": "محمد المطيري",
		"Noura Al-Rasheed":    "نورة الرشيد",
		"Zxqv Mohammed":       "", // unknown word: no guess
		"محمد زكزك":           "",
	} {
		if got := SuggestOtherName(in); got != want {
			t.Errorf("SuggestOtherName(%q) = %q, want %q", in, got, want)
		}
	}
}
