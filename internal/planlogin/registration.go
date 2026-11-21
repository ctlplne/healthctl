// Package planlogin implements OpenAI's public-client "Continue with ChatGPT"
// protocol for a local host. The caller owns encrypted persistence, serialized
// refreshes and atomic, browser-session-bound consumption of callback state.
package planlogin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	issuer             = "https://auth.openai.com"
	authorizeEndpoint  = issuer + "/api/accounts/authorize"
	tokenEndpoint      = issuer + "/api/accounts/oauth/token"
	revocationEndpoint = issuer + "/api/accounts/oauth/revoke"
	discoveryEndpoint  = issuer + "/.well-known/openid-configuration"
	keysEndpoint       = issuer + "/.well-known/jwks.json"
	resource           = "https://api.openai.com/v1"
	dynamicClient      = "dynamic_agent_client"
	planScope          = "chatgpt.tokens.use.direct"
	identityScopes     = "openid profile email"
	requestedScopes    = identityScopes + " offline_access resource.invoke " + planScope
)

var (
	ErrDenied         = errors.New("ChatGPT authorization was declined")
	ErrTemporary      = errors.New("the ChatGPT authorization service is temporarily unavailable")
	ErrInvalidClient  = errors.New("OpenAI did not accept the registered ChatGPT client")
	ErrRefreshExpired = errors.New("the ChatGPT renewable session has ended; sign in again")
	ErrCodeExpired    = errors.New("the authorization code expired; start a fresh ChatGPT sign-in")
	ErrIdentity       = errors.New("the signed identity did not match this ChatGPT authorization")
	ErrCallback       = errors.New("the ChatGPT authorization callback was invalid or expired")
	ErrProtocol       = errors.New("OpenAI returned an unexpected authorization response")
	ErrConfig         = errors.New("ChatGPT sign-in requires a stable host identifier and an HTTP callback on 127.0.0.1")
	hostPattern       = regexp.MustCompile(`^(?:urn:uuid:[[:xdigit:]]{8}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{12}|urn:ietf:params:oauth:jwk-thumbprint:sha-256:[A-Za-z0-9_-]{43}|did:key:z[1-9A-HJ-NP-Za-km-z]{10,250})$`)
	clientPattern     = regexp.MustCompile(`^[a-zA-Z0-9._~-]{1,256}$`)
	pathPattern       = regexp.MustCompile(`^(/[a-zA-Z0-9_-]+)+$`)
)

// Attempt is private server-side transaction state. Never return it to a browser.
type Attempt struct {
	State       string
	Nonce       string
	Verifier    string
	ClientID    string
	Subject     string
	HostID      string
	RedirectURI string
	ExpiresAt   time.Time
	RequestPlan bool
	Reconsent   bool
}

// Session contains credentials intended solely for encrypted local storage.
// Use a separate allowlisted response type for browser-visible status.
type Session struct {
	ClientID          string
	HostID            string
	Issuer            string
	Subject           string
	Email             string
	IDToken           string
	AccessToken       string
	RefreshToken      string
	Scopes            []string
	ExpiresAt         time.Time
	EarliestRefreshAt time.Time
	Nonce             string
	CallbackPath      string
}

func (Attempt) String() string     { return "[private authorization attempt]" }
func (a Attempt) GoString() string { return a.String() }
func (Session) String() string     { return "[private ChatGPT account registration]" }
func (s Session) GoString() string { return s.String() }

func (s Session) HasPlan() bool {
	return s.AccessToken != "" && slices.Contains(s.Scopes, planScope)
}

func (s Session) SignedOut() Session {
	return Session{ClientID: s.ClientID, HostID: s.HostID, Issuer: s.Issuer, Subject: s.Subject, Email: s.Email, CallbackPath: s.CallbackPath}
}

func registeredID(id string) bool { return id != dynamicClient && clientPattern.MatchString(id) }

