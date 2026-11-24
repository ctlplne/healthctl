package clinical

import "testing"

func TestFactCategories(t *testing.T) {
	e := guided(t, "I have a headache.")
	turn(t, e, Proposal{Action: "ask", Question: backgroundTopics[0].question, Reason: "Medicines can matter."})
	apply(t, e, Command{Action: "answer", Text: "I take a blue tablet for my blood pressure."})
	med := e.Answers[len(e.Answers)-1]
	turn(t, e, Proposal{Action: "ask", Question: backgroundTopics[1].question, Reason: "Allergies can matter."})
	apply(t, e, Command{Action: "answer", Text: "Penicillin gives me hives."})
	allergy := e.Answers[len(e.Answers)-1]
	turn(t, e, Proposal{Action: "review", Findings: []FindingProposal{
		find(med, "I take a blue tablet for my blood pressure", "Blue tablet for blood pressure", "reported_present"),
		find(allergy, "Penicillin gives me hives", "Hives from penicillin", "reported_present"),
		find(e.Answers[0], "I have a headache", "Headache", "reported_present"),
	}})
	got := map[string]string{}
	for _, f := range e.Facts {
		got[f.Concept] = f.Category
	}
	if got["Blue tablet for blood pressure"] != "medication" || got["Hives from penicillin"] != "allergy" || got["Headache"] != "" {
		t.Fatalf("categories must come from where each finding was recorded: %v", got)
	}
}
