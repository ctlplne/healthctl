package clinical

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ValidatorVersion identifies the rules that accepted or rejected a proposal.
const ValidatorVersion = "validator-3"

// Reason codes name every way the validator can reject part of a proposal.
// They are fixed application strings with no user or model text, so they are
// safe to store, log and send back to a model in a repair request.
const (
	ReasonQuoteNotFound          = "quote_not_found"
	ReasonQuoteAmbiguous         = "quote_ambiguous"
	ReasonSuperseded             = "superseded_by_user"
	ReasonAttributes             = "invalid_attributes"
	ReasonNonanswer              = "nonanswer_disposition"
	ReasonBackgroundScope        = "background_scope"
	ReasonScreenAnswer           = "screen_answer"
	ReasonAdvice                 = "advice_language"
	ReasonUngroundedTiming       = "ungrounded_timing"
	ReasonValueNotQuoted         = "value_not_in_quote"
	ReasonExperiencer            = "experiencer_mismatch"
	ReasonNotAsked               = "not_asked_status"
	ReasonDenialUnsupported      = "denial_unsupported"
	ReasonNegated                = "negated_context"
	ReasonHypothetical           = "hypothetical_context"
	ReasonUncertain              = "uncertain_context"
	ReasonUncertaintyUnsupported = "uncertainty_unsupported"
	ReasonHistoricalTime         = "historical_needs_time"
	ReasonVague                  = "vague_answer"
	ReasonDeclineUnsupported     = "decline_unsupported"
	ReasonUngroundedNumber       = "ungrounded_number"
	ReasonFactReference          = "invalid_fact_reference"
	ReasonDuplicate              = "duplicate_item"
	ReasonItemContent            = "item_content"
	ReasonTooLong                = "too_long"
	ReasonTooMany                = "too_many_items"
	ReasonReportReference        = "report_reference"
	ReasonInvalidAction          = "invalid_action"
	ReasonQuestion               = "question_rules"
	ReasonMultipleQuestions      = "multiple_questions"
	ReasonAlreadyAnswered        = "already_answered"
	ReasonMissingOutstanding     = "missing_outstanding"
	ReasonAlteredHistory         = "assessment_alters_history"
	ReasonEmptyDifferential      = "empty_differential"
	ReasonUnknownTask            = "unknown_task"
	ReasonUnknownIntent          = "unknown_intent"
	ReasonChoiceSymptom          = "choice_introduces_symptom"
)

// reasonHints are the fixed explanations sent back with a repair request.
var reasonHints = map[string]string{
	ReasonQuoteNotFound:          "quote a contiguous span that exists in the cited active answer",
	ReasonUnknownIntent:          "use one intent from the application catalog, with a topic only where the intent needs one",
	ReasonChoiceSymptom:          "remove choices that name a symptom or urgent sign the user has not reported",
	ReasonQuoteAmbiguous:         "quote enough surrounding words to identify one occurrence",
	ReasonSuperseded:             "do not restore a finding the user corrected",
	ReasonAttributes:             "use the allowed status and subject values and the field length limits",
	ReasonNonanswer:              "a skipped, unknown or not-applicable answer cannot establish a finding",
	ReasonBackgroundScope:        "a none answer only denies the topic of its own question",
	ReasonScreenAnswer:           "never cite the application's emergency screen answer",
	ReasonAdvice:                 "remove advice, certainty, percentages, links and instructions",
	ReasonUngroundedTiming:       "only use numbers in timing that appear in the cited answer",
	ReasonValueNotQuoted:         "values and units must appear in the quote",
	ReasonExperiencer:            "set the subject to the person the quote is about",
	ReasonNotAsked:               "put unasked topics in outstanding instead of findings",
	ReasonDenialUnsupported:      "a denial must quote the negation that applies to that concept",
	ReasonNegated:                "the quote is negated, so it cannot be reported present",
	ReasonHypothetical:           "the quote is hypothetical, so it is not a finding",
	ReasonUncertain:              "the user is unsure, so use status uncertain",
	ReasonUncertaintyUnsupported: "use status uncertain only when the user expressed uncertainty",
	ReasonHistoricalTime:         "the quote is about the past, so set its time context",
	ReasonVague:                  "a vague or uncertain answer cannot be reported present",
	ReasonDeclineUnsupported:     "use status declined only when the user declined",
	ReasonUngroundedNumber:       "numbers in explanations must appear in the cited findings",
	ReasonFactReference:          "cite distinct, active reported, denied or reviewed fact IDs",
	ReasonDuplicate:              "list each possibility once",
	ReasonItemContent:            "each possibility needs a name, explanation, supporting facts and limitations",
	ReasonTooLong:                "shorten the field",
	ReasonTooMany:                "return fewer items",
	ReasonReportReference:        "use one valid reviewed report reference at the start of the reason only",
	ReasonInvalidAction:          "use an action allowed for this task",
	ReasonQuestion:               "an ask action needs one short question and a reason",
	ReasonMultipleQuestions:      "ask one question per turn",
	ReasonAlreadyAnswered:        "do not repeat a question the user already answered",
	ReasonMissingOutstanding:     "list the specific missing information",
	ReasonAlteredHistory:         "an assessment cannot add findings, warnings or questions",
	ReasonEmptyDifferential:      "use insufficient when no possibility is supported",
	ReasonUnknownTask:            "use the task named by the application",
}

// ValidationError carries a reason code and a user-safe message.
type ValidationError struct {
	Code    string
	Message string
	Items   []Rejection
}

func (v *ValidationError) Error() string { return v.Message }

// ReasonCode lets other packages record the failure without importing clinical.
func (v *ValidationError) ReasonCode() string { return v.Code }

// RepairHint is the app-authored text sent with a repair request.
func (v *ValidationError) RepairHint() string {
	parts := []string{fmt.Sprintf("%s: %s", v.Code, reasonHints[v.Code])}
	for _, r := range v.Items {
		parts = append(parts, fmt.Sprintf("%s[%d] %s: %s", r.Field, r.Index, r.Reason, reasonHints[r.Reason]))
	}
	return "The previous proposal failed these application checks. " + strings.Join(parts, "; ") + "."
}

func reasonErr(code, message string) error { return &ValidationError{Code: code, Message: message} }

// ReasonOf returns the reason code for err, or "" when it has none.
func ReasonOf(err error) string {
	var v *ValidationError
	if errors.As(err, &v) {
		return v.Code
	}
	return ""
}

// Rejection records one dropped item of a proposal.
type Rejection struct {
	Field  string `json:"field"`
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

// RejectionCounts summarizes rejections by reason code.
func RejectionCounts(items []Rejection) map[string]int {
	out := map[string]int{}
	for _, r := range items {
		out[r.Reason]++
	}
	return out
}

// ReasonCodes lists every known reason code.
func ReasonCodes() []string {
	out := make([]string, 0, len(reasonHints))
	for k := range reasonHints {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
