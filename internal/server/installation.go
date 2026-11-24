package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"github.com/ctlplne/healthctl/internal/sandbox"
	"net/http"
	"strings"
	"time"
)

// recoveryRecheck is how long a confirmed recovery kit counts as current before
// the app asks the owner to check it again.
const recoveryRecheck = 180 * 24 * time.Hour

func (s *Server) installationRoutes(m *http.ServeMux) {
	s.route(m, "GET /api/installation/safety", true, s.installationSafety)
	s.route(m, "POST /api/installation/recovery", true, s.confirmRecoveryKit)
}

// installationSafety reports whether the journal can be recovered: the
// recovery kit confirmation and the last backup the backup worker recorded.
func (s *Server) installationSafety(w http.ResponseWriter, r *http.Request, owner int) error {
	ctx := r.Context()
	kit := object{"confirmed": false, "needsRecheck": false, "command": "python3 scripts/recovery_kit.py"}
	var digest []byte
	var confirmed time.Time
	err := s.Store.DB.QueryRowContext(ctx, "SELECT fingerprint_digest, confirmed_at FROM recovery_kit WHERE owner_id=$1", owner).Scan(&digest, &confirmed)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return err
	default:
		current := sha256.Sum256([]byte(s.Store.RecoveryFingerprint()))
		if subtle.ConstantTimeCompare(digest, current[:]) == 1 {
			kit["confirmed"] = true
			kit["confirmedAt"] = confirmed
			kit["needsRecheck"] = time.Since(confirmed) > recoveryRecheck
		} else {
			kit["keyChanged"] = true
		}
	}
	backup := object{"recorded": false}
	var attempt time.Time
	var success sql.NullTime
	var lastError, kind string
	err = s.Store.DB.QueryRowContext(ctx, "SELECT last_attempt, last_success, last_error, destination_kind FROM backup_status WHERE id=1").Scan(&attempt, &success, &lastError, &kind)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return err
	default:
		backup = object{"recorded": true, "lastAttempt": attempt, "lastError": lastError, "destinationKind": kind}
		if success.Valid {
			backup["lastSuccess"] = success.Time
		}
	}
	emit(w, 200, object{"recoveryKit": kit, "backup": backup, "readers": sandbox.Status()})
	return nil
}

// confirmRecoveryKit checks the fingerprint printed on the owner's kit. The key
// never reaches the browser; only the derived fingerprint is compared.
func (s *Server) confirmRecoveryKit(w http.ResponseWriter, r *http.Request, owner int) error {
	var in struct {
		Fingerprint string `json:"fingerprint"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	got := strings.ToLower(strings.TrimSpace(in.Fingerprint))
	want := s.Store.RecoveryFingerprint()
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return bad("That fingerprint doesn't match this installation's key. Make a new kit with python3 scripts/recovery_kit.py and enter its fingerprint.")
	}
	digest := sha256.Sum256([]byte(want))
	if _, err := s.Store.DB.ExecContext(r.Context(), `INSERT INTO recovery_kit(owner_id, fingerprint_digest, confirmed_at) VALUES($1,$2,now())
		ON CONFLICT (owner_id) DO UPDATE SET fingerprint_digest=excluded.fingerprint_digest, confirmed_at=now()`, owner, digest[:]); err != nil {
		return err
	}
	return s.installationSafety(w, r, owner)
}
