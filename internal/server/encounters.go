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
)

var requestID = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,96}$`)

type mutation struct {
	clinical.Command
	AssessmentAcknowledged  bool   `json:"assessmentAcknowledged,omitempty"`
	AssessmentNoticeVersion string `json:"assessmentNoticeVersion,omitempty"`
	Revision                int    `json:"revision"`
	RequestID               string `json:"requestId"`
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
	emit(w, 200, e.ForDisplay())
	return nil
}

// usage reports the model calls, tokens, waiting time and estimated cost of
// one encounter. The cost is shown only when every call reported a price.
func (s *Server) usage(w http.ResponseWriter, r *http.Request, owner int) error {
	id := r.PathValue("id")
	if _, err := s.Store.Get(r.Context(), owner, id); err != nil {
		return err
	}
	var calls, failed, missingCost int
	var input, output, duration int64
	var cost float64
	err := s.Store.DB.QueryRowContext(r.Context(), `SELECT count(*), count(*) FILTER (WHERE status='failed'), coalesce(sum(input_tokens),0), coalesce(sum(output_tokens),0),
		coalesce(sum(duration_ms),0), coalesce(sum(estimated_usd),0), count(*) FILTER (WHERE estimated_usd IS NULL)
		FROM model_operations WHERE owner_id=$1 AND encounter_id=$2`, owner, id).Scan(&calls, &failed, &input, &output, &duration, &cost, &missingCost)
	if err != nil {
		return err
	}
	out := object{"calls": calls, "failedCalls": failed, "inputTokens": input, "outputTokens": output, "waitSeconds": float64(duration) / 1000, "costComplete": calls > 0 && missingCost == 0}
	if calls > 0 && missingCost == 0 {
		out["estimatedUSD"] = cost
	}
	emit(w, 200, out)
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
		requestsAssessment := in.Action == "assess" || (in.Action == "retry" && e.FailedTask == "assessment")
		if requestsAssessment && (!in.AssessmentAcknowledged || in.AssessmentNoticeVersion != clinical.AssessmentNotice.Version) {
			return bad("Read and acknowledge the current beta assessment notice before requesting an estimate. Reload if your notice is out of date.")
		}
		if err := clinical.CheckCommand(in.Command); err != nil {
			return bad(err.Error())
		}
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
		if requestsAssessment {
			e.AssessmentAcknowledgments = append(e.AssessmentAcknowledgments, clinical.AssessmentAcknowledgment{
				NoticeVersion: clinical.AssessmentNotice.Version, At: time.Now().UTC(), HistoryRevision: e.HistoryRevision, Provider: e.Provider,
			})
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
	if fresh {
		// A new revision supersedes any model call still working on an older one.
		s.supersede(r.Context(), e.ID, e.Revision)
	}
	if task != "" && !e.Demo && r.Header.Get("Prefer") == "respond-async" {
		// Create/Apply has already committed this revision. The model call is a
		// durable job; replayed receipts only read the snapshot.
		if fresh {
			if err := s.enqueue(r.Context(), owner, e, task, s.localPlanOrigin(r) != ""); err != nil {
				return err
			}
		}
		w.Header().Set("Preference-Applied", "respond-async")
		w.Header().Set("Location", "/api/encounters/"+e.ID)
		emit(w, http.StatusAccepted, e.ForDisplay())
		return nil
	}
	if fresh && task != "" {
		if err := s.finishModel(r.Context(), owner, e, task, s.localPlanOrigin(r) != ""); err != nil {
			return err
		}
	}
	s.refreshEvidence(r.Context(), owner, e)
	emit(w, http.StatusOK, e.ForDisplay())
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
			meta = clinical.ModelMeta{ValidatorVersion: clinical.ValidatorVersion, InputHash: clinical.InputHash(in), Provider: "demo", RequestedModel: e.Provider.Model, ReturnedModel: "synthetic-case-v3", AppVersion: "0.2.0", SchemaVersion: "proposal-v1", PromptVersion: "demo-v3", Settings: map[string]any{"synthetic": true}}
			if e.DemoPractice {
				p = clinical.PracticeHistory(in)
				meta.ReturnedModel, meta.PromptVersion = "offline-history-practice", "practice-v1"
			} else {
				p = clinical.Demo(in)
			}
		} else {
			cfg := provider.Config(e.Provider)
			var m provider.Metadata
			var rejected []clinical.Rejection
			// Each item is checked on its own. Invalid items are dropped with a
			// reason code and the valid remainder is used. Only a problem with
			// the whole proposal fails the call and asks the model to repair it.
			_, m, err = s.Models.Generate(ctx, cfg, task, in, func(b []byte) error {
				var candidate clinical.Proposal
				if err := json.Unmarshal(b, &candidate); err != nil {
					return err
				}
				clean, dropped, err := clinical.Sanitize(in, candidate)
				if err != nil {
					return err
				}
				p, rejected = clean, dropped
				return nil
			})
			s.recordEncounterUsage(ctx, owner, e.ID, cfg, m, err, rejected)
			if m.Settings == nil {
				m.Settings = map[string]any{}
			}
			// Optional stability check: run the assessment a second time and mark
			// possibilities that appear in only one run. A failed second run
			// keeps the first result and says so.
			if err == nil && task == "assessment" && e.StabilityRequested {
				var second clinical.Proposal
				var dropped []clinical.Rejection
				_, m2, err2 := s.Models.Generate(ctx, cfg, task, in, func(b []byte) error {
					var candidate clinical.Proposal
					if err := json.Unmarshal(b, &candidate); err != nil {
						return err
					}
					clean, d, err := clinical.Sanitize(in, candidate)
					second, dropped = clean, d
					return err
				})
				s.recordEncounterUsage(ctx, owner, e.ID, cfg, m2, err2, dropped)
				if err2 == nil {
					p.Items = clinical.MarkUnstable(p.Items, second.Items)
					m.Settings["stabilityCheck"] = true
				} else {
					m.Settings["stabilityCheck"] = "second run failed"
				}
			}
			m.Settings["validatorVersion"] = clinical.ValidatorVersion
			if len(rejected) > 0 {
				m.Settings["rejectedItems"] = clinical.RejectionCounts(rejected)
			}
			meta = clinical.ModelMeta{ValidatorVersion: clinical.ValidatorVersion, InputHash: clinical.InputHash(in), Provider: m.Provider, Endpoint: m.Endpoint, RequestedModel: m.RequestedModel, ReturnedModel: m.ReturnedModel, PromptVersion: m.PromptVersion, SchemaVersion: m.SchemaVersion, AppVersion: m.AppVersion, Settings: m.Settings}
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
