package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
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
		for _, q := range []string{"DELETE FROM sessions", "DELETE FROM brief_access", "DELETE FROM login_budget", "INSERT INTO audit_events(owner_id,event) VALUES(1,'owner_recovered')"} {
			if _, err = tx.ExecContext(ctx, q); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) Rotate(ctx context.Context, key []byte) error {
	if len(key) != 32 {
		return errors.New("a new 256-bit key is required")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	next, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return err
	}
	reseal := func(payload, aad []byte) ([]byte, error) {
		plain, err := s.box.Open(nil, nil, payload, aad)
		if err != nil {
			return nil, errors.New("failed to verify an encrypted record")
		}
		return next.Seal(nil, nil, plain, aad), nil
	}
	return s.maintenance(ctx, func(tx *sql.Tx) error {
		// Keyset pages of one keep memory bounded even for large source documents
		// and avoid parallel statements against an open cursor on one connection.
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
		if _, err := tx.ExecContext(ctx, "UPDATE receipt_key SET wrapped=$1 WHERE id=1", next.Seal(nil, nil, s.receiptKey, []byte("submission-key"))); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE installation_key SET verifier=$1 WHERE id=1", next.Seal(nil, nil, []byte("Clinical History installation key check"), []byte("key-check"))); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) verifyMaintenanceKey(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) error {
	var verifier []byte
	if err := q.QueryRowContext(ctx, "SELECT verifier FROM installation_key WHERE id=1").Scan(&verifier); err != nil {
		return err
	}
	if _, err := s.box.Open(nil, nil, verifier, []byte("key-check")); err != nil {
		return errors.New("installation key changed; reopen the installation")
	}
	return nil
}
