package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const maintenanceLock = 781341904

// HoldRuntime keeps an exclusive rotation from overlapping a live process. The
// connection must be closed only after the HTTP server has finished its work.
func (s *Store) HoldRuntime(ctx context.Context) (*sql.Conn, error) {
	c, err := s.DB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var ok bool
	err = c.QueryRowContext(ctx, "SELECT pg_try_advisory_lock_shared($1)", maintenanceLock).Scan(&ok)
	if err != nil || !ok {
		c.Close()
		return nil, errors.New("maintenance in progress")
	}
	if err = s.verifyMaintenanceKey(ctx, c); err != nil {
		c.ExecContext(ctx, "SELECT pg_advisory_unlock_shared($1)", maintenanceLock)
		c.Close()
		return nil, err
	}
	return c, nil
}
func (s *Store) maintenance(ctx context.Context, work func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var ok bool
	if err = tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock($1)", maintenanceLock).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return errors.New("stop the application before maintenance")
	}
	if err = s.verifyMaintenanceKey(ctx, tx); err != nil {
		return err
	}
	if err = work(tx); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) RecoverOwner(ctx context.Context, password []byte) error {
	if len(password) < 12 || len(password) > 72 {
		return errors.New("use 12–72 bytes for the new password")
	}
	hash, err := bcrypt.GenerateFromPassword(password, 12)
	if err != nil {
		return err
	}
	return s.maintenance(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE owners SET password_hash=$1 WHERE id=1", hash)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("no owner is registered")
		}
		for _, q := range []string{"DELETE FROM sessions", "DELETE FROM brief_access", "DELETE FROM chatgpt_vault", "DELETE FROM login_budget", "DELETE FROM login_attempts", "INSERT INTO audit_events(owner_id,event) VALUES(1,'owner_recovered')"} {
			if _, err = tx.ExecContext(ctx, q); err != nil {
				return err
			}
		}
		return nil
	})
}

// Rotate replaces the installation key and issues a fresh data key. Every
// record is re-encrypted under the new data key, and the data key and the
// receipt key are wrapped under the new installation key, in one transaction.
// Re-wrapping alone would leave the records readable with the old key: on an
// installation created before the key hierarchy the data key is the original
// installation key, and an old backup holds the data key wrapped under the
// old installation key.
func (s *Store) Rotate(ctx context.Context, key []byte) error {
	if len(key) != 32 {
		return errors.New("a new 256-bit key is required")
	}
	next, err := aead(key)
	if err != nil {
		return err
	}
	dek := make([]byte, 32)
	if _, err = rand.Read(dek); err != nil {
		return err
	}
	box, err := aead(dek)
	if err != nil {
		return err
	}
	return s.maintenance(ctx, func(tx *sql.Tx) error {
		var wrapped []byte
		if err := tx.QueryRowContext(ctx, "SELECT wrapped FROM data_key WHERE id=1 FOR UPDATE").Scan(&wrapped); err != nil {
			return err
		}
		current, err := s.kek.Open(nil, nil, wrapped, []byte("data-key"))
		if err != nil {
			return errors.New("failed to verify the data key")
		}
		old, err := aead(current)
		if err != nil {
			return err
		}
		reseal := func(payload, aad []byte) ([]byte, error) {
			plain, err := old.Open(nil, nil, payload, aad)
			if err != nil {
				return nil, errors.New("failed to verify an encrypted record")
			}
			return box.Seal(nil, nil, plain, aad), nil
		}
		if err := resealRecords(ctx, tx, reseal); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE data_key SET wrapped=$1, legacy=false WHERE id=1", next.Seal(nil, nil, dek, []byte("data-key"))); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE receipt_key SET wrapped=$1 WHERE id=1", next.Seal(nil, nil, s.receiptKey, []byte("submission-key"))); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE installation_key SET verifier=$1 WHERE id=1", next.Seal(nil, nil, []byte("Clinical History installation key check"), []byte("key-check"))); err != nil {
			return err
		}
		return nil
	})
}

// resealRecords re-encrypts every diary entry, encounter, uploaded original
// and the subscription credential vault. Keyset pages of one row keep memory
// bounded even for large source documents and avoid parallel statements
// against an open cursor on one connection.
func resealRecords(ctx context.Context, tx *sql.Tx, reseal func(payload, aad []byte) ([]byte, error)) error {
	var lastOwner int
	lastEntry := ""
	for {
		var owner int
		var id string
		var payload []byte
		err := tx.QueryRowContext(ctx, "SELECT owner_id,id,payload FROM diary_entries WHERE (owner_id,id)>($1,$2) ORDER BY owner_id,id LIMIT 1 FOR UPDATE", lastOwner, lastEntry).Scan(&owner, &id, &payload)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		raw, err := reseal(payload, diaryAAD(owner, id))
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE diary_entries SET payload=$3 WHERE owner_id=$1 AND id=$2", owner, id, raw); err != nil {
			return err
		}
		lastOwner, lastEntry = owner, id
	}
	last := ""
	for {
		var id string
		var owner int
		var raw []byte
		err := tx.QueryRowContext(ctx, "SELECT id,owner_id,payload FROM encounters WHERE id>$1 ORDER BY id LIMIT 1 FOR UPDATE", last).Scan(&id, &owner, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		raw, err = reseal(raw, aad(owner, id))
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE encounters SET payload=$1 WHERE id=$2", raw, id); err != nil {
			return err
		}
		lastDoc := ""
		for {
			var doc string
			err = tx.QueryRowContext(ctx, "SELECT document_id,original FROM encounter_documents WHERE encounter_id=$1 AND document_id>$2 ORDER BY document_id LIMIT 1 FOR UPDATE", id, lastDoc).Scan(&doc, &raw)
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			if err != nil {
				return err
			}
			raw, err = reseal(raw, documentAAD(owner, id, doc))
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, "UPDATE encounter_documents SET original=$1 WHERE encounter_id=$2 AND document_id=$3", raw, id, doc); err != nil {
				return err
			}
			lastDoc = doc
		}
		last = id
	}
	// The installation has one owner. Its live subscription credentials are
	// sealed under the data key too.
	var host string
	var credentials []byte
	err := tx.QueryRowContext(ctx, "SELECT host_id, accounts FROM chatgpt_vault WHERE owner_id=1 AND accounts IS NOT NULL FOR UPDATE").Scan(&host, &credentials)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if credentials, err = reseal(credentials, planAAD(1, host)); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE chatgpt_vault SET accounts=$1 WHERE owner_id=1", credentials)
	return err
}

func (s *Store) verifyMaintenanceKey(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) error {
	var verifier []byte
	if err := q.QueryRowContext(ctx, "SELECT verifier FROM installation_key WHERE id=1").Scan(&verifier); err != nil {
		return err
	}
	if _, err := s.kek.Open(nil, nil, verifier, []byte("key-check")); err != nil {
		return errors.New("installation key changed; reopen the installation")
	}
	return nil
}
