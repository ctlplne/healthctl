package planlogin

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

const (
	hostID      = "urn:uuid:a23eb3c5-738d-477d-9cdd-8258b4910a09"
	callbackURI = "http://127.0.0.1:8437/auth/chatgpt/callback"
	clientID    = "oaiapp_synthetic"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fixture struct {
	c                                    *Client
	a                                    Attempt
	key                                  *rsa.PrivateKey
	now                                  time.Time
	meta                                 metadata
	claims                               map[string]any
	reply                                map[string]any
	tokenStatus                          int
	keyStatus                            int
	keyCalls, exchangeCalls, revokeCalls int
	location                             string
	lastForm                             url.Values
	revokeStatus                         []int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{key: key, now: time.Now().UTC().Truncate(time.Second), tokenStatus: 200,
		meta: metadata{issuer, authorizeEndpoint, tokenEndpoint, revocationEndpoint, keysEndpoint, []string{"RS256"}}}
	f.c = newClient(roundTrip(func(r *http.Request) (*http.Response, error) {
		var out any
		status := 200
		switch r.URL.String() {
		case discoveryEndpoint:
			out = f.meta
		case keysEndpoint:
			f.keyCalls++
			if f.keyStatus != 0 {
				status = f.keyStatus
			}
			out = jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &f.key.PublicKey, KeyID: "demo-key", Algorithm: "RS256", Use: "sig"}}}
		case tokenEndpoint:
			f.exchangeCalls++
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			f.lastForm = r.PostForm
			out, status = f.reply, f.tokenStatus
		case revocationEndpoint:
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			f.lastForm = r.PostForm
			if f.revokeCalls < len(f.revokeStatus) {
				status = f.revokeStatus[f.revokeCalls]
			}
			f.revokeCalls++
		default:
			t.Fatalf("unexpected URL %s", r.URL.Path)
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Fatal("unexpected credential header")
		}
		var body []byte
		if out != nil {
			body, _ = json.Marshal(out)
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}, "Location": {f.location}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	}), func() time.Time { return f.now })
	f.a, _, err = f.c.Begin(hostID, callbackURI, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	f.claims = map[string]any{"iss": issuer, "sub": "synthetic-user", "aud": clientID, "iat": f.now.Unix(), "exp": f.now.Add(time.Hour).Unix(), "nonce": f.a.Nonce, "email": "example@example.test", "email_verified": true}
	f.reply = map[string]any{"id_token": f.sign(t, f.key), "scope": requestedScopes, "access_token": "test-access", "refresh_token": "test-refresh", "expires_in": 3600, "token_type": "Bearer"}
	return f
}
func (f *fixture) sign(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "demo-key"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(f.claims)
	if err != nil {
		t.Fatal(err)
	}
	jws, err := signer.Sign(b)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jws.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func query(a Attempt) url.Values {
	return url.Values{"state": {a.State}, "code": {"test-code"}, "client_id": {clientID}}
}
func (f *fixture) session(t *testing.T) Session {
	t.Helper()
	s, err := f.c.Exchange(t.Context(), f.a, query(f.a))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRegistrationAndAccountBinding(t *testing.T) {
	f := newFixture(t)
	a, auth, err := f.c.Begin(hostID, callbackURI, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(auth)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	hash := sha256.Sum256([]byte(a.Verifier))
	wantQuery := url.Values{
		"response_type": {"code"}, "agent_name_hint": {"healthctl"}, "client_id": {dynamicClient},
		"ext_agent_host_id": {hostID}, "nonce": {a.Nonce}, "state": {a.State}, "redirect_uri": {callbackURI},
		"code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])},
		"scope": {requestedScopes}, "resource": {resource},
	}
	if u.Scheme+"://"+u.Host+u.Path != authorizeEndpoint || !reflect.DeepEqual(q, wantQuery) || strings.Contains(auth, a.Verifier) {
		t.Fatal("incorrect first authorization")
	}
	if a.Nonce == f.a.Nonce || a.Verifier == f.a.Verifier || a.State == f.a.State || a.Nonce == a.State {
		t.Fatal("attempt randomness reused")
	}
	s := f.session(t)
	if !s.HasPlan() || s.Email != "example@example.test" || s.Subject != "synthetic-user" || s.ClientID != clientID || s.ExpiresAt != f.now.Add(time.Hour) || f.keyCalls != 1 {
		t.Fatal("incorrect verified session")
	}
	want := url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {"test-code"}, "code_verifier": {f.a.Verifier}, "resource": {resource}, "redirect_uri": {callbackURI}}
	if !reflect.DeepEqual(f.lastForm, want) {
		t.Fatal("incorrect exchange form")
	}
	const newPort = "http://127.0.0.1:7555/auth/chatgpt/callback"
	a, auth, err = f.c.Begin(hostID, newPort, &s, true, true)
	if err != nil {
		t.Fatal(err)
	}
	u, _ = url.Parse(auth)
	q = u.Query()
	if q.Has("agent_name_hint") || q.Get("id_token_hint") != s.IDToken || q.Get("login_hint") != s.Email || q.Get("prompt") != "consent" || q.Has("force_reconsent") || q.Get("client_id") != s.ClientID || a.Subject != s.Subject {
		t.Fatal("incorrect returning account request")
	}
	cb := query(a)
	cb.Del("client_id")
	cb.Set("scope", requestedScopes)
	f.claims["nonce"] = a.Nonce
	f.reply["id_token"] = f.sign(t, f.key)
	f.reply["scope"] = "openid email"
	narrowed, err := f.c.Exchange(t.Context(), a, cb)
	if err != nil || narrowed.HasPlan() || narrowed.Subject != s.Subject {
		t.Fatal("missing explicit plan scope")
	}
	cb.Set("client_id", "oaiapp_different")
	if _, err := f.c.Exchange(t.Context(), a, cb); !errors.Is(err, ErrCallback) {
		t.Fatal("accepted a changed issued client")
	}
	cb.Del("client_id")
	f.claims["sub"] = "different-user"
	f.reply["id_token"] = f.sign(t, f.key)
	if _, err := f.c.Exchange(t.Context(), a, cb); !errors.Is(err, ErrIdentity) {
		t.Fatal("accepted same email with different subject")
	}
	signedOut := s.SignedOut()
	if signedOut.Subject != s.Subject || signedOut.Issuer != issuer || signedOut.HostID != hostID || signedOut.ClientID != clientID || signedOut.Email != s.Email || signedOut.AccessToken != "" || signedOut.RefreshToken != "" || signedOut.IDToken != "" || signedOut.Nonce != "" || signedOut.HasPlan() {
		t.Fatal("incomplete sign-out")
	}
	_, auth, err = f.c.Begin(hostID, newPort, &signedOut, true, false)
	if err != nil {
		t.Fatal(err)
	}
	u, _ = url.Parse(auth)
	if u.Query().Has("id_token_hint") || u.Query().Has("agent_name_hint") || u.Query().Has("prompt") {
		t.Fatal("stale reauthorization metadata")
	}
	for _, rendered := range []string{fmt.Sprint(s), fmt.Sprintf("%+v", s), fmt.Sprintf("%#v", s), fmt.Sprint(f.a), fmt.Sprintf("%#v", f.a)} {
		for _, secret := range []string{s.Email, s.AccessToken, s.RefreshToken, s.IDToken, f.a.Verifier, f.a.State} {
			if strings.Contains(rendered, secret) {
				t.Fatal("diagnostic leaked secret")
			}
		}
	}
}

func TestCallbackValidationBeforeNetwork(t *testing.T) {
	f := newFixture(t)
	for _, tc := range []struct {
		name   string
		change func(url.Values)
		want   error
	}{
		{"wrong state", func(q url.Values) { q.Set("state", "other") }, ErrCallback},
		{"duplicate state", func(q url.Values) { q.Add("state", f.a.State) }, ErrCallback},
		{"malformed first registration", func(q url.Values) { q.Del("client_id") }, ErrCallback},
		{"dynamic instead of issued", func(q url.Values) { q.Set("client_id", dynamicClient) }, ErrCallback},
		{"duplicate code", func(q url.Values) { q.Add("code", "another") }, ErrCallback},
		{"denied", func(q url.Values) { q.Set("error", "access_denied"); q.Del("code") }, ErrDenied},
		{"wrong state on denial", func(q url.Values) { q.Set("error", "access_denied"); q.Set("state", "wrong") }, ErrCallback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := query(f.a)
			tc.change(q)
			_, err := f.c.Exchange(t.Context(), f.a, q)
			if !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
		})
	}
	f.now = f.a.ExpiresAt
	if _, err := f.c.Exchange(t.Context(), f.a, query(f.a)); !errors.Is(err, ErrCallback) {
		t.Fatal("accepted expired state")
	}
	if f.exchangeCalls != 0 || f.keyCalls != 0 {
		t.Fatal("made a network request before validating state")
	}
	for _, uri := range []string{"https://example.com/auth/chatgpt/callback", "http://localhost:8437/auth/chatgpt/callback", "http://127.0.0.1.example.com:8437/callback", callbackURI + "?x=y", callbackURI + "#", "http://user@127.0.0.1:8437/path", "http://127.0.0.1:8437/../other"} {
		if _, _, err := f.c.Begin(hostID, uri, nil, false, false); !errors.Is(err, ErrConfig) {
			t.Fatal("invalid redirect accepted")
		}
	}
}

func TestSignedIdentityValidation(t *testing.T) {
	f := newFixture(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		field string
		value any
	}{
		{"iss", "https://untrusted.test"}, {"aud", "different-client"}, {"aud", []string{clientID, "untrusted"}},
		{"azp", "different-client"}, {"exp", f.now.Unix()}, {"iat", f.now.Add(time.Hour).Unix()},
		{"sub", ""}, {"nonce", "unrelated"}, {"nonce", ""}, {"at_hash", "wrong-hash"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			old, exists := f.claims[tc.field]
			f.claims[tc.field] = tc.value
			f.reply["id_token"] = f.sign(t, f.key)
			if _, err := f.c.Exchange(t.Context(), f.a, query(f.a)); !errors.Is(err, ErrIdentity) {
				t.Fatal("accepted invalid ID token")
			}
			if exists {
				f.claims[tc.field] = old
			} else {
				delete(f.claims, tc.field)
			}
		})
	}
	f.reply["id_token"] = f.sign(t, other)
	if _, err := f.c.Exchange(t.Context(), f.a, query(f.a)); !errors.Is(err, ErrIdentity) {
		t.Fatal("accepted invalid signature")
	}
	f.claims["email_verified"] = false
	f.reply["id_token"] = f.sign(t, f.key)
	s := f.session(t)
	if s.Subject != "synthetic-user" || s.Email != "" {
		t.Fatal("unverified email used as identity")
	}
	f.key = other
	f.reply["id_token"] = f.sign(t, other)
	f.session(t)
}

