package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ctlplne/healthctl/internal/planlogin"
)

// PlanAccount is local credential material. Never serialize it in a public response.
type PlanAccount struct {
	ID          string
	Session     planlogin.Session
	LocalOptIn  bool
	EverEnabled bool
	Generation  uint64
}

func planAAD(owner int, host string) []byte {
	return []byte(fmt.Sprintf("chatgpt-vault/v1/%d/%s", owner, host))
}

func (s *Store) openAccounts(owner int, host string, ciphertext []byte) ([]PlanAccount, error) {
	if len(ciphertext) == 0 {
		return []PlanAccount{}, nil
	}
	data, err := s.box.Open(nil, nil, ciphertext, planAAD(owner, host))
	if err != nil {
		return nil, errors.New("could not decrypt ChatGPT accounts")
	}
	var accounts []PlanAccount
	err = json.Unmarshal(data, &accounts)
	return accounts, err
}

func (s *Store) PlanAccounts(ctx context.Context, owner int) ([]PlanAccount, error) {
	var host string
	var sealed []byte
	err := s.DB.QueryRowContext(ctx, "SELECT host_id,accounts FROM chatgpt_vault WHERE owner_id=$1", owner).Scan(&host, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return []PlanAccount{}, nil
	}
	if err != nil {
		return nil, err
	}
	return s.openAccounts(owner, host, sealed)
}

// EditPlanAccounts holds the row lock through refresh and credential rotation.
// A restart or concurrent HTTP request always sees one complete generation.
func (s *Store) EditPlanAccounts(ctx context.Context, owner int, edit func(string, *[]PlanAccount) (bool, error)) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO chatgpt_vault(owner_id) VALUES($1) ON CONFLICT DO NOTHING", owner); err != nil {
		return err
	}
	var host string
	var sealed []byte
	if err = tx.QueryRowContext(ctx, "SELECT host_id,accounts FROM chatgpt_vault WHERE owner_id=$1 FOR UPDATE", owner).Scan(&host, &sealed); err != nil {
		return err
	}
	accounts, err := s.openAccounts(owner, host, sealed)
	if err != nil {
		return err
	}
	changed, err := edit(host, &accounts)
	if err != nil {
		return err
	}
	if changed {
		raw, err := json.Marshal(accounts)
		if err != nil {
			return err
		}
		sealed = s.box.Seal(nil, nil, raw, planAAD(owner, host))
		if _, err = tx.ExecContext(ctx, "UPDATE chatgpt_vault SET accounts=$1 WHERE owner_id=$2", sealed, owner); err != nil {
			return err
		}
	}
	return tx.Commit()
}
