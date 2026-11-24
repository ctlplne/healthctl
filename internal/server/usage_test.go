package server

import (
	"errors"
	"io"
	"testing"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/provider"
)

func TestEncounterUsagePostgres(t *testing.T) {
	db, _, _ := testStore(t)
	s := newTestServer(db, io.Discard)
	c := caller{h: s.Handler()}
	c.signup(t)
	demo := clinical.ProviderRef{Kind: "demo", Model: "synthetic", Structured: "auto"}
	selectProvider(t, c, demo)
	var e clinical.Encounter
	check(t, c.call("POST", "/api/encounters", intake(demo, clinical.DemoConcern)), 200, &e)
	var usage struct {
		Calls, FailedCalls        int
		InputTokens, OutputTokens int64
		WaitSeconds               float64
		EstimatedUSD              *float64
		CostComplete              bool
	}
	check(t, c.call("GET", "/api/encounters/"+e.ID+"/usage", nil), 200, &usage)
	if usage.Calls != 0 || usage.CostComplete || usage.EstimatedUSD != nil {
		t.Fatalf("a new encounter has no model use: %+v", usage)
	}
	price := 0.012
	cfg := provider.Config{Kind: "openai", Model: "fixture"}
	s.recordEncounterUsage(t.Context(), 1, e.ID, cfg, provider.Metadata{Usage: provider.Usage{Requests: 1, Reported: 1, InputTokens: 1000, OutputTokens: 200, DurationMS: 2500, EstimatedUSD: &price}}, nil, nil)
	s.recordEncounterUsage(t.Context(), 1, e.ID, cfg, provider.Metadata{Usage: provider.Usage{Requests: 1, Reported: 1, InputTokens: 500, OutputTokens: 0, DurationMS: 1500, EstimatedUSD: &price}}, errors.New("failed"), nil)
	s.recordEncounterUsage(t.Context(), 1, "another-encounter", cfg, provider.Metadata{Usage: provider.Usage{Requests: 1, InputTokens: 99999}}, nil, nil)
	check(t, c.call("GET", "/api/encounters/"+e.ID+"/usage", nil), 200, &usage)
	if usage.Calls != 2 || usage.FailedCalls != 1 || usage.InputTokens != 1500 || usage.OutputTokens != 200 || usage.WaitSeconds != 4 || !usage.CostComplete || usage.EstimatedUSD == nil || *usage.EstimatedUSD != 0.024 {
		t.Fatalf("usage must add up this encounter's calls only: %+v", usage)
	}
	s.recordEncounterUsage(t.Context(), 1, e.ID, cfg, provider.Metadata{Usage: provider.Usage{Requests: 1, InputTokens: 10}}, nil, nil)
	usage.EstimatedUSD = nil
	check(t, c.call("GET", "/api/encounters/"+e.ID+"/usage", nil), 200, &usage)
	if usage.CostComplete || usage.EstimatedUSD != nil {
		t.Fatalf("a call without a price makes the cost unknown, not lower: %+v", usage)
	}
	check(t, c.call("GET", "/api/encounters/missing/usage", nil), 404, nil)
}
