package clinical

import (
	"strings"
	"testing"
)

func interviewInput(t *testing.T, e *Encounter) Input {
	t.Helper()
	in, err := e.ModelInput("interview")
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestQuestionIntents(t *testing.T) {
	e := intake(t, "I have a cough.", false)
	turn(t, e, Proposal{Action: "ask", Reason: "[intent:onset:the cough] timing is unknown"})
	if e.Question != "When did the cough start?" || e.QuestionSource.Key != "intent:onset" || e.QuestionSource.Topic != "the cough" {
		t.Fatalf("the app did not write the intent question: %q %+v", e.Question, e.QuestionSource)
	}
	if e.QuestionSource.Reason != intentCatalog["onset"].reason || e.QuestionSource.ModelNote != "timing is unknown" {
		t.Fatalf("reason must be the app's and the model note labeled: %+v", e.QuestionSource)
	}
	apply(t, e, Command{Action: "answer", Text: "Two days ago."})
	in := interviewInput(t, e)
	if ReasonOf(Validate(in, Proposal{Action: "ask", Reason: "[intent:onset:the cough]"})) != ReasonAlreadyAnswered {
		t.Fatal("a repeated intent was not caught")
	}
	if ReasonOf(Validate(in, Proposal{Action: "ask", Reason: "[intent:diagnose] guess"})) != ReasonUnknownIntent {
		t.Fatal("an intent outside the catalog was accepted")
	}
	if ReasonOf(Validate(in, Proposal{Action: "ask", Reason: "Timing [intent:onset]"})) != ReasonUnknownIntent {
		t.Fatal("an intent tag must start the reason")
	}
	// A topic the user never used is dropped; the app falls back to "this".
	turn(t, e, Proposal{Action: "ask", Reason: "[intent:course:your pneumonia]"})
	if e.Question != "How has this changed since it started?" || e.QuestionSource.Topic != "" {
		t.Fatalf("an ungrounded topic reached the question: %q", e.Question)
	}
	apply(t, e, Command{Action: "answer", Text: "Getting worse"})
	turn(t, e, Proposal{Action: "ask", Reason: "[intent:also_have:high temperature]"})
	if e.Question != "Have you also noticed fever?" || !equal(e.Choices, yesNoUnsure) || e.QuestionSource.Topic != "Fever" {
		t.Fatalf("also_have must use the vocabulary term: %q %v", e.Question, e.Choices)
	}
	apply(t, e, Command{Action: "answer", Text: "No"})
	in = interviewInput(t, e)
	if ReasonOf(Validate(in, Proposal{Action: "ask", Reason: "[intent:also_have:glowing toes]"})) != ReasonUnknownIntent {
		t.Fatal("also_have accepted a symptom outside the vocabulary")
	}
	// Free text is a fallback with stricter checks.
	for q, code := range map[string]string{
		"Tell me about the cough":                             ReasonQuestion,
		"When did it start and how bad is it?":                ReasonMultipleQuestions,
		"Is it " + strings.Repeat("really ", 50) + "a cough?": ReasonTooLong,
	} {
		if got := ReasonOf(Validate(in, Proposal{Action: "ask", Question: q, Reason: "Needed."})); got != code {
			t.Fatalf("%q: got %q, want %q", q, got, code)
		}
	}
	if err := Validate(in, Proposal{Action: "ask", Question: "Is the cough worse at night?", Reason: "Pattern."}); err != nil {
		t.Fatalf("a plain free-text question was rejected: %v", err)
	}
	if !strings.Contains(IntentText(), "[intent:onset]") || !strings.Contains(IntentText(), "also_have") {
		t.Fatal("the prompt text must list the catalog")
	}
}

func equal(a, b []string) bool { return strings.Join(a, "\x00") == strings.Join(b, "\x00") }

func TestIntentChoices(t *testing.T) {
	e := intake(t, "My stomach hurts after meals.", false)
	in := interviewInput(t, e)
	// An intent question always uses the app's template choices.
	turn(t, e, Proposal{Action: "ask", Reason: "[intent:course]", Choices: []string{"Vomiting blood", "Fine"}})
	if !equal(e.Choices, intentCatalog["course"].choices) {
		t.Fatalf("model choices replaced the template: %v", e.Choices)
	}
	// A free-text choice may not name a symptom the user hasn't reported.
	p := Proposal{Action: "ask", Question: "What comes with the pain?", Reason: "Associated symptoms.", Choices: []string{"Fever", "Nothing else", "Stomach pain after meals"}}
	if ReasonOf(Validate(in, p)) != ReasonChoiceSymptom {
		t.Fatal("a choice introduced an unreported symptom")
	}
	clean, rejected, err := Sanitize(in, p)
	if err != nil {
		t.Fatal(err)
	}
	if !equal(clean.Choices, []string{"Nothing else", "Stomach pain after meals"}) || len(rejected) != 1 || rejected[0].Reason != ReasonChoiceSymptom {
		t.Fatalf("choices were not filtered one by one: %v %v", clean.Choices, rejected)
	}
	if choiceAllowed(in, "Crushing chest pain right now") || !choiceAllowed(in, "After eating") {
		t.Fatal("urgent signs must never be suggested as choices")
	}
}

func TestCompletenessGate(t *testing.T) {
	e := guided(t, "I have a headache.")
	if len(e.Gaps) == 0 || len(e.RedFlagsDue) != 2 {
		t.Fatalf("the app must track what the headache history still needs: %+v %+v", e.Gaps, e.RedFlagsDue)
	}
	turn(t, e, Proposal{Action: "review"})
	// The first app step is the opening detail, then background questions.
	for guard := 0; e.State == "interviewing" && guard < 20; guard++ {
		if e.Pending == "interview" {
			turn(t, e, Proposal{Action: "review"})
			continue
		}
		if !appQuestion(e.QuestionSource) && e.QuestionSource.Key != "background" && e.QuestionSource.Key != "symptom_story" {
			t.Fatalf("unexpected question source %+v", e.QuestionSource)
		}
		reply := "Not sure"
		if strings.HasPrefix(e.QuestionSource.Key, "red_flag:") {
			reply = "No"
		}
		apply(t, e, Command{Action: "answer", Text: reply})
	}
	if e.State != "history_review" || len(e.MissingElements()) != 0 || len(e.RedFlagsDue) != 0 {
		t.Fatalf("the interview ended before the app's own questions: %s %+v %+v", e.State, e.MissingElements(), e.RedFlagsDue)
	}
	for _, el := range completeness["headache"] {
		if !e.covered(el) {
			t.Fatalf("%s not covered", el)
		}
	}

	// When the user stops early, the gaps stay in the app's own words, the model
	// can't clear them, and the confirmed summary records them.
	e = guided(t, "I have a headache.")
	turn(t, e, Proposal{Action: "review", Outstanding: []string{}})
	apply(t, e, Command{Action: "review"})
	if e.State != "history_review" || len(e.Gaps) == 0 || !strings.HasPrefix(e.Gaps[0].Note, "Not yet asked:") {
		t.Fatalf("gaps lost after the model's review: %+v", e.Gaps)
	}
	apply(t, e, Command{Action: "confirm"})
	s := e.Summaries[len(e.Summaries)-1]
	if len(s.Gaps) == 0 || len(s.UnansweredRedFlags) != 2 {
		t.Fatalf("the confirmed summary must keep what the app still expected: %+v", s)
	}
}

func TestRedFlagCoverage(t *testing.T) {
	concerns := map[string]string{
		"respiratory": "I have a cough.", "headache": "I have a headache.", "abdominal": "My stomach hurts.",
		"dizziness": "I feel dizzy.", "bowel_change": "I have loose stools.", "eye_appearance": "My eye whites look yellow.",
	}
	for pathway, flags := range redFlags {
		concern, ok := concerns[pathway]
		if !ok {
			t.Fatalf("no test concern for %s", pathway)
		}
		for i := range flags {
			e := guided(t, concern)
			if e.hasAnyWarning() {
				// A concern that already warns is interrupted before any red flag.
				continue
			}
			for e.Question == "" || !strings.HasPrefix(e.QuestionSource.Key, "red_flag:"+pathway+":"+flags[i].key) {
				switch {
				case e.Pending == "interview":
					turn(t, e, Proposal{Action: "review"})
				case e.State == "interviewing" && strings.HasPrefix(e.QuestionSource.Key, "red_flag:"):
					apply(t, e, Command{Action: "answer", Text: "No"})
				case e.State == "interviewing":
					apply(t, e, Command{Action: "answer", Text: "Not sure"})
				default:
					t.Fatalf("%s: red flag %s was never asked (state %s)", pathway, flags[i].key, e.State)
				}
			}
			apply(t, e, Command{Action: "answer", Text: "Yes"})
			if e.State != "interrupted_for_safety" || !e.hasWarning() {
				t.Fatalf("%s/%s: a yes to a red flag must interrupt for safety", pathway, flags[i].key)
			}
		}
	}
	// The review screen asks any red flag still due, for every interview mode.
	e := intake(t, "I have a cough.", false)
	turn(t, e, Proposal{Action: "review"})
	if e.State != "history_review" || len(e.RedFlagsDue) != 2 {
		t.Fatalf("red flags must be listed at review: %+v", e.RedFlagsDue)
	}
	apply(t, e, Command{Action: "red_flag", Category: e.RedFlagsDue[0].Key, Status: "no"})
	apply(t, e, Command{Action: "red_flag", Category: e.RedFlagsDue[0].Key, Status: "unsure"})
	if len(e.RedFlagsDue) != 0 || e.hasAnyWarning() || !strings.HasPrefix(e.Gaps[len(e.Gaps)-1].Note, "Not sure about a safety question") {
		t.Fatalf("review answers were not recorded: %+v %+v", e.RedFlagsDue, e.Gaps)
	}
	if err := e.Apply(Command{Action: "red_flag", Category: "red_flag:respiratory:rest", Status: "yes"}, ""); err == nil {
		t.Fatal("an answered red flag was accepted again")
	}
	e = intake(t, "I have a cough.", false)
	turn(t, e, Proposal{Action: "review"})
	apply(t, e, Command{Action: "red_flag", Category: e.RedFlagsDue[1].Key, Status: "yes"})
	if e.State != "interrupted_for_safety" || !e.hasWarning() {
		t.Fatal("a yes on the review screen must interrupt for safety")
	}
}

func TestQuestionBudget(t *testing.T) {
	e := intake(t, "I have a rash on my arm.", false)
	for i := 0; i < maxModelQuestions; i++ {
		q := "Is detail number " + string(rune('A'+i)) + " present?"
		turn(t, e, Proposal{Action: "ask", Question: q, Reason: "Needed."})
		if e.Question != q {
			t.Fatalf("question %d was not asked", i)
		}
		apply(t, e, Command{Action: "answer", Text: "Not sure"})
	}
	extra := "Is one more detail present?"
	turn(t, e, Proposal{Action: "ask", Question: extra, Reason: "Needed."})
	if e.Question != "" || e.State != "history_review" || !contains(e.Outstanding, extra) || e.modelQuestionsAsked() != maxModelQuestions {
		t.Fatalf("the app must stop the model at its budget and keep the question open: %q %s %v", e.Question, e.State, e.Outstanding)
	}
	// Dedup by element: after the app asked when it started, a reworded onset
	// question is a repeat.
	e = guided(t, "I have a rash on my arm.")
	turn(t, e, Proposal{Action: "review"})
	if e.QuestionSource.Key != "symptom_story" {
		t.Fatalf("expected the app's opening detail, got %+v", e.QuestionSource)
	}
	apply(t, e, Command{Action: "answer", Text: "Yesterday"})
	if ReasonOf(Validate(interviewInput(t, e), Proposal{Action: "ask", Question: "When did it start?", Reason: "Timing."})) != ReasonAlreadyAnswered {
		t.Fatal("a reworded onset question was asked again")
	}
	if Validate(interviewInput(t, e), Proposal{Action: "ask", Question: "When did you start that cream?", Reason: "Timing of a treatment."}) != nil {
		t.Fatal("a medicine timing question is not an onset repeat")
	}
}

func TestQuestionReasonAppAuthored(t *testing.T) {
	e := intake(t, "I have a cough.", false)
	turn(t, e, Proposal{Action: "ask", Question: "Is the cough worse at night?", Reason: "Night cough could suggest asthma."})
	if e.QuestionSource.Reason != freeTextReason || e.QuestionSource.ModelNote != "Night cough could suggest asthma." {
		t.Fatalf("a free-text reason must be the app's, with the model's note labeled: %+v", e.QuestionSource)
	}
	if last := e.Turns[len(e.Turns)-1]; last.Reason != freeTextReason || last.QuestionSource.ModelNote == "" {
		t.Fatalf("the turn record must keep both: %+v", last)
	}
	apply(t, e, Command{Action: "answer", Text: "Yes, mostly at night."})
	turn(t, e, Proposal{Action: "ask", Reason: "[intent:severity] impact unknown"})
	if e.QuestionSource.Reason != intentCatalog["severity"].reason {
		t.Fatalf("an intent reason must come from the catalog: %+v", e.QuestionSource)
	}
}

func TestConceptVocabulary(t *testing.T) {
	for concept, want := range map[string]string{"Breathlessness": "Shortness of breath", "dry cough": "Cough", "Sudden severe headache": "Headache", "Throat feels sore": "Sore throat"} {
		if got, ok := lookupConcept(concept); !ok || got.Term != want {
			t.Fatalf("%q mapped to %q", concept, got.Term)
		}
	}
	for _, concept := range []string{"Ankle swelling", "Pain", "Self-reported fingernail change"} {
		if got, ok := lookupConcept(concept); ok {
			t.Fatalf("%q must not map to %q", concept, got.Term)
		}
	}
	e := intake(t, "I get breathless on stairs.", false)
	apply(t, e, Command{Action: "answer", Text: "I have no shortness of breath at rest."})
	first, second := e.Answers[0], e.Answers[len(e.Answers)-1]
	turn(t, e, Proposal{Action: "review", Findings: []FindingProposal{
		find(first, "I get breathless on stairs", "Breathlessness", "reported_present"),
		find(second, "no shortness of breath", "Shortness of breath", "explicitly_denied"),
	}})
	if len(e.Contradictions) != 1 {
		t.Fatalf("synonyms must be compared as one finding: %+v", e.Contradictions)
	}
	if e.Facts[0].Term != "Shortness of breath" || e.Facts[0].Code != "R06.0" {
		t.Fatalf("facts must carry the vocabulary term and code: %+v", e.Facts[0])
	}
}
