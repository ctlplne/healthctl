package server

import (
	"context"
	"encoding/base64"
	"io"
	"testing"

	"github.com/ctlplne/healthctl/internal/store"
)

// The same vector is checked by tests/test_recovery_kit.py.
func TestRecoveryKitFingerprint(t *testing.T) {
	key, _ := base64.StdEncoding.DecodeString("AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	if got := store.FingerprintForTest(key); got != "5bde-af0d-59df" {
		t.Fatalf("fingerprint %s differs from the recovery kit script", got)
	}
}

func TestInstallationSafetyPostgres(t *testing.T) {
	db, _, _ := testStore(t)
	s := newTestServer(db, io.Discard)
	c := caller{h: s.Handler()}
	check(t, c.call("GET", "/api/installation/safety", nil), 401, nil)
	c.signup(t)
	var safety struct {
		RecoveryKit struct {
			Confirmed, NeedsRecheck bool
			Command                 string
		}
		Backup struct {
			Recorded        bool
			LastSuccess     *string
			LastError       string
			DestinationKind string
		}
	}
	check(t, c.call("GET", "/api/installation/safety", nil), 200, &safety)
	if safety.RecoveryKit.Confirmed || safety.Backup.Recorded || safety.RecoveryKit.Command == "" {
		t.Fatalf("a new installation must ask for a recovery kit and show no backup: %+v", safety)
	}
	check(t, c.call("POST", "/api/installation/recovery", object{"fingerprint": "0000-0000-0000"}), 400, nil)
	check(t, c.call("POST", "/api/installation/recovery", object{"fingerprint": db.RecoveryFingerprint()}), 200, &safety)
	if !safety.RecoveryKit.Confirmed {
		t.Fatal("the matching fingerprint was not accepted")
	}
	if _, err := db.DB.ExecContext(context.Background(), "INSERT INTO backup_status(id,last_attempt,last_success,last_error,destination_kind) VALUES(1,now(),now(),'','host')"); err != nil {
		t.Fatal(err)
	}
	check(t, c.call("GET", "/api/installation/safety", nil), 200, &safety)
	if !safety.Backup.Recorded || safety.Backup.LastSuccess == nil || safety.Backup.DestinationKind != "host" {
		t.Fatalf("backup status was not shown: %+v", safety.Backup)
	}
}
