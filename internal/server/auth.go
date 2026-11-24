package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ctlplne/healthctl/internal/clinical"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "clinical_session"

type credentials struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	SetupCode string `json:"setupCode,omitempty"`
}

func csrf(raw string) string {
	h := sha256.Sum256([]byte("csrf-v1:" + raw))
	return hex.EncodeToString(h[:])
}
func (s *Server) identity(r *http.Request) (int, string, error) {
	owner, raw, _, err := s.identityAt(r)
	return owner, raw, err
}

// identityAt also returns when the password was last entered. A session idle
// for longer than the idle limit has ended.
func (s *Server) identityAt(r *http.Request) (int, string, time.Time, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || len(c.Value) != 64 {
		return 0, "", time.Time{}, nil
	}
	digest := sha256.Sum256([]byte(c.Value))
	var owner int
	var authenticated time.Time
	err = s.Store.DB.QueryRowContext(r.Context(), `UPDATE sessions SET last_seen=now() WHERE digest=$1 AND expires_at>now() AND last_seen>now()-make_interval(secs => $2)
		RETURNING owner_id, authenticated_at`, digest[:], idleLimit().Seconds()).Scan(&owner, &authenticated)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", time.Time{}, nil
	}
	return owner, c.Value, authenticated, err
}
func (s *Server) session(w http.ResponseWriter, r *http.Request, _ int) error {
	owner, raw, err := s.identity(r)
	if err != nil {
		return err
	}
	var setup bool
	if err = s.Store.DB.QueryRowContext(r.Context(), "SELECT NOT EXISTS(SELECT 1 FROM owners)").Scan(&setup); err != nil {
		return err
	}
	s.emitSession(w, setup, owner, raw)
	return nil
}
func (s *Server) emitSession(w http.ResponseWriter, setup bool, owner int, raw string) {
	t := ""
	if owner != 0 {
		t = csrf(raw)
	}
	emit(w, 200, object{"setupRequired": setup, "setupCodeRequired": setup && s.setupCodeRequired(), "authenticated": owner != 0, "csrf": t, "displayName": s.Name, "notice": clinical.Notice, "noticeVersion": clinical.NoticeVersion, "assessmentNotice": clinical.AssessmentNotice, "modelGuidance": clinical.ModelGuidance(), "emergency": s.Emergency})
}
func (s *Server) setCookie(w http.ResponseWriter, raw string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: raw, Path: "/", MaxAge: maxAge, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.Secure})
}
func (s *Server) setup(w http.ResponseWriter, r *http.Request, _ int) error {
	var in credentials
	if err := decode(w, r, &in); err != nil {
		return err
	}
	in.Username = strings.TrimSpace(in.Username)
	if utf8.RuneCountInString(in.Username) < 2 || len(in.Username) > 80 || utf8.RuneCountInString(in.Password) < 12 || len(in.Password) > 72 {
		return bad("Use a username of 2–80 characters and a password of at least 12 characters (maximum 72 bytes).")
	}
	if err := s.throttle(w, r); err != nil {
		return err
	}
	if !s.checkSetupCode(in.SetupCode) {
		return problem{403, "Enter the one-time setup code from the server log to create the owner account."}
	}
	tx, err := s.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), "SELECT pg_advisory_xact_lock(781341903)"); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM owners)").Scan(&exists); err != nil {
		return err
	}
	if exists {
		return problem{409, "The owner account already exists. Sign in."}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO owners(id,username,password_hash) VALUES(1,$1,$2)", in.Username, hash); err != nil {
		return err
	}
	client, _ := clientKey(r)
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM login_attempts WHERE client=$1", client); err != nil {
		return err
	}
	return s.finishLogin(w, r, tx, 1)
}
func (s *Server) finishLogin(w http.ResponseWriter, r *http.Request, tx *sql.Tx, owner int) error {
	raw := token(32)
	digest := sha256.Sum256([]byte(raw))
	if _, err := tx.ExecContext(r.Context(), "DELETE FROM sessions WHERE expires_at<=now()"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(r.Context(), "INSERT INTO sessions VALUES($1,$2,now()+interval '12 hours')", digest[:], owner); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.setCookie(w, raw, int(SessionLife.Seconds()))
	s.emitSession(w, false, owner, raw)
	return nil
}
func (s *Server) login(w http.ResponseWriter, r *http.Request, _ int) error {
	var in credentials
	if err := decode(w, r, &in); err != nil {
		return err
	}
	// Attempts are counted per client: a remote client is locked out after ten
	// failures, while this machine is only slowed down, so the owner can always
	// sign in locally.
	if err := s.throttle(w, r); err != nil {
		return err
	}
	var owner int
	var err error
	var name string
	var hash []byte
	err = s.Store.DB.QueryRowContext(r.Context(), "SELECT id,username,password_hash FROM owners WHERE id=1").Scan(&owner, &name, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if owner == 0 || bcrypt.CompareHashAndPassword(hash, []byte(in.Password)) != nil || strings.TrimSpace(in.Username) != name {
		return problem{401, "The username or password did not match."}
	}
	tx, err := s.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM login_budget WHERE id=1"); err != nil {
		return err
	}
	client, _ := clientKey(r)
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM login_attempts WHERE client=$1", client); err != nil {
		return err
	}
	return s.finishLogin(w, r, tx, owner)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request, owner int) error {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(c.Value))
	if _, err = s.Store.DB.ExecContext(r.Context(), "DELETE FROM sessions WHERE owner_id=$1 AND digest=$2", owner, digest[:]); err != nil {
		return err
	}
	s.setCookie(w, "", -1)
	s.emitSession(w, false, 0, "")
	return nil
}
