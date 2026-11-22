package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/provider"
	"github.com/ctlplne/healthctl/internal/store"
)

var requestID = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,96}$`)

type mutation struct {
	clinical.Command
	Revision  int    `json:"revision"`
	RequestID string `json:"requestId"`
}

func (s *Server) list(w http.ResponseWriter, r *http.Request, owner int) error {
	rows, err := s.Store.List(r.Context(), owner)
	if err != nil {
		return err
	}
	emit(w, 200, object{"encounters": rows})
	return nil
}
func (s *Server) get(w http.ResponseWriter, r *http.Request, owner int) error {
	e, err := s.Store.Get(r.Context(), owner, r.PathValue("id"))
	if err != nil {
		return err
	}
	s.refreshEvidence(r.Context(), owner, e)
	emit(w, 200, e)
	return nil
}
func (s *Server) create(w http.ResponseWriter, r *http.Request, owner int) error {
	var in struct {
		clinical.Intake
		ReturnVisit     *clinical.ReturnVisit `json:"returnVisit,omitempty"`
		PregnancyAnswer *bool                 `json:"pregnancyRelated"`
		EmergencyAnswer *bool                 `json:"urgentNow"`
		RequestID       string                `json:"requestId"`
		Disclosed       clinical.ProviderRef  `json:"disclosedProvider"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if in.PregnancyAnswer == nil || in.EmergencyAnswer == nil {
		return bad("Answer the pregnancy-scope and current-emergency questions before submitting intake.")
	}
	in.Intake.PregnancyRelated = *in.PregnancyAnswer
	in.Intake.UrgentNow = *in.EmergencyAnswer

	if !requestID.MatchString(in.RequestID) {
		return bad("A submission identifier is required.")
	}
	if err := s.Models.Validate(provider.Config(in.Disclosed)); err != nil {
		return bad(err.Error())
	}
	e, err := clinical.New(token(16), in.Intake, in.Disclosed, s.Emergency)
	if err != nil {
		return bad(err.Error())
	}
	digest, err := s.Store.Digest(in)
	if err != nil {
		return err
	}
	e, fresh, err := s.Store.CreateVisit(r.Context(), owner, e, in.RequestID, digest, in.Disclosed, in.ReturnVisit, s.Emergency)
	if err != nil {
		return err
	}
	return s.deliver(w, r, owner, e, fresh)
}
func (s *Server) action(w http.ResponseWriter, r *http.Request, owner int) error {
	var in mutation
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if !requestID.MatchString(in.RequestID) || in.Revision <= 0 {
		return bad("A revision and submission identifier are required.")
	}
	if in.Action == "change_provider" {
		if err := s.validateModelChoice(r, owner, provider.Config(in.Provider)); err != nil {
			return bad(err.Error())
		}
	}
	digest, err := s.Store.Digest(in)
	if err != nil {
		return err
	}
	related := ""
	if in.Action == "reuse_fact" || in.Action == "set_episode" {
		related = in.SourceEncounterID
	}
	if related != "" && related == r.PathValue("id") {
		return bad("Select a different source encounter.")
	}
	if in.Action == "reuse_fact" && related == "" {
		return bad("Select an earlier encounter.")
	}
	e, fresh, err := s.Store.ApplyRelated(r.Context(), owner, r.PathValue("id"), related, in.Revision, in.RequestID, digest, func(e, prior *clinical.Encounter) error {
		var actionErr error
		switch {
		case in.Action == "set_episode" && prior != nil:
			actionErr = e.JoinEpisode(prior)
		case in.Action == "reuse_fact":
			actionErr = e.ReconcileFact(prior, in.Command, s.Emergency)
		default:
			actionErr = e.Apply(in.Command, s.Emergency)
		}
		if actionErr != nil {
			return bad(actionErr.Error())
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.deliver(w, r, owner, e, fresh)
}
func (s *Server) deliver(w http.ResponseWriter, r *http.Request, owner int, e *clinical.Encounter, fresh bool) error {
	task := e.PendingTask()
	if task != "" && !e.Demo && r.Header.Get("Prefer") == "respond-async" {
		// Create/Apply has already committed this revision. Encode the snapshot
		// before the worker takes ownership of it. Replayed receipts only read it.
		w.Header().Set("Preference-Applied", "respond-async")
		w.Header().Set("Location", "/api/encounters/"+e.ID)
		emit(w, http.StatusAccepted, e)
		if fresh {
			localPlan := s.localPlanOrigin(r) != ""
			ctx := s.Lifetime
			if ctx == nil {
				ctx = context.Background()
			}
			go func() {
				if err := s.finishModel(ctx, owner, e, task, localPlan); err != nil && !errors.Is(err, store.ErrConflict) && s.Logger != nil {
					s.Logger.Print("event=encounter_completion_unavailable")
				}
			}()
		}
		return nil
	}
	if fresh && task != "" {
		if err := s.finishModel(r.Context(), owner, e, task, s.localPlanOrigin(r) != ""); err != nil {
			return err
		}
	}
	s.refreshEvidence(r.Context(), owner, e)
	emit(w, http.StatusOK, e)
	return nil
}

func (s *Server) finishModel(ctx context.Context, owner int, e *clinical.Encounter, task string, localPlan bool) error {
	rev := e.Revision
	in, err := e.ModelInput(task)
	var p clinical.Proposal
	var meta clinical.ModelMeta
	if err == nil && e.Provider.Kind == "chatgpt" && !localPlan {
		err = bad("Open this encounter at the local 127.0.0.1 address to use its ChatGPT connection.")
	}
	if err == nil {
		if e.Demo {
			p = clinical.Demo(in)
			meta = clinical.ModelMeta{Provider: "demo", RequestedModel: e.Provider.Model, ReturnedModel: "synthetic-case-v3", AppVersion: "0.2.0", SchemaVersion: "proposal-v1", PromptVersion: "demo-v3", Settings: map[string]any{"synthetic": true}}
		} else {
			cfg := provider.Config(e.Provider)
			var m provider.Metadata
			var raw []byte
			raw, m, err = s.Models.Generate(ctx, cfg, task, in, func(b []byte) error {
				var candidate clinical.Proposal
				if err := json.Unmarshal(b, &candidate); err != nil {
					return err
				}
				return clinical.Validate(in, candidate)
			})
			s.recordModelUsage(ctx, owner, cfg, m, err)
			if err == nil {
				err = json.Unmarshal(raw, &p)
			}
			meta = clinical.ModelMeta{Provider: m.Provider, Endpoint: m.Endpoint, RequestedModel: m.RequestedModel, ReturnedModel: m.ReturnedModel, PromptVersion: m.PromptVersion, SchemaVersion: m.SchemaVersion, AppVersion: m.AppVersion, Settings: m.Settings}
		}
	}
	if err == nil {
		err = e.Accept(task, p, meta, s.Emergency)
		if err == nil && task == "assessment" {
			s.attachAssessmentEvidence(ctx, owner, e)
		}
	}
	if err != nil {
		e.Fail(task)
		var planErr *provider.ChatGPTError
		var localErr problem
		if errors.Is(err, context.DeadlineExceeded) {
			e.Failure += " The provider reached the request time limit."
		} else if errors.As(err, &planErr) {
			e.Failure += " " + planErr.Error()
		} else if e.Provider.Kind == "chatgpt" && errors.As(err, &localErr) {
			e.Failure += " " + localErr.message
		}
		// Only operator setup errors have actionable, nonclinical details.
		if strings.HasPrefix(err.Error(), "configure ") || strings.HasPrefix(err.Error(), "set BYOM_KEY_ENDPOINT") {
			e.Failure += " Provider setup: " + err.Error() + "."
		}
	}
	// A canceled HTTP request still needs a durable failure state. A late result
	// has no write authority once another submission has advanced the revision.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return s.Store.Finish(writeCtx, owner, e, rev)
}
func (s *Server) export(w http.ResponseWriter, r *http.Request, owner int) error {
	e, err := s.Store.Get(r.Context(), owner, r.PathValue("id"))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Disposition", `attachment; filename="healthctl.json"`)
	s.refreshEvidence(r.Context(), owner, e)
	emit(w, 200, e.Report())
	return nil
}
func (s *Server) remove(w http.ResponseWriter, r *http.Request, owner int) error {
	var in struct {
		Revision int `json:"revision"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if err := s.Store.Delete(r.Context(), owner, r.PathValue("id"), in.Revision); err != nil {
		return err
	}
	emit(w, 200, object{"deleted": true})
	return nil
}
