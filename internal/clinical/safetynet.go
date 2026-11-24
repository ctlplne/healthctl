package clinical

import (
	"errors"
	"time"
)

// The application asks its own safety-net question at fixed points: before the
// history is confirmed and at the start of a return visit. The wording and the
// follow-up belong to the app, not the model. Keyword rules miss many ways of
// describing an emergency, so asking directly is the stronger check.
const (
	SafetyNetQuestion = "Before you go on: is anything new, or getting rapidly worse, since you started? For example chest pain, trouble breathing, weakness or numbness on one side, confusion, or thoughts of harming yourself."
	safetyNetChanged  = "Something is new or getting worse."
)

// SafetyCheck records one answer to the safety-net question.
type SafetyCheck struct {
	HistoryRevision int       `json:"historyRevision"`
	Point           string    `json:"point"`
	Answer          string    `json:"answer"`
	At              time.Time `json:"at"`
}

// SafetyCheckDue reports whether the app should ask its safety-net question now.
func (e *Encounter) SafetyCheckDue(point string) bool {
	for _, c := range e.SafetyChecks {
		if c.Point == point && c.HistoryRevision == e.HistoryRevision {
			return false
		}
	}
	return true
}

func (e *Encounter) safetyCheck(c Command, emergency string) error {
	point := c.Category
	if point != "review" && point != "return" {
		return errors.New("Choose where the safety question was asked")
	}
	switch c.Status {
	case "none", "changed", "emergency":
	default:
		return errors.New("Choose one answer to the safety question")
	}
	e.SafetyChecks = append(e.SafetyChecks, SafetyCheck{HistoryRevision: e.HistoryRevision, Point: point, Answer: c.Status, At: time.Now().UTC()})
	switch c.Status {
	case "emergency":
		e.changed()
		e.appendAnswer("screen", urgentQuestion, urgentYes, "", emergency)
		e.routeInterview()
	case "changed":
		e.changed()
		a := e.appendAnswer("safety_net", SafetyNetQuestion, safetyNetChanged, "", emergency)
		e.addWarning(Evidence{a.ID, a.Text}, "safety_net", warningCheck, emergency)
		// Go back to the interview so the change can be described and screened.
		if e.ScopeReason == "" && !e.hasWarning() {
			e.Question = "What is new, or getting worse?"
			e.QuestionSource = &QuestionSource{Key: "safety_net", Reason: "You said something is new or getting worse. Describing it lets the app check whether it needs urgent care."}
			e.Choices = []string{}
			e.State = "interviewing"
			e.Pending = ""
		}
	}
	return nil
}
