package server

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"io"
	"testing"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/store"
)

func TestReceiptKeyIndependentPostgres(t *testing.T) {
	db, dsn, key := isolatedStore(t, true)
	ctx := t.Context()
	value := object{"action": "review"}
	digest, err := db.Digest(value)
	if err != nil {
		t.Fatal(err)
	}
	fromInstallationKey := func(k []byte) []byte {
		m := hmac.New(sha256.New, k)
		m.Write([]byte("submission\x00"))
		b, _ := json.Marshal(value)
		m.Write(b)
		return m.Sum(nil)
	}
	if hmac.Equal(digest, fromInstallationKey(key)) {
		t.Fatal("the receipt key must not be the installation key")
	}

	c := caller{h: newTestServer(db, io.Discard).Handler()}
	c.signup(t)
	demo := clinical.ProviderRef{Kind: "demo", Model: "synthetic", Structured: "auto"}
	selectProvider(t, c, demo)
	var e clinical.Encounter
	check(t, c.call("POST", "/api/encounters", intake(demo, clinical.DemoConcern)), 200, &e)
	payload := func(s *store.Store) []byte {
		var b []byte
		if err := s.DB.QueryRowContext(ctx, "SELECT payload FROM encounters WHERE id=$1", e.ID).Scan(&b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	before := payload(db)

	// Rotation re-wraps the keys and leaves every record as it was.
	next := bytes.Repeat([]byte{0x5a}, 32)
	if err := db.Rotate(ctx, next); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(ctx, dsn, next)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	if !bytes.Equal(before, payload(reopened)) {
		t.Fatal("rotation rewrote a record instead of re-wrapping the keys")
	}
	if got, err := reopened.Get(ctx, 1, e.ID); err != nil || got.ID != e.ID {
		t.Fatalf("the rotated installation can't read its records: %v", err)
	}
	if again, _ := reopened.Digest(value); !hmac.Equal(again, digest) {
		t.Fatal("rotation changed the receipt key")
	}
	if old, err := store.Open(ctx, dsn, key); err == nil {
		old.DB.Close()
		t.Fatal("the old installation key still opens the installation")
	}

	// An older installation used its installation key for receipts. It gets an
	// independent receipt key once.
	block, _ := aes.NewCipher(next)
	kek, _ := cipher.NewGCMWithRandomNonce(block)
	if _, err := reopened.DB.ExecContext(ctx, "UPDATE receipt_key SET wrapped=$1, independent=false WHERE id=1", kek.Seal(nil, nil, next, []byte("submission-key"))); err != nil {
		t.Fatal(err)
	}
	upgraded, err := store.Open(ctx, dsn, next)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.DB.Close()
	replaced, _ := upgraded.Digest(value)
	var independent bool
	upgraded.DB.QueryRowContext(ctx, "SELECT independent FROM receipt_key WHERE id=1").Scan(&independent)
	if !independent || hmac.Equal(replaced, fromInstallationKey(next)) {
		t.Fatal("a receipt key equal to the installation key was not replaced")
	}
}
