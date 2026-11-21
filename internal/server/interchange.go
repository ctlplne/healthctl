package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ctlplne/healthctl/internal/interchange"
)

func (s *Server) interchangeRoutes(m *http.ServeMux) {
	s.route(m, "GET /api/encounters/{id}/brief", true, s.brief)
	s.route(m, "GET /api/encounters/{id}/fhir", true, s.fhir)
	s.route(m, "GET /api/encounters/{id}/access", true, s.access)
	s.route(m, "POST /api/encounters/{id}/access", true, s.access)
	s.route(m, "DELETE /api/encounters/{id}/access", true, s.access)
	s.route(m, "GET /api/integration/brief", false, s.sharedBrief)
}
func (s *Server) brief(w http.ResponseWriter, r *http.Request, owner int) error {
	e, err := s.Store.Get(r.Context(), owner, r.PathValue("id"))
	if err != nil {
		return err
	}
	s.refreshEvidence(r.Context(), owner, e)
	emit(w, 200, interchange.FromEncounter(e))
	return nil
}
func (s *Server) fhir(w http.ResponseWriter, r *http.Request, owner int) error {
	e, err := s.Store.Get(r.Context(), owner, r.PathValue("id"))
	if err != nil {
		return err
	}
	s.refreshEvidence(r.Context(), owner, e)
	w.Header().Set("Content-Type", "application/fhir+json")
	w.Header().Set("Content-Disposition", `attachment; filename="healthctl-r4.json"`)
	return json.NewEncoder(w).Encode(interchange.FHIR(interchange.FromEncounter(e)))
}
func (s *Server) access(w http.ResponseWriter, r *http.Request, owner int) error {
	ctx, id := r.Context(), r.PathValue("id")
	e, err := s.Store.Get(ctx, owner, id)
	if err != nil {
		return err
	}
	switch r.Method {
	case http.MethodGet:
		var expiry time.Time
		err := s.Store.DB.QueryRowContext(ctx, "SELECT expires_at FROM brief_access WHERE encounter_id=$1 AND history_revision=$2 AND expires_at>now()", id, e.Revision).Scan(&expiry)
		if errors.Is(err, sql.ErrNoRows) {
			emit(w, 200, object{"active": false})
			return nil
		}
		if err != nil {
			return err
		}
		emit(w, 200, object{"active": true, "expiresAt": expiry, "revision": e.Revision})
	case http.MethodDelete:
		if _, err = s.Store.DB.ExecContext(ctx, "DELETE FROM brief_access WHERE encounter_id=$1", id); err != nil {
			return err
		}
		emit(w, 200, object{"revoked": true})
	default:
		if e.ConfirmedRevision != e.HistoryRevision {
			return bad("Confirm the current history before granting brief access.")
		}
		raw := token(32)
		digest := sha256.Sum256([]byte(raw))
		expiry := time.Now().UTC().Add(24 * time.Hour)
		res, err := s.Store.DB.ExecContext(ctx, `INSERT INTO brief_access(encounter_id,digest,history_revision,expires_at)
          SELECT id,$2,revision,$3 FROM encounters WHERE id=$1 AND owner_id=$4 AND revision=$5
          ON CONFLICT(encounter_id) DO UPDATE SET digest=excluded.digest,history_revision=excluded.history_revision,expires_at=excluded.expires_at`, id, digest[:], expiry, owner, e.Revision)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return problem{409, "The encounter changed. Review the current brief and try again."}
		}
		emit(w, 200, object{"token": raw, "expiresAt": expiry, "revision": e.Revision})
	}
	return nil
}
func (s *Server) sharedBrief(w http.ResponseWriter, r *http.Request, _ int) error {
	raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	denied := problem{401, "A current, unexpired brief access token is required."}
	if !ok || len(raw) != 64 {
		return denied
	}
	digest := sha256.Sum256([]byte(raw))
	var owner, revision int
	var id string
	err := s.Store.DB.QueryRowContext(r.Context(), `SELECT e.owner_id,e.id,a.history_revision FROM brief_access a JOIN encounters e ON e.id=a.encounter_id WHERE a.digest=$1 AND a.expires_at>now() AND a.history_revision=e.revision`, digest[:]).Scan(&owner, &id, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return denied
	}
	if err != nil {
		return err
	}
	e, err := s.Store.Get(r.Context(), owner, id)
	if err != nil {
		return err
	}
	if e.Revision != revision || e.ConfirmedRevision != e.HistoryRevision {
		return denied
	}
	s.refreshEvidence(r.Context(), owner, e)
	emit(w, 200, interchange.FromEncounter(e))
	return nil
}
