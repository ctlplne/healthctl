package clinical

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Encounter states.
const (
	StateInterviewing      = "interviewing"
	StateInterrupted       = "interrupted_for_safety"
	StateUnsupported       = "unsupported_scope"
	StateHistoryReview     = "history_review"
	StateNeedsDetail       = "needs_clarification"
	StateAssessmentReady   = "assessment_ready"
	StateAssessing         = "assessing"
	StateFailedRecoverably = "failed_recoverably"
)

var allStates = []string{StateInterviewing, StateInterrupted, StateUnsupported, StateHistoryReview, StateNeedsDetail, StateAssessmentReady, StateAssessing, StateFailedRecoverably}

// same marks a transition that may leave the state unchanged.
const same = "same"

type transitionRule struct {
	from []string
	to   []string
	note string
}

func except(states ...string) []string {
	return slices.DeleteFunc(slices.Clone(allStates), func(s string) bool { return slices.Contains(states, s) })
}

var (
	anyState    = allStates
	routedState = []string{StateInterviewing, StateInterrupted, StateUnsupported, StateHistoryReview}
)

// transitionTable is the explicit state machine. For each action it lists the
// states the action may start from and the states it may end in. Apply refuses
// an action from any other state, and the tests check every observed change
// against the table. Model results and failures are listed under "model:" and
// "fail:" because they arrive through Accept and Fail rather than Apply.
var transitionTable = map[string]transitionRule{
	"answer":              {anyState, routedState, "Records a reply, screens it and routes the interview."},
	"correct_answer":      {anyState, routedState, "Replaces an answer, screens the new words and routes the interview."},
	"correct_intake":      {anyState, routedState, "Changes age or pregnancy scope and routes the interview."},
	"correct_fact":        {anyState, []string{StateHistoryReview, StateInterrupted, StateUnsupported}, "Replaces one finding and returns to review."},
	"report_emergency":    {anyState, []string{StateInterrupted, StateUnsupported}, "Records an emergency answer, which always warns."},
	"visual_observation":  {[]string{StateInterviewing, StateHistoryReview, StateNeedsDetail, StateAssessmentReady, StateFailedRecoverably}, []string{StateInterviewing, StateInterrupted, StateHistoryReview}, "Adds the user's own observation and routes the interview."},
	"safety_check":        {anyState, []string{same, StateInterviewing, StateInterrupted, StateUnsupported}, "The app's safety-net question."},
	"red_flag":            {anyState, []string{same, StateInterrupted}, "One of the app's red-flag questions, answered on the review screen."},
	"set_episode":         {anyState, []string{same}, "Names or joins an episode."},
	"episode_log":         {anyState, []string{same, StateInterviewing, StateInterrupted, StateUnsupported, StateHistoryReview}, "A return-visit entry."},
	"witness":             {anyState, []string{same, StateInterviewing, StateInterrupted, StateUnsupported, StateHistoryReview}, "A witness account."},
	"medication_use":      {anyState, []string{same, StateInterviewing, StateInterrupted, StateUnsupported, StateHistoryReview}, "How a medicine is actually taken."},
	"resolve_sources":     {anyState, []string{same, StateInterviewing, StateInterrupted, StateUnsupported, StateHistoryReview}, "A reconciliation of two sources."},
	"clinician_feedback":  {anyState, []string{same, StateInterviewing, StateInterrupted, StateUnsupported, StateHistoryReview}, "Feedback relayed by the user."},
	"review_report_row":   {anyState, anyState, "Report review keeps or routes the interview."},
	"withdraw_report_row": {anyState, anyState, "Report review keeps or routes the interview."},
	"authorize_report":    {anyState, anyState, "Report review keeps or routes the interview."},
	"report_identity":     {anyState, anyState, "Report review keeps or routes the interview."},
	"add_report_passage":  {anyState, anyState, "Report review keeps or routes the interview."},
	"add_outcome":         {anyState, []string{same, StateInterviewing, StateInterrupted, StateHistoryReview}, "Records what happened after the visit."},
	"review":              {except(StateUnsupported), []string{StateHistoryReview}, "The user asks to review the history."},
	"confirm":             {[]string{StateHistoryReview}, []string{StateHistoryReview}, "The user confirms this history revision."},
	"assess":              {[]string{StateHistoryReview, StateNeedsDetail, StateAssessmentReady}, []string{StateAssessing}, "The user requests an assessment of a confirmed history."},
	"retry":               {[]string{StateFailedRecoverably}, []string{StateAssessing, StateInterviewing}, "The user retries a failed model request."},
	"focus_interview":     {[]string{StateInterviewing}, []string{StateInterviewing, StateHistoryReview}, "The user chooses a shorter interview."},
	"continue":            {except(StateUnsupported), []string{StateInterviewing, StateInterrupted, StateHistoryReview}, "The user asks for more questions."},
	"change_provider":     {anyState, []string{StateHistoryReview, StateUnsupported}, "A confirmed change of model destination."},
	"model:interview":     {[]string{StateInterviewing, StateHistoryReview}, []string{StateInterviewing, StateInterrupted, StateHistoryReview}, "A validated interview proposal."},
	"model:assessment":    {[]string{StateAssessing}, []string{StateAssessmentReady, StateNeedsDetail}, "A validated assessment."},
	"fail:interview":      {[]string{StateInterviewing, StateHistoryReview}, []string{StateFailedRecoverably}, "A failed interview request keeps the history."},
	"fail:assessment":     {[]string{StateAssessing}, []string{StateFailedRecoverably}, "A failed assessment keeps the history."},
}

