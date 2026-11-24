package clinical

import (
	"strings"
	"testing"
)

func TestContextEngineScopesEachCue(t *testing.T) {
	cases := []struct {
		text, target string
		src          textSource
		want         spanContext
	}{
		{"I have chest pain and no fever.", "chest pain", fromUser, spanContext{}},
		{"I have chest pain and no fever.", "fever", fromUser, spanContext{Negated: true}},
		{"No fever, chills, or difficulty breathing.", "difficulty breathing", fromUser, spanContext{Negated: true}},
		{"No fever, chest pain since this morning.", "chest pain", fromUser, spanContext{}},
		{"My chest pain isn't going away.", "chest pain", fromUser, spanContext{}},
		{"I've never had chest pain like this before.", "chest pain", fromUser, spanContext{}},
		{"I can't move my wrist without pain.", "pain", fromUser, spanContext{}},
		{"My mother has chest pain.", "chest pain", fromUser, spanContext{Other: true}},
		{"My husband says I keep passing out.", "passing out", fromUser, spanContext{}},
		{"She was struggling to breathe.", "struggling to breathe", fromWitness, spanContext{}},
		{"She was struggling to breathe.", "struggling to breathe", fromUser, spanContext{Other: true}},
		{"I was so worried I couldn't breathe, but he was calm.", "couldn't breathe", fromWitness, spanContext{Other: true}},
		{"What if I get chest pain again?", "chest pain", fromUser, spanContext{Hypothetical: true}},
		{"If I lie down I can't breathe.", "can't breathe", fromUser, spanContext{}},
		{"What if this is a heart attack?", "heart attack", fromUser, spanContext{}},
		{"I had chest pain years ago.", "chest pain", fromUser, spanContext{History: historyRemote}},
		{"I had chest pain years ago and now it's back.", "chest pain", fromUser, spanContext{}},
		{"I passed out at home last week.", "passed out", fromUser, spanContext{History: historyRecent}},
		{"I might have chest pain, I'm not sure.", "chest pain", fromUser, spanContext{Uncertain: true}},
	}
	for _, c := range cases {
		at := strings.Index(c.text, c.target)
		got := analyzeSpan(c.text, at, at+len(c.target), c.src)
		if got != c.want {
			t.Errorf("%q in %q: got %+v, want %+v", c.target, c.text, got, c.want)
		}
	}
}

func TestWitnessThirdPersonWarns(t *testing.T) {
	e := intake(t, "Recording an account for this visit.", false)
	apply(t, e, Command{Action: "witness", Speaker: "Fictional neighbour", ConfirmedDestination: true, Text: "She was struggling to breathe and her lips turned blue."})
	if !e.hasWarning() || e.State != "interrupted_for_safety" {
		t.Fatalf("third-person witness account raised no warning: %+v", e.Warnings)
	}
	f := e.Facts[len(e.Facts)-1]
	if f.SourceType != "witness_observation" || f.Subject != "self" {
		t.Fatalf("witness finding lost its attribution: %+v", f)
	}
	other := intake(t, "Recording an account for this visit.", false)
	apply(t, other, Command{Action: "witness", Speaker: "Fictional neighbour", ConfirmedDestination: true, Text: "Her mother had a stroke."})
	if other.hasAnyWarning() {
		t.Fatal("a relative's history raised a personal warning")
	}
}

func TestModelUrgentFlagNeverFailsTurn(t *testing.T) {
	for _, tc := range []struct {
		text, quote, level string
	}{
		{"My throat feels scratchy and odd.", "throat feels scratchy and odd", warningEmergency},
		{"I have no chest pain right now.", "no chest pain right now", warningCheck},
		{"My father has chest pain.", "father has chest pain", warningCheck},
	} {
		e := intake(t, tc.text, false)
		in, err := e.ModelInput("interview")
		if err != nil {
			t.Fatal(err)
		}
		p := Proposal{Action: "review", UrgentEvidence: []Evidence{{e.Answers[0].ID, tc.quote}}}
		clean, rejected, err := Sanitize(in, p)
		if err != nil || len(rejected) != 0 || len(clean.UrgentEvidence) != 1 {
			t.Fatalf("%q: urgent flag failed the turn: %v %v", tc.text, err, rejected)
		}
		if err := e.Accept("interview", clean, ModelMeta{}, ""); err != nil {
			t.Fatal(err)
		}
		w := e.Warnings[len(e.Warnings)-1]
		level := w.Level
		if level == "" {
			level = warningEmergency
		}
		if !w.Active || w.Rule != "model_flag" || level != tc.level {
			t.Fatalf("%q: want an active %s model warning, got %+v", tc.text, tc.level, w)
		}
	}
	e := intake(t, "My throat feels scratchy.", false)
	in, _ := e.ModelInput("interview")
	clean, rejected, err := Sanitize(in, Proposal{Action: "review", UrgentEvidence: []Evidence{{e.Answers[0].ID, "invented words"}}})
	if err != nil || len(clean.UrgentEvidence) != 0 || len(rejected) != 1 || rejected[0].Reason != ReasonQuoteNotFound {
		t.Fatalf("a flag without an exact quote must be dropped, not fail the turn: %v %v", err, rejected)
	}
}

