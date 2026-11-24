package clinical

import (
	"testing"
	"time"
)

const urgentWords = "I have crushing chest pain right now."

// TestEveryInputPathScreens sends one urgent sentence through each way user
// text enters an encounter. Every path must raise a warning.
func TestEveryInputPathScreens(t *testing.T) {
	now := time.Now().UTC()
	paths := map[string]func(t *testing.T) *Encounter{
		"intake concern": func(t *testing.T) *Encounter { return intake(t, urgentWords, false) },
		"intake background": func(t *testing.T) *Encounter {
			e, err := New("bg", Intake{Concern: "A routine visit.", Background: urgentWords, Age: 40, NoticeVersion: NoticeVersion}, ProviderRef{Kind: "demo", Model: "demo"}, "")
			if err != nil {
				t.Fatal(err)
			}
			return e
		},
		"intake agenda": func(t *testing.T) *Encounter {
			e, err := New("agenda", Intake{Concern: "A routine visit.", Agenda: urgentWords, Age: 40, NoticeVersion: NoticeVersion}, ProviderRef{Kind: "demo", Model: "demo"}, "")
			if err != nil {
				t.Fatal(err)
			}
			return e
		},
		"interview answer": func(t *testing.T) *Encounter {
			e := intake(t, "A routine visit.", false)
			turn(t, e, Proposal{Action: "ask", Question: "What would you like to add?", Reason: "This could change what to ask next."})
			apply(t, e, Command{Action: "answer", Text: urgentWords})
			return e
		},
		"corrected answer": func(t *testing.T) *Encounter {
			e := intake(t, "A routine visit.", false)
			apply(t, e, Command{Action: "correct_answer", AnswerID: e.Answers[0].ID, Text: urgentWords})
			return e
		},
		"corrected finding": func(t *testing.T) *Encounter {
			e := intake(t, "My nose is runny", false)
			turn(t, e, Proposal{Action: "review", Findings: []FindingProposal{find(e.Answers[0], "nose is runny", "Runny nose", "reported_present")}})
			apply(t, e, Command{Action: "correct_fact", FactID: e.Facts[0].ID, Text: "My nose is runny, but now I have crushing chest pain.", Concept: "Runny nose", Subject: "self", Status: "reported_present"})
			return e
		},
		"witness account": func(t *testing.T) *Encounter {
			e := intake(t, "A routine visit.", false)
			apply(t, e, Command{Action: "witness", Speaker: "Fictional partner", ConfirmedDestination: true, Text: "She has crushing chest pain right now."})
			return e
		},
		"visual note": func(t *testing.T) *Encounter {
			e := intake(t, "My nails have changed.", false)
			turn(t, e, Proposal{Action: "ask", Question: "When did you notice it?", Reason: "The timing could matter."})
			apply(t, e, Command{Action: "visual_observation", Category: "Hand", Text: "My nail looks odd and " + urgentWords})
			return e
		},
		"episode log": func(t *testing.T) *Encounter {
			e := intake(t, "A routine visit.", false)
			apply(t, e, Command{Action: "episode_log", Onset: now.Add(-time.Hour).Format(time.RFC3339), Text: urgentWords})
			return e
		},
		"diary entry in a visit": func(t *testing.T) *Encounter {
			e := intake(t, "A routine visit.", false)
			if err := e.IncludeDiary(DiaryEntry{ID: "diary-urgent", Text: urgentWords, Onset: now.Add(-time.Hour).Format(time.RFC3339), Recorded: now}, ""); err != nil {
				t.Fatal(err)
			}
			return e
		},
		"visit outcome": func(t *testing.T) *Encounter {
			e := intake(t, "A routine visit.", false)
			apply(t, e, Command{Action: "add_outcome", Source: "symptom_change", Date: now.Format(time.DateOnly), Text: urgentWords})
			return e
		},
		"report passage": func(t *testing.T) *Encounter {
			e := reportCase(t, "Review my notes", false)
			d := docFixture()
			d.IssueDate = now.Format(time.DateOnly)
			attach(t, e, d)
			apply(t, e, Command{Action: "add_report_passage", DocumentID: d.ID, Text: "The patient currently has crushing chest pain.", Locator: "page 2, line 4"})
			return e
		},
		"report review note": func(t *testing.T) *Encounter {
			e := reportCase(t, "Review my notes", false)
			d := docFixture()
			attach(t, e, d)
			r := d.Rows[1]
			r.Note = urgentWords
			apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: r.ID, Row: r})
			return e
		},
		"return-visit clarification": func(t *testing.T) *Encounter {
			prior := intake(t, "Fictional history", false)
			prior.ConfirmedRevision = prior.HistoryRevision
			apply(t, prior, Command{Action: "clinician_feedback", ForRevision: prior.HistoryRevision, Text: "Asked again about timing", Category: "retake", Speaker: "Practice clinician", ConfirmedDestination: true})
			next := intake(t, "Fictional follow-up", false)
			next.ID = "return"
			if err := next.StartReturn(prior, ReturnVisit{Source: prior.ID, Revision: prior.Revision, Consent: true, Clarifications: []ReturnClarification{{Index: 0, Words: urgentWords}}}, ""); err != nil {
				t.Fatal(err)
			}
			return next
		},
		"reconciliation note": func(t *testing.T) *Encounter {
			source := intake(t, "I used an inhaler yesterday", false)
			turn(t, source, Proposal{Action: "review", Findings: []FindingProposal{find(source.Answers[0], "used an inhaler yesterday", "Inhaler use", "reported_present")}})
			apply(t, source, Command{Action: "confirm"})
			target := intake(t, "A routine visit.", false)
			target.ID = "target"
			if err := target.ReconcileFact(source, Command{FactID: source.Facts[0].ID, Date: now.Format(time.DateOnly), Text: urgentWords, ReconciliationStatus: "still_applies", Category: "event"}, ""); err != nil {
				t.Fatal(err)
			}
			return target
		},
	}
	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			if e := path(t); !e.hasAnyWarning() {
				t.Fatalf("%s raised no warning", name)
			}
		})
	}
	if res, ok := ScreenText(urgentWords, ""); !ok || res.Level != warningEmergency {
		t.Fatal("the shared screen missed the sentence")
	}
	if res, ok := ScreenDiary(DiaryEntry{Text: urgentWords, Onset: now.Add(-72 * time.Hour).Format(time.RFC3339)}, ""); !ok || res.Level != warningCheck {
		t.Fatal("an older diary episode should ask for a prompt check")
	}
}
