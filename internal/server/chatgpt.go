package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/ctlplne/healthctl/internal/planlogin"
	"github.com/ctlplne/healthctl/internal/store"
)

type planProtocol interface {
	Begin(string, string, *planlogin.Session, bool, bool) (planlogin.Attempt, string, error)
	Exchange(context.Context, planlogin.Attempt, url.Values) (planlogin.Session, error)
	Retry(planlogin.Attempt, error) (planlogin.Attempt, string, error)
	Refresh(context.Context, planlogin.Session) (planlogin.Session, error)
	Revoke(context.Context, planlogin.Session) error
}

type planFlow struct {
	attempt                      planlogin.Attempt
	destination, origin, account string
	browser                      [32]byte
	generation                   uint64
	requestPlan                  bool
	retried                      bool
}

type ChatGPTIntegration struct {
	db      *store.Store
	auth    planProtocol
	mu      sync.Mutex
	pending map[string]planFlow
}

func NewChatGPTIntegration(db *store.Store) *ChatGPTIntegration {
	return &ChatGPTIntegration{db: db, auth: planlogin.New(), pending: make(map[string]planFlow)}
}

func (s *Server) localPlanOrigin(r *http.Request) string {
	if s.ChatGPT == nil || s.PublicOrigin != "" || s.Secure || r.TLS != nil {
		return ""
	}
	for _, key := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
		if r.Header.Get(key) != "" {
			return ""
		}
	}
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil || host != "127.0.0.1" {
		return ""
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return ""
	}
	origin := "http://" + r.Host
	if sent := r.Header.Get("Origin"); sent != "" && sent != origin {
		return ""
	}
	return origin
}

func (s *Server) chatGPTRoutes(m *http.ServeMux) {
	s.route(m, "GET /api/chatgpt", true, s.planStatus)
	local := func(pattern string, handler apiHandler) {
		s.route(m, pattern, true, func(w http.ResponseWriter, r *http.Request, owner int) error {
			if s.localPlanOrigin(r) == "" {
				return problem{403, "Open your personal healthctl installation at its 127.0.0.1 address to use a ChatGPT plan."}
			}
			return handler(w, r, owner)
		})
	}
	local("POST /api/chatgpt/start", s.planStart)
	local("GET /api/chatgpt/launch/{state}", s.planLaunch)
	local("POST /api/chatgpt/finish", s.planFinish)
	local("DELETE /api/chatgpt/accounts/{id}", s.planDisconnect)
	local("GET /api/chatgpt/accounts/{id}/models", s.planModels)
	m.HandleFunc("GET /auth/callback", func(w http.ResponseWriter, r *http.Request) {
		if s.localPlanOrigin(r) == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(s.Assets, "index.html"))
	})
}

type planAccountView struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Connected   bool   `json:"connected"`
	PlanEnabled bool   `json:"planEnabled"`
}

