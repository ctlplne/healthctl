// Package ehr reads patient-authorized laboratory results from a pinned SMART on FHIR portal.
package ehr

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	SMARTObservationLimit = 30
	maxFHIRBytes          = 2 << 20
	maxOAuthBytes         = 64 << 10
	// A patient-scoped standalone launch needs no refresh token or user identity.
	laboratoryScope    = "patient/Observation.rs"
	category           = "http://terminology.hl7.org/CodeSystem/observation-category|laboratory"
	smartPartialSystem = "urn:clinical-history:ehr-import"
)

type Config struct {
	Name, FHIRBase, AuthorizationEndpoint, TokenEndpoint, ClientID, RedirectURI string
	AuthorizationIssuer                                                         string
}

type Client struct {
	cfg  Config
	http *http.Client
}

var (
	verifierPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)
	statePattern    = regexp.MustCompile(`^[A-Za-z0-9._~-]{22,256}$`)
	fhirPatientID   = regexp.MustCompile(`^[A-Za-z0-9.-]{1,64}$`)
	bearerPattern   = regexp.MustCompile(`^[a-zA-Z0-9._~+/-]+=*$`)
)

func New(c Config) (*Client, error) {
	if _, ok := cleanSourceName(c.Name); !ok || !boundedText(c.ClientID, 512) {
		return nil, errors.New("configure the portal name and registered public client ID")
	}
	c.FHIRBase = strings.TrimSuffix(c.FHIRBase, "/")
	for _, v := range []string{c.FHIRBase, c.AuthorizationEndpoint, c.TokenEndpoint, c.RedirectURI} {
		if !validEndpoint(v) {
			return nil, errors.New("configure exact portal and callback URLs using HTTPS or an explicit loopback address")
		}
	}
	if c.AuthorizationIssuer != "" && !validEndpoint(c.AuthorizationIssuer) {
		return nil, errors.New("configure an exact authorization issuer URL")
	}
	// Browser authorization and server token exchange are separate, pinned endpoints.
	if c.AuthorizationEndpoint == c.TokenEndpoint || c.RedirectURI == c.AuthorizationEndpoint || c.RedirectURI == c.TokenEndpoint {
		return nil, errors.New("configure separate authorization, token and callback endpoints")
	}
	dialer := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if strings.EqualFold(host, "localhost") {
				address = net.JoinHostPort("127.0.0.1", port)
			}
			return dialer.DialContext(ctx, network, address)
		},
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 32 << 10, IdleConnTimeout: 30 * time.Second,
	}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Client{cfg: c, http: client}, nil
}

