package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ctlplne/healthctl/internal/planlogin"
	"github.com/ctlplne/healthctl/internal/provider"
	"github.com/ctlplne/healthctl/internal/store"
)

// Begin is the real protocol implementation. Exchange represents a completed
// identity verification by the protocol layer, which has its own signed fixtures.
type planFixture struct {
	*planlogin.Client
	refreshes             atomic.Int32
	refreshErr, revokeErr error
}

func (p *planFixture) Exchange(ctx context.Context, a planlogin.Attempt, q url.Values) (planlogin.Session, error) {
	if q.Has("error") {
		return planlogin.Session{}, planlogin.ErrDenied
	}
	scopes := []string{"openid", "resource.invoke"}
	if q.Get("code") != "identity-only" {
		scopes = append(scopes, "chatgpt.tokens.use.direct")
	}
	id := q.Get("client_id")
	if id == "" {
		id = a.ClientID
	}
	return planlogin.Session{ClientID: id, HostID: a.HostID, Issuer: "https://auth.openai.com", Subject: "fictional-user", Email: "same@example.invalid", CallbackPath: "/auth/callback",
		AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", IDToken: "synthetic-id", ExpiresAt: time.Now().Add(time.Hour), Scopes: scopes}, nil
}
func (p *planFixture) Refresh(_ context.Context, s planlogin.Session) (planlogin.Session, error) {
	p.refreshes.Add(1)
	if p.refreshErr != nil {
		return planlogin.Session{}, p.refreshErr
	}
	s.AccessToken, s.RefreshToken = "synthetic-rotated-access", "synthetic-rotated-refresh"
	s.ExpiresAt = time.Now().Add(time.Hour)
	return s, nil
}
func (p *planFixture) Revoke(context.Context, planlogin.Session) error { return p.revokeErr }

func localhostHandler(h http.Handler, host string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = host; h.ServeHTTP(w, r) })
}

type planFixtureFlow struct{ state, launch string }

func beginPlanFixture(t *testing.T, c *caller, id string) planFixtureFlow {
	t.Helper()
	return beginPlanChoice(t, c, id, true)
}
func beginPlanChoice(t *testing.T, c *caller, id string, optIn bool) planFixtureFlow {
	t.Helper()
	var data struct{ AuthorizationURL string }
	w := c.call("POST", "/api/chatgpt/start", object{"accountId": id, "enablePlan": optIn})
	check(t, w, 200, &data)
	if !strings.HasPrefix(data.AuthorizationURL, "/api/chatgpt/launch/") || strings.Contains(data.AuthorizationURL, "synthetic-") {
		t.Fatal("launch response exposed credential material")
	}
	parts := strings.Split(data.AuthorizationURL, "/")
	return planFixtureFlow{parts[len(parts)-1], data.AuthorizationURL}
}
func launchPlanFixture(t *testing.T, c *caller, f planFixtureFlow) url.Values {
	t.Helper()
	w := c.call("GET", f.launch, nil)
	check(t, w, 303, nil)
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil || location.Host != "auth.openai.com" || location.Query().Get("state") != f.state || w.Body.Len() != 0 {
		t.Fatal("unexpected sign-in destination")
	}
	return location.Query()
}
func finishPlanFixture(t *testing.T, c *caller, f planFixtureFlow, client, code string, want int) (planAccountView, bool) {
	t.Helper()
	var out struct {
		Account      planAccountView
		FirstPlanUse bool
	}
	w := c.call("POST", "/api/chatgpt/finish", object{"clientId": client, "code": code, "state": f.state})
	check(t, w, want, &out)
	if strings.Contains(w.Body.String(), "synthetic-access") || strings.Contains(w.Body.String(), "synthetic-id") {
		t.Fatal("public projection includes secret")
	}
	return out.Account, out.FirstPlanUse
}