func TestRefreshAndRevocation(t *testing.T) {
	f := newFixture(t)
	s := f.session(t)
	delete(f.reply, "id_token")
	f.reply["access_token"] = "test-access-2"
	f.reply["refresh_token"] = "test-refresh-2"
	f.reply["scope"] = "openid resource.invoke"
	f.reply["earliest_refresh_at"] = f.now.Add(3590 * time.Second).Unix()
	rotated, err := f.c.Refresh(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.RefreshToken != "test-refresh-2" || rotated.AccessToken != "test-access-2" || rotated.HasPlan() || rotated.IDToken != s.IDToken || rotated.Subject != s.Subject || rotated.ClientID != s.ClientID {
		t.Fatal("incorrect rotated registration")
	}
	want := url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {s.RefreshToken}, "resource": {resource}}
	if !reflect.DeepEqual(want, f.lastForm) {
		t.Fatal("incorrect refresh form")
	}
	if rotated.NeedsRefresh(f.now.Add(3550*time.Second)) || !rotated.NeedsRefresh(f.now.Add(3591*time.Second)) {
		t.Fatal("earliest refresh or expiry ignored")
	}
	if _, err := f.c.Refresh(t.Context(), rotated); !errors.Is(err, ErrTooEarly) {
		t.Fatal("refresh too early")
	}
	f.claims["sub"] = "wrong-user"
	f.reply["id_token"] = f.sign(t, f.key)
	if _, err := f.c.Refresh(t.Context(), s); !errors.Is(err, ErrIdentity) {
		t.Fatal("changed subject on refresh")
	}
	delete(f.claims, "nonce")
	f.claims["sub"] = s.Subject
	f.reply["id_token"] = f.sign(t, f.key)
	if _, err := f.c.Refresh(t.Context(), s); err != nil {
		t.Fatal("refresh without nonce", err)
	}
	f.claims["nonce"] = "unrelated"
	f.reply["id_token"] = f.sign(t, f.key)
	if _, err := f.c.Refresh(t.Context(), s); !errors.Is(err, ErrIdentity) {
		t.Fatal("changed nonce on refresh")
	}
	delete(f.reply, "refresh_token")
	if _, err := f.c.Refresh(t.Context(), s); !errors.Is(err, ErrProtocol) {
		t.Fatal("lost rotating refresh token")
	}
	f.revokeStatus = []int{503, 200}
	if err := f.c.Revoke(t.Context(), rotated); err != nil {
		t.Fatal(err)
	}
	want = url.Values{"token": {rotated.RefreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {clientID}}
	if f.revokeCalls != 2 || !reflect.DeepEqual(want, f.lastForm) {
		t.Fatal("incorrect revocation or retry")
	}
	if err := f.c.Revoke(t.Context(), s.SignedOut()); err != nil || f.revokeCalls != 2 {
		t.Fatal("redundant revocation")
	}
}

func TestErrorsAndOneTimeCodeRecovery(t *testing.T) {
	f := newFixture(t)
	s := f.session(t)
	f.tokenStatus = 400
	f.reply = map[string]any{"error": "invalid_grant", "error_description": "sensitive-upstream-response"}
	_, err := f.c.Exchange(t.Context(), f.a, query(f.a))
	if !errors.Is(err, ErrCodeExpired) || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("incorrect code exhaustion classification")
	}
	fresh, auth, err := f.c.Retry(f.a, err)
	if err != nil || fresh.State == f.a.State || fresh.Nonce == f.a.Nonce || fresh.Verifier == f.a.Verifier || fresh.ClientID != clientID || strings.Contains(auth, dynamicClient) {
		t.Fatal("failed to reauthorize with issued client")
	}
	if _, _, err = f.c.Retry(f.a, ErrCodeExpired); !errors.Is(err, ErrCallback) {
		t.Fatal("unbound retry accepted")
	}
	for _, code := range []string{"invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused", "invalid_client", "sensitive-upstream-code"} {
		f.reply["error"] = code
		_, err := f.c.Refresh(t.Context(), s)
		want := ErrRefreshExpired
		if code == "invalid_client" {
			want = ErrInvalidClient
		} else if code == "sensitive-upstream-code" {
			want = ErrProtocol
		}
		if !errors.Is(err, want) || strings.Contains(err.Error(), "sensitive") {
			t.Fatal("incorrect refresh classification for", code)
		}
	}
	for _, status := range []int{429, 500, 503} {
		f.tokenStatus = status
		f.reply["error"] = "invalid_grant"
		_, err := f.c.Refresh(t.Context(), s)
		if !errors.Is(err, ErrTemporary) {
			t.Fatal("cleared session on transient failure")
		}
	}
	f.tokenStatus = 401
	f.reply["error"] = map[string]string{"code": "refresh_token_reused", "message": "sensitive-upstream-response"}
	if _, err := f.c.Refresh(t.Context(), s); !errors.Is(err, ErrRefreshExpired) {
		t.Fatal("unrecognized structured refresh rejection")
	}
}

