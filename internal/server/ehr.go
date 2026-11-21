package server

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ctlplne/healthctl/internal/ehr"
	"github.com/ctlplne/healthctl/internal/store"
)

// A portal grant is for a single import. Its verifier and preview live only in
// the process that started the flow. No portal access token enters this cache.
type ehrVisit struct {
	session   [32]byte
	encounter string
	revision  int
	verifier  string
	preview   *ehr.Preview
	imported  bool
	expires   time.Time
}

type EHRIntegration struct {
	config ehr.Config
	client *ehr.Client
	mu     sync.Mutex
	// Serialize saving and cancelling a preview so a successful cancellation
	// cannot race a database commit. Each installation has a single owner.
	importMu sync.Mutex
	visits   map[string]ehrVisit
}

func NewEHRIntegration(cfg ehr.Config) (*EHRIntegration, error) {
	if cfg == (ehr.Config{}) {
		return nil, nil
	}
	callback, err := url.Parse(cfg.RedirectURI)
	if err != nil || callback.EscapedPath() != "/ehr/callback" || callback.RawQuery != "" || callback.Fragment != "" {
		return nil, errors.New("register the exact application URL ending in /ehr/callback")
	}
	client, err := ehr.New(cfg)
	if err != nil {
		return nil, err
	}
	return &EHRIntegration{config: cfg, client: client, visits: make(map[string]ehrVisit)}, nil
}

func (x *EHRIntegration) keep(v ehrVisit) (string, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if len(x.visits) >= 32 {
		return "", problem{429, "Several portal imports are pending. Finish one or try again in ten minutes."}
	}
	key := token(32)
	v.expires = time.Now().Add(10 * time.Minute)
	x.visits[key] = v
	time.AfterFunc(time.Until(v.expires), func() {
		x.mu.Lock()
		delete(x.visits, key)
		x.mu.Unlock()
	})
	return key, nil
}

func (x *EHRIntegration) visit(key string, session [32]byte, preview, consume bool) (ehrVisit, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	v, ok := x.visits[key]
	if !ok || v.session != session || (v.preview != nil || v.imported) != preview || !time.Now().Before(v.expires) {
		return ehrVisit{}, bad("This portal import has expired or belongs to a different sign-in. Start again from the encounter.")
	}
	if consume {
		delete(x.visits, key)
	}
	return v, nil
}

func browserSession(r *http.Request) [32]byte {
	c, _ := r.Cookie(sessionCookie) // Protected routes have already authenticated this cookie.
	return sha256.Sum256([]byte(c.Value))
}

func (s *Server) ehrRoutes(m *http.ServeMux) {
	s.route(m, "GET /api/ehr", true, func(w http.ResponseWriter, _ *http.Request, _ int) error {
		if s.EHR == nil {
			emit(w, 200, object{"enabled": false})
		} else {
			cfg := s.EHR.config
			emit(w, 200, object{"enabled": true, "name": cfg.Name, "fhirBase": cfg.FHIRBase, "authorizationEndpoint": cfg.AuthorizationEndpoint})
		}
		return nil
	})
	s.route(m, "POST /api/encounters/{id}/ehr/start", true, s.ehrStart)
	s.route(m, "POST /api/ehr/preview", true, s.ehrPreview)
	s.route(m, "POST /api/ehr/import", true, s.ehrImport)
	s.route(m, "POST /api/ehr/cancel", true, s.ehrCancel)
	m.HandleFunc("GET /ehr/callback", func(w http.ResponseWriter, r *http.Request) {
		// An external return may omit SameSite=Strict cookies. Serve only the
		// public shell here. The authenticated same-origin POST checks state.
		http.ServeFile(w, r, filepath.Join(s.Assets, "index.html"))
	})
}

func (s *Server) ehrStart(w http.ResponseWriter, r *http.Request, owner int) error {
	if s.EHR == nil {
		return bad("The installation has no configured patient portal.")
	}
	var empty struct{}
	if err := decode(w, r, &empty); err != nil {
		return err
	}
	redirect, _ := url.Parse(s.EHR.config.RedirectURI)
	if !strings.EqualFold(r.Host, redirect.Host) || r.Header.Get("Origin") != "" && !strings.EqualFold(r.Header.Get("Origin"), redirect.Scheme+"://"+redirect.Host) {
		return bad("Open the installation at its registered portal callback origin before connecting.")
	}
	e, err := s.Store.Get(r.Context(), owner, r.PathValue("id"))
	if err != nil {
		return err
	}
	v := ehrVisit{session: browserSession(r), encounter: e.ID, revision: e.Revision, verifier: token(32)}
	state, err := s.EHR.keep(v)
	if err != nil {
		return err
	}
	dest, err := s.EHR.client.Begin(r.Context(), state, v.verifier)
	if err != nil {
		s.EHR.visit(state, v.session, false, true)
		return problem{502, "Could not start portal authorization. Check the installation's SMART endpoints, registration and supported capabilities."}
	}
	emit(w, 200, object{"authorizationUrl": dest})
	return nil
}