func boundedText(s string, limit int) bool {
	return len(s) > 0 && len(s) <= limit && s == strings.TrimSpace(s) && strings.IndexFunc(s, unicode.IsControl) < 0
}
func validEndpoint(s string) bool {
	if !boundedText(s, 2048) || strings.ContainsAny(s, `\?#%`) {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || (u.Path != "" && path.Clean(u.Path) != strings.TrimSuffix(u.Path, "/") && u.Path != "/") {
		return false
	}
	if u.Port() != "" {
		v, err := strconv.Atoi(u.Port())
		if err != nil || v < 1 || v > 65535 {
			return false
		}
	}
	if u.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "http" && (strings.EqualFold(u.Hostname(), "localhost") || ip != nil && ip.IsLoopback())
}

// Begin validates the portal's live discovery document on this user-initiated request.
// State, the verifier, and their single-use authenticated session binding belong to the caller.
func (c *Client) Begin(ctx context.Context, state, verifier string) (string, error) {
	if !statePattern.MatchString(state) || !verifierPattern.MatchString(verifier) {
		return "", errors.New("invalid SMART state or PKCE verifier")
	}
	var d struct {
		Authorization  string   `json:"authorization_endpoint"`
		Token          string   `json:"token_endpoint"`
		Issuer         string   `json:"issuer"`
		ResponseIssuer bool     `json:"authorization_response_iss_parameter_supported"`
		Challenge      []string `json:"code_challenge_methods_supported"`
		Capabilities   []string `json:"capabilities"`
		Grants         []string `json:"grant_types_supported"`
		Responses      []string `json:"response_types_supported"`
		Auth           []string `json:"token_endpoint_auth_methods_supported"`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.FHIRBase+"/.well-known/smart-configuration", nil)
	if err != nil {
		return "", errors.New("invalid SMART discovery request")
	}
	req.Header.Set("Accept", "application/json")
	raw, err := c.read(req, maxOAuthBytes)
	if err != nil {
		return "", errors.New("SMART discovery was unavailable at the configured FHIR base")
	}
	if json.Unmarshal(raw, &d) != nil || d.Authorization != c.cfg.AuthorizationEndpoint || d.Token != c.cfg.TokenEndpoint {
		return "", errors.New("SMART discovery did not match the configured authorization and token endpoints")
	}
	if c.cfg.AuthorizationIssuer != "" && d.Issuer != "" && d.Issuer != c.cfg.AuthorizationIssuer {
		return "", errors.New("SMART discovery did not match the configured authorization issuer")
	}
	if d.ResponseIssuer && c.cfg.AuthorizationIssuer == "" {
		return "", errors.New("configure this portal's authorization issuer before beginning consent")
	}
	if !slices.Contains(d.Challenge, "S256") || !allowsWhenDeclared(d.Grants, "authorization_code") || !allowsWhenDeclared(d.Responses, "code") || !allowsWhenDeclared(d.Auth, "none") {
		return "", errors.New("the portal did not advertise compatible public-client PKCE authorization")
	}
	for _, capability := range []string{"launch-standalone", "client-public", "context-standalone-patient", "permission-patient", "permission-v2"} {
		if !slices.Contains(d.Capabilities, capability) {
			return "", errors.New("the portal did not advertise the required capabilities for read-only access to your records")
		}
	}
	challenge := sha256.Sum256([]byte(verifier))
	u, _ := url.Parse(c.cfg.AuthorizationEndpoint)
	u.RawQuery = url.Values{"response_type": {"code"}, "client_id": {c.cfg.ClientID}, "redirect_uri": {c.cfg.RedirectURI}, "state": {state}, "aud": {c.cfg.FHIRBase}, "scope": {"launch/patient " + laboratoryScope}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}}.Encode()
	return u.String(), nil
}

func allowsWhenDeclared(options []string, value string) bool {
	return options == nil || slices.Contains(options, value)
}

type tokenGrant struct {
	Access  string `json:"access_token"`
	Type    string `json:"token_type"`
	Scope   string `json:"scope"`
	Patient string `json:"patient"`
	Expires *int64 `json:"expires_in"`
	Refresh string `json:"refresh_token"`
}

