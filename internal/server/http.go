// Package server serves a same-origin clinical workspace and its authenticated HTTP API.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/provider"
	"github.com/ctlplne/healthctl/internal/store"
)

type Server struct {
	Lifetime                              context.Context
	Store                                 *store.Store
	Models                                *provider.Client
	ChatGPT                               *ChatGPTIntegration
	EHR                                   *EHRIntegration
	Name, Emergency, Assets, PublicOrigin string
	Locale, Units, CareDestination        string
	Secure                                bool
	Logger                                *log.Logger

	jobs        *jobRunner
	jobsInit    sync.Once
	setup1      setupCode
	loginDelay  func(attempts int) time.Duration // a test hook for the local slow-down
	loginGate   loginGate
	beforeModel func(*clinical.Encounter) // a test hook that runs before each queued model call
}
type object = map[string]any
type problem struct {
	code    int
	message string
}

func (p problem) Error() string { return p.message }
func bad(message string) error  { return problem{400, message} }
func token(n int) string        { b := make([]byte, n); rand.Read(b); return hex.EncodeToString(b) }
func emit(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if ct := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])); ct != "application/json" {
		return problem{415, "Submit a JSON request."}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 48<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return bad("The request is too large or contains invalid fields.")
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return bad("Submit one JSON object.")
	}
	return nil
}
func (s *Server) respondErr(w http.ResponseWriter, err error) {
	var p problem
	switch {
	case errors.As(err, &p):
		emit(w, p.code, object{"error": p.message})
	case errors.Is(err, errReauth):
		emit(w, 403, object{"error": "Enter your password again to continue.", "reauth": true})
	case errors.Is(err, sql.ErrNoRows):
		emit(w, 404, object{"error": "This record could not be found."})
	case errors.Is(err, store.ErrConflict):
		emit(w, 409, object{"error": "The record has changed. Reload it before trying again."})
	case errors.Is(err, store.ErrReturnVisit):
		emit(w, 409, object{"error": "Review the current prior visit and selected findings before starting again."})
	case errors.Is(err, store.ErrProviderChanged):
		emit(w, 409, object{"error": "The default provider changed. Review its destination before starting."})
	default:
		id := token(8)
		s.Logger.Printf("event=internal_failure id=%s", id)
		emit(w, 500, object{"error": "The operation could not be completed.", "eventId": id})
	}
}

type apiHandler func(http.ResponseWriter, *http.Request, int) error

func (s *Server) route(m *http.ServeMux, pattern string, protected bool, fn apiHandler) {
	m.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		var owner int
		if protected {
			var raw string
			var err error
			var authenticated time.Time
			owner, raw, authenticated, err = s.identityAt(r)
			r = r.WithContext(context.WithValue(r.Context(), authKey{}, authenticated))
			if err != nil {
				s.respondErr(w, err)
				return
			}
			if owner == 0 {
				emit(w, 401, object{"error": "Sign in to access your workspace."})
				return
			}
			if r.Method != "GET" && r.Method != "HEAD" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrf(raw))) != 1 {
				emit(w, 403, object{"error": "Refresh the page and try again."})
				return
			}
		}
		if err := fn(w, r, owner); err != nil {
			s.respondErr(w, err)
		}
	})
}
func (s *Server) Handler() http.Handler {
	// A running server resumes queued and interrupted model calls at start.
	if s.Lifetime != nil && s.Store != nil {
		s.startWorkers()
	}
	m := http.NewServeMux()
	s.route(m, "GET /api/session", false, s.session)
	s.route(m, "POST /api/setup", false, s.setup)
	s.route(m, "POST /api/login", false, s.login)
	s.route(m, "POST /api/logout", true, s.logout)
	s.route(m, "POST /api/logout-everywhere", true, s.logoutEverywhere)
	s.route(m, "POST /api/reauth", true, s.reauth)
	s.route(m, "GET /api/settings", true, s.settings)
	s.route(m, "PUT /api/settings", true, s.sensitive(s.updateSettings))
	s.route(m, "POST /api/settings/test", true, s.testSettings)
	s.route(m, "GET /api/encounters", true, s.list)
	s.route(m, "POST /api/encounters", true, s.create)
	s.route(m, "GET /api/encounters/{id}", true, s.get)
	s.route(m, "DELETE /api/encounters/{id}", true, s.sensitive(s.remove))
	s.route(m, "POST /api/encounters/{id}/actions", true, s.action)
	s.route(m, "GET /api/encounters/{id}/export", true, s.sensitive(s.export))
	s.route(m, "GET /api/encounters/{id}/report", true, s.report)
	s.route(m, "GET /api/encounters/{id}/usage", true, s.usage)
	s.route(m, "GET /api/voice", true, s.voice)
	s.route(m, "POST /api/voice", true, limited(voiceLimit, "recordings", s.voice))
	s.route(m, "GET /api/diary", true, s.diary)
	s.route(m, "POST /api/diary", true, s.diary)
	s.route(m, "DELETE /api/diary/{id}", true, s.sensitive(s.diary))
	s.documentRoutes(m)
	s.ehrRoutes(m)
	s.chatGPTRoutes(m)
	s.interchangeRoutes(m)
	s.evidenceRoutes(m)
	s.installationRoutes(m)
	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { emit(w, 404, object{"error": "Unknown API operation."}) })
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); io.WriteString(w, "ok\n") })
	m.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := s.Store.DB.PingContext(ctx); err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		io.WriteString(w, "ready\n")
	})

	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		path := filepath.Join(s.Assets, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if r.URL.Path == "/" {
			path = filepath.Join(s.Assets, "index.html")
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			// A page route such as /encounters/<id> is the app itself, so a
			// link or a reload opens the same screen. Files and the API still 404.
			if !strings.HasPrefix(r.URL.Path, "/api/") && !strings.Contains(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], ".") && appRoute(r.URL.Path) {
				http.ServeFile(w, r, filepath.Join(s.Assets, "index.html"))
				return
			}
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, path)
	})
	cross := http.NewCrossOriginProtection().Handler(m)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(self), geolocation=()")
		if s.Secure {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		trusted := host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
		if s.PublicOrigin != "" {
			u, err := url.Parse(s.PublicOrigin)
			trusted = trusted || (err == nil && strings.EqualFold(u.Host, r.Host))
		}
		if !trusted {
			emit(w, 403, object{"error": "Unrecognized application hostname."})
			return
		}
		cross.ServeHTTP(w, r)
	})
}

const SessionLife = 12 * time.Hour

// appRoute reports whether a path is one of the app's own screens.
func appRoute(p string) bool {
	switch p {
	case "/settings", "/new", "/timeline", "/library":
		return true
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	return parts[0] == "encounters" && (len(parts) == 2 || len(parts) == 3) && encounterRouteID.MatchString(parts[1])
}

var encounterRouteID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// contentSecurityPolicy allows only the app's own resources and, with Trusted
// Types, refuses script-built HTML: the app never writes markup from strings.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'; require-trusted-types-for 'script'; trusted-types 'none'"
