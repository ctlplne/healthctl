package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/evidence"
	"github.com/ctlplne/healthctl/internal/evidence/contextual"
	"github.com/ctlplne/healthctl/internal/provider"
)

func (s *Server) evidenceRoutes(m *http.ServeMux) {
	s.route(m, "GET /api/evidence", true, s.evidenceList)
	s.route(m, "POST /api/evidence/search", true, s.evidenceSearch)
	s.route(m, "GET /api/evidence/{collection}/{version}/{passage}", true, s.evidenceGet)
	s.route(m, "POST /api/evidence/packs", true, s.evidenceInstall)
	s.route(m, "POST /api/evidence/collections/{collection}/{version}/status", true, s.evidenceChange)
	s.route(m, "GET /api/qualification", true, s.qualification)
}
func (s *Server) library() evidence.Library { return evidence.Library{DB: s.Store.DB} }
func (s *Server) evidenceList(w http.ResponseWriter, r *http.Request, owner int) error {
	lib := s.library()
	collections, err := lib.Collections(r.Context(), owner)
	if err != nil {
		return err
	}
	cat, err := lib.Catalog(r.Context(), owner)
	if err != nil {
		return err
	}
	emit(w, 200, object{"collections": collections, "corpusVersion": cat.Version})
	return nil
}
func (s *Server) evidenceSearch(w http.ResponseWriter, r *http.Request, owner int) error {
	var in struct {
		Query string `json:"query"`
		Mode  string `json:"mode"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Query) == "" || len(in.Query) > 512 {
		return bad("Enter a topic or question of up to 512 bytes.")
	}
	cat, err := s.library().Catalog(r.Context(), owner)
	if err != nil {
		return err
	}
	if in.Mode == "browse" {
		emit(w, 200, cat.Browse(in.Query))
		return nil
	}
	if in.Mode != "" && in.Mode != "question" {
		return bad("Choose a question or passage search.")
	}
	emit(w, 200, cat.Search(in.Query))
	return nil
}
func (s *Server) evidenceGet(w http.ResponseWriter, r *http.Request, owner int) error {
	p, err := s.library().Get(r.Context(), owner, r.PathValue("collection"), r.PathValue("version"), r.PathValue("passage"))
	if err != nil {
		return err
	}
	emit(w, 200, p)
	return nil
}
func evidenceErr(err error) error {
	switch {
	case errors.Is(err, evidence.ErrInvalid):
		return bad(err.Error())
	case errors.Is(err, evidence.ErrCapacity):
		return problem{409, "The installation reached its collection-version limit."}
	case errors.Is(err, evidence.ErrConflict):
		return problem{409, "The existing version is immutable. Supply a new version identifier."}
	}
	return err
}
func (s *Server) evidenceInstall(w http.ResponseWriter, r *http.Request, owner int) error {
	var in struct {
		Pack            evidence.Pack `json:"pack"`
		RightsConfirmed bool          `json:"rightsConfirmed"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if !in.RightsConfirmed {
		return bad("Confirm local licensing or reuse rights for every passage before installing the collection.")
	}
	if err := s.library().Install(r.Context(), owner, in.Pack); err != nil {
		return evidenceErr(err)
	}
	return s.evidenceList(w, r, owner)
}
func (s *Server) evidenceChange(w http.ResponseWriter, r *http.Request, owner int) error {
	var in struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if err := s.library().Change(r.Context(), owner, r.PathValue("collection"), r.PathValue("version"), in.Status, in.Reason); err != nil {
		return evidenceErr(err)
	}
	return s.evidenceList(w, r, owner)
}
func (s *Server) attachAssessmentEvidence(ctx context.Context, owner int, e *clinical.Encounter) {
	if len(e.Assessments) == 0 {
		return
	}
	a := &e.Assessments[len(e.Assessments)-1]
	a.Literature = []evidence.Support{}
	cat, err := s.library().Catalog(ctx, owner)
	if err == nil {
		a.CorpusVersion = cat.Version
	}
	for i, item := range a.Items {
		result := evidence.Result{Outcome: "unavailable", Reason: "The local reference library was unavailable at the time of assessment.", Passages: []evidence.Passage{}, Conflicts: []string{}}
		support := evidence.Support{ItemIndex: i, Relation: "unavailable", Result: result, ClaimLinks: []evidence.ClaimLink{}}
		if err == nil {
			support = contextual.Explain(cat, e, i, item)
		}
		a.Literature = append(a.Literature, support)
	}
}
func (s *Server) refreshEvidence(ctx context.Context, owner int, e *clinical.Encounter) {
	needed := false
	for _, a := range e.Assessments {
		if len(a.Literature) > 0 {
			needed = true
			break
		}
	}
	if !needed {
		return
	}
	all, err := s.library().Collections(ctx, owner)
	status := map[string]string{}
	if err == nil {
		for _, c := range all {
			status[c.ID+"/"+c.Version] = c.Status
		}
	}
	for ai := range e.Assessments {
		for i := range e.Assessments[ai].Literature {
			for j := range e.Assessments[ai].Literature[i].Passages {
				p := &e.Assessments[ai].Literature[i].Passages[j]
				v := status[p.Collection+"/"+p.Version]
				if v == "" {
					v = "unavailable"
				}
				p.Status = v
			}
		}
	}
}
func (s *Server) recordModelUsage(parent context.Context, owner int, cfg provider.Config, m provider.Metadata, callErr error, rejected []clinical.Rejection) {
	s.recordEncounterUsage(parent, owner, "", cfg, m, callErr, rejected)
}

// recordEncounterUsage records one model call, linked to its encounter.
func (s *Server) recordEncounterUsage(parent context.Context, owner int, encounterID string, cfg provider.Config, m provider.Metadata, callErr error, rejected []clinical.Rejection) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
	defer cancel()
	status := "returned"
	if callErr != nil {
		status = "failed"
	}
	// Log the provider's filtered support identifiers only. The error type
	// discards the remote body, message, credentials and unknown parameters.
	var planErr *provider.ChatGPTError
	if s.Logger != nil && errors.As(callErr, &planErr) {
		s.Logger.Printf("event=chatgpt_request_failed http=%d shape=%s code=%s param=%s request_id=%s", planErr.HTTPStatus, planErr.BodyShape, planErr.Code, planErr.Param, planErr.RequestID)
	}
	u := m.Usage
	counts, _ := json.Marshal(clinical.RejectionCounts(rejected))
	_, err := s.Store.DB.ExecContext(ctx, `INSERT INTO model_operations(owner_id,provider,model,returned_model,destination_hash,status,requests,retries,input_tokens,output_tokens,usage_complete,duration_ms,estimated_usd,validator_version,rejected_items,failure_reason,encounter_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,NULLIF($17,''))`, owner, cfg.Kind, cfg.Model, m.ReturnedModel, evidence.Hash([]byte(cfg.Destination())), status, u.Requests, u.Retries, u.InputTokens, u.OutputTokens, u.Requests > 0 && u.Requests == u.Reported, u.DurationMS, u.EstimatedUSD, clinical.ValidatorVersion, counts, m.FailureReason, encounterID)
	if err != nil && s.Logger != nil {
		s.Logger.Print("event=usage_record_unavailable")
	}
}
func (s *Server) qualification(w http.ResponseWriter, r *http.Request, owner int) error {
	ctx := r.Context()
	db := s.Store.DB
	var raw []byte
	var cfg provider.Config
	if err := db.QueryRowContext(ctx, "SELECT provider FROM owners WHERE id=$1", owner).Scan(&raw); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	configs := []object{}
	for _, kind := range []string{"compatible", "openai", "anthropic", "chatgpt"} {
		configs = append(configs, object{"provider": kind, "model": "", "status": "protocol_fixtures", "detail": "Wire-contract regression fixtures exercised this adapter. Live model quality has not been established."})
	}
	rows, err := db.QueryContext(ctx, `SELECT provider,model,destination_hash,count(*) FILTER (WHERE status='returned'),count(*) FILTER (WHERE status='failed'),max(occurred_at) FROM model_operations WHERE owner_id=$1 GROUP BY provider,model,destination_hash ORDER BY max(occurred_at) DESC`, owner)
	if err != nil {
		return err
	}
	for rows.Next() {
		var kind, model, hash string
		var ok, failed int
		var date time.Time
		if err = rows.Scan(&kind, &model, &hash, &ok, &failed, &date); err != nil {
			rows.Close()
			return err
		}
		status, detail := "observed_failure", "The configured endpoint has returned no valid proposal in the recorded operations."
		if ok > 0 {
			status = "observed_response"
			detail = "This destination returned a validated proposal. The response may come from a local or test server; this is not clinical qualification."
		}
		configs = append(configs, object{"provider": kind, "model": model, "destinationId": hash[:12], "status": status, "detail": detail, "successes": ok, "failures": failed, "lastObserved": date})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	var operations, failed, requests, retries, inputs, outputs, incomplete, unpriced int64
	var cost, p50, p95 sql.NullFloat64
	err = db.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE status='failed'),COALESCE(sum(requests),0),COALESCE(sum(retries),0),COALESCE(sum(input_tokens),0),COALESCE(sum(output_tokens),0),count(*) FILTER(WHERE NOT usage_complete),count(*) FILTER(WHERE estimated_usd IS NULL),sum(estimated_usd),percentile_cont(0.5) WITHIN GROUP (ORDER BY duration_ms),percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) FROM model_operations WHERE owner_id=$1`, owner).Scan(&operations, &failed, &requests, &retries, &inputs, &outputs, &incomplete, &unpriced, &cost, &p50, &p95)
	if err != nil {
		return err
	}
	value := func(n sql.NullFloat64) any {
		if !n.Valid {
			return nil
		}
		return n.Float64
	}
	validation, err := s.validationSummary(ctx, owner)
	if err != nil {
		return err
	}
	rate := provider.ReadRate(cfg)
	emit(w, 200, object{"configurations": configs, "budget": provider.Budgets(), "rates": object{"inputUSDPerMillion": rate.Input, "outputUSDPerMillion": rate.Output}, "usage": object{"operations": operations, "failures": failed, "requests": requests, "retries": retries, "inputTokens": inputs, "outputTokens": outputs, "incompleteUsageOperations": incomplete, "unpricedOperations": unpriced, "pricedSubtotalUSD": value(cost), "p50LatencyMs": value(p50), "p95LatencyMs": value(p95)}, "validation": validation})
	return nil
}

// validationSummary counts dropped items and failed calls by reason code. The
// codes are fixed application strings, so they reveal nothing about a history.
func (s *Server) validationSummary(ctx context.Context, owner int) (object, error) {
	dropped := object{}
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT key, sum(value::int) FROM model_operations, jsonb_each_text(rejected_items) WHERE owner_id=$1 GROUP BY key ORDER BY key`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var n int64
		if err := rows.Scan(&code, &n); err != nil {
			return nil, err
		}
		dropped[code] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	failures := object{}
	rows2, err := s.Store.DB.QueryContext(ctx, `SELECT failure_reason, count(*) FROM model_operations WHERE owner_id=$1 AND status='failed' AND failure_reason<>'' GROUP BY failure_reason ORDER BY failure_reason`, owner)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var code string
		var n int64
		if err := rows2.Scan(&code, &n); err != nil {
			return nil, err
		}
		failures[code] = n
	}
	return object{"validatorVersion": clinical.ValidatorVersion, "droppedItems": dropped, "failedCalls": failures}, rows2.Err()
}
