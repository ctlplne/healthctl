package server

import (
	"errors"
	"io"
	"testing"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/provider"
)

func TestRejectionReasonsRecordedPostgres(t *testing.T) {
	db, _, _ := testStore(t)
	s := newTestServer(db, io.Discard)
	c := caller{h: s.Handler()}
	c.signup(t)
	cfg := provider.Config{Kind: "openai", Model: "fixture"}
	s.recordModelUsage(t.Context(), 1, cfg, provider.Metadata{Usage: provider.Usage{Requests: 1, Reported: 1}}, nil, []clinical.Rejection{{Field: "findings", Index: 0, Reason: clinical.ReasonNegated}, {Field: "findings", Index: 2, Reason: clinical.ReasonNegated}, {Field: "choices", Index: 1, Reason: clinical.ReasonAdvice}})
	s.recordModelUsage(t.Context(), 1, cfg, provider.Metadata{FailureReason: clinical.ReasonMultipleQuestions, Usage: provider.Usage{Requests: 2, Reported: 2}}, errors.New("failed"), nil)
	var metrics struct {
		Validation struct {
			ValidatorVersion string
			DroppedItems     map[string]int
			FailedCalls      map[string]int
		}
	}
	check(t, c.call("GET", "/api/qualification", nil), 200, &metrics)
	v := metrics.Validation
	if v.ValidatorVersion != clinical.ValidatorVersion || v.DroppedItems[clinical.ReasonNegated] != 2 || v.DroppedItems[clinical.ReasonAdvice] != 1 || v.FailedCalls[clinical.ReasonMultipleQuestions] != 1 {
		t.Fatalf("validation outcomes were not recorded: %+v", v)
	}
}

func TestProvenanceValidatorVersionPostgres(t *testing.T) {
	db, _, _ := testStore(t)
	s := newTestServer(db, io.Discard)
	c := caller{h: s.Handler()}
	c.signup(t)
	demo := clinical.ProviderRef{Kind: "demo", Model: "synthetic", Structured: "auto"}
	selectProvider(t, c, demo)
	var e clinical.Encounter
	check(t, c.call("POST", "/api/encounters", intake(demo, clinical.DemoConcern)), 200, &e)
	if len(e.Turns) == 0 {
		t.Fatal("the demo interview made no turn")
	}
	meta := e.Turns[0].Meta
	if meta.ValidatorVersion != clinical.ValidatorVersion || len(meta.InputHash) != 24 {
		t.Fatalf("provenance lacks the validator version or input hash: %+v", meta)
	}
	var report clinical.Report
	check(t, c.call("GET", "/api/encounters/"+e.ID+"/export", nil), 200, &report)
	if report.PolicyReview.Version != clinical.PolicyVersion || report.PolicyReview.Status != "development" {
		t.Fatalf("the export must state the policy review status: %+v", report.PolicyReview)
	}
}

func TestClinicalConnectionProbe(t *testing.T) {
	s := &Server{}
	_ = s
	in := clinical.Input{Task: "interview", Answers: []clinical.Answer{{ID: "a1", Kind: "concern", Text: "Synthetic connection check: a mild cough for two days, no fever.", Active: true}}}
	good := clinical.Proposal{Action: "ask", Question: "When did the cough start?", Reason: "Timing helps.", Findings: []clinical.FindingProposal{{Evidence: clinical.Evidence{AnswerID: "a1", Quote: "a mild cough for two days"}, Concept: "cough", Status: "reported_present", Subject: "self"}}}
	if _, _, err := clinical.Sanitize(in, good); err != nil {
		t.Fatalf("a correct probe answer must pass: %v", err)
	}
	bad := clinical.Proposal{Action: "ask", Question: "When? Why?", Reason: "x"}
	if _, _, err := clinical.Sanitize(in, bad); clinical.ReasonOf(err) == "" {
		t.Fatal("an invalid probe answer must fail with a reason code")
	}
}
