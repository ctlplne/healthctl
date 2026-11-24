package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Session policy: a session ends after an idle period, the owner can sign out
// everywhere, and export, sharing, deletion and a provider change need a
// password entered in the last few minutes.
const recentAuth = 10 * time.Minute

func idleLimit() time.Duration {
	if m, err := strconv.Atoi(os.Getenv("CLINICAL_IDLE_MINUTES")); err == nil && m >= 5 && m <= 720 {
		return time.Duration(m) * time.Minute
	}
	return 60 * time.Minute
}

type authKey struct{}

// authenticatedAt is set by the route wrapper for signed-in requests.
func authenticatedAt(ctx context.Context) time.Time {
	t, _ := ctx.Value(authKey{}).(time.Time)
	return t
}

// errReauth asks the user to enter the password again.
var errReauth = errors.New("reauthentication required")

// sensitive wraps a handler that needs a recent password.
func (s *Server) sensitive(fn apiHandler) apiHandler {
	return func(w http.ResponseWriter, r *http.Request, owner int) error {
		if time.Since(authenticatedAt(r.Context())) > recentAuth {
			return errReauth
		}
		return fn(w, r, owner)
	}
}

func (s *Server) reauth(w http.ResponseWriter, r *http.Request, owner int) error {
	var in credentials
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if err := s.throttle(w, r); err != nil {
		return err
	}
	var hash []byte
	if err := s.Store.DB.QueryRowContext(r.Context(), "SELECT password_hash FROM owners WHERE id=$1", owner).Scan(&hash); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(in.Password)) != nil {
		return problem{401, "That password did not match."}
	}
	c, _ := r.Cookie(sessionCookie)
	digest := sha256.Sum256([]byte(c.Value))
	if _, err := s.Store.DB.ExecContext(r.Context(), "UPDATE sessions SET authenticated_at=now() WHERE digest=$1", digest[:]); err != nil {
		return err
	}
	s.clearAttempts(r)
	emit(w, 200, object{"ok": true})
	return nil
}

func (s *Server) logoutEverywhere(w http.ResponseWriter, r *http.Request, owner int) error {
	if _, err := s.Store.DB.ExecContext(r.Context(), "DELETE FROM sessions WHERE owner_id=$1", owner); err != nil {
		return err
	}
	s.setCookie(w, "", -1)
	s.emitSession(w, false, 0, "")
	return nil
}

// clientKey identifies who is signing in, and whether it is this machine.
// Forwarding headers are trusted only from CLINICAL_TRUSTED_PROXY, so each
// client behind a reverse proxy gets its own budget.
func clientKey(r *http.Request) (string, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if proxy := strings.TrimSpace(os.Getenv("CLINICAL_TRUSTED_PROXY")); proxy != "" && host == proxy {
		if parts := strings.Split(r.Header.Get("X-Forwarded-For"), ","); len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) != "" {
			client := strings.TrimSpace(parts[len(parts)-1])
			return "forwarded:" + client, false
		}
	}
	ip := net.ParseIP(host)
	return host, ip != nil && ip.IsLoopback()
}

// throttle counts a sign-in attempt for this client. Another client gets ten
// attempts in ten minutes and then waits; a connection from this machine is
// never locked out, only slowed down, so the owner can always sign in locally.
func (s *Server) throttle(w http.ResponseWriter, r *http.Request) error {
	client, _ := clientKey(r)
	var attempts int
	err := s.Store.DB.QueryRowContext(r.Context(), `INSERT INTO login_attempts VALUES($1,now(),1)
	ON CONFLICT (client) DO UPDATE SET attempts=CASE WHEN login_attempts.window_start<now()-interval '10 minutes' THEN 1 ELSE least(login_attempts.attempts+1,1000) END,
	window_start=CASE WHEN login_attempts.window_start<now()-interval '10 minutes' THEN now() ELSE login_attempts.window_start END RETURNING attempts`, client).Scan(&attempts)
	if err != nil {
		return err
	}
	if attempts <= 10 {
		return nil
	}
	if _, loopback := clientKey(r); loopback {
		delay := s.loginDelay
		if delay == nil {
			delay = func(n int) time.Duration { return time.Duration(min(n-10, 5)) * time.Second }
		}
		select {
		case <-time.After(delay(attempts)):
		case <-r.Context().Done():
		}
		return nil
	}
	w.Header().Set("Retry-After", "600")
	return problem{429, "Too many sign-in attempts. Try again in ten minutes."}
}

func (s *Server) clearAttempts(r *http.Request) {
	client, _ := clientKey(r)
	_, _ = s.Store.DB.ExecContext(r.Context(), "DELETE FROM login_attempts WHERE client=$1", client)
}

// A public installation asks for a one-time setup code before creating the
// owner, so a stranger who finds the address first can't claim it. The code
// is printed to the server log once.
type setupCode struct {
	once sync.Once
	code string
}

func (s *Server) setupCodeRequired() bool { return s.PublicOrigin != "" }

func (s *Server) currentSetupCode() string {
	s.setup1.once.Do(func() {
		s.setup1.code = strings.ToUpper(token(4)[:4] + "-" + token(4)[:4])
		if s.Logger != nil {
			s.Logger.Printf("event=setup_code code=%s note=enter-this-code-to-create-the-owner-account", s.setup1.code)
		}
	})
	return s.setup1.code
}

func (s *Server) checkSetupCode(given string) bool {
	if !s.setupCodeRequired() {
		return true
	}
	want := s.currentSetupCode()
	return subtle.ConstantTimeCompare([]byte(strings.ToUpper(strings.TrimSpace(given))), []byte(want)) == 1
}