func TestChatGPTAccountConsentAndLifecyclePostgres(t *testing.T) {
	db, dsn, _ := isolatedStore(t, true)
	var logs bytes.Buffer
	app := newTestServer(db, &logs)
	p := NewChatGPTIntegration(db)
	fixture := &planFixture{Client: planlogin.New()}
	p.auth = fixture
	app.ChatGPT = p
	app.Models = provider.NewWithChatGPT(p.Token)
	appHandler := app.Handler()
	c := caller{h: localhostHandler(appHandler, "127.0.0.1:8437")}
	check(t, c.call("GET", "/api/chatgpt", nil), 401, nil)
	c.signup(t)
	copyClient := c
	copyClient.csrf = ""
	check(t, copyClient.call("POST", "/api/chatgpt/start", object{}), 403, nil)
	copyClient = c
	copyClient.h = localhostHandler(appHandler, "localhost:8437")
	check(t, copyClient.call("POST", "/api/chatgpt/start", object{}), 403, nil)
	var state struct {
		AvailableHere bool
		Accounts      []planAccountView
	}
	check(t, copyClient.call("GET", "/api/chatgpt", nil), 200, &state)
	if state.AvailableHere {
		t.Fatal("plan enabled outside local origin")
	}
	first := beginPlanChoice(t, &c, "", false)
	// The same owner in another authenticated browser cannot launch or finish it.
	other := caller{h: c.h}
	signin := other.call("POST", "/api/login", object{"username": "synthetic-owner", "password": "A synthetic owner password!"})
	var auth struct{ CSRF string }
	check(t, signin, 200, &auth)
	other.cookie, other.csrf = signin.Result().Cookies()[0], auth.CSRF
	check(t, other.call("GET", first.launch, nil), 400, nil)
	q := launchPlanFixture(t, &c, first)
	if q.Get("agent_name_hint") != "healthctl" || q.Get("redirect_uri") != "http://127.0.0.1:8437/auth/callback" || q.Get("client_id") != "dynamic_agent_client" || !strings.HasPrefix(q.Get("ext_agent_host_id"), "urn:uuid:") {
		t.Fatal("incorrect first registration")
	}
	check(t, c.call("GET", first.launch, nil), 400, nil)
	finishPlanFixture(t, &other, first, "fixture-workspace-a", "ok", 400)
	account, welcome := finishPlanFixture(t, &c, first, "fixture-workspace-a", "unexpected-extra-scope", 200)
	if account.ID == "" || !account.Connected || account.PlanEnabled || welcome {
		t.Fatal("identity-only grant enabled plan inference")
	}
	finishPlanFixture(t, &c, first, "fixture-workspace-a", "ok", 400)
	if _, err := p.Token(context.Background(), account.ID); err == nil {
		t.Fatal("identity-only token released")
	}
	cfg := provider.Config{Kind: "chatgpt", Endpoint: account.ID, Model: "synthetic-model", Structured: "auto"}
	check(t, c.call("PUT", "/api/settings", object{"provider": cfg}), 400, nil)
	reconsent := beginPlanFixture(t, &c, account.ID)
	rq := launchPlanFixture(t, &c, reconsent)
	if rq.Get("client_id") != "fixture-workspace-a" || rq.Get("ext_agent_host_id") != q.Get("ext_agent_host_id") || rq.Get("prompt") != "consent" || rq.Get("state") == q.Get("state") {
		t.Fatal("registration did not preserve identity and request fresh consent")
	}
	active, welcome := finishPlanFixture(t, &c, reconsent, "", "ok", 200)
	if !active.PlanEnabled || active.ID != account.ID || !welcome {
		t.Fatal("failed to enable the selected registration")
	}
	check(t, c.call("PUT", "/api/settings", object{"provider": cfg}), 200, nil)
	second := beginPlanFixture(t, &c, "")
	launchPlanFixture(t, &c, second)
	b, _ := finishPlanFixture(t, &c, second, "fixture-workspace-b", "ok", 200)
	if b.ID == account.ID || b.Label == active.Label {
		t.Fatal("workspace accounts merged")
	}
	var settings struct{ Provider provider.Config }
	check(t, c.call("GET", "/api/settings", nil), 200, &settings)
	if settings.Provider.Endpoint != active.ID {
		t.Fatal("signing in changed the active encounter destination")
	}

	ctx := context.Background()
	if err := db.EditPlanAccounts(ctx, 1, func(_ string, accounts *[]store.PlanAccount) (bool, error) {
		findAccount(*accounts, active.ID).Session.ExpiresAt = time.Now()
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			access, err := p.Token(ctx, active.ID)
			if err != nil || access != "synthetic-rotated-access" {
				t.Error("failed concurrent token rotation")
			}
		})
	}
	wg.Wait()
	if fixture.refreshes.Load() != 1 {
		t.Fatal("rotating refresh token reused")
	}
	var ciphertext []byte
	if err := db.DB.QueryRow("SELECT accounts FROM chatgpt_vault WHERE owner_id=1").Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("synthetic-")) || bytes.Contains(ciphertext, []byte("example.invalid")) {
		t.Fatal("credential material persisted in plaintext")
	}

	failedSignIn := beginPlanFixture(t, &c, active.ID)
	launchPlanFixture(t, &c, failedSignIn)
	check(t, c.call("POST", "/api/chatgpt/finish", object{"state": failedSignIn.state, "denied": true}), 400, nil)
	if _, err := p.Token(ctx, active.ID); err != nil {
		t.Fatal("decline altered an existing plan")
	}
	pending := beginPlanFixture(t, &c, active.ID)
	launchPlanFixture(t, &c, pending)
	fixture.revokeErr = planlogin.ErrTemporary
	var signedOut struct{ RevocationConfirmed bool }
	check(t, c.call("DELETE", "/api/chatgpt/accounts/"+active.ID, nil), 200, &signedOut)
	if signedOut.RevocationConfirmed {
		t.Fatal("failed revocation reported as confirmed")
	}
	finishPlanFixture(t, &c, pending, "", "ok", 409)
	if _, err := p.Token(ctx, active.ID); err == nil {
		t.Fatal("disconnected account still usable")
	}
	if _, err := p.Token(ctx, b.ID); err != nil {
		t.Fatal("sign-out affected another registration")
	}
	reauth := beginPlanFixture(t, &c, active.ID)
	uri := launchPlanFixture(t, &c, reauth)
	if uri.Get("client_id") != "fixture-workspace-a" || uri.Get("id_token_hint") != "" {
		t.Fatal("sign-out did not preserve only the registration")
	}
	_, welcome = finishPlanFixture(t, &c, reauth, "", "ok", 200)
	if welcome {
		t.Fatal("welcome repeated on reconnect")
	}
	// Rotation preserves both credentials and the stable, encounter-pinned account IDs.
	nextKey := bytes.Repeat([]byte{0xfa}, 32)
	if err := db.Rotate(ctx, nextKey); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(ctx, dsn, nextKey)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	newPlan := NewChatGPTIntegration(reopened)
	if _, err = newPlan.Token(ctx, active.ID); err != nil {
		t.Fatal("rotated installation no longer resolved the original account")
	}
	if err = reopened.RecoverOwner(ctx, []byte("A second fictional password")); err != nil {
		t.Fatal(err)
	}
	accounts, err := reopened.PlanAccounts(ctx, 1)
	if err != nil || len(accounts) != 0 {
		t.Fatal("owner recovery retained plan credentials")
	}
	if logs.Len() != 0 && strings.Contains(logs.String(), "synthetic-") {
		t.Fatal("application logged private credentials")
	}
}