func TestKeyServiceOutageRetainsAccount(t *testing.T) {
	f := newFixture(t)
	f.keyStatus = 503
	if _, err := f.c.Exchange(t.Context(), f.a, query(f.a)); !errors.Is(err, ErrTemporary) || errors.Is(err, ErrIdentity) {
		t.Fatal("misclassified temporary JWKS outage", err)
	}
	f.keyStatus = 200
	f.session(t)
}

func TestPinnedDestinationsAndLimits(t *testing.T) {
	f := newFixture(t)
	for _, key := range []string{"discovery issuer", "authorization", "token", "revocation", "jwks", "signing algorithm"} {
		old := f.meta
		switch key {
		case "discovery issuer":
			f.meta.Issuer = "https://example.test"
		case "authorization":
			f.meta.Authorization = "https://example.test"
		case "token":
			f.meta.Token = "https://example.test"
		case "revocation":
			f.meta.Revocation = "https://example.test"
		case "jwks":
			f.meta.Keys = "https://example.test"
		case "signing algorithm":
			f.meta.Signing = []string{"HS256"}
		}
		_, err := f.c.Exchange(t.Context(), f.a, query(f.a))
		if !errors.Is(err, ErrProtocol) || f.exchangeCalls != 0 {
			t.Fatal("unsafe discovery accepted:", key)
		}
		f.meta = old
	}
	f.tokenStatus = 307
	f.location = "https://elsewhere.test/exchange"
	_, err := f.c.Exchange(t.Context(), f.a, query(f.a))
	if !errors.Is(err, ErrProtocol) || f.exchangeCalls != 1 {
		t.Fatal("redirect accepted")
	}
	f.tokenStatus = 200
	f.reply["ignored"] = strings.Repeat("x", 130<<10)
	if _, err := f.c.Exchange(t.Context(), f.a, query(f.a)); !errors.Is(err, ErrProtocol) {
		t.Fatal("oversized token response accepted")
	}
	guard := guardedTransport{roundTrip(func(r *http.Request) (*http.Response, error) { t.Fatal("made unexpected request"); return nil, nil })}
	for _, address := range []string{"https://example.test/.well-known/jwks.json", keysEndpoint + "?q=other", issuer + "/other"} {
		r, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, address, nil)
		if _, err := guard.RoundTrip(r); !errors.Is(err, ErrProtocol) {
			t.Fatal("untrusted target accepted")
		}
	}
}

