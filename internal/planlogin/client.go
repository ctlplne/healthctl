package planlogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Client struct {
	http           *http.Client
	keys           oidc.KeySet
	now            func() time.Time
	mu             sync.Mutex
	discoveryUntil time.Time
}

func New() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return newClient(transport, time.Now)
}

func newClient(transport http.RoundTripper, now func() time.Time) *Client {
	h := &http.Client{Transport: guardedTransport{transport}, Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &Client{http: h, now: now, keys: oidc.NewRemoteKeySet(oidc.ClientContext(context.Background(), h), keysEndpoint)}
}

// The same bounds and destination checks cover discovery, library JWKS fetches,
// revocation, and exchanges. No redirect can transfer a code or rotating token.
type guardedTransport struct{ inner http.RoundTripper }

func (t guardedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	switch r.URL.String() {
	case keysEndpoint, discoveryEndpoint:
		if r.Method != http.MethodGet {
			return nil, ErrProtocol
		}
	case tokenEndpoint, revocationEndpoint:
		if r.Method != http.MethodPost {
			return nil, ErrProtocol
		}
	default:
		return nil, ErrProtocol
	}
	res, err := t.inner.RoundTrip(r)
	if err != nil {
		return nil, ErrTemporary
	}
	defer res.Body.Close()
	// go-oidc includes non-200 JWKS response bodies in its internal error.
	// A key service outage is retryable and says nothing about an account.
	if r.URL.String() == keysEndpoint && (res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500) {
		return nil, ErrTemporary
	}
	const limit = 128 << 10
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, ErrTemporary
	}
	if len(data) > limit {
		return nil, ErrProtocol
	}
	res.Body = io.NopCloser(bytes.NewReader(data))
	return res, nil
}

type metadata struct {
	Issuer        string   `json:"issuer"`
	Authorization string   `json:"authorization_endpoint"`
	Token         string   `json:"token_endpoint"`
	Revocation    string   `json:"revocation_endpoint"`
	Keys          string   `json:"jwks_uri"`
	Signing       []string `json:"id_token_signing_alg_values_supported"`
}

func (c *Client) discover(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.discoveryUntil.After(c.now()) {
		return nil
	}
	data, status, err := c.request(ctx, discoveryEndpoint, nil)
	if err != nil {
		return err
	}
	var d metadata
	if status != http.StatusOK || json.Unmarshal(data, &d) != nil ||
		d.Issuer != issuer || d.Authorization != authorizeEndpoint || d.Token != tokenEndpoint ||
		d.Revocation != revocationEndpoint || d.Keys != keysEndpoint || !slices.Contains(d.Signing, oidc.RS256) {
		return ErrProtocol
	}
	c.discoveryUntil = c.now().Add(time.Hour)
	return nil
}

func (c *Client) request(ctx context.Context, endpoint string, form url.Values) ([]byte, int, error) {
	method := http.MethodGet
	var body io.Reader
	if form != nil {
		method, body = http.MethodPost, strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, 0, ErrProtocol
	}
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, ErrProtocol) {
			return nil, 0, ErrProtocol
		}
		return nil, 0, ErrTemporary
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
		return nil, res.StatusCode, ErrTemporary
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, res.StatusCode, ErrTemporary
	}
	return data, res.StatusCode, nil
}

func (c *Client) Exchange(ctx context.Context, a Attempt, query url.Values) (Session, error) {
	id, code, err := c.callback(a, query)
	if err != nil {
		return Session{}, err
	}
	if err := c.discover(ctx); err != nil {
		return Session{}, err
	}
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {id}, "code": {code},
		"code_verifier": {a.Verifier}, "redirect_uri": {a.RedirectURI}, "resource": {resource}}
	reply, err := c.exchange(ctx, form, false)
	if errors.Is(err, ErrCodeExpired) {
		return Session{}, &expiredCode{a.State, id}
	}
	if err != nil {
		return Session{}, err
	}
	mapping := Session{ClientID: id, HostID: a.HostID, CallbackPath: callbackPath(a.RedirectURI), Issuer: issuer, Subject: a.Subject, Nonce: a.Nonce}
	return c.accept(ctx, mapping, reply, false)
}

func (c *Client) Refresh(ctx context.Context, s Session) (Session, error) {
	if !registeredID(s.ClientID) || s.Issuer != issuer || s.Subject == "" || !hostPattern.MatchString(s.HostID) {
		return Session{}, ErrIdentity
	}
	if !tokenString(s.RefreshToken) {
		return Session{}, ErrRefreshExpired
	}
	if c.now().Before(s.EarliestRefreshAt) {
		return Session{}, ErrTooEarly
	}
	if err := c.discover(ctx); err != nil {
		return Session{}, err
	}
	reply, err := c.exchange(ctx, url.Values{"grant_type": {"refresh_token"}, "client_id": {s.ClientID}, "refresh_token": {s.RefreshToken}, "resource": {resource}}, true)
	if err != nil {
		return Session{}, err
	}
	return c.accept(ctx, s, reply, true)
}

// Revoke retries a transient error once while the refresh token is available.
// The caller must clear credentials even if remote revocation is unconfirmed.
func (c *Client) Revoke(ctx context.Context, s Session) error {
	if s.RefreshToken == "" {
		return nil
	}
	if !registeredID(s.ClientID) || !tokenString(s.RefreshToken) || s.Issuer != issuer || s.Subject == "" {
		return ErrIdentity
	}
	fn := func() error {
		if err := c.discover(ctx); err != nil {
			return err
		}
		data, status, err := c.request(ctx, revocationEndpoint, url.Values{"token": {s.RefreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {s.ClientID}})
		if err != nil {
			return err
		}
		if status != http.StatusOK || len(bytes.TrimSpace(data)) != 0 {
			return ErrProtocol
		}
		return nil
	}
	if err := fn(); !errors.Is(err, ErrTemporary) {
		return err
	}
	select {
	case <-ctx.Done():
		return ErrTemporary
	case <-time.After(300 * time.Millisecond):
		return fn()
	}
}
