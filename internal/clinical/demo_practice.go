package clinical

import "strings"

// PracticeHistory collects a fictional narrative offline. Only the user
// supplies observations. The practice exercise does no clinical extraction.
func PracticeHistory(in Input) Proposal {
	if in.Task != "interview" || in.Age < 18 || len(in.ActiveWarnings) > 0 {
		return Proposal{Action: "insufficient", Outstanding: []string{"Offline history practice records your words and does not generate an estimate."}}
	}
	review := Proposal{Action: "review"}
	if in.ReviewOnly {
		return review
	}
	const timing = "When did you first notice what brought you here?"
	const course = "What has changed since you first noticed it?"
	const impact = "How, if at all, has this affected your usual day?"
	const goal = "What would you most like to understand about this?"
	answered := map[string]bool{}
	story := []string{in.Concern}
	for _, a := range in.Answers {
		if a.Kind == "reply" {
			answered[a.Question] = true // A skipped question has been offered.
			if evidenceDisposition(a) == "provided" && backgroundFor(a.Question) == nil {
				story = append(story, a.Text)
			}
		}
	}
	words := strings.Join(story, " ")
	var question, reason string
	switch {
	case !answered[timing] && !openingTiming.MatchString(words):
		question, reason = timing, "This places the beginning of your concern on a timeline."
	case !answered[course] && !openingCourse.MatchString(words):
		question, reason = course, "Your description can show whether the concern is changing over time."
	case !answered[impact] && !openingImpact.MatchString(words):
		question, reason = impact, "A concrete example helps someone understand what this means for you."
	case !answered[goal] && in.Agenda == "":
		question, reason = goal, "Your question gives the visit a focus."
	default:
		return review
	}
	return Proposal{Action: "ask", Question: question, Reason: reason, Outstanding: []string{question}}
}