func TestSanitizeKeepsValidItems(t *testing.T) {
	e := intake(t, "Sore throat for two days, no fever.", false)
	a := e.Answers[0]
	in, _ := e.ModelInput("interview")
	p := Proposal{Action: "ask", Question: "Does anything change your pain?", Reason: "Triggers could change what this might be.", Choices: []string{"Swallowing", "Talking", "Visit https://example.com"},
		Findings: []FindingProposal{
			find(a, "Sore throat for two days", "Sore throat", "reported_present"),
			find(a, "no fever", "Fever", "explicitly_denied"),
			find(a, "fever", "Fever", "reported_present"),
			find(a, "invented", "Cough", "reported_present"),
		}}
	clean, rejected, err := Sanitize(in, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(clean.Findings) != 2 || clean.Question != p.Question || len(clean.Choices) != 2 {
		t.Fatalf("valid items were not kept: %+v", clean)
	}
	codes := map[string]bool{}
	for _, r := range rejected {
		codes[r.Field+":"+r.Reason] = true
	}
	for _, want := range []string{"findings:" + ReasonNegated, "findings:" + ReasonQuoteNotFound, "choices:" + ReasonAdvice} {
		if !codes[want] {
			t.Fatalf("missing rejection %s in %v", want, rejected)
		}
	}
	if err := Validate(in, clean); err != nil {
		t.Fatalf("sanitized proposal must pass the strict gate: %v", err)
	}
	if err := e.Accept("interview", clean, ModelMeta{}, ""); err != nil || e.Question != p.Question {
		t.Fatalf("sanitized proposal was not applied: %v", err)
	}
	// Whole-proposal problems still fail and carry repair hints.
	_, _, err = Sanitize(in, Proposal{Action: "ask", Question: "When? And why?", Reason: "Timing."})
	if ReasonOf(err) != ReasonMultipleQuestions {
		t.Fatalf("expected a fatal question error, got %v", err)
	}
	var v *ValidationError
	if !strings.Contains(err.Error(), "one") || !strings.Contains(err.(*ValidationError).RepairHint(), ReasonMultipleQuestions) || v != nil {
		t.Fatal("repair hint lost the reason code")
	}
}

func TestAdviceFilterAllowsQuestions(t *testing.T) {
	for _, q := range []string{"Does anything change your pain?", "Did you start taking any new medicine before this began?", "Did you stop using your inhaler recently?"} {
		if badOutput(q, 500) {
			t.Errorf("a question about history was treated as advice: %q", q)
		}
	}
	for _, s := range []string{"You should start taking medicine", "Stop taking your tablets.", "This is ruled out.", "You are safe.", "This is 98% likely.", "Did you know this is definitely benign?"} {
		if !badOutput(s, 500) {
			t.Errorf("advice or certainty was accepted: %q", s)
		}
	}
}

func TestGroundedNumbersIgnoresNames(t *testing.T) {
	facts := map[string]Fact{"f1": {FindingProposal: FindingProposal{Evidence: Evidence{AnswerID: "a1", Quote: "cough and fever"}, Concept: "cough", Status: "reported_present", Subject: "self"}, ID: "f1", Active: true}}
	for expl, want := range map[string]bool{
		"A viral infection such as COVID-19 can cause cough and fever.": true,
		"Low vitamin B12 can cause tiredness.":                          true,
		"Type 2 diabetes can raise the risk of infections.":             true,
		"Symptoms lasting more than 10 days can suggest an infection.":  false,
		"A fever above 38.5 can occur.":                                 false,
	} {
		item := Item{Name: "x", Explanation: expl, Supporting: []string{"f1"}, Limitations: []string{"No examination"}}
		if got := groundedNumbers(item, facts); got != want {
			t.Errorf("groundedNumbers(%q) = %t, want %t", expl, got, want)
		}
	}
}

func TestReasonCodes(t *testing.T) {
	for _, code := range ReasonCodes() {
		if reasonHints[code] == "" || strings.ContainsAny(code, " \n") {
			t.Fatalf("reason code %q needs a fixed hint", code)
		}
	}
	in := Input{Task: "interview", Answers: []Answer{{ID: "a1", Kind: "concern", Text: "I have a cough.", Active: true}}}
	if ReasonOf(validateFinding(in, FindingProposal{Evidence: Evidence{"a1", "cough"}, Concept: "cough", Status: "explicitly_denied", Subject: "self"})) != ReasonDenialUnsupported {
		t.Fatal("denial without negation must name its reason")
	}
}

func TestWarningTiers(t *testing.T) {
	e := intake(t, "I passed out at home last week.", false)
	if !e.hasAnyWarning() || e.hasWarning() {
		t.Fatal("a recent past event must raise a check warning that doesn't interrupt")
	}
	w := e.Warnings[len(e.Warnings)-1]
	if w.Level != warningCheck || !strings.Contains(w.Message, "checked promptly") || e.State == "interrupted_for_safety" {
		t.Fatalf("check warning has the wrong level, wording or state: %+v %s", w, e.State)
	}
	if !e.Report().Warnings[len(e.Warnings)-1].Active {
		t.Fatal("the export lost the check warning")
	}
	apply(t, e, Command{Action: "correct_answer", AnswerID: e.Answers[0].ID, Text: "I passed out years ago, never since."})
	if e.hasAnyWarning() {
		t.Fatal("correcting the source must resolve its warning")
	}
	now := intake(t, "I passed out at home an hour ago.", false)
	if !now.hasWarning() || now.State != "interrupted_for_safety" {
		t.Fatal("a current event must raise an interrupting warning")
	}
}

func TestSafetyNetQuestions(t *testing.T) {
	e := intake(t, "My nose is runny", false)
	turn(t, e, Proposal{Action: "review"})
	if !e.SafetyCheckDue("review") {
		t.Fatal("the safety question must be due before confirming")
	}
	apply(t, e, Command{Action: "safety_check", Category: "review", Status: "none"})
	if e.SafetyCheckDue("review") || e.hasAnyWarning() {
		t.Fatal("a no answer is recorded without a warning")
	}
	apply(t, e, Command{Action: "safety_check", Category: "review", Status: "changed"})
	if e.State != "interviewing" || e.QuestionSource == nil || e.QuestionSource.Key != "safety_net" || !e.hasAnyWarning() || e.hasWarning() {
		t.Fatalf("a change must ask what changed and raise a check warning: %s %+v", e.State, e.Warnings)
	}
	apply(t, e, Command{Action: "answer", Text: "Now I can't breathe."})
	if !e.hasWarning() {
		t.Fatal("the described change must be screened")
	}
	urgent := intake(t, "My nose is runny", false)
	turn(t, urgent, Proposal{Action: "review"})
	apply(t, urgent, Command{Action: "safety_check", Category: "review", Status: "emergency"})
	if !urgent.hasWarning() || urgent.State != "interrupted_for_safety" {
		t.Fatal("an emergency answer must interrupt with the explicit warning")
	}
	if res, ok := ScreenDiary(DiaryEntry{Text: "It happened again.", UrgentNow: true}, ""); !ok || res.Level != warningEmergency {
		t.Fatal("a diary emergency answer must warn")
	}
}

func TestPolicyReviewRecord(t *testing.T) {
	for _, v := range []string{"development-1", "development-2", "development-3", PolicyVersion} {
		r := PolicyReview(v)
		if r.Version != v || r.Status == "unknown" {
			t.Fatalf("policy %s has no review record", v)
		}
		if r.Status == "reviewed" && (r.Reviewer == "" || r.Date == "" || r.Scope == "") {
			t.Fatalf("policy %s is marked reviewed without a reviewer, date and scope", v)
		}
		if r.Status != "reviewed" && !strings.Contains(r.Note, "not been clinically reviewed") {
			t.Fatalf("policy %s must say it has not been clinically reviewed", v)
		}
	}
	if PolicyReview("made-up").Status != "unknown" {
		t.Fatal("an unknown policy must not look reviewed")
	}
	a, b := InputHash(Input{Concern: "one"}), InputHash(Input{Concern: "two"})
	if a == b || len(a) != 24 || a != InputHash(Input{Concern: "one"}) {
		t.Fatal("the input hash must be stable and distinguish inputs")
	}
}

// TestNegationReachesOnlyAFewWords checks the engine's backstop: a negation
// governs at most eight words, even when nothing else ends its scope. Real
// sentences usually end the scope sooner with a clause break, "but", a comma
// or "and" (see runon.txt in the corpus).
func TestNegationReachesOnlyAFewWords(t *testing.T) {
	for _, tc := range []struct {
		text    string
		negated bool
	}{
		{"No alpha beta chest pain", true},
		{"No alpha beta gamma delta epsilon zeta eta theta iota kappa chest pain", false},
	} {
		at := strings.Index(tc.text, "chest pain")
		if got := analyzeSpan(tc.text, at, at+len("chest pain"), fromUser).Negated; got != tc.negated {
			t.Fatalf("%q: negated=%v, want %v", tc.text, got, tc.negated)
		}
	}
}
