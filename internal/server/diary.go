package server

import (
	"net/http"

	"github.com/ctlplne/healthctl/internal/clinical"
)

func (s *Server) diary(w http.ResponseWriter, r *http.Request, owner int) error {
	switch r.Method {
	case "GET":
		entries, err := s.Store.Diary(r.Context(), owner)
		if err != nil {
			return err
		}
		emit(w, 200, entries)
	case "DELETE":
		_, err := s.Store.DB.ExecContext(r.Context(), "DELETE FROM diary_entries WHERE owner_id=$1 AND id=$2", owner, r.PathValue("id"))
		if err != nil {
			return err
		}
		emit(w, 200, object{"deleted": true})
	case "POST":
		var d clinical.DiaryEntry
		if err := decode(w, r, &d); err != nil {
			return err
		}
		if !requestID.MatchString(d.ID) || !d.Valid() {
			return bad("Check the episode times and describe what happened.")
		}
		saved, err := s.Store.SaveDiary(r.Context(), owner, d)
		if err != nil {
			return err
		}
		emit(w, 200, saved)
	}
	return nil
}
