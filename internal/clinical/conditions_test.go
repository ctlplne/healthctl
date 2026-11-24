package clinical

import (
	"strings"
	"testing"
)

func assessed(t *testing.T, concern, quote, concept string, items []Item, meta ModelMeta) *Encounter {
	t.Helper()
	e := intake(t, concern, false)
	turn(t, e, Proposal{Action: "review", Findings: []FindingProposal{find(e.Answers[0], quote, concept, "reported_present")}})
	apply(t, e, Command{Action: "confirm"})
	apply(t, e, Command{Action: "assess"})
	for i := range items {
		items[i].Supporting = []string{e.Facts[0].ID}
		items[i].Limitations = []string{"An examination may distinguish the possibilities."}
		items[i].Explanation = "This condition can cause a cough."
	}
	if err := e.Accept("assessment", Proposal{Action: "differential", Items: items}, meta, ""); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestAssessmentRendering(t *testing.T) {
	e := assessed(t, "I have had a cough for two days.", "a cough for two days", "Cough", []Item{{Name: "chest cold"}, {Name: "Mystery syndrome"}}, ModelMeta{})
	a := e.Assessments[len(e.Assessments)-1]
	vetted, other := a.Items[0], a.Items[1]
	if !vetted.Vetted || vetted.Name != "Acute bronchitis" || vetted.ClinicalTerm != "Acute bronchitis" || vetted.Description == "" || !strings.Contains(vetted.DescriptionStatus, "not clinically reviewed") {
		t.Fatalf("a listed condition must use the app's name and description: %+v", vetted)
	}
	if other.Vetted || other.Name != "Mystery syndrome" || other.Description != "" {
		t.Fatalf("an unlisted name must stay labeled as the model's suggestion: %+v", other)
	}
	// The explanation is the app's rationale from the cited facts.
	if !strings.Contains(vetted.Explanation, "Cough") {
		t.Fatalf("rationale must be built from the cited fact: %q", vetted.Explanation)
	}
}

func TestDontMissLines(t *testing.T) {
	e := assessed(t, "I have had a cough for two days.", "a cough for two days", "Cough", []Item{{Name: "Common cold"}}, ModelMeta{})
	a := e.Assessments[len(e.Assessments)-1]
	if len(a.DontMiss) != 2 || !strings.HasPrefix(a.DontMiss[0], "Ask your clinician") {
		t.Fatalf("a respiratory assessment without pneumonia or a clot must carry both lines: %v", a.DontMiss)
	}
	e = assessed(t, "I have had a cough for two days.", "a cough for two days", "Cough", []Item{{Name: "Pneumonia"}, {Name: "pulmonary embolism"}}, ModelMeta{})
	if a := e.Assessments[len(e.Assessments)-1]; len(a.DontMiss) != 0 {
		t.Fatalf("a listed dangerous cause needs no extra line: %v", a.DontMiss)
	}
	for _, lines := range dontMiss {
		for _, d := range lines {
			if _, ok := findCondition(d.condition); !ok {
				t.Fatalf("don't-miss condition %q is not on the condition list", d.condition)
			}
			if advice := alwaysBlocked.MatchString(d.line); advice {
				t.Fatalf("a don't-miss line reads as certainty or advice: %q", d.line)
			}
		}
	}
}

func TestAssessmentStability(t *testing.T) {
	first := []Item{{Name: "Common cold"}, {Name: "flu"}, {Name: "Mystery syndrome"}}
	second := []Item{{Name: "viral URI"}, {Name: "Influenza"}}
	marked := MarkUnstable(first, second)
	if marked[0].Unstable || marked[1].Unstable || !marked[2].Unstable || len(marked) != 3 {
		t.Fatalf("only items missing from the second run are unstable, and none is dropped: %+v", marked)
	}
	apply := func(e *Encounter, c Command) error { return e.Apply(c, "") }
	e := intake(t, "I have had a cough for two days.", false)
	turn(t, e, Proposal{Action: "review", Findings: []FindingProposal{find(e.Answers[0], "a cough for two days", "Cough", "reported_present")}})
	if err := apply(e, Command{Action: "confirm"}); err != nil {
		t.Fatal(err)
	}
	if err := apply(e, Command{Action: "assess", StabilityCheck: true}); err != nil || !e.StabilityRequested {
		t.Fatalf("the stability request was not recorded: %v", err)
	}
	items := MarkUnstable([]Item{{Name: "Common cold", Supporting: []string{e.Facts[0].ID}, Limitations: []string{"Unknown course."}, Explanation: "This condition can cause a cough."}}, nil)
	if err := e.Accept("assessment", Proposal{Action: "differential", Items: items}, ModelMeta{Settings: map[string]any{"stabilityCheck": true}}, ""); err != nil {
		t.Fatal(err)
	}
	a := e.Assessments[len(e.Assessments)-1]
	if !a.StabilityChecked || !a.Items[0].Unstable {
		t.Fatalf("the assessment must record the comparison and keep the marks: %+v", a)
	}
}
