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
