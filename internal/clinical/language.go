package clinical

import (
	"regexp"
	"slices"
	"strings"
)

// Match role labels, including chart headings, without rewriting an adjective
// such as “stay patient”, or words such as “outpatient”. Preserve quoted sources.
var roleLabels = regexp.MustCompile(`(?i)“[^”]*”|"[^"]*"|\b(?:the|this|these|a|each) patients?\b|\bpatients?[- ]reported\b|\bpatients?(?:['’]s|s['’])|(?:^|[.!?]\s+|\n)patients?\b|\bpatients\b`)

func userReference(text string) string {
	if !strings.Contains(strings.ToLower(text), "patient") {
		return text
	}
	return roleLabels.ReplaceAllStringFunc(text, func(match string) string {
		if strings.HasPrefix(match, "“") || strings.HasPrefix(match, `"`) {
			return match
		}
		lower := strings.ToLower(match)
		i := strings.Index(lower, "patient")
		if i < 0 {
			return match
		}
		replacement := "user"
		if strings.HasPrefix(lower[i:], "patient-reported") {
			replacement = "self"
		}
		if match[i] == 'P' {
			replacement = strings.ToUpper(replacement[:1]) + replacement[1:]
		}
		return match[:i] + replacement + match[i+len("patient"):]
	})
}

func userLabels(labels []string) []string {
	result := slices.Clone(labels)
	for i, label := range result {
		result[i] = userReference(label)
	}
	return result
}

func displaySource(source *QuestionSource) *QuestionSource {
	if source == nil {
		return nil
	}
	result := *source
	result.Reason = userReference(result.Reason)
	return &result
}

func displayItems(items []Item) []Item {
	result := slices.Clone(items)
	for i := range result {
		result[i].Explanation = userReference(result[i].Explanation)
		result[i].Limitations = userLabels(result[i].Limitations)
	}
	return result
}

// migrateWording rewrites role labels in older model text once, when a
// version 1 payload is read. It never changes verbatim evidence, report labels
// or the user's corrections. New model text is rewritten when it is accepted.
func migrateWording(e *Encounter) {
	e.Question = userReference(e.Question)
	e.QuestionSource = displaySource(e.QuestionSource)
	e.Choices = userLabels(e.Choices)
	e.Outstanding = userLabels(e.Outstanding)
	for i := range e.Answers {
		e.Answers[i].Question = userReference(e.Answers[i].Question)
		e.Answers[i].QuestionSource = displaySource(e.Answers[i].QuestionSource)
	}
	for i := range e.Turns {
		e.Turns[i].Question = userReference(e.Turns[i].Question)
		e.Turns[i].Reason = userReference(e.Turns[i].Reason)
		e.Turns[i].QuestionSource = displaySource(e.Turns[i].QuestionSource)
	}
	for i := range e.Summaries {
		e.Summaries[i].Outstanding = userLabels(e.Summaries[i].Outstanding)
	}
	for i := range e.Assessments {
		e.Assessments[i].Items = displayItems(e.Assessments[i].Items)
		e.Assessments[i].Outstanding = userLabels(e.Assessments[i].Outstanding)
	}
	for i, f := range e.Facts {
		if (f.SourceType == "patient_answer" || f.SourceType == "witness_observation") && !f.Corrected {
			e.Facts[i].Concept = userReference(f.Concept)
		}
	}
}

// ForDisplay returns the encounter as stored. Display never rewrites text:
// older wording is migrated once by DecodeEncounter.
func (e Encounter) ForDisplay() Encounter { return e }
