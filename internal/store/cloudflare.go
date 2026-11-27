package store

import (
	"database/sql"
	"errors"
	"time"

	"pulse/internal/model"
)

const cfAccountColumns = `id,name,token,enabled,created_at,updated_at`

func scanCFAccount(sc rowScanner) (model.CFAccount, error) {
	var a model.CFAccount
	var enabled int
	err := sc.Scan(&a.ID, &a.Name, &a.Token, &enabled, &a.CreatedAt, &a.UpdatedAt)
	a.Enabled = enabled == 1
	return a, err
}

func (s *Store) ListCFAccounts() ([]model.CFAccount, error) {
	rows, err := s.db.Query(`SELECT ` + cfAccountColumns + ` FROM cf_accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CFAccount{}
	for rows.Next() {
		a, err := scanCFAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetCFAccount(id int64) (*model.CFAccount, error) {
	row := s.db.QueryRow(`SELECT `+cfAccountColumns+` FROM cf_accounts WHERE id = ?`, id)
	a, err := scanCFAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) FirstCFAccount() (*model.CFAccount, error) {
	row := s.db.QueryRow(`SELECT ` + cfAccountColumns + ` FROM cf_accounts WHERE enabled = 1 ORDER BY id LIMIT 1`)
	a, err := scanCFAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) CountCFAccounts() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM cf_accounts`).Scan(&n)
	return n, err
}

func (s *Store) CreateCFAccount(a *model.CFAccount) error {
	now := time.Now().Unix()
	a.CreatedAt = now
	a.UpdatedAt = now
	res, err := s.db.Exec(`INSERT INTO cf_accounts(name,token,enabled,created_at,updated_at) VALUES(?,?,?,?,?)`,
		a.Name, a.Token, boolInt(a.Enabled), a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) UpdateCFAccount(a *model.CFAccount) error {
	a.UpdatedAt = time.Now().Unix()
	_, err := s.db.Exec(`UPDATE cf_accounts SET name=?,token=?,enabled=?,updated_at=? WHERE id=?`,
		a.Name, a.Token, boolInt(a.Enabled), a.UpdatedAt, a.ID)
	return err
}

// DeleteCFAccount removes the token and disables monitors that pointed at it.
// The DNS record details are kept so the user can reconnect the monitor to
// another account without losing its configuration.
func (s *Store) DeleteCFAccount(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	if _, err := tx.Exec(`UPDATE monitors SET cf_enabled=0, cf_account_id=0, cf_last_action=?, cf_last_action_at=?, updated_at=? WHERE cf_account_id=?`,
		"حساب Cloudflare حذف شد؛ سوییچ خودکار غیرفعال شد", now, now, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM cf_accounts WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
