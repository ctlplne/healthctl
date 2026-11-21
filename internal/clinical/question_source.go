package clinical

import (
	"errors"
	"regexp"
	"strings"
)

var reportReason = regexp.MustCompile(`^\[report:(rr[1-9][0-9]{0,6})\] ([^\n]+)$`)
var refersToReport = regexp.MustCompile(`(?i)\b((?:this|the|your|that) (?:reported value|report|report entry|test result|lab result|sample|collection date|imaging finding)|according to (?:the|your) (?:note|scan)|the reported finding)\b`)

func questionSource(in Input, p Proposal) (*QuestionSource, error) {
	if strings.Contains(strings.ToLower(p.Question), "[report:") {
		return nil, errors.New("Keep source identifiers in the question reason")
	}
	for _, fields := range [][]string{p.Choices, p.Outstanding} {
		for _, s := range fields {
			if strings.Contains(strings.ToLower(s), "[report:") {
				return nil, errors.New("Keep source identifiers in the question reason")
			}
		}
	}
	if p.Action != "ask" {
		if strings.Contains(strings.ToLower(p.Reason), "[report:") {
			return nil, errors.New("Only an interview question can identify a report source")
		}
		return nil, nil
	}
	text := strings.TrimSpace(p.Reason)
	match := reportReason.FindStringSubmatch(text)
	if match == nil {
		if strings.Contains(strings.ToLower(text), "[report") {
			return nil, errors.New("Use exactly one valid reviewed report reference at the start of the question reason")
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
				return nil, errors.New("Identify the approved reviewed row used for this question")
			}
		}
		return &QuestionSource{Reason: text}, nil
	}
	reason := strings.TrimSpace(match[2])
	if strings.Contains(strings.ToLower(reason), "[report") || reason == "" {
		return nil, errors.New("Use one principal source and a plain explanation for the question")
	}
	for _, r := range in.ReportEntries {
		if r.ReviewID == match[1] {
			words := strings.ToLower(p.Question + " " + reason)
			target := strings.ToLower(strings.TrimSpace(r.Name))
			if !strings.Contains(words, target) {
				for _, other := range in.ReportEntries {
					name := strings.ToLower(strings.TrimSpace(other.Name))
					if other.ReviewID != r.ReviewID && len(name) >= 4 && strings.Contains(words, name) {
						return nil, errors.New("The named report entry does not match the question reference")
					}
				}
			}
			return &QuestionSource{DocumentID: r.DocumentID, RowID: r.RowID, ReviewID: r.ReviewID, Reason: reason, Key: "model_report"}, nil
		}
	}
	return nil, errors.New("The question source is not a currently approved reviewed report row")
}
