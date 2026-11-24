package server

import (
	"encoding/json"
	"net/http"

	"github.com/ctlplne/healthctl/internal/clinical"

	"github.com/ctlplne/healthctl/internal/provider"
)

type settingsRequest struct {
	Provider provider.Config `json:"provider"`
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request, owner int) error {
	var raw []byte
	if err := s.Store.DB.QueryRowContext(r.Context(), "SELECT provider FROM owners WHERE id=$1", owner).Scan(&raw); err != nil {
		return err
	}
	var cfg provider.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	allowed, creds := s.Models.Status()
	emit(w, 200, object{"provider": cfg, "allowlist": allowed, "credentialStatus": creds})
	return nil
}
func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request, owner int) error {
	var in settingsRequest
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if err := s.validateModelChoice(r, owner, in.Provider); err != nil {
		return bad(err.Error())
	}
	b, err := json.Marshal(in.Provider)
	if err != nil {
		return err
	}
	tx, err := s.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var previous []byte
	if err = tx.QueryRowContext(r.Context(), "SELECT provider FROM owners WHERE id=$1 FOR UPDATE", owner).Scan(&previous); err != nil {
		return err
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE owners SET provider=$2 WHERE id=$1", owner, b); err != nil {
		return err
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO audit_events(owner_id,event,settings_change) VALUES($1,'provider_selected',jsonb_build_object('previous',$2::jsonb,'current',$3::jsonb))`, owner, previous, b); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.settings(w, r, owner)
}
func (s *Server) testSettings(w http.ResponseWriter, r *http.Request, owner int) error {
	var in settingsRequest
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if err := s.validateModelChoice(r, owner, in.Provider); err != nil {
		return bad(err.Error())
	}
	probe := object{}
	if in.Provider.Kind != "demo" {
		if err := s.Models.Test(r.Context(), in.Provider); err != nil {
			return problem{422, err.Error()}
		}
		var failed bool
		probe, failed = s.clinicalProbe(r, in.Provider)
		if failed {
			return problem{422, "The model connected, but its answer to a short synthetic interview didn't pass the app's checks (" + probe["reason"].(string) + "). Choose another model or structured-output setting."}
		}
	}
	emit(w, 200, object{"ok": true, "synthetic": in.Provider.Kind == "demo", "clinicalProbe": probe})
	return nil
}

// clinicalProbe sends a tiny fictional interview through the real validator, so
// a model that can't meet the contract fails at setup rather than mid-interview.
// A transport or provider error leaves the probe inconclusive.
func (s *Server) clinicalProbe(r *http.Request, cfg provider.Config) (object, bool) {
	text := "Synthetic connection check: a mild cough for two days, no fever."
	in := clinical.Input{Task: "interview", Age: 40, Concern: text, Answers: []clinical.Answer{{ID: "a1", Kind: "concern", Question: "What brings you here?", Text: text, Active: true}}, Facts: []clinical.Fact{}, Contradictions: []clinical.Contradiction{}, Outstanding: []string{}, ActiveWarnings: []clinical.Warning{}, HistoryRevision: 1}
	var dropped []clinical.Rejection
	_, m, err := s.Models.Generate(r.Context(), cfg, "interview", in, func(b []byte) error {
		var candidate clinical.Proposal
		if err := json.Unmarshal(b, &candidate); err != nil {
			return err
		}
		_, rejected, err := clinical.Sanitize(in, candidate)
		dropped = rejected
		return err
	})
	if err == nil {
		return object{"passed": true, "droppedItems": clinical.RejectionCounts(dropped)}, false
	}
	switch reason := m.FailureReason; {
	case clinical.ReasonOf(err) != "" || reason == "schema_mismatch" || reason == "unreadable_response":
		if reason == "" {
			reason = clinical.ReasonOf(err)
		}
		return object{"passed": false, "reason": reason}, true
	default:
		return object{"passed": false, "inconclusive": true, "reason": reason}, false
	}
}

func (s *Server) validateModelChoice(r *http.Request, owner int, cfg provider.Config) error {
	if err := s.Models.Validate(cfg); err != nil {
		return err
	}
	if cfg.Kind != "chatgpt" {
		return nil
	}
	if s.localPlanOrigin(r) == "" {
		return bad("Use the local 127.0.0.1 installation address for your ChatGPT plan.")
	}
	accounts, err := s.Store.PlanAccounts(r.Context(), owner)
	if err != nil {
		return bad("Could not load the ChatGPT accounts.")
	}
	a := findAccount(accounts, cfg.Endpoint)
	if a == nil || !accountView(*a).PlanEnabled {
		return bad("Connect the selected ChatGPT account and enable plan usage first.")
	}
	return nil
}
