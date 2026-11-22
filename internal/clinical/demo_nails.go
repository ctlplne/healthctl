package clinical

const DemoNailConcern = "My fingernails have changed shape."

type demoStep struct {
	question string
	reason   string
	choices  []string
	concept  string
}

func nailDemo(in Input) Proposal {
	p := Proposal{Action: "insufficient", Outstanding: []string{"The appearance and its cause need review by a healthcare professional."}}
	if in.Age < 18 || in.Background != "" || len(in.ReportEntries) != 0 || len(in.ActiveWarnings) != 0 || in.Task != "interview" {
		return p
	}
	appearance := demoStep{"What looks different about your fingernails?", "Your own description tells us what changed.", []string{"The centers dip inward", "Not sure"}, "User description of nail shape"}
	location := demoStep{"Which nails have changed?", "A change confined to one nail has a different history from a change across several.", []string{"One thumbnail", "Several nails on both hands", "Not sure"}, "Distribution of nail change"}
	timing := demoStep{"When did you first notice the change?", "Timing helps distinguish a recent change from a gradual one.", []string{"Gradually over three months", "Last week", "Not sure"}, "First noticed nail change"}
	trauma := demoStep{"What happened to that thumb before you first noticed the change?", "An injury to a single thumb would change the next questions.", []string{"I caught it in a drawer", "Nothing I remember", "Not sure"}, "Events before the single-nail change"}
	impact := demoStep{"What else, if anything, has changed in your day-to-day activities over that time?", "A change in your usual activities helps your clinician understand the bigger picture.", []string{"I need to rest halfway up the stairs", "Nothing else I've noticed", "Not sure"}, "Self-reported change in daily activity"}
	baseline := demoStep{"How does climbing stairs now compare with before the nail changes?", "A concrete comparison records how much your usual activity has changed.", []string{"I used to climb two flights in one go", "Not sure"}, "Previous activity level"}
	var findings []FindingProposal
	steps := []demoStep{appearance, location, timing}
	done := 0
	for _, a := range in.Answers {
		if fixtureBackground(a) {
			continue
		}
		switch {
		case a.Kind == "concern" && a.Text == DemoNailConcern:
			findings = append(findings, FindingProposal{Evidence: Evidence{a.ID, a.Text}, Concept: "Self-reported fingernail change", Status: "reported_present", Subject: "self"})
		case a.Kind == "screen" && a.Text == urgentNo:
		case a.Kind == "reply" && done < len(steps) && a.Question == steps[done].question:
			step := steps[done]
			if !contains(step.choices, a.Text) && evidenceDisposition(a) != "skipped" && a.Disposition != "not_applicable" {
				return p
			}
			if evidenceDisposition(a) == "provided" && a.Text != "Nothing I remember" && a.Text != "Nothing else I've noticed" {
				finding := FindingProposal{Evidence: Evidence{a.ID, a.Text}, Concept: step.concept, Status: "reported_present", Subject: "self"}
				if step.question == baseline.question {
					finding.TimeContext = "Before the nail changes"
				}
				findings = append(findings, finding)
			}
			if a.Question == location.question {
				switch a.Text {
				case "One thumbnail":
					steps = append(steps, trauma)
				case "Several nails on both hands":
					steps = append(steps, impact)
				}
			}
			if a.Question == impact.question && a.Text == impact.choices[0] {
				steps = append(steps, baseline)
			}
			done++
		default:
			return p
		}
	}
	p.Findings = findings
	if done == len(steps) {
		p.Action = "review"
		return p
	}
	step := steps[done]
	p.Action, p.Question, p.Reason, p.Choices = "ask", step.question, step.reason, step.choices
	p.Outstanding = []string{step.question}
	return p
}