func TestChatGPTRefreshFailureAndRemoteDeploymentPostgres(t *testing.T) {
	db, _, _ := testStore(t)
	app := newTestServer(db, &bytes.Buffer{})
	p := NewChatGPTIntegration(db)
	fixture := &planFixture{Client: planlogin.New()}
	p.auth = fixture
	app.ChatGPT = p
	c := caller{h: localhostHandler(app.Handler(), "127.0.0.1:18437")}
	c.signup(t)
	f := beginPlanFixture(t, &c, "")
	launchPlanFixture(t, &c, f)
	a, _ := finishPlanFixture(t, &c, f, "fixture", "ok", 200)
	ctx := context.Background()
	if err := db.EditPlanAccounts(ctx, 1, func(_ string, accounts *[]store.PlanAccount) (bool, error) {
		(*accounts)[0].Session.ExpiresAt = time.Now()
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	fixture.refreshErr = planlogin.ErrTemporary
	if _, err := p.Token(ctx, a.ID); !errors.Is(err, planlogin.ErrTemporary) {
		t.Fatal("unexpected transient refresh result")
	}
	accounts, err := db.PlanAccounts(ctx, 1)
	if err != nil || !accounts[0].Session.HasPlan() {
		t.Fatal("transient failure cleared connection")
	}
	fixture.refreshErr = planlogin.ErrRefreshExpired
	if _, err := p.Token(ctx, a.ID); !errors.Is(err, planlogin.ErrRefreshExpired) {
		t.Fatal("terminal refresh accepted")
	}
	accounts, err = db.PlanAccounts(ctx, 1)
	if err != nil || accounts[0].Session.RefreshToken != "" || accounts[0].Session.AccessToken != "" || accounts[0].Session.ClientID != "fixture" {
		t.Fatal("terminal refresh retained credentials or lost mapping")
	}

	app.PublicOrigin = "https://example.invalid"
	check(t, c.call("POST", "/api/chatgpt/start", object{}), 403, nil)
}
