package clinical

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// TestRandomActionInvariants drives encounters through random sequences of user
// actions and model turns and checks invariants after every step:
//   - no panic;
//   - an active warning is cleared only by correcting its source;
//   - no active finding cites a missing or superseded answer;
//   - an assessment is never current after the history changes;
//   - every export carries the notice, review status and every active warning.
func TestRandomActionInvariants(t *testing.T) {
	texts := []string{"Yesterday", "No", "not sure", "skip", "I have chest pain and no fever", "It started 3 days ago after a run", "My mother has asthma", "None", "Getting worse", "I'd rather not answer", "doesn't apply to me", "I take ibuprofen 400 mg", DemoConcern, "I feel suicidal", "I'm fine", "She was struggling to breathe."}
	actions := []string{"answer", "answer", "answer", "correct_answer", "correct_fact", "review", "confirm", "assess", "retry", "continue", "focus_interview", "report_emergency", "correct_intake", "visual_observation", "set_episode"}
	const sequences, steps = 1500, 25
	checked := 0
	for seed := int64(0); seed < sequences; seed++ {
		r := rand.New(rand.NewSource(seed))
		concern := texts[r.Intn(len(texts))]
		if seed%3 == 0 {
			concern = DemoConcern
		}
		intake := Intake{Concern: concern, Age: 16 + r.Intn(70), NoticeVersion: NoticeVersion, GuidedHistory: r.Intn(2) == 0, NarrativeFirst: r.Intn(2) == 0, Focused: r.Intn(3) == 0, DemoPractice: r.Intn(2) == 0, UrgentNow: r.Intn(8) == 0}
		var log []string
		fail := func(format string, args ...any) {
			t.Helper()
			t.Fatalf("seed %d after %s: %s", seed, strings.Join(log, " > "), fmt.Sprintf(format, args...))
		}
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					fail("panic: %v", rec)
				}
			}()
			e, err := New("property", intake, ProviderRef{Kind: "demo", Model: "demo"}, "")
			if err != nil {
				return
			}
			for step := 0; step < steps; step++ {
				before := map[string]bool{}
				for _, w := range e.Warnings {
					if w.Active {
						before[w.ID] = true
					}
				}
				history := e.HistoryRevision
				action := "model"
				if task := e.PendingTask(); task != "" {
					in, err := e.ModelInput(task)
					if err == nil {
						var p Proposal
						if e.DemoPractice {
							p = PracticeHistory(in)
						} else {
							p = Demo(in)
						}
						if err = e.Accept(task, p, ModelMeta{Provider: "demo"}, ""); err != nil {
							e.Fail(task)
						}
					} else {
						e.Fail(task)
					}
				} else {
					action = actions[r.Intn(len(actions))]
					c := Command{Action: action, Text: texts[r.Intn(len(texts))]}
					switch action {
					case "correct_answer":
						if len(e.Answers) > 0 {
							c.AnswerID = e.Answers[r.Intn(len(e.Answers))].ID
						}
						b := r.Intn(2) == 0
						c.UrgentNow = &b
					case "correct_fact":
						if len(e.Facts) > 0 {
							c.FactID = e.Facts[r.Intn(len(e.Facts))].ID
						}
						c.Concept, c.Status, c.Subject = "cough", []string{"reported_present", "explicitly_denied", "uncertain"}[r.Intn(3)], "self"
					case "correct_intake":
						b := r.Intn(2) == 0
						c.Age, c.PregnancyRelated = 10+r.Intn(80), &b
					case "visual_observation":
						c.Category = "Eye"
					case "set_episode":
						c.EpisodeTitle = "Episode"
					}
					_ = e.Apply(c, "")
				}
				log = append(log, action)
				for _, w := range e.Warnings {
					if before[w.ID] && !w.Active && action != "correct_answer" && action != "correct_fact" {
						fail("warning %s (%s) was cleared by %s", w.ID, w.Rule, action)
					}
				}
				for _, f := range e.Facts {
					if f.Active && f.SourceType != "report_entry" && f.SourceType != "reconciled_history" && e.answer(f.AnswerID) == nil {
						fail("active finding %s cites missing answer %s", f.ID, f.AnswerID)
					}
				}
				report := e.Report()
				if e.HistoryRevision != history && report.CurrentAssessmentVersion != 0 && action != "model" {
					fail("assessment %d stayed current after the history changed", report.CurrentAssessmentVersion)
				}
				if report.Notice == "" || report.ReviewStatus != ReviewStatus || report.PreviewNotice == "" || report.ModelGuidance == "" {
					fail("export lost its notice or review status")
				}
				exported := map[string]bool{}
				for _, w := range report.Warnings {
					if w.Active {
						exported[w.ID] = true
					}
				}
				for _, w := range e.Warnings {
					if w.Active && !exported[w.ID] {
						fail("export lost active warning %s", w.ID)
					}
				}
				checked++
			}
		}()
	}
	fmt.Printf("PROPERTY sequences=%d checked_steps=%d\n", sequences, checked)
}
