package store

import (
	"strings"

	"pulse/internal/model"
)

// LogFilter selects application logs for the UI and CLI.
type LogFilter struct {
	Category string
	Level    string
	Query    string
	AfterID  int64
	BeforeID int64
	Limit    int
}

func (s *Store) AddLog(entry model.LogEntry) error {
	if entry.At <= 0 {
		entry.At = nowUnix()
	}
	if entry.Level == "" {
		entry.Level = "info"
	}
	if entry.Category == "" {
		entry.Category = "panel"
	}
	if len(entry.Message) > 2000 {
		entry.Message = entry.Message[:2000] + "…"
	}
	if len(entry.Detail) > 16000 {
		entry.Detail = entry.Detail[:16000] + "…"
	}
	_, err := s.db.Exec(`INSERT INTO app_logs(at,level,category,message,detail) VALUES(?,?,?,?,?)`,
		entry.At, entry.Level, entry.Category, entry.Message, entry.Detail)
	return err
}

func (s *Store) ListLogs(f LogFilter) ([]model.LogEntry, error) {
	if f.Limit <= 0 {
		f.Limit = 200
	}
	if f.Limit > 1000 {
		f.Limit = 1000
	}
	var where []string
	var args []any
	if strings.TrimSpace(f.Category) != "" && f.Category != "all" {
		where = append(where, "category=?")
		args = append(args, f.Category)
	}
	if strings.TrimSpace(f.Level) != "" && f.Level != "all" {
		where = append(where, "level=?")
		args = append(args, f.Level)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, "(message LIKE ? OR detail LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	if f.AfterID > 0 {
		where = append(where, "id>?")
		args = append(args, f.AfterID)
	}
	if f.BeforeID > 0 {
		where = append(where, "id<?")
		args = append(args, f.BeforeID)
	}
	sql := `SELECT id,at,level,category,message,detail FROM app_logs`
	if len(where) > 0 {
		sql += ` WHERE ` + strings.Join(where, ` AND `)
	}
	sql += ` ORDER BY id DESC LIMIT ?`
	args = append(args, f.Limit)
	rows, err := s.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.LogEntry{}
	for rows.Next() {
		var e model.LogEntry
		if err := rows.Scan(&e.ID, &e.At, &e.Level, &e.Category, &e.Message, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CleanupLogs drops old entries and caps the table so an unattended panel
// cannot grow the database forever.
func (s *Store) CleanupLogs(cutoff int64, keep int) {
	if keep <= 0 {
		keep = 20000
	}
	_, _ = s.db.Exec(`DELETE FROM app_logs WHERE at < ?`, cutoff)
	_, _ = s.db.Exec(`DELETE FROM app_logs WHERE id NOT IN (SELECT id FROM app_logs ORDER BY id DESC LIMIT ?)`, keep)
}
