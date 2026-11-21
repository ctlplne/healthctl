package clinical

import (
	"strings"
	"testing"
	"time"
)

func TestFocusedReviewKeepsGapsAndInterruptsForUrgency(t *testing.T) {
	e := intake(t, "Synthetic symptoms after a walk", false)
	e.Focused = true
	e.GuidedHistory = true
	e.Question = "What happened?"
	apply(t, e, Command{Action: "answer", Text: "I sat on the bench."})
	if e.Pending != "" || e.Question == "" {
		t.Fatal("short interview waited on remote inference between questions")
	}
	e.Question = "How long?"
	apply(t, e, Command{Action: "answer", Text: "About ten minutes."})
	if !e.ReviewExtraction || e.Pending != "interview" {
		t.Fatal("did not pause for review")
	}
	turn(t, e, Proposal{Action: "review", Outstanding: []string{"When did this change?"}})
	if e.State != "history_review" || len(e.Outstanding) < 2 || e.Outstanding[0] != "When did this change?" {
		t.Fatalf("gaps not preserved: %v", e.Outstanding)
	}
	apply(t, e, Command{Action: "report_emergency"})
	if e.State != "interrupted_for_safety" || e.Pending != "" {
		t.Fatal("short mode suppressed current warning")
	}
}
func TestEpisodeAndWitnessProvenance(t *testing.T) {
	e := intake(t, "Recurring episodes", false)
	log := func(start string) {
		t.Helper()
		if err := e.IncludeDiary(DiaryEntry{ID: "saved-" + start, Text: "I had to stop walking.", Onset: start, Recorded: time.Date(2025, 1, 4, 8, 0, 0, 0, time.UTC)}, ""); err != nil {
			t.Fatal(err)
		}
	}
	log("2025-01-01T10:00:00Z")
	log("2025-01-02T08:00:00Z")
	if e.QuestionSource == nil || e.QuestionSource.Key != "episode_change" {
		t.Fatal("lost comparison")
	}
	apply(t, e, Command{Action: "answer", Text: "This time it lasted longer."})
	log("2025-01-01T11:00:00Z")
	if e.QuestionSource != nil && e.QuestionSource.Key == "episode_change" {
		t.Fatal("repeated the same comparison")
	}
	log("2025-01-03T08:00:00Z")
	if e.QuestionSource == nil || e.QuestionSource.Key != "episode_change" {
		t.Fatal("missed the newer event")
	}
	witness := Command{Action: "witness", Speaker: "Fictional observer", Text: "I saw them sit on the floor."}
	if e.Apply(witness, "") == nil {
		t.Fatal("silent witness disclosure")
	}
	witness.ConfirmedDestination = true
	apply(t, e, witness)
	a := e.Answers[len(e.Answers)-1]
	if a.Detail.Speaker != witness.Speaker || e.Apply(Command{Action: "correct_answer", AnswerID: a.ID, Text: "Changed words"}, "") == nil {
		t.Fatal("lost original attribution")
	}
	e.ConfirmedRevision = e.HistoryRevision
	next := intake(t, "Changed pattern", false)
	next.ID = "return"
	v := ReturnVisit{Source: e.ID, Revision: e.Revision, Consent: true, Entries: []string{a.ID}}
	if err := next.StartReturn(e, v, ""); err != nil {
		t.Fatal(err)
	}
	copied := next.Answers[len(next.Answers)-1]
	if copied.Detail.Speaker != witness.Speaker || copied.Detail.PriorAnswer != a.ID || !copied.Detail.FirstRecorded.Equal(a.Detail.FirstRecorded) {
		t.Fatal("lost provenance on return")
	}
}
func TestTwoSourceMedicationTimelineAndReturn(t *testing.T) {
	e := reportCase(t, "Synthetic dizziness", false)
	add := func(id, name, quote string, med bool) string {
		d := docFixture()
		d.ID = id
		d.Title = name
		d.MedicationLabel = med
		row := ReportRow{ID: "line", Original: quote, Excerpt: quote, Name: name, Subject: "self", Locator: "line 1"}
		d.Rows = []ReportRow{row}
		attach(t, e, d)
		apply(t, e, Command{Action: "review_report_row", DocumentID: id, RowID: "line", Row: row})
		apply(t, e, Command{Action: "authorize_report", DocumentID: id, RowIDs: []string{"line"}})
		return e.ReportReviews[len(e.ReportReviews)-1].ID
	}
	label := add("label", "Fictional tablet", "Example tablet 5 mg once daily", true)
	note := add("note", "Visit note", "Started Fictional tablet on Monday", false)
	apply(t, e, Command{Action: "continue"})
	if e.QuestionSource == nil || e.QuestionSource.Key != "medication_start" {
		t.Fatal("missed shared medication name and start in second record")
	}
	apply(t, e, Command{Action: "answer", Text: "My first dose was Friday.", Date: "2025-02-07"})
	if e.QuestionSource == nil || e.QuestionSource.Key != "event_timing" || !strings.Contains(e.Question, "2025-02-07") {
		t.Fatal("did not ask about the patient's actual first-dose date")
	}
	med := Command{Action: "medication_use", ReviewIDs: []string{label}, Prescribed: "Once each morning", Text: "I actually take it every other day.", Date: "2025-02-07", ConfirmedDestination: true}
	apply(t, e, med)
	a := e.Answers[len(e.Answers)-1]
	if a.Detail.Printed != "Example tablet 5 mg once daily" || a.Detail.Prescribed != med.Prescribed || a.Text != med.Text {
		t.Fatal("collapsed three sources of medication history")
	}
	med.ReviewIDs = []string{note}
	if e.Apply(med, "") == nil {
		t.Fatal("accepted nonlabel")
	}
	reconcile := Command{Action: "resolve_sources", Text: "My first dose was Friday.", Date: "2025-02-07", ReviewIDs: []string{label, note}, ConfirmedDestination: true}
	apply(t, e, reconcile)
	apply(t, e, Command{Action: "continue"})
	if e.QuestionSource == nil || e.QuestionSource.Key != "event_timing" || !strings.Contains(e.Question, reconcile.Date) {
		t.Fatal("failed to anchor symptom timing to the patient’s event")
	}
	apply(t, e, Command{Action: "answer", Text: "My symptoms began before I took it."})
	e.ConfirmedRevision = e.HistoryRevision
	next := reportCase(t, "Return with two documents", false)
	next.ID = "return"
	v := ReturnVisit{Source: e.ID, Revision: e.Revision, Consent: true, ReviewIDs: []string{label, note}}
	if err := next.StartReturn(e, v, ""); err != nil {
		t.Fatal(err)
	}
	if len(next.disclosed()) != 2 || len(next.Documents) != 2 || next.Documents[0].PriorEncounter != e.ID || next.QuestionSource.Key != "return_change" {
		t.Fatal("return did not preserve selected originals and destination")
	}
	apply(t, e, Command{Action: "withdraw_report_row", DocumentID: "label", RowID: "line"})
	if e.answer(a.ID) == nil || !e.answer(a.ID).SourceOutdated || len(e.disclosed()) != 1 {
		t.Fatal("withdrawal did not retire dependent evidence")
	}
	if next.StartReturn(e, v, "") == nil {
		t.Fatal("accepted an amended source without re-review")
	}
}
func TestFeedbackRequiresCurrentPatientApproval(t *testing.T) {
	e := intake(t, "Fictional history", false)
	e.ConfirmedRevision = e.HistoryRevision
	c := Command{Action: "clinician_feedback", ForRevision: e.HistoryRevision, Text: "Asked again about timing", Category: "retake", Speaker: "Practice clinician"}
	if e.Apply(c, "") == nil {
		t.Fatal("accepted unapproved feedback")
	}
	c.ConfirmedDestination = true
	apply(t, e, c)
	if len(e.Feedback) != 1 || e.ConfirmedRevision != e.HistoryRevision {
		t.Fatal("feedback mutated patient history")
	}
	next := intake(t, "Fictional follow-up", false)
	next.ID = "return"
	v := ReturnVisit{Source: e.ID, Revision: e.Revision, Consent: true, Clarifications: []ReturnClarification{{Index: 0, Words: "I first noticed it on Friday."}}}
	if err := next.StartReturn(e, v, ""); err != nil {
		t.Fatal(err)
	}
	a := next.Answers[len(next.Answers)-1]
	if a.Kind != "patient_clarification" || a.Detail.FeedbackText != c.Text || a.Text != v.Clarifications[0].Words {
		t.Fatal("lost distinction between feedback and patient clarification")
	}
	c.ForRevision--
	if e.Apply(c, "") == nil {
		t.Fatal("accepted feedback on a stale brief")
	}
}

func TestFaintDenialPreservesExactEvidence(t *testing.T) {
	a := Answer{Kind: "reply", Text: "It began after the tablet. I have not fainted."}
	if !denialSupports("Syncope", "I have not fainted.", a) || denialSupports("Chest pain", "I have not fainted.", a) {
		t.Fatal("explicit denial failed to keep the correct concept")
	}
	a.Text = "I have not fainted until today."
	if denialSupports("Syncope", "I have not fainted", a) {
		t.Fatal("accepted a qualified negation")
	}
}