// checkFrom refuses an action the table doesn't allow from this state.
func checkFrom(action, state string) error {
	r, ok := transitionTable[action]
	if !ok {
		return errors.New("Unknown encounter action")
	}
	if !slices.Contains(r.from, state) {
		return fmt.Errorf("This action isn't available while the encounter is %s", strings.ReplaceAll(state, "_", " "))
	}
	return nil
}

// checkTransition reports whether a change from one state to another is in the table.
func checkTransition(action, from, to string) error {
	r, ok := transitionTable[action]
	if !ok {
		return fmt.Errorf("%s is not in the transition table", action)
	}
	if !slices.Contains(r.from, from) {
		return fmt.Errorf("%s is not allowed from %s", action, from)
	}
	if slices.Contains(r.to, to) || to == from && slices.Contains(r.to, same) {
		return nil
	}
	return fmt.Errorf("%s from %s ended in %s, which the table does not allow", action, from, to)
}

// StateDiagram renders the transition table as Markdown with a Mermaid diagram
// of the actions that change the main flow. docs/state-machine.md is generated
// from it.
func StateDiagram() string {
	var b strings.Builder
	b.WriteString("# Encounter state machine\n\nGenerated from `internal/clinical/transitions.go` by `UPDATE_STATE_DIAGRAM=1 go test -run TestTransitionTable ./internal/clinical`. Do not edit by hand.\n\n")
	b.WriteString("```mermaid\nstateDiagram-v2\n")
	actions := make([]string, 0, len(transitionTable))
	for a := range transitionTable {
		actions = append(actions, a)
	}
	slices.Sort(actions)
	edges := map[string][]string{}
	var order []string
	for _, a := range actions {
		r := transitionTable[a]
		if len(r.from) == len(allStates) {
			continue // shown in the table only
		}
		for _, f := range r.from {
			for _, t := range r.to {
				if t == same {
					t = f
				}
				k := f + " --> " + t
				if _, ok := edges[k]; !ok {
					order = append(order, k)
				}
				edges[k] = append(edges[k], a)
			}
		}
	}
	slices.Sort(order)
	for _, k := range order {
		fmt.Fprintf(&b, "    %s: %s\n", k, strings.Join(edges[k], ", "))
	}
	b.WriteString("```\n\n| Action | From | To | Note |\n|---|---|---|---|\n")
	list := func(states []string) string {
		if len(states) == len(allStates) {
			return "any state"
		}
		return "`" + strings.Join(states, "`, `") + "`"
	}
	for _, a := range actions {
		r := transitionTable[a]
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", a, list(r.from), list(r.to), r.note)
	}
	return b.String()
}
