package clinical

import (
	"errors"
	"strings"

	"github.com/ctlplne/healthctl/internal/contract"
)

// Sanitize validates each item of a model proposal on its own. Invalid
// findings, urgent flags, choices, outstanding questions and assessment items
// are dropped with a reason code, and the valid remainder is kept. Problems
// that affect the whole proposal, such as an invalid action or question, still
// return an error so the caller can request one repair.
//
// The result always passes Validate, so Accept stays a strict gate.
func Sanitize(in Input, p Proposal) (Proposal, []Rejection, error) {
	var rejected []Rejection
	drop := func(field string, i int, code string) { rejected = append(rejected, Rejection{field, i, code}) }
	strs := func(field string, items []string, maxN, maxLen int) []string {
		kept := []string{}
		for i, s := range items {
			switch {
			case len(kept) >= maxN:
				drop(field, i, ReasonTooMany)
			case !validText(s, maxLen):
				drop(field, i, ReasonTooLong)
			case badOutput(s, maxLen):
				drop(field, i, ReasonAdvice)
			case strings.Contains(strings.ToLower(s), "[report:"):
				drop(field, i, ReasonReportReference)
			default:
				kept = append(kept, s)
			}
		}
		return kept
	}
	out := p
	out.Outstanding = strs("outstanding", p.Outstanding, contract.MaxOutstanding, contract.MaxOutstandingBytes)
	out.Choices = strs("choices", p.Choices, contract.MaxChoices, contract.MaxChoiceBytes)
	out.Findings = []FindingProposal{}
	out.UrgentEvidence = []Evidence{}
	out.Items = []Item{}
	switch in.Task {
	case "interview":
		if p.Action != "ask" {
			for i := range out.Choices {
				drop("choices", i, ReasonQuestion)
			}
			out.Choices = []string{}
		}
		if p.Action == "ask" && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(p.Reason)), "[intent") {
			kept := []string{}
			for i, c := range out.Choices {
				if choiceAllowed(in, c) {
					kept = append(kept, c)
				} else {
					drop("choices", i, ReasonChoiceSymptom)
				}
			}
			out.Choices = kept
		}
		for i, f := range p.Findings {
			if len(out.Findings) >= contract.MaxFindings {
				drop("findings", i, ReasonTooMany)
				continue
			}
			if err := validateFinding(in, f); err != nil {
				drop("findings", i, codeOr(err, ReasonAttributes))
				continue
			}
			out.Findings = append(out.Findings, f)
		}
		for i, ev := range p.UrgentEvidence {
			if len(out.UrgentEvidence) >= contract.MaxUrgent {
				drop("urgentEvidence", i, ReasonTooMany)
				continue
			}
			if err := validateUrgent(in, ev); err != nil {
				drop("urgentEvidence", i, codeOr(err, ReasonQuoteNotFound))
				continue
			}
			out.UrgentEvidence = append(out.UrgentEvidence, ev)
		}
		for i := range p.Items {
			drop("items", i, ReasonInvalidAction)
		}
	case "assessment":
		// An assessment cannot change the history. Extra fields are ignored.
		for i := range p.Findings {
			drop("findings", i, ReasonAlteredHistory)
		}
		for i := range p.UrgentEvidence {
			drop("urgentEvidence", i, ReasonAlteredHistory)
		}
		for i := range out.Choices {
			drop("choices", i, ReasonAlteredHistory)
		}
		out.Choices = []string{}
		if out.Question != "" {
			drop("question", 0, ReasonAlteredHistory)
			out.Question = ""
		}
		if p.Action == "differential" {
			facts := activeFacts(in)
			names := map[string]bool{}
			for i, item := range p.Items {
				if len(out.Items) >= contract.MaxItems {
					drop("items", i, ReasonTooMany)
					continue
				}
				if err := validateItem(item, facts); err != nil {
					drop("items", i, codeOr(err, ReasonItemContent))
					continue
				}
				name := strings.ToLower(strings.TrimSpace(item.Name))
				if names[name] {
					drop("items", i, ReasonDuplicate)
					continue
				}
				names[name] = true
				out.Items = append(out.Items, item)
			}
		} else {
			for i := range p.Items {
				drop("items", i, ReasonInvalidAction)
			}
		}
	}
	if err := Validate(in, out); err != nil {
		var v *ValidationError
		if errors.As(err, &v) {
			v.Items = rejected
		}
		return Proposal{}, rejected, err
	}
	return out, rejected, nil
}

func codeOr(err error, fallback string) string {
	if code := ReasonOf(err); code != "" {
		return code
	}
	return fallback
}
