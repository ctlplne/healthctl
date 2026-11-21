package clinical

import (
	"encoding/json"
	"slices"
	"testing"
)

func guided(t *testing.T, concern string) *Encounter {
	t.Helper()
	e, err := New("synthetic", Intake{GuidedHistory: true, Age: 38, Concern: concern, NoticeVersion: NoticeVersion}, ProviderRef{Kind: "compatible"}, "")
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestGuidedPlanOpeningIsAnswerableBeforeInference(t *testing.T) {
	for _, tc := range []struct{ concern, question string }{
		{"My shoulder aches.", "When did you first notice this change?"},
		{"My shoulder started aching three days ago.", "How has this changed since you first noticed it?"},
		{"My shoulder started aching three days ago and is getting worse.", "How is this affecting what you can usually do?"},
		{"Since Monday my shoulder is getting worse and it is hard to lift a bag.", "What concerns you most about this change?"},
	} {
		in := Intake{GuidedHistory: true, Age: 38, Concern: tc.concern, NoticeVersion: NoticeVersion}
		e, err := New("synthetic", in, ProviderRef{Kind: "chatgpt"}, "")
		if err != nil || e.Question != tc.question || e.Pending != "" || len(e.Facts) != 0 || len(e.Turns) != 0 {
			t.Fatalf("expected immediate question for %q", tc.concern)
		}
		apply(t, e, Command{Action: "answer", Text: "Skip"})
		payload, err := e.ModelInput("interview")
		last := e.Answers[len(e.Answers)-1]
		if err != nil || payload.Concern != tc.concern || e.Pending != "interview" || last.Disposition != "skipped" || last.Question != tc.question {
			t.Fatal("opening evidence or skipped response lost before model follow-up")
		}
	}
	in := Intake{GuidedHistory: true, Age: 38, Concern: "My shoulder aches.", NoticeVersion: NoticeVersion}
	cfg := ProviderRef{Kind: "chatgpt"}
	e, _ := New("synthetic", in, cfg, "")
	apply(t, e, Command{Action: "review"})
	if !e.ReviewExtraction || e.Pending != "interview" {
		t.Fatal("early review missed opening extraction")
	}
	for _, change := range []func(*Intake){
		func(i *Intake) { i.UrgentNow = true },
		func(i *Intake) { i.Concern = "I cannot breathe." },
		func(i *Intake) { i.Age = 12 },
		func(i *Intake) { i.PregnancyRelated = true },
		func(i *Intake) { i.ReportFirst = true },
	} {
		i := in
		change(&i)
		e, err := New("synthetic", i, cfg, "")
		if err != nil || e.Question != "" || e.Pending != "" || e.State == "interviewing" {
			t.Fatal("routine opening overrode scope, urgency or report-first flow")
		}
	}
}

func TestGuidedHistoryFromSparseConcernToCorrectedExport(t *testing.T) {
	e := guided(t, "I feel dizzy")
	turn(t, e, Proposal{Action: "review"})
	if e.Question != "When did you first notice this change?" {
		t.Fatalf("thin opening went to %q", e.Question)
	}
	apply(t, e, Command{Action: "answer", Text: "Yesterday"})
	turn(t, e, Proposal{Action: "ask", Question: "What are you doing when you notice it?", Reason: "The circumstances can clarify the pattern."})
	apply(t, e, Command{Action: "answer", Text: "I notice it when I stand up."})
	turn(t, e, Proposal{Action: "review"})
	var medication Answer
	for i, choice := range []string{backgroundTopics[0].none, "Not sure", "N/A", "I prefer not to answer."} {
		topic := backgroundTopics[i]
		if e.State != "interviewing" || e.Question != topic.question || e.Pending != "" || e.QuestionSource.Reason == "" {
			t.Fatalf("topic %d: %q, %q", i, e.State, e.Question)
		}
		apply(t, e, Command{Action: "answer", Text: choice})
		if i == 0 {
			medication = e.Answers[len(e.Answers)-1]
		}
	}
	turn(t, e, Proposal{Action: "review"})
	if e.State != "history_review" || len(e.Facts) != 1 || e.Facts[0].Status != "explicitly_denied" || e.Facts[0].Concept != backgroundTopics[0].concept {
		t.Fatalf("unsafe or missing background facts: %+v", e.Facts)
	}
	r := e.Report()
	for i, status := range []string{"none", "unknown", "not_applicable", "skipped"} {
		if got := r.Answers[len(r.Answers)-4+i]; got.Disposition != status || got.Question != backgroundTopics[i].question {
			t.Fatalf("export %d: %+v", i, got)
		}
	}
	apply(t, e, Command{Action: "confirm"})
	apply(t, e, Command{Action: "assess"})
	turn(t, e, Proposal{Action: "insufficient", Outstanding: []string{"More details are needed about the dizziness."}})
	const words = "I take a tablet called Example A in the evening."
	apply(t, e, Command{Action: "correct_answer", AnswerID: medication.ID, Text: words})
	in, err := e.ModelInput("interview")
	if err != nil {
		t.Fatal(err)
	}
	newAnswer := in.Answers[len(in.Answers)-1]
	if newAnswer.Text != words || newAnswer.Disposition != "provided" || newAnswer.Supersedes != medication.ID || len(in.Facts) > 0 || len(e.Report().OutdatedAssessmentVersions) != 1 {
		t.Fatalf("correction left stale history: %+v", newAnswer)
	}
	turn(t, e, Proposal{Action: "ask", Question: "When did you start that tablet?", Reason: "Timing may clarify whether it coincided with the dizziness."})
	if e.Question != "When did you start that tablet?" {
		t.Fatal("a new medication detail failed to reopen the interview")
	}
}

func TestGuidedHistoryProtectsNonanswersAndScopeOfNone(t *testing.T) {
	cases := []struct{ text, status, concept, proposed string }{
		{"Not sure", "unknown", "Allergies", "reported_present"},
		{"Not needed", "not_applicable", "Allergies", "reported_present"},
		{"N/A", "not_applicable", "Penicillin allergy", "explicitly_denied"},
		{"I prefer not to answer.", "skipped", "Penicillin allergy", "reported_present"},
		{"No known allergies.", "none", "Penicillin allergy", "explicitly_denied"},
	}
	for _, c := range cases {
		t.Run(c.status, func(t *testing.T) {
			e := guided(t, "A visit about my health")
			turn(t, e, Proposal{Action: "ask", Question: backgroundTopics[1].question, Reason: "This could change what we ask next."})
			apply(t, e, Command{Action: "answer", Text: c.text})
			a := e.Answers[len(e.Answers)-1]
			in := e.inputHistory("interview")
			if a.Disposition != c.status || Validate(in, Proposal{Action: "review", Findings: []FindingProposal{find(a, c.text, c.concept, c.proposed)}}) == nil {
				t.Fatalf("fabricated finding from %q", c.text)
			}
			if Validate(in, Proposal{Action: "review", UrgentEvidence: []Evidence{{a.ID, a.Text}}}) == nil {
				t.Fatalf("false urgency from %q", c.text)
			}
			if slices.Contains(in.BackgroundQuestions, backgroundTopics[1].question) {
				t.Fatal("answered topic asked again")
			}
		})
	}
}

func TestGuidedHistoryPrioritiesAndResume(t *testing.T) {
	e := guided(t, "I have been dizzy since yesterday")
	turn(t, e, Proposal{Action: "review"})
	if e.Question != "How is this affecting what you can usually do?" {
		t.Fatal("repeated onset when timing was supplied")
	}
	apply(t, e, Command{Action: "answer", Text: "Not sure"})
	turn(t, e, Proposal{Action: "review"})
	if e.Question != backgroundTopics[0].question {
		t.Fatal("did not respect uncertainty")
	}
	apply(t, e, Command{Action: "answer", Text: "I take a blue tablet."})
	if e.Pending != "interview" {
		t.Fatal("substantive background was withheld from adaptive follow-up")
	}
	turn(t, e, Proposal{Action: "ask", Question: "What do you take the blue tablet for?", Reason: "Its purpose may help explain what changed."})
	apply(t, e, Command{Action: "answer", Text: "For my blood pressure."})
	turn(t, e, Proposal{Action: "review"})
	if e.Question != backgroundTopics[1].question {
		t.Fatal("failed to return to the next new background topic")
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var resumed Encounter
	if err = json.Unmarshal(b, &resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.Question != backgroundTopics[1].question {
		t.Fatal("lost saved interview position")
	}
	apply(t, &resumed, Command{Action: "report_emergency"})
	if resumed.Question != "" || resumed.State != "interrupted_for_safety" || resumed.Pending != "" {
		t.Fatal("background blocked immediate interruption")
	}
	apply(t, &resumed, Command{Action: "review"})
	turn(t, &resumed, Proposal{Action: "review"})
	if resumed.Question != "" || resumed.State != "history_review" || !resumed.Report().Warnings[0].Active {
		t.Fatal("warning or patient-directed review lost")
	}
}

func TestVolunteeredBackgroundAndPreviouslyDeclinedQuestions(t *testing.T) {
	e := guided(t, "I have a headache. No known allergies.")
	in, err := e.ModelInput("interview")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(in.BackgroundQuestions, backgroundTopics[1].question) {
		t.Fatal("explicitly volunteered allergy history was requested again")
	}
	turn(t, e, Proposal{Action: "ask", Question: "What medications and supplements do you take?", Reason: "Recent changes could affect the chronology."})
	apply(t, e, Command{Action: "answer", Text: "Skip"})
	in = e.inputHistory("interview")
	if Validate(in, Proposal{Action: "ask", Question: "Are you taking any medicines or supplements?", Reason: "Find current medicines."}) == nil {
		t.Fatal("rephrased a declined background question")
	}
	if Validate(in, Proposal{Action: "ask", Question: "When did it start? How severe is it?", Reason: "Find timing and impact."}) == nil {
		t.Fatal("accepted multiple questions in a turn")
	}
	if Validate(in, Proposal{Action: "ask", Question: "How is the headache affecting your usual activities?", Reason: "Its effect on daily life is still unknown."}) != nil {
		t.Fatal("blocked a distinct follow-up")
	}
	for _, narrative := range []string{"My father has a headache. No known allergies.", "I take only my blue tablet."} {
		e = guided(t, narrative)
		if left := e.remainingBackground(); len(left) != len(backgroundTopics) {
			t.Fatal("assumed complete personal history from a partial or family account")
		}
	}
}

func TestNoneOnAnOpenQuestionDoesNotDenyTheSymptom(t *testing.T) {
	e := guided(t, "I have a cough.")
	turn(t, e, Proposal{Action: "ask", Question: "What activities make your cough worse?", Reason: "A pattern could help clarify what changed."})
	apply(t, e, Command{Action: "answer", Text: "None"})
	a := e.Answers[len(e.Answers)-1]
	if err := Validate(e.inputHistory("interview"), Proposal{Action: "review", Findings: []FindingProposal{find(a, a.Text, "Cough", "explicitly_denied")}}); err == nil {
		t.Fatal("no reported triggers became no cough")
	}
}
