package evidence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

type Library struct{ DB *sql.DB }

var cache struct {
	sync.Mutex
	key     string
	catalog Catalog
}
var ErrConflict = errors.New("that immutable content version already exists with a different manifest")
var ErrInvalid = errors.New("invalid collection operation")
var ErrCapacity = errors.New("installation collection limit reached")

func (l Library) seed(ctx context.Context, owner int) error {
	p := Starter()
	var exists bool
	const check = "SELECT EXISTS (SELECT 1 FROM evidence_versions WHERE owner_id=$1 AND collection=$2 AND version=$3)"
	if err := l.DB.QueryRowContext(ctx, check, owner, p.ID, p.Version).Scan(&exists); err != nil || exists {
		return err
	}
	tx, err := l.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT id FROM owners WHERE id=$1 FOR UPDATE", owner); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, check, owner, p.ID, p.Version).Scan(&exists); err != nil || exists {
		return err
	}
	// Preserve a withdrawal or an explicitly selected rollback. The newer bundled
	// version remains available for the owner to activate in either situation.
	var chosen bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM evidence_events WHERE owner_id=$1 AND collection=$2 AND action IN ('active','withdrawn'))", owner, p.ID).Scan(&chosen); err != nil {
		return err
	}
	status, reason := "active", "Bundled with the application"
	if chosen {
		status, reason = "retired", "Available for activation; the previous owner choice was kept"
	} else if err := retireActive(ctx, tx, owner, p.ID, "New bundled version "+p.Version); err != nil {
		return err
	}
	data, _ := json.Marshal(p)
	if _, err = tx.ExecContext(ctx, `INSERT INTO evidence_versions(owner_id,collection,version,origin,status,reason,manifest,content_hash)
        VALUES($1,$2,$3,'bundled',$4,$5,$6::jsonb,$7)`, owner, p.ID, p.Version, status, reason, string(data), Hash(data)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO evidence_events(owner_id,collection,version,action,reason) VALUES($1,$2,$3,'installed',$4)`, owner, p.ID, p.Version, reason); err != nil {
		return err
	}
	return tx.Commit()
}
func (l Library) Collections(ctx context.Context, owner int) ([]Collection, error) {
	if err := l.seed(ctx, owner); err != nil {
		return nil, err
	}
	rows, err := l.DB.QueryContext(ctx, `SELECT collection,version,manifest->>'title',origin,status,reason,installed_at,jsonb_array_length(manifest->'passages') FROM evidence_versions WHERE owner_id=$1 ORDER BY collection, installed_at DESC`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Collection{}
	for rows.Next() {
		var c Collection
		var t time.Time
		if err := rows.Scan(&c.ID, &c.Version, &c.Title, &c.Origin, &c.Status, &c.Reason, &t, &c.Passages); err != nil {
			return nil, err
		}
		c.InstalledAt = t.UTC().Format(time.RFC3339)
		result = append(result, c)
	}
	return result, rows.Err()
}

func (l Library) Catalog(ctx context.Context, owner int) (Catalog, error) {
	if err := l.seed(ctx, owner); err != nil {
		return Catalog{}, err
	}
	tx, err := l.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return Catalog{}, err
	}
	defer tx.Rollback()
	var digest sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT string_agg(content_hash,',' ORDER BY collection) FROM evidence_versions WHERE owner_id=$1 AND status='active'`, owner).Scan(&digest); err != nil {
		return Catalog{}, err
	}
	key := Hash([]byte(digest.String))
	cache.Lock()
	defer cache.Unlock()
	if cache.key == key {
		return cache.catalog, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT manifest,content_hash FROM evidence_versions WHERE owner_id=$1 AND status='active' ORDER BY collection`, owner)
	if err != nil {
		return Catalog{}, err
	}
	defer rows.Close()
	packs := []ActivePack{}
	for rows.Next() {
		var raw []byte
		var p ActivePack
		if err := rows.Scan(&raw, &p.Hash); err != nil {
			return Catalog{}, err
		}
		if err := json.Unmarshal(raw, &p.Pack); err != nil {
			return Catalog{}, err
		}
		packs = append(packs, p)
	}
	if err := rows.Err(); err != nil {
		return Catalog{}, err
	}
	cache.catalog = Build(packs)
	cache.key = key
	return cache.catalog, nil
}

func (l Library) Get(ctx context.Context, owner int, id, version, passageID string) (Passage, error) {
	if !ValidID(id) || !ValidID(version) || !ValidID(passageID) {
		return Passage{}, sql.ErrNoRows
	}
	if err := l.seed(ctx, owner); err != nil {
		return Passage{}, err
	}
	var raw []byte
	var status string
	err := l.DB.QueryRowContext(ctx, `SELECT p,status FROM evidence_versions v CROSS JOIN LATERAL jsonb_array_elements(v.manifest->'passages') p WHERE owner_id=$1 AND collection=$2 AND version=$3 AND p->>'id'=$4`, owner, id, version, passageID).Scan(&raw, &status)
	if err != nil {
		return Passage{}, err
	}
	var p Passage
	if err = json.Unmarshal(raw, &p); err != nil {
		return Passage{}, err
	}
	p.Collection = id
	p.Version = version
	p.Status = status
	return p, nil
}

func (l Library) Install(ctx context.Context, owner int, p Pack) error {
	if err := Validate(p); err != nil {
		return errors.Join(ErrInvalid, err)
	}
	if p.ID == Starter().ID {
		return errors.Join(ErrInvalid, errors.New("operator collections require a distinct collection ID"))
	}
	if err := l.seed(ctx, owner); err != nil {
		return err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if len(data) > 48<<10 {
		return errors.Join(ErrInvalid, errors.New("collection exceeds the import size limit"))
	}
	tx, err := l.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT id FROM owners WHERE id=$1 FOR UPDATE`, owner); err != nil {
		return err
	}
	var old string
	err = tx.QueryRowContext(ctx, `SELECT content_hash FROM evidence_versions WHERE owner_id=$1 AND collection=$2 AND version=$3`, owner, p.ID, p.Version).Scan(&old)
	digest := Hash(data)
	if err == nil {
		if old == digest {
			return nil
		}
		return ErrConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM evidence_versions WHERE owner_id=$1`, owner).Scan(&count); err != nil {
		return err
	}
	if count >= 128 {
		return ErrCapacity
	}
	if err := retireActive(ctx, tx, owner, p.ID, "Replaced by "+p.Version); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO evidence_versions(owner_id,collection,version,origin,status,manifest,content_hash) VALUES($1,$2,$3,'operator','active',$4::jsonb,$5)`, owner, p.ID, p.Version, string(data), digest)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO evidence_events(owner_id,collection,version,action,reason) VALUES($1,$2,$3,'installed','Local operator confirmed content rights')`, owner, p.ID, p.Version); err != nil {
		return err
	}
	return tx.Commit()
}

func retireActive(ctx context.Context, tx *sql.Tx, owner int, id, reason string) error {
	_, err := tx.ExecContext(ctx, `WITH retired AS (UPDATE evidence_versions SET status='retired',reason=$3 WHERE owner_id=$1 AND collection=$2 AND status='active' RETURNING owner_id,collection,version,reason)
	INSERT INTO evidence_events(owner_id,collection,version,reason,action) SELECT owner_id,collection,version,reason,'retired' FROM retired`, owner, id, reason)
	return err
}
func (l Library) Change(ctx context.Context, owner int, id, version, status, reason string) error {
	if !ValidID(id) || !ValidID(version) || !bounded(reason, 300) || (status != "active" && status != "withdrawn") {
		return errors.Join(ErrInvalid, errors.New("select an installed version, new status and short reason"))
	}
	if err := l.seed(ctx, owner); err != nil {
		return err
	}
	tx, err := l.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT id FROM owners WHERE id=$1 FOR UPDATE`, owner); err != nil {
		return err
	}
	var current string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM evidence_versions WHERE owner_id=$1 AND collection=$2 AND version=$3`, owner, id, version).Scan(&current); err != nil {
		return err
	}
	if current == status {
		return nil
	}
	if status == "active" {
		if err := retireActive(ctx, tx, owner, id, "Replaced by "+version); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE evidence_versions SET status=$4,reason=$5 WHERE owner_id=$1 AND collection=$2 AND version=$3`, owner, id, version, status, strings.TrimSpace(reason)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO evidence_events(owner_id,collection,version,action,reason) VALUES($1,$2,$3,$4,$5)`, owner, id, version, status, strings.TrimSpace(reason)); err != nil {
		return err
	}
	return tx.Commit()
}