func callbackPath(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Hostname() != "127.0.0.1" ||
		u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(s, "#") ||
		!pathPattern.MatchString(u.Path) {
		return ""
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || u.Host != "127.0.0.1:"+strconv.Itoa(port) {
		return ""
	}
	return u.Path
}

func secret() string {
	var b [32]byte
	rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func (c *Client) Begin(hostID, redirectURI string, saved *Session, requestPlan, reconsent bool) (Attempt, string, error) {
	path := callbackPath(redirectURI)
	if !hostPattern.MatchString(hostID) || path == "" {
		return Attempt{}, "", ErrConfig
	}
	a := Attempt{ClientID: dynamicClient, HostID: hostID, RedirectURI: redirectURI, RequestPlan: requestPlan, Reconsent: requestPlan && reconsent}
	var hint, email string
	if saved != nil {
		if !registeredID(saved.ClientID) || saved.HostID != hostID || saved.Issuer != issuer || saved.Subject == "" || saved.CallbackPath != path {
			return Attempt{}, "", ErrIdentity
		}
		a.ClientID, a.Subject, hint, email = saved.ClientID, saved.Subject, saved.IDToken, saved.Email
	}
	return c.start(a, hint, email)
}

func (c *Client) start(a Attempt, idHint, email string) (Attempt, string, error) {
	a.State, a.Nonce, a.Verifier = secret(), secret(), secret()
	a.ExpiresAt = c.now().UTC().Add(10 * time.Minute)
	hash := sha256.Sum256([]byte(a.Verifier))
	scopes := identityScopes
	if a.RequestPlan {
		scopes = requestedScopes
	}
	q := url.Values{
		"response_type": {"code"}, "client_id": {a.ClientID}, "ext_agent_host_id": {a.HostID},
		"redirect_uri": {a.RedirectURI}, "scope": {scopes}, "resource": {resource},
		"state": {a.State}, "nonce": {a.Nonce}, "code_challenge_method": {"S256"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])},
	}
	if a.ClientID == dynamicClient {
		q.Set("agent_name_hint", "healthctl")
	}
	if idHint != "" {
		q.Set("id_token_hint", idHint)
	}
	if email != "" {
		q.Set("login_hint", email)
	}
	if a.Reconsent {
		q.Set("prompt", "consent")
	}
	return a, authorizeEndpoint + "?" + q.Encode(), nil
}

type expiredCode struct{ state, issuedClient string }

func (e *expiredCode) Error() string    { return ErrCodeExpired.Error() }
func (e *expiredCode) Unwrap() error    { return ErrCodeExpired }
func (e *expiredCode) GoString() string { return e.Error() }

// Retry prepares a fresh attempt after invalid_grant, including for an issued
// client that has not yet established its first verified account identity.
func (c *Client) Retry(a Attempt, failure error) (Attempt, string, error) {
	var e *expiredCode
	if !errors.As(failure, &e) || e.state != a.State || !registeredID(e.issuedClient) || !a.ExpiresAt.After(c.now()) {
		return Attempt{}, "", ErrCallback
	}
	a.ClientID = e.issuedClient
	return c.start(a, "", "")
}

func (c *Client) callback(a Attempt, q url.Values) (string, string, error) {
	if len(q["state"]) != 1 || a.State == "" ||
		subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(a.State)) != 1 ||
		!a.ExpiresAt.After(c.now()) || a.Nonce == "" || a.Verifier == "" ||
		callbackPath(a.RedirectURI) == "" || !hostPattern.MatchString(a.HostID) {
		return "", "", ErrCallback
	}
	if q.Has("error") {
		if len(q["error"]) == 1 && q.Get("error") == "access_denied" {
			return "", "", ErrDenied
		}
		return "", "", ErrCallback
	}
	if len(q["code"]) != 1 || !tokenString(q.Get("code")) || len(q["client_id"]) > 1 {
		return "", "", ErrCallback
	}
	id := a.ClientID
	if id == dynamicClient {
		id = q.Get("client_id")
	}
	if !registeredID(id) || (q.Has("client_id") && q.Get("client_id") != id) {
		return "", "", ErrCallback
	}
	return id, q.Get("code"), nil
}
