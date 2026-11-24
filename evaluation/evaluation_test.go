package evaluation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ctlplne/healthctl/internal/clinical"
)

func TestBothArmsAndAllCaseInvariants(t *testing.T) {
	cases, err := Cases()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 15 || len(cases) > 20 {
		t.Fatal("unexpected case count")
	}
	for _, c := range cases {
		for _, adaptive := range []bool{false, true} {
			r := Run(t.Context(), c, adaptive, Fixture{}, 5)
			if r.Failed() {
				t.Errorf("%s %s: %+v", c.ID, r.Arm, r)
			}
			if r.RevisedHistory <= r.AssessedRevision || r.AssessedRevision == 0 {
				t.Fatalf("%s missing revision check", c.ID)
			}
			if !adaptive && r.InterviewQuestions != 0 {
				t.Fatal("free-text baseline received follow-ups")
			}
		}
	}
}

type observingModel struct {
	t     *testing.T
	calls int
}

func (m *observingModel) Reference() clinical.ProviderRef { return Fixture{}.Reference() }
func (m *observingModel) Propose(ctx context.Context, in clinical.Input) (clinical.Proposal, clinical.ModelMeta, error) {
	m.calls++
	b, err := json.Marshal(in)
	if err != nil {
		m.t.Fatal(err)
	}
	for _, marker := range []string{"hidden-reference-482", "hidden-case-482", "hidden-concept-482", "hidden-answer-482"} {
		if strings.Contains(string(b), marker) {
			m.t.Fatalf("hidden marker was delivered to interviewer: %s", marker)
		}
	}
	return Fixture{}.Propose(ctx, in)
}

func TestHiddenDataAndUnaskedRepliesStayOutOfRequests(t *testing.T) {
	m := &observingModel{t: t}
	c := Case{ID: "hidden-case-482", Opening: "I have a headache.", Age: 40, Reference: "hidden-reference-482",
		Facts:   []ExpectedFact{{Label: "hidden-concept-482", Concept: "headache", Source: "headache", Subject: "self", Status: "reported_present"}},
		Replies: []Reply{{Topic: "trigger", Answer: "hidden-answer-482"}},
	}
	r := Run(t.Context(), c, true, m, 3)
	if r.Failed() || m.calls < 2 {
		t.Fatal(r)
	}
	p := patient{replies: []Reply{{Topic: "associated", WhenAsked: "(?i)nausea", Answer: "No nausea."}}, sent: map[int]bool{}}
	if strings.Contains(p.answer("Do you have a cough?"), "nausea") {
		t.Fatal("irrelevant specific question disclosed an unrelated fact")
	}
	if p.answer("Any nausea?") != "No nausea." {
		t.Fatal("matching question did not reveal fact")
	}
}

func TestScorerReportsIndividualInvariantFailures(t *testing.T) {
	e := clinical.Encounter{HistoryRecord: clinical.HistoryRecord{Answers: []clinical.Answer{{ID: "a1", Active: true, Text: "My father had chest pain last year."}}, Facts: []clinical.Fact{{
		ID: "f1", Active: true, FindingProposal: clinical.FindingProposal{
			Evidence: clinical.Evidence{AnswerID: "a1", Quote: "chest pain"}, Concept: "Chest pain", Status: "reported_present", Subject: "self",
		},
	}}}}
	c := Case{Facts: []ExpectedFact{{Label: "family history", Concept: "chest pain", Source: "father", Status: "reported_present", Subject: "family"}}}
	r := Result{}
	scoreFacts(&r, c, &e)
	if len(r.FabricatedFindings) != 1 || r.KeyHistory.Matched != 0 {
		t.Fatal("wrong subject with real quote was missed")
	}
	e.Assessments = []clinical.Assessment{{Version: 1, HistoryRevision: 1}}
	e.HistoryRevision = 1
	e.State = "assessment_ready"
	before, err := json.Marshal(e.Assessments)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkRevision(&r, before, e.Report()); err != nil {
		t.Fatal(err)
	}
	checkWarnings(&r, map[string]bool{"w1": true}, e.Report(), "after assessment")
	if len(r.StaleAssessments) == 0 || len(r.LostWarnings) == 0 || !r.Failed() {
		t.Fatal("individual regressions were hidden")
	}
}
