package ehr

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
)

const fixtureVerifier = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVXYZ-0123456789"
const fixtureState = "opaque_state_12345678901"

func loopback(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if errors.Is(err, syscall.EPERM) {
		t.Skip("loopback networking unavailable in sandbox")
	}
	if err != nil {
		t.Fatal(err)
	}
	server := &httptest.Server{Listener: ln, Config: &http.Server{Handler: h}}
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func metadata(cfg Config) map[string]any {
	return map[string]any{
		"authorization_endpoint": cfg.AuthorizationEndpoint, "token_endpoint": cfg.TokenEndpoint,
		"issuer": cfg.AuthorizationIssuer, "code_challenge_methods_supported": []string{"S256"},
		"capabilities":          []string{"launch-standalone", "client-public", "context-standalone-patient", "permission-patient", "permission-v2"},
		"grant_types_supported": []string{"authorization_code"}, "token_endpoint_auth_methods_supported": []string{"none"},
	}
}
func grant() map[string]any {
	return map[string]any{"access_token": "syntheticBearer", "token_type": "Bearer", "scope": "launch/patient patient/Observation.rs", "patient": "fixture-patient", "expires_in": 60}
}
func config(base string) Config {
	return Config{Name: "Fictional portal", FHIRBase: base + "/fhir", AuthorizationEndpoint: base + "/authorize", TokenEndpoint: base + "/token", ClientID: "local-public", RedirectURI: "http://localhost:8743/api/ehr/callback"}
}

func TestStandaloneWireAndBoundedSearch(t *testing.T) {
	var cfg Config
	var searches, discoveries, exchanges atomic.Int32
	fhir := loopback(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fhir/.well-known/smart-configuration":
			discoveries.Add(1)
			if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.URL.RawQuery != "" {
				t.Error("unexpected discovery request")
			}
			json.NewEncoder(w).Encode(metadata(cfg))
		case "/fhir/Observation":
			searches.Add(1)
			q := r.URL.Query()
			if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer syntheticBearer" || len(q) != 3 || q.Get("patient") != "fixture-patient" || q.Get("category") != category || q.Get("_count") != strconv.Itoa(SMARTObservationLimit) {
				t.Error("search left patient, type, or budget boundary")
			}
			resource := map[string]any{"resourceType": "Observation", "id": "ob1", "meta": map[string]string{"versionId": "3"}, "valueString": "Synthetic fixture only", "subject": map[string]string{"reference": "Patient/fixture-patient"}, "modifierExtension": []map[string]string{{"url": "urn:fictional:qualifier"}}}
			// The local parser needs unmodified resources (even unverified attributes)
			// to audit skips. Paging URLs belong to the external service.
			json.NewEncoder(w).Encode(map[string]any{"resourceType": "Bundle", "type": "searchset", "entry": []any{map[string]any{"resource": resource}}, "link": []any{map[string]any{"relation": "next", "url": cfg.AuthorizationEndpoint + "/unexpected"}}})
		default:
			t.Error("followed an unexpected FHIR path")
			w.WriteHeader(404)
		}
	})
	auth := loopback(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Error("followed untrusted link or attempted to perform browser authorization")
			w.WriteHeader(404)
			return
		}
		exchanges.Add(1)
		if r.Method != "POST" || r.Header.Get("Authorization") != "" || r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Error("invalid public-client token request")
		}
		r.ParseForm()
		if len(r.PostForm) != 5 || r.PostForm.Get("client_id") != cfg.ClientID || r.PostForm.Get("code") != "opaque-code" || r.PostForm.Get("code_verifier") != fixtureVerifier || r.PostForm.Get("redirect_uri") != cfg.RedirectURI || r.PostForm.Get("grant_type") != "authorization_code" {
			t.Error("incorrect exchange form")
		}
		json.NewEncoder(w).Encode(grant())
	})
	cfg = config(fhir.URL)
	cfg.AuthorizationEndpoint = auth.URL + "/authorize"
	cfg.TokenEndpoint = auth.URL + "/token"
	cfg.AuthorizationIssuer = auth.URL
	t.Setenv("HTTP_PROXY", auth.URL+"/unexpected")
	t.Setenv("HTTPS_PROXY", auth.URL+"/unexpected")
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if discoveries.Load() != 0 {
		t.Fatal("construction performed discovery")
	}
	target, err := c.Begin(t.Context(), fixtureState, fixtureVerifier)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	challenge := sha256.Sum256([]byte(fixtureVerifier))
	if u.Scheme+"://"+u.Host+u.Path != cfg.AuthorizationEndpoint || q.Get("aud") != cfg.FHIRBase || q.Get("scope") != "launch/patient patient/Observation.rs" || q.Get("response_type") != "code" || q.Get("state") != fixtureState || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(challenge[:]) || len(q) != 8 || strings.Contains(target, fixtureVerifier) {
		t.Fatal("invalid standalone PKCE launch")
	}
	data, id, err := c.Results(t.Context(), "opaque-code", fixtureVerifier)
	if err != nil || id != "fixture-patient" || !strings.Contains(string(data), "modifierExtension") || !strings.Contains(string(data), smartPartialSystem) || strings.Contains(string(data), "/unexpected") || strings.Contains(string(data), "syntheticBearer") {
		t.Fatal("missing original, accidental outward link, or incomplete marker")
	}
	if searches.Load() != 1 || exchanges.Load() != 1 || discoveries.Load() != 1 {
		t.Fatal("unexpected outbound operation count")
	}
}

