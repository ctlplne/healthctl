package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// from sends a request as a given client address.
func from(c caller, addr, method, path string, in any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(in)
	req := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(b))
	req.RemoteAddr = addr
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, req)
	return w
}

func TestLoginThrottlePostgres(t *testing.T) {
	db, _, _ := testStore(t)
	s := newTestServer(db, io.Discard)
	s.loginDelay = func(int) time.Duration { return time.Millisecond }
	c := caller{h: s.Handler()}
	c.signup(t)
	wrong := object{"username": "synthetic-owner", "password": "not the password"}
	right := object{"username": "synthetic-owner", "password": "A synthetic owner password!"}
	for range 10 {
		check(t, from(c, "203.0.113.5:4000", "POST", "/api/login", wrong), 401, nil)
	}
	check(t, from(c, "203.0.113.5:4000", "POST", "/api/login", right), 429, nil)
	// Another client is not locked out by the first one.
	check(t, from(c, "203.0.113.9:4000", "POST", "/api/login", right), 200, nil)
	// This machine is slowed down but never locked out.
	for range 14 {
		check(t, from(c, "127.0.0.1:5000", "POST", "/api/login", wrong), 401, nil)
	}
	check(t, from(c, "127.0.0.1:5000", "POST", "/api/login", right), 200, nil)
	// Behind a trusted proxy each forwarded client has its own budget.
	t.Setenv("CLINICAL_TRUSTED_PROXY", "10.0.0.2")
	for range 11 {
		b, _ := json.Marshal(wrong)
		req := httptest.NewRequest("POST", "http://localhost/api/login", bytes.NewReader(b))
		req.RemoteAddr, req.Header["X-Forwarded-For"] = "10.0.0.2:80", []string{"198.51.100.7"}
		req.Header.Set("Content-Type", "application/json")
		c.h.ServeHTTP(httptest.NewRecorder(), req)
	}
	b, _ := json.Marshal(right)
	req := httptest.NewRequest("POST", "http://localhost/api/login", bytes.NewReader(b))
	req.RemoteAddr, req.Header["X-Forwarded-For"] = "10.0.0.2:80", []string{"198.51.100.8"}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, req)
	check(t, w, 200, nil)
}

func TestSetupTokenPostgres(t *testing.T) {
	db, _, _ := testStore(t)
	var logs strings.Builder
	s := newTestServer(db, &logs)
	s.PublicOrigin, s.Secure = "https://history.example.test", true
	h := s.Handler()
	post := func(body object) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "https://history.example.test/api/setup", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://history.example.test")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	account := object{"username": "synthetic-owner", "password": "A synthetic owner password!"}
	if w := post(account); w.Code != http.StatusForbidden {
		t.Fatalf("a public installation must ask for the setup code, got %d", w.Code)
	}
	account["setupCode"] = "WRONG-CODE"
	if w := post(account); w.Code != http.StatusForbidden {
		t.Fatalf("a wrong setup code was accepted, got %d", w.Code)
	}
	code := s.currentSetupCode()
	if !strings.Contains(logs.String(), "event=setup_code code="+code) {
		t.Fatal("the setup code must be printed to the server log for the operator")
	}
	account["setupCode"] = strings.ToLower(code)
	if w := post(account); w.Code != http.StatusOK {
		t.Fatalf("the right setup code was refused: %d %s", w.Code, w.Body.String())
	}
	// A local installation needs no code.
	local, _, _ := testStore(t)
	c := caller{h: newTestServer(local, io.Discard).Handler()}
	c.signup(t)
}

func TestSessionPolicyPostgres(t *testing.T) {
	db, _, _ := testStore(t)
	s := newTestServer(db, io.Discard)
	c := caller{h: s.Handler()}
	c.signup(t)
	check(t, c.call("GET", "/api/settings", nil), 200, nil)

	// Sensitive actions need a password entered in the last few minutes.
	if _, err := db.DB.Exec("UPDATE sessions SET authenticated_at=now()-interval '1 hour'"); err != nil {
		t.Fatal(err)
	}
	var reauth struct{ Reauth bool }
	check(t, c.call("PUT", "/api/settings", object{"provider": object{"kind": "demo", "model": "synthetic", "endpoint": "", "structured": "auto"}}), 403, &reauth)
	if !reauth.Reauth {
		t.Fatal("the refusal must ask for the password again")
	}
	check(t, c.call("POST", "/api/reauth", object{"password": "not it"}), 401, nil)
	check(t, c.call("POST", "/api/reauth", object{"password": "A synthetic owner password!"}), 200, nil)
	check(t, c.call("PUT", "/api/settings", object{"provider": object{"kind": "demo", "model": "synthetic", "endpoint": "", "structured": "auto"}}), 200, nil)

	// An idle session ends.
	if _, err := db.DB.Exec("UPDATE sessions SET last_seen=now()-interval '2 hours'"); err != nil {
		t.Fatal(err)
	}
	check(t, c.call("GET", "/api/settings", nil), 401, nil)

	// Sign out everywhere ends every session.
	first := caller{h: c.h}
	first.login(t)
	second := caller{h: c.h}
	second.login(t)
	check(t, first.call("POST", "/api/logout-everywhere", object{}), 200, nil)
	check(t, second.call("GET", "/api/settings", nil), 401, nil)
	check(t, first.call("GET", "/api/settings", nil), 401, nil)
}

func (c *caller) login(t *testing.T) {
	t.Helper()
	w := c.call("POST", "/api/login", object{"username": "synthetic-owner", "password": "A synthetic owner password!"})
	var s struct{ CSRF string }
	check(t, w, 200, &s)
	c.csrf, c.cookie = s.CSRF, w.Result().Cookies()[0]
}
