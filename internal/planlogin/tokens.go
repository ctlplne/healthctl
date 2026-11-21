package planlogin

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

var ErrTooEarly = errors.New("the ChatGPT session cannot yet be refreshed")

func (s Session) NeedsRefresh(now time.Time) bool {
	if s.AccessToken == "" || s.ExpiresAt.IsZero() || now.Before(s.EarliestRefreshAt) {
		return false
	}
	if s.RefreshToken == "" {
		return !now.Before(s.ExpiresAt)
	}
	return !now.Add(time.Minute).Before(s.ExpiresAt)
}

type tokenReply struct {
	Access   string          `json:"access_token"`
	Refresh  string          `json:"refresh_token"`
	ID       string          `json:"id_token"`
	Type     string          `json:"token_type"`
	Expires  int64           `json:"expires_in"`
	Scope    string          `json:"scope"`
	Earliest json.RawMessage `json:"earliest_refresh_at"`
}

func tokenString(s string) bool {
	if len(s) == 0 || len(s) > 32<<10 {
		return false
	}
	for i := range len(s) {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

func (c *Client) exchange(ctx context.Context, form url.Values, refresh bool) (tokenReply, error) {
	data, status, err := c.request(ctx, tokenEndpoint, form)
	var t tokenReply
	if err != nil {
		return t, err
	}
	if status != http.StatusOK {
		// Only recognized protocol codes from a client-error response change
		// local credential state. Provider diagnostics can contain credentials.
		if status != http.StatusBadRequest && status != http.StatusUnauthorized {
			return t, ErrProtocol
		}
		switch oauthErrorCode(data) {
		case "invalid_client":
			return t, ErrInvalidClient
		case "invalid_grant":
			if !refresh {
				return t, ErrCodeExpired
			}
			return t, ErrRefreshExpired
		case "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused":
			if refresh {
				return t, ErrRefreshExpired
			}
		case "temporarily_unavailable", "server_error":
			return t, ErrTemporary
		}
		return t, ErrProtocol
	}
	if json.Unmarshal(data, &t) != nil || !strings.EqualFold(t.Type, "Bearer") ||
		!tokenString(t.Access) || t.Expires <= 0 || t.Expires > 30*24*60*60 ||
		(t.Refresh != "" && !tokenString(t.Refresh)) || (refresh && t.Refresh == "") {
		return tokenReply{}, ErrProtocol
	}
	return t, nil
}

func oauthErrorCode(data []byte) string {
	var response struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &response) != nil {
		return ""
	}
	var code string
	if json.Unmarshal(response.Error, &code) == nil {
		return code
	}
	var detail struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(response.Error, &detail) != nil {
		return ""
	}
	return detail.Code
}

// The OIDC verifier intentionally keeps verification errors opaque. Remember a
// temporary transport failure from its key lookup without returning raw JWTs,
// claims, URLs or upstream response bodies through an error.
type verificationKeys struct {
	source    oidc.KeySet
	temporary bool
}

func (k *verificationKeys) VerifySignature(ctx context.Context, jwt string) ([]byte, error) {
	payload, err := k.source.VerifySignature(ctx, jwt)
	k.temporary = errors.Is(err, ErrTemporary) || ctx.Err() != nil
	return payload, err
}

// The reference lists earliest_refresh_at without prescribing one JSON spelling.
// Accept absolute Unix seconds/milliseconds and RFC3339, never a relative TTL.
func refreshTime(raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, nil
	}
	s := string(raw)
	if s[0] == '"' {
		if json.Unmarshal(raw, &s) != nil {
			return time.Time{}, ErrProtocol
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC(), nil
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return time.Time{}, ErrProtocol
	}
	if n >= 1e12 {
		return time.UnixMilli(n).UTC(), nil
	}
	return time.Unix(n, 0).UTC(), nil
}

func (c *Client) accept(ctx context.Context, s Session, t tokenReply, refresh bool) (Session, error) {
	now := c.now().UTC()
	expiry := now.Add(time.Duration(t.Expires) * time.Second)
	earliest, err := refreshTime(t.Earliest)
	if err != nil || earliest.After(expiry) {
		return Session{}, ErrProtocol
	}
	scopes := strings.Fields(t.Scope)
	if slices.Contains(scopes, "offline_access") && t.Refresh == "" {
		return Session{}, ErrProtocol
	}
	if !refresh || t.ID != "" {
		if !tokenString(t.ID) {
			return Session{}, ErrIdentity
		}
		keys := &verificationKeys{source: c.keys}
		id, err := oidc.NewVerifier(issuer, keys, &oidc.Config{ClientID: s.ClientID, SupportedSigningAlgs: []string{oidc.RS256}, Now: c.now}).Verify(ctx, t.ID)
		if err != nil {
			if keys.temporary {
				return Session{}, ErrTemporary
			}
			return Session{}, ErrIdentity
		}
		var claims struct {
			AuthorizedParty string `json:"azp"`
			Email           string `json:"email"`
			EmailVerified   bool   `json:"email_verified"`
		}
		if id.Claims(&claims) != nil || id.Subject == "" || len(id.Subject) > 512 ||
			(s.Subject != "" && id.Subject != s.Subject) || !id.Expiry.After(now) ||
			id.IssuedAt.IsZero() || id.IssuedAt.After(now.Add(time.Minute)) || !id.Expiry.After(id.IssuedAt) {
			return Session{}, ErrIdentity
		}
		// healthctl trusts no other audience. An explicit authorized party must
		// also match this particular registration, even if emails are identical.
		if len(id.Audience) != 1 || id.Audience[0] != s.ClientID || (claims.AuthorizedParty != "" && claims.AuthorizedParty != s.ClientID) {
			return Session{}, ErrIdentity
		}
		// Refresh ID tokens may omit their nonce. If provided it must still
		// match the nonce established by this account's last successful sign-in.
		if (!refresh || id.Nonce != "") && (s.Nonce == "" || subtle.ConstantTimeCompare([]byte(id.Nonce), []byte(s.Nonce)) != 1) {
			return Session{}, ErrIdentity
		}
		if id.AccessTokenHash != "" && id.VerifyAccessToken(t.Access) != nil {
			return Session{}, ErrIdentity
		}
		s.Subject, s.IDToken = id.Subject, t.ID
		s.Email = ""
		if claims.EmailVerified && len(claims.Email) <= 254 && !strings.ContainsAny(claims.Email, "\r\n\t") {
			s.Email = claims.Email
		}
	}
	// Scope comes exclusively from this token response. A verified identity
	// without explicit plan permission remains signed in with inference disabled.
	s.AccessToken, s.RefreshToken, s.Scopes, s.ExpiresAt, s.EarliestRefreshAt = t.Access, t.Refresh, scopes, expiry, earliest
	return s, nil
}