func TestDeniedOrBroadGrantStopsBeforeSearch(t *testing.T) {
	var searches atomic.Int32
	for _, setup := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"missing patient", func(v map[string]any) { delete(v, "patient") }},
		{"untrusted patient", func(v map[string]any) { v["patient"] = "somewhere/else" }},
		{"empty grant", func(v map[string]any) { delete(v, "scope") }},
		{"read only without search", func(v map[string]any) { v["scope"] = "patient/Observation.r" }},
		{"write", func(v map[string]any) { v["scope"] = "launch/patient patient/Observation.cruds" }},
		{"wildcard", func(v map[string]any) { v["scope"] = "patient/*.rs" }},
		{"other patient resource", func(v map[string]any) { v["scope"] = "patient/Observation.rs patient/Patient.r" }},
		{"refresh", func(v map[string]any) { v["refresh_token"] = "unexpected" }},
		{"expired", func(v map[string]any) { v["expires_in"] = 0 }},
		{"wrong category", func(v map[string]any) { v["scope"] = "patient/Observation.rs?category=vital-signs" }},
		{"identity scope", func(v map[string]any) { v["scope"] = "patient/Observation.rs openid" }},
	} {
		t.Run(setup.name, func(t *testing.T) {
			s := loopback(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/token" {
					searches.Add(1)
					w.WriteHeader(403)
					return
				}
				g := grant()
				setup.change(g)
				json.NewEncoder(w).Encode(g)
			})
			c, err := New(config(s.URL))
			if err != nil {
				t.Fatal(err)
			}
			if data, id, err := c.Results(t.Context(), "code", fixtureVerifier); err == nil || data != nil || id != "" {
				t.Error("unsafe or denied grant accepted")
			}
		})
	}
	if searches.Load() != 0 {
		t.Fatal("denied grant disclosed a token at the FHIR endpoint")
	}
	for _, scope := range []string{"patient/Observation.s", "launch/patient patient/Observation.rs?category=" + url.QueryEscape(category)} {
		if !patientReadScope(scope) {
			t.Error("proper narrowed search grant refused")
		}
	}
}

func TestPinsAndIssuerBeforeAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"changed authorize", func(v map[string]any) { v["authorization_endpoint"] = "https://different.example/authorize" }},
		{"changed token", func(v map[string]any) { v["token_endpoint"] = "https://different.example/token" }},
		{"plain PKCE only", func(v map[string]any) { v["code_challenge_methods_supported"] = []string{"plain"} }},
		{"different issuer", func(v map[string]any) { v["issuer"] = "https://different.example" }},
		{"missing public client", func(v map[string]any) { v["token_endpoint_auth_methods_supported"] = []string{"client_secret_post"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg Config
			s := loopback(t, func(w http.ResponseWriter, r *http.Request) {
				v := metadata(cfg)
				tc.change(v)
				json.NewEncoder(w).Encode(v)
			})
			cfg = config(s.URL)
			cfg.AuthorizationIssuer = s.URL
			c, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if target, err := c.Begin(t.Context(), fixtureState, fixtureVerifier); err == nil || target != "" {
				t.Fatal("unverified portal metadata accepted")
			}
		})
	}
}

