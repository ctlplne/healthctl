package clinical

import (
	"errors"
	"regexp"
	"strings"
)

var reportReason = regexp.MustCompile(`^\[report:(rr[1-9][0-9]{0,6})\] ([^\n]+)$`)
var refersToReport = regexp.MustCompile(`(?i)\b((?:this|the|your|that) (?:reported value|report|report entry|test result|lab result|sample|collection date|imaging finding)|according to (?:the|your) (?:note|scan)|the reported finding)\b`)

// Free-text questions keep the model's note, labeled as a model suggestion.
// The "Why this question?" text is the app's.
const freeTextReason = "The model suggested this question to fill in your history."
const reportQuestionReason = "This question is about a report entry you reviewed."

// questionSource works out who wrote each part of a proposed question. An
// intent question gets the app's own wording, choices and reason.
func questionSource(in Input, p Proposal) (resolved, error) {
	tagged := func(s string) bool {
		s = strings.ToLower(s)
		return strings.Contains(s, "[report:") || strings.Contains(s, "[intent")
	}
	if tagged(p.Question) {
		return resolved{}, errors.New("Keep source identifiers in the question reason")
	}
	for _, fields := range [][]string{p.Choices, p.Outstanding} {
		for _, s := range fields {
			if tagged(s) {
				return resolved{}, errors.New("Keep source identifiers in the question reason")
			}
		}
	}
	if p.Action != "ask" {
		if tagged(p.Reason) {
			return resolved{}, errors.New("Only an interview question can identify a report source or intent")
		}
		return resolved{}, nil
	}
	text := strings.TrimSpace(p.Reason)
	if strings.HasPrefix(strings.ToLower(text), "[intent") {
		return intentSource(in, text)
	}
	if strings.Contains(strings.ToLower(text), "[intent") {
		return resolved{}, reasonErr(ReasonUnknownIntent, "Put the intent tag at the start of the question reason")
	}
	free := func(src *QuestionSource) resolved {
		return resolved{source: src, question: p.Question, choices: p.Choices}
	}
	match := reportReason.FindStringSubmatch(text)
	if match == nil {
		if strings.Contains(strings.ToLower(text), "[report") {
			return resolved{}, errors.New("Use exactly one valid reviewed report reference at the start of the question reason")
		}
		if len(in.ReportEntries) > 0 {
			s := strings.ToLower(p.Question + " " + p.Reason)
			usesRow := refersToReport.MatchString(s)
			for _, r := range in.ReportEntries {
				name := strings.ToLower(strings.TrimSpace(r.Name))
				if len(name) >= 4 && strings.Contains(s, name) {
					usesRow = true
				}
			}
			if usesRow {
				return resolved{}, errors.New("Identify the approved reviewed row used for this question")
			}
		}
		return free(&QuestionSource{Reason: freeTextReason, ModelNote: text}), nil
	}
	reason := strings.TrimSpace(match[2])
	if strings.Contains(strings.ToLower(reason), "[report") || reason == "" {
		return resolved{}, errors.New("Use one principal source and a plain explanation for the question")
	}
	for _, r := range in.ReportEntries {
		if r.ReviewID == match[1] {
			words := strings.ToLower(p.Question + " " + reason)
			target := strings.ToLower(strings.TrimSpace(r.Name))
			if !strings.Contains(words, target) {
				for _, other := range in.ReportEntries {
					name := strings.ToLower(strings.TrimSpace(other.Name))
					if other.ReviewID != r.ReviewID && len(name) >= 4 && strings.Contains(words, name) {
						return resolved{}, errors.New("The named report entry does not match the question reference")
					}
				}
			}
			return free(&QuestionSource{DocumentID: r.DocumentID, RowID: r.RowID, ReviewID: r.ReviewID, Reason: reportQuestionReason, ModelNote: reason, Key: "model_report"}), nil
		}
	}
	return resolved{}, errors.New("The question source is not a currently approved reviewed report row")
}
