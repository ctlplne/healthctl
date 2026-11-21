package clinical

import "strings"

// Demo is an authored example with an exact synthetic storyline. It cannot
// produce fixture diagnoses for arbitrary symptoms supplied by the operator.
func Demo(in Input) Proposal {
	if in.Concern == DemoReportConcern {
		return reportDemo(in)
	}
	const onset = "When did the runny nose and sneezing begin?"
	const trigger = "When did you notice the symptoms most?"
	p := Proposal{Action: "insufficient", Findings: []FindingProposal{}, UrgentEvidence: []Evidence{}, Choices: []string{}, Items: []Item{}, Outstanding: []string{"The synthetic example uses the provided scenario and suggested answers. Use a configured provider for other histories."}}
	if len(in.ReportEntries) > 0 || in.Concern != DemoConcern || in.Age < 18 || in.Background != "" || len(in.ActiveWarnings) > 0 {
		return p
	}
	finding := func(a Answer, quote, concept string) FindingProposal {
		return FindingProposal{Evidence: Evidence{a.ID, quote}, Concept: concept, Status: "reported_present", Subject: "self"}
	}
	var first, second bool
	var facts []FindingProposal
	for _, a := range in.Answers {
		if fixtureBackground(a) {
			continue
		}
		switch a.Kind {
		case "concern":
			if a.Text != DemoConcern {
				return p
			}
			facts = append(facts, finding(a, "nose is runny", "Runny nose"), finding(a, "sneezing", "Sneezing"))
		case "screen":
			if a.Text != urgentNo {
				return p
			}
		case "reply":
			if a.Question == onset && a.Text == "Yesterday" {
				f := finding(a, a.Text, "Symptom onset")
				f.TimeContext = "Yesterday"
				facts = append(facts, f)
				first = true
			} else if a.Question == trigger && a.Text == "After being outdoors" {
				facts = append(facts, finding(a, a.Text, "Outdoor association"))
				second = true
			} else {
				return p
			}
		default:
			return p
		}
	}
	if in.Task == "interview" {
		p.Findings = facts
		switch {
		case !first:
			p.Action = "ask"
			p.Question = onset
			p.Choices = []string{"Yesterday", "Not sure"}
			p.Reason = "Timing helps distinguish a new episode from a longer-running concern."
			p.Outstanding = []string{onset}
		case !second:
			p.Action = "ask"
			p.Question = trigger
			p.Choices = []string{"After being outdoors", "Not sure"}
			p.Reason = "A consistent exposure could change the explanations considered."
			p.Outstanding = []string{trigger}
		default:
			p.Action = "review"
			p.Outstanding = []string{"No examination or testing has been recorded."}
		}
		return p
	}
	if in.Task != "assessment" || !first || !second {
		p.Outstanding = []string{"The synthetic history needs the timing and circumstances of the symptoms."}
		return p
	}
	ids := map[string]string{}
	for _, f := range in.Facts {
		if f.Active && f.Status == "reported_present" {
			ids[f.Concept] = f.ID
		}
	}
	for _, c := range []string{"Runny nose", "Sneezing", "Outdoor association"} {
		if ids[c] == "" {
			return p
		}
	}
	p.Action = "differential"
	p.Outstanding = []string{"No examination or testing has been recorded."}
	p.Items = []Item{
		{Name: "Nasal allergies", ClinicalTerm: "Allergic rhinitis", Explanation: "Nasal allergies can cause a runny nose and sneezing.", Supporting: []string{ids["Runny nose"], ids["Sneezing"], ids["Outdoor association"]}, Conflicting: []string{}, Limitations: []string{"The specific exposure and a pattern of recurring symptoms are unknown."}},
		{Name: "Common cold", ClinicalTerm: "Viral upper respiratory infection", Explanation: "This condition can cause nasal irritation and sneezing.", Supporting: []string{ids["Runny nose"], ids["Sneezing"], ids["Symptom onset"]}, Conflicting: []string{}, Limitations: []string{"The subsequent course and details of sick contacts are unknown."}},
	}
	return p
}

const DemoReportConcern = "It burns when I urinate."

func reportDemo(in Input) Proposal {
	p := Proposal{Action: "insufficient", Outstanding: []string{"The fictional report example needs its reviewed, selected urine entry and the timing of symptoms relative to the sample."}}
	if in.Age < 18 || in.Background != "" || in.Concern != DemoReportConcern || len(in.ReportEntries) != 1 {
		return p
	}
	entry := in.ReportEntries[0]
	if !strings.EqualFold(entry.Name, "Leukocyte esterase") || !strings.EqualFold(entry.Value, "trace") || entry.CollectionDate == "" || entry.Subject != "self" || entry.Uncertain || entry.Note != "" {
		return p
	}
	var symptom, timing *Answer
	for i := range in.Answers {
		a := &in.Answers[i]
		if fixtureBackground(*a) {
			continue
		}
		switch a.Kind {
		case "concern":
			if a.Text != DemoReportConcern {
				return p
			}
			symptom = a
		case "screen": // The urgency stream remains independent from this example.
		case "reply":
			if a.Text != "Before the sample" {
				return p
			}
			timing = a
		default:
			return p
		}
	}
	if symptom == nil {
		return p
	}
	for _, w := range in.ActiveWarnings {
		if w.Rule != "explicit_screen" {
			return p
		}
	}
	if in.Task == "interview" {
		p.Findings = []FindingProposal{{Evidence: Evidence{symptom.ID, symptom.Text}, Concept: "Burning with urination", Status: "reported_present", Subject: "self"}}
		if timing != nil {
			p.Findings = append(p.Findings, FindingProposal{Evidence: Evidence{timing.ID, timing.Text}, Concept: "Symptom onset relative to sample", Status: "reported_present", Subject: "self", TimeContext: timing.Text})
		}
		p.Action = "review"
		p.Outstanding = []string{"The course of the symptoms and further clinical findings are unknown."}
		return p
	}
	if in.Task != "assessment" || timing == nil {
		return p
	}
	var symptomID string
	for _, f := range in.Facts {
		if f.Active && f.SourceType == "patient_answer" && f.Status == "reported_present" && f.AnswerID == symptom.ID && f.Concept == "Burning with urination" {
			symptomID = f.ID
		}
	}
	if symptomID == "" {
		return p
	}
	p.Action = "differential"
	p.Outstanding = []string{"Other symptoms and clinical examination findings are unknown."}
	p.Items = []Item{
		{Name: "Bladder infection", ClinicalTerm: "Cystitis", Explanation: "This condition can cause burning with urination.", Supporting: []string{symptomID, entry.FactID}, Conflicting: []string{}, Limitations: []string{"The reported trace result alone cannot identify a cause."}},
		{Name: "Urinary irritation", ClinicalTerm: "", Explanation: "This condition can cause discomfort with urination.", Supporting: []string{symptomID}, Conflicting: []string{}, Limitations: []string{"Possible exposures and other clinical findings are unknown."}},
	}
	return p
}
