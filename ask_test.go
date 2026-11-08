package main

import "testing"

func TestQuestionToQuery(t *testing.T) {
	srcs := []string{"LinkedIn", "Spreadsheet"}
	for q, want := range map[string]string{
		"who do I know at Gulf Bank in marketing?":                      `company:"gulf bank" marketing`,
		"people I haven't talked to in a year":                          "talked:>1y",
		"favorites in Kuwait without an email":                          "no:email is:favorite country:kuwait",
		"who did I meet at GITEX that I haven't talked to in 6 months?": "talked:>6m gitex",
		"people from LinkedIn in my Clients group":                      "group:clients source:LinkedIn",
		"engineers at Boubyan?":                                         "company:boubyan engineer",
	} {
		if got := QuestionToQuery(q, srcs); got != want {
			t.Errorf("%q\n got  %s\n want %s", q, got, want)
		}
	}
	if looksLikeQuestion("ali hassan") || !looksLikeQuestion("who works at zain") {
		t.Error("question detection")
	}
}
