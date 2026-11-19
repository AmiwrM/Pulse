package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"pulse/internal/model"
)

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func NewAgentToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) CreateAgent(name string) (*model.Agent, string, error) {
	token, err := NewAgentToken()
	if err != nil {
		return nil, "", err
	}
	now := nowUnix()
	res, err := s.db.Exec(`INSERT INTO agents(name,token_hash,created_at) VALUES(?,?,?)`, name, HashToken(token), now)
	if err != nil {
		return nil, "", err
	}
	id, _ := res.LastInsertId()
	return &model.Agent{ID: id, Name: name, CreatedAt: now, Services: json.RawMessage("[]"), WireGuard: json.RawMessage("[]")}, token, nil
}

func (s *Store) RotateAgentToken(id int64) (string, error) {
	token, err := NewAgentToken()
	if err != nil {
		return "", err
	}
	res, err := s.db.Exec(`UPDATE agents SET token_hash=? WHERE id=?`, HashToken(token), id)
	if err != nil {
		return "", err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", sql.ErrNoRows
	}
	return token, nil
}

const agentColumns = `id,name,hostname,os,arch,version,ip,last_seen,created_at,cpu_pct,ram_pct,disk_pct,load1,
uptime_sec,net_rx_bps,net_tx_bps,ram_total,disk_total,services,wireguard,notes,offline_notified`

func scanAgent(sc rowScanner) (model.Agent, error) {
	var a model.Agent
	var services, wg string
	var notified int
	err := sc.Scan(&a.ID, &a.Name, &a.Hostname, &a.OS, &a.Arch, &a.Version, &a.IP, &a.LastSeen, &a.CreatedAt,
		&a.CPUPct, &a.RAMPct, &a.DiskPct, &a.Load1, &a.UptimeSec, &a.NetRx, &a.NetTx, &a.RAMTotal, &a.DiskTotal, &services, &wg, &a.Notes, &notified)
	if err != nil {
		return a, err
	}
	a.OfflineNotified = notified == 1
	if services == "" {
		services = "[]"
	}
	if wg == "" {
		wg = "[]"
	}
	a.Services = json.RawMessage(services)
	a.WireGuard = json.RawMessage(wg)
	return a, nil
}

func (s *Store) ListAgents() ([]model.Agent, error) {
	rows, err := s.db.Query(`SELECT ` + agentColumns + ` FROM agents ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Agent{}
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAgent(id int64) (*model.Agent, error) {
	row := s.db.QueryRow(`SELECT `+agentColumns+` FROM agents WHERE id=?`, id)
	a, err := scanAgent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) GetAgentByToken(token string) (*model.Agent, error) {
	row := s.db.QueryRow(`SELECT `+agentColumns+` FROM agents WHERE token_hash=?`, HashToken(token))
	a, err := scanAgent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) DeleteAgent(id int64) error {
	_, err := s.db.Exec(`DELETE FROM agents WHERE id=?`, id)
	return err
}

func (s *Store) UpdateAgentMeta(id int64, name, notes string) error {
	_, err := s.db.Exec(`UPDATE agents SET name=?, notes=? WHERE id=?`, name, notes, id)
	return err
}

func (s *Store) RecordAgentReport(id int64, r model.AgentReport) error {
	now := nowUnix()
	if len(r.Services) == 0 {
		r.Services = json.RawMessage("[]")
	}
	if len(r.WireGuard) == 0 {
		r.WireGuard = json.RawMessage("[]")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE agents SET hostname=?,os=?,arch=?,version=?,ip=?,last_seen=?,cpu_pct=?,ram_pct=?,
		disk_pct=?,load1=?,uptime_sec=?,net_rx_bps=?,net_tx_bps=?,ram_total=?,disk_total=?,services=?,wireguard=?,offline_notified=0
		WHERE id=?`,
		r.Hostname, r.OS, r.Arch, r.Version, r.IP, now, r.CPUPct, r.RAMPct, r.DiskPct, r.Load1, r.UptimeSec,
		r.NetRx, r.NetTx, r.RAMTotal, r.DiskTotal, string(r.Services), string(r.WireGuard), id); err != nil {
		return err
	}
	// The agent sends a live REST report every second so the panel can update
	// immediately, but history is sampled at a coarser resolution to keep
	// agent_metrics compact and charts cheap to query.
	var lastMetric int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(at),0) FROM agent_metrics WHERE agent_id=?`, id).Scan(&lastMetric); err != nil {
		return err
	}
	if now-lastMetric >= 10 {
		if _, err := tx.Exec(`INSERT INTO agent_metrics(agent_id,at,cpu,ram,disk,load1,net_rx_bps,net_tx_bps)
			VALUES(?,?,?,?,?,?,?,?)`, id, now, r.CPUPct, r.RAMPct, r.DiskPct, r.Load1, r.NetRx, r.NetTx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) AgentMetrics(id int64, since int64) ([]model.AgentMetric, error) {
	rows, err := s.db.Query(`SELECT at,cpu,ram,disk,load1,net_rx_bps,net_tx_bps FROM agent_metrics
		WHERE agent_id=? AND at>=? ORDER BY at`, id, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AgentMetric{}
	for rows.Next() {
		var m model.AgentMetric
		if err := rows.Scan(&m.At, &m.CPU, &m.RAM, &m.Disk, &m.Load1, &m.NetRx, &m.NetTx); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) OfflineAgents(cutoff int64) ([]model.Agent, error) {
	rows, err := s.db.Query(`SELECT `+agentColumns+` FROM agents WHERE last_seen>0 AND last_seen<?`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Agent{}
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ClaimAgentOffline marks an offline agent as notified. It returns true only for
// the first caller, making notifications idempotent across restarts.
func (s *Store) ClaimAgentOffline(id int64) bool {
	res, err := s.db.Exec(`UPDATE agents SET offline_notified=1 WHERE id=? AND offline_notified=0`, id)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

func (s *Store) SavePushSub(sub model.PushSub) error {
	if sub.CreatedAt == 0 {
		sub.CreatedAt = nowUnix()
	}
	_, err := s.db.Exec(`INSERT INTO push_subs(endpoint,p256dh,auth,user_agent,created_at) VALUES(?,?,?,?,?)
		ON CONFLICT(endpoint) DO UPDATE SET p256dh=excluded.p256dh, auth=excluded.auth, user_agent=excluded.user_agent`,
		sub.Endpoint, sub.P256DH, sub.Auth, sub.UserAgent, sub.CreatedAt)
	return err
}

func (s *Store) ListPushSubs() ([]model.PushSub, error) {
	rows, err := s.db.Query(`SELECT id,endpoint,p256dh,auth,user_agent,created_at FROM push_subs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.PushSub{}
	for rows.Next() {
		var p model.PushSub
		if err := rows.Scan(&p.ID, &p.Endpoint, &p.P256DH, &p.Auth, &p.UserAgent, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) DeletePushSub(endpoint string) error {
	_, err := s.db.Exec(`DELETE FROM push_subs WHERE endpoint=?`, endpoint)
	return err
}

func (s *Store) CountPushSubs() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM push_subs`).Scan(&n)
	return n, err
}

// ----- users -----

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(username, hash string) error {
	_, err := s.db.Exec(`INSERT INTO users(username,password_hash,created_at) VALUES(?,?,?)`, username, hash, time.Now().Unix())
	return err
}

func (s *Store) GetUser(username string) (*model.User, error) {
	var u model.User
	err := s.db.QueryRow(`SELECT id,username,password_hash,created_at FROM users WHERE username=?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) UpdatePassword(username, hash string) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash=? WHERE username=?`, hash, username)
	return err
}