func accountView(a store.PlanAccount) planAccountView {
	label := a.Session.Email
	if label == "" {
		label = "ChatGPT account"
	}
	// Distinguish workspace registrations even when the email is identical.
	label += " · " + a.ID[:8]
	return planAccountView{a.ID, label, a.Session.AccessToken != "", a.LocalOptIn && a.Session.HasPlan()}
}
func (s *Server) planStatus(w http.ResponseWriter, r *http.Request, owner int) error {
	views := []planAccountView{}
	local := s.localPlanOrigin(r) != ""
	if local {
		accounts, err := s.Store.PlanAccounts(r.Context(), owner)
		if err != nil {
			return err
		}
		for _, a := range accounts {
			views = append(views, accountView(a))
		}
	}
	emit(w, 200, object{"availableHere": local, "accounts": views})
	return nil
}
func (x *ChatGPTIntegration) keep(f planFlow) (string, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	for k, old := range x.pending {
		if !time.Now().Before(old.attempt.ExpiresAt) {
			delete(x.pending, k)
		}
	}
	if len(x.pending) >= 32 {
		return "", problem{429, "Several ChatGPT sign-ins are pending. Finish one or wait for them to expire."}
	}
	x.pending[f.attempt.State] = f
	return "/api/chatgpt/launch/" + url.PathEscape(f.attempt.State), nil
}
func findAccount(accounts []store.PlanAccount, id string) *store.PlanAccount {
	for i := range accounts {
		if accounts[i].ID == id {
			return &accounts[i]
		}
	}
	return nil
}
func (s *Server) planStart(w http.ResponseWriter, r *http.Request, owner int) error {
	var in struct {
		AccountID  string `json:"accountId"`
		EnablePlan bool   `json:"enablePlan"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	f := planFlow{browser: browserSession(r), origin: s.localPlanOrigin(r), account: in.AccountID, requestPlan: in.EnablePlan}
	err := s.Store.EditPlanAccounts(r.Context(), owner, func(host string, accounts *[]store.PlanAccount) (bool, error) {
		var saved *planlogin.Session
		reconsent := false
		if f.account != "" {
			a := findAccount(*accounts, f.account)
			if a == nil {
				return false, problem{404, "This ChatGPT account is unavailable."}
			}
			saved = &a.Session
			reconsent = in.EnablePlan && (!a.LocalOptIn || (a.Session.AccessToken != "" && !a.Session.HasPlan()))
			f.generation = a.Generation
		}
		var err error
		f.attempt, f.destination, err = s.ChatGPT.auth.Begin(host, f.origin+"/auth/callback", saved, in.EnablePlan, reconsent)
		return false, err
	})
	if err != nil {
		return err
	}
	dest, err := s.ChatGPT.keep(f)
	if err != nil {
		return err
	}
	emit(w, 200, object{"authorizationUrl": dest})
	return nil
}
func (s *Server) takeFlow(r *http.Request, state string, launch bool) (planFlow, error) {
	x := s.ChatGPT
	x.mu.Lock()
	defer x.mu.Unlock()
	f, ok := x.pending[state]
	if !ok || f.origin != s.localPlanOrigin(r) || f.browser != browserSession(r) || !time.Now().Before(f.attempt.ExpiresAt) || (launch && f.destination == "") || (!launch && f.destination != "") {
		return planFlow{}, bad("This ChatGPT sign-in expired or belongs to another browser session. Start again in Model settings.")
	}
	if launch {
		saved := f
		saved.destination = ""
		x.pending[state] = saved
	} else {
		delete(x.pending, state)
	}
	return f, nil
}
func (s *Server) planLaunch(w http.ResponseWriter, r *http.Request, _ int) error {
	f, err := s.takeFlow(r, r.PathValue("state"), true)
	if err != nil {
		return err
	}
	// Only the browser navigation to OpenAI receives a returning ID token hint.
	// The JSON API, healthctl JavaScript and HTTP response body never receive it.
	w.Header().Set("Location", f.destination)
	w.WriteHeader(http.StatusSeeOther)
	return nil
}
func (s *Server) planFinish(w http.ResponseWriter, r *http.Request, owner int) error {
	var in struct {
		State, Code, ClientID string
		Denied                bool
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	f, err := s.takeFlow(r, in.State, false)
	if err != nil {
		return err
	}
	query := url.Values{"state": {in.State}}
	if in.Denied {
		query.Set("error", "access_denied")
	} else {
		query.Set("code", in.Code)
		if in.ClientID != "" {
			query.Set("client_id", in.ClientID)
		}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
	defer cancel()
	var view planAccountView
	var first bool
	var retryURL string
	err = s.Store.EditPlanAccounts(ctx, owner, func(host string, accounts *[]store.PlanAccount) (bool, error) {
		a := findAccount(*accounts, f.account)
		if f.account != "" && (a == nil || a.Generation != f.generation) {
			return false, problem{409, "The selected ChatGPT connection changed. Start a new sign-in."}
		}
		session, err := s.ChatGPT.auth.Exchange(ctx, f.attempt, query)
		if err != nil {
			if !f.retried && errors.Is(err, planlogin.ErrCodeExpired) {
				next, dest, retryErr := s.ChatGPT.auth.Retry(f.attempt, err)
				if retryErr == nil {
					f.attempt, f.destination, f.retried = next, dest, true
					retryURL, retryErr = s.ChatGPT.keep(f)
					return false, retryErr
				}
			}
			return false, authorizationError(err)
		}
		if session.HostID != host || session.Subject == "" || session.Issuer == "" || session.ClientID == "" {
			return false, authorizationError(planlogin.ErrIdentity)
		}
		for i := range *accounts {
			current := &(*accounts)[i]
			if current.Session.ClientID == session.ClientID {
				if current.Session.Subject != session.Subject || current.Session.Issuer != session.Issuer || (a != nil && a.ID != current.ID) {
					return false, authorizationError(planlogin.ErrIdentity)
				}
				if a == nil {
					return false, problem{409, "This registration is already saved. Reconnect from its account entry."}
				}
			}
		}
		if a == nil {
			*accounts = append(*accounts, store.PlanAccount{ID: token(16)})
			a = &(*accounts)[len(*accounts)-1]
		} else if a.Session.ClientID != session.ClientID || a.Session.Subject != session.Subject || a.Session.Issuer != session.Issuer {
			return false, authorizationError(planlogin.ErrIdentity)
		}
		a.LocalOptIn = f.requestPlan
		first = a.LocalOptIn && session.HasPlan() && !a.EverEnabled
		a.EverEnabled = a.EverEnabled || first
		a.Session = session
		a.Generation++
		view = accountView(*a)
		return true, nil
	})
	if err != nil {
		return err
	}
	if retryURL != "" {
		emit(w, 200, object{"retryAuthorizationUrl": retryURL})
		return nil
	}
	emit(w, 200, object{"account": view, "firstPlanUse": first})
	return nil
}
func authorizationError(err error) error {
	switch {
	case errors.Is(err, planlogin.ErrDenied):
		return bad("ChatGPT sign-in was declined. You can continue with your current model.")
	case errors.Is(err, planlogin.ErrTemporary):
		return problem{503, "ChatGPT sign-in is temporarily unavailable. Try again later."}
	case errors.Is(err, planlogin.ErrInvalidClient):
		return bad("ChatGPT no longer recognizes this registration. Add a new account connection and review the model destination.")
	default:
		return bad("ChatGPT sign-in could not be verified. Start again from Model settings.")
	}
}
func (s *Server) planDisconnect(w http.ResponseWriter, r *http.Request, owner int) error {
	id := r.PathValue("id")
	confirmed := false
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
	defer cancel()
	err := s.Store.EditPlanAccounts(ctx, owner, func(_ string, accounts *[]store.PlanAccount) (bool, error) {
		a := findAccount(*accounts, id)
		if a == nil {
			return false, problem{404, "This ChatGPT account is unavailable."}
		}
		// Leave time to commit the local sign-out even if OpenAI is unreachable.
		revokeCtx, stop := context.WithTimeout(ctx, 8*time.Second)
		confirmed = a.Session.RefreshToken != "" && s.ChatGPT.auth.Revoke(revokeCtx, a.Session) == nil
		stop()
		a.Session = a.Session.SignedOut()
		a.Generation++
		return true, nil
	})
	if err != nil {
		return err
	}
	emit(w, 200, object{"revocationConfirmed": confirmed})
	return nil
}

func (s *Server) planModels(w http.ResponseWriter, r *http.Request, _ int) error {
	models, err := s.Models.ChatGPTModels(r.Context(), r.PathValue("id"))
	if err != nil {
		return problem{422, err.Error()}
	}
	emit(w, 200, object{"models": models})
	return nil
}

// Token resolves only the registration explicitly pinned in the encounter.
// The application's data model permits exactly one owner (owners.id = 1).
func (x *ChatGPTIntegration) Token(parent context.Context, id string) (string, error) {
	if err := parent.Err(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 30*time.Second)
	defer cancel()
	var access string
	var unavailable error
	err := x.db.EditPlanAccounts(ctx, 1, func(_ string, accounts *[]store.PlanAccount) (bool, error) {
		a := findAccount(*accounts, id)
		if a == nil || !a.LocalOptIn || !a.Session.HasPlan() {
			return false, errors.New("connect the selected ChatGPT plan")
		}
		if !a.Session.NeedsRefresh(time.Now()) {
			access = a.Session.AccessToken
			return false, nil
		}
		next, err := x.auth.Refresh(ctx, a.Session)
		if err != nil {
			if errors.Is(err, planlogin.ErrRefreshExpired) {
				a.Session = a.Session.SignedOut()
				a.Generation++
				unavailable = err
				return true, nil
			}
			return false, err
		}
		a.Session = next
		if next.HasPlan() {
			access = next.AccessToken
		} else {
			unavailable = errors.New("ChatGPT plan use needs consent")
		}
		return true, nil
	})
	if err != nil {
		return "", err
	}
	return access, unavailable
}