func TestEarliestRefreshRepresentations(t *testing.T) {
	want := time.Date(2026, time.November, 20, 10, 0, 0, 0, time.UTC)
	for _, repr := range []any{want.Unix(), want.UnixMilli(), want.Format(time.RFC3339)} {
		b, _ := json.Marshal(repr)
		v, err := refreshTime(b)
		if err != nil || !v.Equal(want) {
			t.Fatal("unsupported timestamp")
		}
	}
	if _, err := refreshTime(json.RawMessage(`"unsupported"`)); !errors.Is(err, ErrProtocol) {
		t.Fatal("malformed earliest refresh accepted")
	}
	if !slices.Contains(strings.Fields(requestedScopes), planScope) {
		t.Fatal("plan scope omitted")
	}
}

func TestIdentityOnlyChoiceCarriesThroughReturnAndRetry(t *testing.T) {
	f := newFixture(t)
	account := f.session(t)
	for _, saved := range []*Session{nil, &account} {
		a, auth, err := f.c.Begin(hostID, callbackURI, saved, false, true)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(auth)
		if a.RequestPlan || u.Query().Get("scope") != identityScopes || u.Query().Has("prompt") {
			t.Fatal("identity-only sign-in requested plan privileges")
		}
		if saved != nil && (u.Query().Get("client_id") != saved.ClientID || u.Query().Get("id_token_hint") != saved.IDToken) {
			t.Fatal("identity-only sign-in lost account binding")
		}
		f.tokenStatus, f.reply = 400, map[string]any{"error": "invalid_grant"}
		_, failure := f.c.Exchange(t.Context(), a, query(a))
		next, auth, err := f.c.Retry(a, failure)
		if err != nil {
			t.Fatal(err)
		}
		u, _ = url.Parse(auth)
		if next.RequestPlan || u.Query().Get("scope") != identityScopes || u.Query().Has("prompt") || next.ClientID != clientID {
			t.Fatal("retry widened requested privileges")
		}
		f.claims["nonce"] = next.Nonce
		f.tokenStatus, f.reply = 200, map[string]any{"id_token": f.sign(t, f.key), "scope": identityScopes, "access_token": "identity-only", "expires_in": 3600, "token_type": "Bearer"}
		s, err := f.c.Exchange(t.Context(), next, query(next))
		if err != nil || s.HasPlan() || s.Subject != account.Subject || s.RefreshToken != "" {
			t.Fatal("failed to retain identity-only sign-in", err)
		}
	}
}
