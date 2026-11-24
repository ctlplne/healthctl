package clinical

import "encoding/json"

// modelInputBudget is the largest model input the app sends, in bytes of JSON.
const modelInputBudget = 64000

// keepNewest is how many of the most recent answers are always sent.
const keepNewest = 8

// fitBudget keeps a model input within the budget by fixed rules instead of
// failing: the concern, emergency screens, answers behind active warnings and
// the newest answers are always kept; older answers are left out oldest first,
// with the findings that quote them. The user's words are never summarized or
// shortened. The IDs of left-out answers are sent so the model knows.
func fitBudget(in Input) (Input, bool) {
	size := func() int {
		b, _ := json.Marshal(in)
		return len(b)
	}
	if size() <= modelInputBudget {
		return in, true
	}
	keep := map[string]bool{}
	for i, a := range in.Answers {
		if a.Kind == "concern" || a.Kind == "screen" || i >= len(in.Answers)-keepNewest {
			keep[a.ID] = true
		}
	}
	for _, w := range in.ActiveWarnings {
		keep[w.AnswerID] = true
	}
	for _, f := range in.Facts {
		if f.Active && f.Corrected {
			keep[f.AnswerID] = true
		}
	}
	for i := 0; i < len(in.Answers) && size() > modelInputBudget; {
		a := in.Answers[i]
		if keep[a.ID] {
			i++
			continue
		}
		in.Answers = append(in.Answers[:i:i], in.Answers[i+1:]...)
		facts := in.Facts[:0:0]
		for _, f := range in.Facts {
			if f.AnswerID != a.ID {
				facts = append(facts, f)
			}
		}
		in.Facts = facts
		in.OmittedAnswerIDs = append(in.OmittedAnswerIDs, a.ID)
	}
	return in, size() <= modelInputBudget
}