func TestNoRedirectOnAnyEndpoint(t *testing.T) {
	var followed atomic.Int32
	stranger := loopback(t, func(w http.ResponseWriter, r *http.Request) { followed.Add(1) })
	for _, stage := range []string{"discovery", "token", "search"} {
		t.Run(stage, func(t *testing.T) {
			var cfg Config
			s := loopback(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case stage == "discovery" && strings.HasSuffix(r.URL.Path, "smart-configuration"), stage == "token" && r.URL.Path == "/token", stage == "search" && r.URL.Path == "/fhir/Observation":
					http.Redirect(w, r, stranger.URL+"/other", http.StatusTemporaryRedirect)
				case r.URL.Path == "/token":
					json.NewEncoder(w).Encode(grant())
				default:
					json.NewEncoder(w).Encode(metadata(cfg))
				}
			})
			cfg = config(s.URL)
			c, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if stage == "discovery" {
				_, err = c.Begin(t.Context(), fixtureState, fixtureVerifier)
			} else {
				_, _, err = c.Results(t.Context(), "code", fixtureVerifier)
			}
			if err == nil {
				t.Fatal("redirect silently accepted")
			}
		})
	}
	if followed.Load() != 0 {
		t.Fatal("sent a request to a redirect destination")
	}
}

func TestSearchsetLimitAndInvalidEnvelope(t *testing.T) {
	entries := make([]any, SMARTObservationLimit+2)
	for i := range entries {
		entries[i] = map[string]any{"resource": map[string]any{"resourceType": "Observation", "id": strconv.Itoa(i)}}
	}
	raw, _ := json.Marshal(map[string]any{"resourceType": "Bundle", "type": "searchset", "total": len(entries), "entry": entries})
	bounded, err := boundedSearchset(raw)
	if err != nil {
		t.Fatal(err)
	}
	var b struct{ Entry []json.RawMessage }
	if json.Unmarshal(bounded, &b) != nil || len(b.Entry) != SMARTObservationLimit || !strings.Contains(string(bounded), "partial") {
		t.Fatal("partial page cap missing")
	}
	empty, err := boundedSearchset([]byte(`{"resourceType":"Bundle","type":"searchset","total":0}`))
	if err != nil || strings.Contains(string(empty), "partial") {
		t.Fatal("complete empty result misreported")
	}
	for _, invalid := range []string{`{"resourceType":"OperationOutcome"}`, `{"resourceType":"Bundle","type":"collection"}`, `{"resourceType":"Bundle","type":"searchset","total":-1}`, `[]`} {
		if _, err := boundedSearchset([]byte(invalid)); err == nil {
			t.Fatal("invalid FHIR envelope accepted")
		}
	}
	s := loopback(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			json.NewEncoder(w).Encode(grant())
			return
		}
		io.WriteString(w, strings.Repeat("x", maxFHIRBytes+1))
	})
	c, err := New(config(s.URL))
	if err != nil {
		t.Fatal(err)
	}
	if data, id, err := c.Results(t.Context(), "code", fixtureVerifier); err == nil || len(data) != 0 || id != "" {
		t.Fatal("oversized search accepted")
	}
}

func TestRejectExplicitOtherFHIRVersion(t *testing.T) {
	s := loopback(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			json.NewEncoder(w).Encode(grant())
			return
		}
		w.Header().Set("Content-Type", "application/fhir+json; fhirVersion=5.0")
		io.WriteString(w, `{"resourceType":"Bundle","type":"searchset"}`)
	})
	c, err := New(config(s.URL))
	if err != nil {
		t.Fatal(err)
	}
	if data, _, err := c.Results(t.Context(), "code", fixtureVerifier); err == nil || data != nil {
		t.Fatal("accepted a different FHIR version")
	}
	if _, err := boundedSearchset([]byte(`{"resourceType":"Bundle","type":"searchset","implicitRules":"urn:portal:requires-custom-interpretation"}`)); err == nil {
		t.Fatal("dropped bundle-level interpretation rule")
	}
}

func TestSMARTConfiguration(t *testing.T) {
	for _, s := range []string{"http://ehr.example/fhir", "http://127.0.0.1.ehr.example/fhir", "https://user@ehr.example/fhir", "https://ehr.example/fhir#other", "https://ehr.example/a/../fhir", "https://ehr.example/fhir%2Fother", "ftp://ehr.example/fhir", "https://ehr.example:99999/fhir"} {
		cfg := config("https://ehr.example")
		cfg.FHIRBase = s
		if _, err := New(cfg); err == nil {
			t.Errorf("unexpectedly accepted %q", s)
		}
	}
	for _, s := range []string{"https://ehr.example/fhir", "http://localhost:1234", "http://127.0.0.1:1234", "http://[::1]:1234"} {
		if _, err := New(config(s)); err != nil {
			t.Errorf("unusable configured endpoint %q", s)
		}
	}
}
