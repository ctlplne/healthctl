// Package store owns PostgreSQL migrations, authenticated record ownership and encryption.
package store

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/ctlplne/healthctl/internal/clinical"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrConflict = errors.New("record changed or request identifier was reused")
var ErrProviderChanged = errors.New("provider settings changed")

type Store struct {
	DB         *sql.DB
	box        cipher.AEAD
	key        []byte
	receiptKey []byte
}

func Open(ctx context.Context, dsn string, key []byte) (*Store, error) {
	block, err := aes.NewCipher(key)
	if err != nil || len(key) != 32 {
		return nil, errors.New("expected a 256-bit installation key")
	}
	box, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(12)
	s := &Store{DB: db, box: box, key: key}
	if err = s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(781341902)"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, installed_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	for _, name := range files {
		var exists bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)", name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		content, err := migrations.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(name) VALUES ($1)", name); err != nil {
			return err
		}
	}
	const check = "Clinical History installation key check"
	sealed := s.box.Seal(nil, nil, []byte(check), []byte("key-check"))
	if _, err = tx.ExecContext(ctx, "INSERT INTO installation_key(id, verifier) VALUES(1,$1) ON CONFLICT DO NOTHING", sealed); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, "SELECT verifier FROM installation_key WHERE id=1").Scan(&sealed); err != nil {
		return err
	}
	plain, err := s.box.Open(nil, nil, sealed, []byte("key-check"))
	if err != nil || string(plain) != check {
		return errors.New("installation data key mismatch")
	}
	wrapped := s.box.Seal(nil, nil, s.key, []byte("submission-key"))
	if _, err = tx.ExecContext(ctx, "INSERT INTO receipt_key VALUES(1,$1) ON CONFLICT DO NOTHING", wrapped); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, "SELECT wrapped FROM receipt_key WHERE id=1").Scan(&wrapped); err != nil {
		return err
	}
	if s.receiptKey, err = s.box.Open(nil, nil, wrapped, []byte("submission-key")); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Digest(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	m := hmac.New(sha256.New, s.receiptKey)
	m.Write([]byte("submission\x00"))
	m.Write(b)
	return m.Sum(nil), nil
}
func aad(owner int, id string) []byte { return []byte(fmt.Sprintf("encounter/v1/%d/%s", owner, id)) }
func (s *Store) encode(owner int, e *clinical.Encounter) ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return s.box.Seal(nil, nil, b, aad(owner, e.ID)), nil
}
func (s *Store) decode(owner int, id string, b []byte) (*clinical.Encounter, error) {
	plain, err := s.box.Open(nil, nil, b, aad(owner, id))
	if err != nil {
		return nil, errors.New("record decryption failed")
	}
	var e clinical.Encounter
	if err = json.Unmarshal(plain, &e); err != nil {
		return nil, err
	}
	if e.ID != id {
		return nil, errors.New("record identity mismatch")
	}
	return &e, nil
}
func (s *Store) Get(ctx context.Context, owner int, id string) (*clinical.Encounter, error) {
	var b []byte
	err := s.DB.QueryRowContext(ctx, "SELECT payload FROM encounters WHERE id=$1 AND owner_id=$2", id, owner).Scan(&b)
	if err != nil {
		return nil, err
	}
	return s.decode(owner, id, b)
}