func (s *Server) ehrPreview(w http.ResponseWriter, r *http.Request, owner int) error {
	if s.EHR == nil {
		return bad("The installation has no configured patient portal.")
	}
	var in struct {
		Code, State, Issuer string
		Denied              bool
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	v, err := s.EHR.visit(in.State, browserSession(r), false, true)
	if err != nil {
		return err
	}
	if in.Issuer != s.EHR.config.AuthorizationIssuer {
		return bad("The portal identity did not match the registered authorization server. Start again.")
	}
	if in.Denied || in.Code == "" || len(in.Code) > 8192 {
		return bad("Portal authorization ended without a usable grant. You can continue with your own report.")
	}
	e, err := s.Store.Get(r.Context(), owner, v.encounter)
	if err != nil {
		return err
	}
	if e.Revision != v.revision {
		return store.ErrConflict
	}
	raw, patient, err := s.EHR.client.Results(r.Context(), in.Code, v.verifier)
	if err != nil {
		return problem{502, "The portal could not return authorized laboratory results. Reconnect to retry or upload a downloaded report."}
	}
	p, err := ehr.ParseObservations(raw, patient, s.EHR.config.FHIRBase)
	if err != nil {
		return bad("The portal returned an unsupported laboratory response. You can upload a downloaded report.")
	}
	v.verifier = ""
	v.preview = &p
	ticket, err := s.EHR.keep(v)
	if err != nil {
		return err
	}
	emit(w, 200, object{"ticket": ticket, "encounterId": v.encounter, "name": s.EHR.config.Name, "fhirBase": s.EHR.config.FHIRBase, "rows": p.Rows, "skipped": p.Skipped, "skippedByReason": p.SkippedByReason, "more": p.More})
	return nil
}

func (s *Server) ehrImport(w http.ResponseWriter, r *http.Request, owner int) error {
	if s.EHR == nil {
		return bad("The installation has no configured patient portal.")
	}
	var in struct {
		Ticket           string
		Keys             []string
		ConfirmedPatient bool
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if !in.ConfirmedPatient {
		return bad("Confirm that the selected results belong to you before importing them.")
	}
	s.EHR.importMu.Lock()
	defer s.EHR.importMu.Unlock()
	v, err := s.EHR.visit(in.Ticket, browserSession(r), true, false)
	if err != nil {
		return err
	}
	// A ticket identifies one immutable portal response and encounter revision.
	// The receipt therefore needs only its selected keys to recognize a retry.
	digest, err := s.Store.Digest([]any{"ehr-import", v.encounter, v.revision, in.Ticket, in.Keys})
	if err != nil {
		return err
	}
	if v.imported {
		previous, err := s.Store.Uploaded(r.Context(), owner, v.encounter, in.Ticket, digest)
		if err != nil {
			return err
		}
		if previous == nil {
			return store.ErrConflict
		}
		emit(w, 200, previous)
		return nil
	}
	doc, original, err := ehr.SelectedDocument(*v.preview, in.Keys, s.EHR.config.Name)
	if err != nil {
		return bad("Select supported results from this preview.")
	}
	doc.ID, doc.BelongsTo = token(16), "self"
	e, err := s.Store.AttachDocument(r.Context(), owner, v.encounter, v.revision, in.Ticket, digest, doc, original, s.Emergency)
	if errors.Is(err, store.ErrDocumentInput) {
		return bad("The selected results could not be attached to this encounter. Return to it and check the report limits.")
	}
	if err != nil {
		return err
	}
	s.EHR.mu.Lock()
	if _, ok := s.EHR.visits[in.Ticket]; ok {
		v.preview, v.imported = nil, true
		s.EHR.visits[in.Ticket] = v
	}
	s.EHR.mu.Unlock()
	emit(w, 200, e)
	return nil
}

func (s *Server) ehrCancel(w http.ResponseWriter, r *http.Request, _ int) error {
	if s.EHR == nil {
		return bad("The installation has no configured patient portal.")
	}
	var in struct{ Ticket string }
	if err := decode(w, r, &in); err != nil {
		return err
	}
	s.EHR.importMu.Lock()
	defer s.EHR.importMu.Unlock()
	v, err := s.EHR.visit(in.Ticket, browserSession(r), true, true)
	if err != nil {
		return err
	}
	if v.imported {
		return problem{409, "These results have already been attached. Open the encounter to review them."}
	}
	emit(w, 200, object{"cancelled": true})
	return nil
}
