package clinical

import "testing"

// Patients often say "they" for their own symptoms. A live interview lost
// "They have become a bit more frequent" (about headaches) because the
// pronoun made the finding look like someone else's. Development content,
// not clinically reviewed.
func TestTheyCanMeanTheUsersSymptoms(t *testing.T) {
	check := func(question, text, quote, subject string) error {
		a := Answer{ID: "a1", Kind: "reply", Question: question, Text: text, Active: true}
		f := FindingProposal{Evidence: Evidence{a.ID, quote}, Concept: "headache", Status: "reported_present", Subject: subject}
		return validateFinding(Input{Task: "interview", Answers: []Answer{a}}, f)
	}
	experiencer := func(err error) bool { return ReasonOf(err) == ReasonExperiencer }
	cases := []struct {
		name, question, text, quote, subject string
		rejected                             bool
	}{
		{"the user's own headaches", "How has this changed since you first noticed it?", "They have become a bit more frequent.", "They have become a bit more frequent", "self", false},
		{"symptoms come and go", "How long does each episode last?", "They come and go over a few hours.", "They come and go", "self", false},
		{"a relative named first", "How has this changed since you first noticed it?", "My parents get migraines and they have become more frequent.", "they have become more frequent", "self", true},
		{"a family question", "Does anyone in your family get headaches?", "They both get them every week.", "They both get them every week", "self", true},
		{"a family member's symptom stays theirs", "Does anyone in your family get headaches?", "They both get them every week.", "They both get them every week", "family", false},
		{"he still means someone else", "How has this changed since you first noticed it?", "He has had them more often.", "He has had them more often", "self", true},
	}
	for _, c := range cases {
		err := check(c.question, c.text, c.quote, c.subject)
		if experiencer(err) != c.rejected {
			t.Errorf("%s: experiencer rejection = %v, want %v (err %v)", c.name, experiencer(err), c.rejected, err)
		}
	}
	// Screening for warnings is unchanged: there "they" is still someone else.
	text := "They have chest pain."
	if c := analyzeSpan(text, 10, 20, fromUser); !c.Other {
		t.Fatal("warning screening must keep reading a bare they as someone else")
	}
}