type Entry struct {
	Pending         string               `json:"pending,omitempty"`
	ID              string               `json:"id"`
	Concern         string               `json:"concern"`
	State           string               `json:"state"`
	Revision        int                  `json:"revision"`
	HistoryRevision int                  `json:"historyRevision"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
	Provider        clinical.ProviderRef `json:"provider"`
	Demo            bool                 `json:"demo"`
}

func (s *Store) List(ctx context.Context, owner int) ([]Entry, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,payload FROM encounters WHERE owner_id=$1 ORDER BY updated_at DESC,id", owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var id string
		var b []byte
		if err = rows.Scan(&id, &b); err != nil {
			return nil, err
		}
		e, err := s.decode(owner, id, b)
		if err != nil {
			return nil, err
		}
		out = append(out, Entry{e.Pending, e.ID, e.Concern, e.State, e.Revision, e.HistoryRevision, e.CreatedAt, e.UpdatedAt, e.Provider, e.Demo})
	}
	return out, rows.Err()
}

// Create pins the exact disclosed provider in the same transaction as record creation.
// A retry after a settings change still finds the original encounter.
func (s *Store) Create(ctx context.Context, owner int, e *clinical.Encounter, key string, digest []byte, disclosed clinical.ProviderRef) (*clinical.Encounter, bool, error) {
	return s.CreateVisit(ctx, owner, e, key, digest, disclosed, nil, "")
}

var ErrReturnVisit = errors.New("earlier confirmed visit or selected evidence changed")

func (s *Store) CreateVisit(ctx context.Context, owner int, e *clinical.Encounter, key string, digest []byte, disclosed clinical.ProviderRef, v *clinical.ReturnVisit, emergency string) (*clinical.Encounter, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var cfgJSON []byte
	if err = tx.QueryRowContext(ctx, "SELECT provider FROM owners WHERE id=$1 FOR UPDATE", owner).Scan(&cfgJSON); err != nil {
		return nil, false, err
	}
	var existingID string
	var existing, prev []byte
	err = tx.QueryRowContext(ctx, "SELECT id,payload,create_digest FROM encounters WHERE owner_id=$1 AND create_request=$2", owner, key).Scan(&existingID, &existing, &prev)
	if err == nil {
		if !bytes.Equal(prev, digest) {
			return nil, false, ErrConflict
		}
		found, err := s.decode(owner, existingID, existing)
		return found, false, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	var cfg clinical.ProviderRef
	if err = json.Unmarshal(cfgJSON, &cfg); err != nil {
		return nil, false, err
	}
	if cfg != disclosed {
		return nil, false, ErrProviderChanged
	}
	if v != nil && (len(v.DiaryIDs) > 30 || !v.Consent || v.Source == "" && (len(v.DiaryIDs) == 0 || len(v.ReviewIDs)+len(v.Facts)+len(v.Entries)+len(v.Clarifications) > 0)) {
		return nil, false, ErrReturnVisit
	}
	if v != nil && len(v.DiaryIDs) > 0 {
		if err = s.attachDiary(ctx, tx, owner, e, v.DiaryIDs, emergency); err != nil {
			return nil, false, err
		}
	}
	if v != nil && v.Source != "" {
		var sealed []byte
		if err = tx.QueryRowContext(ctx, "SELECT payload FROM encounters WHERE id=$1 AND owner_id=$2 FOR SHARE", v.Source, owner).Scan(&sealed); err != nil {
			return nil, false, err
		}
		prior, err := s.decode(owner, v.Source, sealed)
		if err != nil {
			return nil, false, err
		}
		if err = e.StartReturn(prior, *v, emergency); err != nil {
			return nil, false, ErrReturnVisit
		}
	}
	b, err := s.encode(owner, e)
	if err != nil {
		return nil, false, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO encounters(id,owner_id,revision,payload,create_request,create_digest) VALUES($1,$2,$3,$4,$5,$6)", e.ID, owner, e.Revision, b, key, digest)
	if err != nil {
		return nil, false, err
	}
	for _, doc := range e.Documents {
		if doc.PriorEncounter == "" {
			continue
		}
		var original []byte
		if err = tx.QueryRowContext(ctx, "SELECT original FROM encounter_documents WHERE encounter_id=$1 AND document_id=$2", doc.PriorEncounter, doc.ID).Scan(&original); err != nil {
			return nil, false, err
		}
		plain, err := s.box.Open(nil, nil, original, documentAAD(owner, doc.PriorEncounter, doc.ID))
		if err != nil {
			return nil, false, err
		}
		sealed := s.box.Seal(nil, nil, plain, documentAAD(owner, e.ID, doc.ID))
		clear(plain)
		if _, err = tx.ExecContext(ctx, "INSERT INTO encounter_documents(encounter_id,document_id,original) VALUES($1,$2,$3)", e.ID, doc.ID, sealed); err != nil {
			return nil, false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	return e, true, nil
}

// Apply persists the patient's mutation and its receipt before inference starts.
func (s *Store) Apply(ctx context.Context, owner int, id string, revision int, key string, digest []byte, mutate func(*clinical.Encounter) error) (*clinical.Encounter, bool, error) {
	return s.apply(ctx, owner, id, "", revision, key, digest, func(_ *sql.Tx, e, _ *clinical.Encounter) error { return mutate(e) })
}

// ApplyRelated reads both sides of a reconciliation under the same ownership
// and revision transaction. Lock order also permits simultaneous opposite joins.
func (s *Store) ApplyRelated(ctx context.Context, owner int, id, source string, revision int, key string, digest []byte, mutate func(*clinical.Encounter, *clinical.Encounter) error) (*clinical.Encounter, bool, error) {
	return s.apply(ctx, owner, id, source, revision, key, digest, func(_ *sql.Tx, e, prior *clinical.Encounter) error { return mutate(e, prior) })
}
func (s *Store) apply(ctx context.Context, owner int, id, source string, revision int, key string, digest []byte, mutate func(*sql.Tx, *clinical.Encounter, *clinical.Encounter) error) (*clinical.Encounter, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT id,payload FROM encounters WHERE owner_id=$1 AND (id=$2 OR id=NULLIF($3,'')) ORDER BY id FOR UPDATE", owner, id, source)
	if err != nil {
		return nil, false, err
	}
	var b, sourcePayload []byte
	for rows.Next() {
		var found string
		var sealed []byte
		if err = rows.Scan(&found, &sealed); err != nil {
			rows.Close()
			return nil, false, err
		}
		if found == id {
			b = sealed
		} else {
			sourcePayload = sealed
		}
	}
	if err = rows.Close(); err != nil {
		return nil, false, err
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	if b == nil {
		return nil, false, sql.ErrNoRows
	}
	e, err := s.decode(owner, id, b)
	if err != nil {
		return nil, false, err
	}
	var prev []byte
	err = tx.QueryRowContext(ctx, "SELECT digest FROM submissions WHERE encounter_id=$1 AND request_id=$2", id, key).Scan(&prev)
	if err == nil {
		if !bytes.Equal(prev, digest) {
			return nil, false, ErrConflict
		}
		return e, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	if e.Revision != revision {
		return nil, false, ErrConflict
	}
	var prior *clinical.Encounter
	if source != "" {
		if sourcePayload == nil {
			return nil, false, sql.ErrNoRows
		}
		prior, err = s.decode(owner, source, sourcePayload)
		if err != nil {
			return nil, false, err
		}
	}
	if err = mutate(tx, e, prior); err != nil {
		return nil, false, err
	}
	b, err = s.encode(owner, e)
	if err != nil {
		return nil, false, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE encounters SET payload=$1,revision=$2,updated_at=now() WHERE owner_id=$3 AND id=$4", b, e.Revision, owner, id); err != nil {
		return nil, false, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO submissions VALUES($1,$2,$3)", id, key, digest); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	return e, true, nil
}

func (s *Store) Finish(ctx context.Context, owner int, e *clinical.Encounter, expected int) error {
	b, err := s.encode(owner, e)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, "UPDATE encounters SET payload=$1,revision=$2,updated_at=now() WHERE owner_id=$3 AND id=$4 AND revision=$5", b, e.Revision, owner, e.ID, expected)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return nil
}
func (s *Store) Delete(ctx context.Context, owner int, id string, expected int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM encounters WHERE id=$1 AND owner_id=$2 FOR UPDATE", id, owner).Scan(&version); err != nil {
		return err
	}
	if version != expected {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM encounters WHERE id=$1 AND owner_id=$2", id, owner); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO audit_events(owner_id,event) VALUES($1,'encounter_deleted')", owner); err != nil {
		return err
	}
	return tx.Commit()
}

// RecoverPending runs on startup of the single application instance. Reads never
// dispatch a model call. Recovery enables an explicit retry following a power loss.
func (s *Store) RecoverPending(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, "SELECT owner_id,id,payload FROM encounters")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var owner int
		var id string
		var b []byte
		if err := rows.Scan(&owner, &id, &b); err != nil {
			return err
		}
		e, err := s.decode(owner, id, b)
		if err != nil {
			return err
		}
		if task := e.PendingTask(); task != "" {
			rev := e.Revision
			e.Fail(task)
			if err = s.Finish(ctx, owner, e, rev); err != nil && !errors.Is(err, ErrConflict) {
				return err
			}
		}
	}
	return rows.Err()
}

const recoveryLabel = "healthctl-recovery-kit-v1"

// RecoveryFingerprint identifies the installation key that a recovery kit must
// hold. It is derived one way from the key and changes when the key rotates.
// scripts/recovery_kit.py computes the same value.
func (s *Store) RecoveryFingerprint() string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(recoveryLabel))
	h := fmt.Sprintf("%x", m.Sum(nil))[:12]
	return h[0:4] + "-" + h[4:8] + "-" + h[8:12]
}

// FingerprintForTest computes the recovery fingerprint for a raw key.
func FingerprintForTest(key []byte) string { return (&Store{key: key}).RecoveryFingerprint() }
