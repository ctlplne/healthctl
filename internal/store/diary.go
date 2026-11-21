package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ctlplne/healthctl/internal/clinical"
)

func diaryAAD(owner int, id string) []byte {
	return []byte(fmt.Sprintf("healthctl/diary/%d/%s", owner, id))
}
func (s *Store) readDiary(owner int, id string, raw []byte) (clinical.DiaryEntry, error) {
	var item clinical.DiaryEntry
	b, err := s.box.Open(nil, nil, raw, diaryAAD(owner, id))
	if err == nil {
		err = json.Unmarshal(b, &item)
	}
	return item, err
}
func (s *Store) SaveDiary(ctx context.Context, owner int, item clinical.DiaryEntry) (clinical.DiaryEntry, error) {
	item.Recorded = time.Time{}
	digest, err := s.Digest(item)
	if err != nil {
		return item, err
	}
	item.Recorded = time.Now().UTC()
	b, err := json.Marshal(item)
	if err != nil {
		return item, err
	}
	raw := s.box.Seal(nil, nil, b, diaryAAD(owner, item.ID))
	var stored, prior []byte
	err = s.DB.QueryRowContext(ctx, `INSERT INTO diary_entries(owner_id,id,payload,digest) VALUES($1,$2,$3,$4)
 ON CONFLICT(owner_id,id) DO UPDATE SET id=excluded.id RETURNING payload,digest`, owner, item.ID, raw, digest).Scan(&stored, &prior)
	if err != nil {
		return item, err
	}
	if !bytes.Equal(prior, digest) {
		return item, ErrConflict
	}
	return s.readDiary(owner, item.ID, stored)
}
func (s *Store) Diary(ctx context.Context, owner int) ([]clinical.DiaryEntry, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,payload FROM diary_entries WHERE owner_id=$1 ORDER BY recorded_at DESC,id", owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []clinical.DiaryEntry{}
	for rows.Next() {
		var id string
		var raw []byte
		if err = rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		item, err := s.readDiary(owner, id, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (s *Store) attachDiary(ctx context.Context, tx *sql.Tx, owner int, e *clinical.Encounter, ids []string, emergency string) error {
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return ErrReturnVisit
		}
		seen[id] = true
		var payload []byte
		err := tx.QueryRowContext(ctx, "SELECT payload FROM diary_entries WHERE owner_id=$1 AND id=$2 FOR SHARE", owner, id).Scan(&payload)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrReturnVisit
		}
		if err != nil {
			return err
		}
		item, err := s.readDiary(owner, id, payload)
		if err != nil {
			return err
		}
		if err = e.IncludeDiary(item, emergency); err != nil {
			return ErrReturnVisit
		}
	}
	return nil
}