// Results returns one bounded FHIR searchset for local validation. It does not
// follow paginated links. A partial result carries a local tag and no search links.
func (c *Client) Results(ctx context.Context, code, verifier string) ([]byte, string, error) {
	if !boundedText(code, 2048) || !verifierPattern.MatchString(verifier) {
		return nil, "", errors.New("invalid authorization code or PKCE verifier")
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {c.cfg.RedirectURI}, "client_id": {c.cfg.ClientID}, "code_verifier": {verifier}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, "", errors.New("invalid token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	raw, err := c.read(req, maxOAuthBytes)
	if err != nil {
		return nil, "", errors.New("portal authorization was declined, expired, or unavailable")
	}
	var grant tokenGrant
	if json.Unmarshal(raw, &grant) != nil || !strings.EqualFold(grant.Type, "Bearer") || len(grant.Access) > 8192 || !bearerPattern.MatchString(grant.Access) || !fhirPatientID.MatchString(grant.Patient) || grant.Patient == "." || grant.Patient == ".." || grant.Refresh != "" || (grant.Expires != nil && *grant.Expires <= 0) || !patientReadScope(grant.Scope) {
		return nil, "", errors.New("the portal did not grant read-only access to observations from your record")
	}
	endpoint, _ := url.Parse(c.cfg.FHIRBase + "/Observation")
	endpoint.RawQuery = url.Values{"patient": {grant.Patient}, "category": {category}, "_count": {strconv.Itoa(SMARTObservationLimit)}}.Encode()
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, "", errors.New("invalid FHIR search")
	}
	req.Header.Set("Accept", "application/fhir+json; fhirVersion=4.0")
	req.Header.Set("Authorization", "Bearer "+grant.Access)
	raw, err = c.read(req, maxFHIRBytes)
	if err != nil {
		return nil, "", errors.New("laboratory search failed or exceeded the local response limit")
	}
	searchset, err := boundedSearchset(raw)
	if err != nil {
		return nil, "", err
	}
	return searchset, grant.Patient, nil
}

func patientReadScope(scope string) bool {
	search := false
	for _, s := range strings.Fields(scope) {
		base, filter, restricted := strings.Cut(s, "?")
		if restricted {
			q, err := url.ParseQuery(filter)
			if err != nil || len(q) != 1 || len(q["category"]) != 1 || (q.Get("category") != "laboratory" && q.Get("category") != category) {
				return false
			}
		}
		switch base {
		case "launch/patient":
			if restricted {
				return false
			}
		case "patient/Observation.s", laboratoryScope:
			search = true
		case "patient/Observation.r":
			// An additional read-only resource scope still requires a search grant.
		default:
			return false
		}
	}
	return search
}

func (c *Client) read(req *http.Request, limit int64) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("portal unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > limit {
		return nil, errors.New("portal returned an unavailable or oversized result")
	}
	if strings.HasPrefix(req.Header.Get("Accept"), "application/fhir+json") {
		contentType := resp.Header.Get("Content-Type")
		if contentType != "" {
			_, params, e := mime.ParseMediaType(contentType)
			if e != nil {
				return nil, errors.New("invalid FHIR response media type")
			}
			if version := params["fhirversion"]; version != "" && version != "4.0" && version != "4.0.0" && version != "4.0.1" {
				return nil, errors.New("portal did not return R4 laboratory results")
			}
		}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, errors.New("could not read bounded portal response")
	}
	return b, nil
}

func boundedSearchset(raw []byte) ([]byte, error) {
	var source struct {
		ResourceType      string            `json:"resourceType"`
		ImplicitRules     json.RawMessage   `json:"implicitRules"`
		ModifierExtension json.RawMessage   `json:"modifierExtension"`
		Type              string            `json:"type"`
		Entry             []json.RawMessage `json:"entry"`
		Total             *int              `json:"total"`
		Link              []struct {
			Relation string `json:"relation"`
		} `json:"link"`
	}
	if json.Unmarshal(raw, &source) != nil || source.ResourceType != "Bundle" || source.Type != "searchset" || len(source.ImplicitRules) > 0 || len(source.ModifierExtension) > 0 || (source.Total != nil && *source.Total < 0) {
		return nil, errors.New("the portal returned an invalid FHIR searchset")
	}
	partial := len(source.Entry) > SMARTObservationLimit || (source.Total != nil && *source.Total > len(source.Entry))
	for _, link := range source.Link {
		if link.Relation == "next" {
			partial = true
		}
	}
	result := map[string]any{"resourceType": "Bundle", "type": "searchset"}
	if n := min(SMARTObservationLimit, len(source.Entry)); n > 0 {
		result["entry"] = source.Entry[:n]
	}
	if source.Total != nil {
		result["total"] = *source.Total
	}
	if partial {
		result["meta"] = map[string]any{"tag": []map[string]string{{"system": smartPartialSystem, "code": "partial"}}}
	}
	return json.Marshal(result)
}
