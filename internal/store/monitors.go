package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"pulse/internal/model"
)

func nowUnix() int64 { return time.Now().Unix() }

const monitorColumns = `id,name,type,target,group_name,interval_sec,timeout_ms,enabled,config,probe_agent_id,
status,last_checked_at,last_latency_ms,last_error,last_status_code,ssl_expires_at,
consecutive_fails,consecutive_oks,down_since,last_notified_at,fail_threshold,recover_threshold,
cf_enabled,cf_zone_id,cf_record_id,cf_record_name,cf_primary,cf_failover,cf_last_action,cf_last_action_at,
created_at,updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMonitor(sc rowScanner) (model.Monitor, error) {
	var m model.Monitor
	var enabled, cfEnabled int
	var cfg string
	err := sc.Scan(
		&m.ID, &m.Name, &m.Type, &m.Target, &m.Group, &m.Interval, &m.Timeout, &enabled, &cfg, &m.ProbeAgentID,
		&m.Status, &m.LastCheckedAt, &m.LastLatencyMS, &m.LastError, &m.LastStatusCode, &m.SSLExpiresAt,
		&m.ConsecutiveFails, &m.ConsecutiveOks, &m.DownSince, &m.LastNotifiedAt, &m.FailThreshold, &m.RecoverThreshold,
		&cfEnabled, &m.CFZoneID, &m.CFRecordID, &m.CFRecordName, &m.CFPrimary, &m.CFFailover, &m.CFLastAction, &m.CFLastActionAt,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return m, err
	}
	m.Enabled = enabled == 1
	m.CFEnabled = cfEnabled == 1
	if strings.TrimSpace(cfg) == "" {
		cfg = "{}"
	}
	m.Config = json.RawMessage(cfg)
	m.Spark = []int64{}
	return m, nil
}

func (s *Store) ListMonitors() ([]model.Monitor, error) {
	rows, err := s.db.Query(`SELECT ` + monitorColumns + ` FROM monitors ORDER BY enabled DESC, group_name, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Monitor{}
	for rows.Next() {
		m, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) ListEnabledMonitors() ([]model.Monitor, error) {
	rows, err := s.db.Query(`SELECT ` + monitorColumns + ` FROM monitors WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Monitor{}
	for rows.Next() {
		m, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GetMonitor(id int64) (*model.Monitor, error) {
	row := s.db.QueryRow(`SELECT `+monitorColumns+` FROM monitors WHERE id = ?`, id)
	m, err := scanMonitor(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) CreateMonitor(m *model.Monitor) error {
	now := nowUnix()
	if m.CreatedAt == 0 {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	if len(m.Config) == 0 {
		m.Config = json.RawMessage(`{}`)
	}
	res, err := s.db.Exec(`INSERT INTO monitors(
		name,type,target,group_name,interval_sec,timeout_ms,enabled,config,probe_agent_id,status,
		last_checked_at,last_latency_ms,last_error,last_status_code,ssl_expires_at,
		consecutive_fails,consecutive_oks,down_since,last_notified_at,fail_threshold,recover_threshold,
		cf_enabled,cf_zone_id,cf_record_id,cf_record_name,cf_primary,cf_failover,cf_last_action,cf_last_action_at,
		created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.Name, m.Type, m.Target, m.Group, m.Interval, m.Timeout, boolInt(m.Enabled), string(m.Config), m.ProbeAgentID, "unknown",
		0, 0, "", 0, 0, 0, 0, 0, 0, m.FailThreshold, m.RecoverThreshold,
		boolInt(m.CFEnabled), m.CFZoneID, m.CFRecordID, m.CFRecordName, m.CFPrimary, m.CFFailover, "", 0,
		m.CreatedAt, m.UpdatedAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	m.ID = id
	m.Status = "unknown"
	return nil
}

func (s *Store) UpdateMonitor(m *model.Monitor) error {
	now := nowUnix()
	if len(m.Config) == 0 {
		m.Config = json.RawMessage(`{}`)
	}
	_, err := s.db.Exec(`UPDATE monitors SET
		name=?,type=?,target=?,group_name=?,interval_sec=?,timeout_ms=?,enabled=?,config=?,probe_agent_id=?,
		fail_threshold=?,recover_threshold=?,cf_enabled=?,cf_zone_id=?,cf_record_id=?,cf_record_name=?,
		cf_primary=?,cf_failover=?,updated_at=? WHERE id=?`,
		m.Name, m.Type, m.Target, m.Group, m.Interval, m.Timeout, boolInt(m.Enabled), string(m.Config), m.ProbeAgentID,
		m.FailThreshold, m.RecoverThreshold, boolInt(m.CFEnabled), m.CFZoneID, m.CFRecordID, m.CFRecordName,
		m.CFPrimary, m.CFFailover, now, m.ID)
	if err != nil {
		return err
	}
	m.UpdatedAt = now
	return nil
}

func (s *Store) ResetMonitorState(id int64) error {
	now := nowUnix()
	_, err := s.db.Exec(`UPDATE monitors SET status='unknown', consecutive_fails=0, consecutive_oks=0,
		last_checked_at=0, last_latency_ms=0, last_error='', last_status_code=0, ssl_expires_at=0, down_since=0,
		updated_at=? WHERE id=?`, now, id)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`UPDATE incidents SET ended_at=?, resolved_by='reset' WHERE monitor_id=? AND ended_at=0`, now, id)
	return nil
}

func (s *Store) SetMonitorEnabled(id int64, enabled bool) error {
	var status string
	err := s.db.QueryRow(`SELECT status FROM monitors WHERE id=?`, id).Scan(&status)
	if err != nil {
		return err
	}
	if !enabled {
		status = "paused"
	} else if status == "paused" {
		status = "unknown"
	}
	now := nowUnix()
	_, err = s.db.Exec(`UPDATE monitors SET enabled=?, status=?, last_checked_at=0, updated_at=? WHERE id=?`,
		boolInt(enabled), status, now, id)
	if err != nil {
		return err
	}
	// A paused or restarted monitor must not leave a stale open incident behind.
	resolvedBy := "paused"
	if enabled {
		resolvedBy = "re-enabled"
	}
	_, _ = s.db.Exec(`UPDATE incidents SET ended_at=?, resolved_by=? WHERE monitor_id=? AND ended_at=0`,
		now, resolvedBy, id)
	return nil
}

func (s *Store) DeleteMonitor(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE rules SET monitor_id=0 WHERE monitor_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM monitors WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateMonitorNotified(id int64, at int64) error {
	_, err := s.db.Exec(`UPDATE monitors SET last_notified_at=? WHERE id=?`, at, id)
	return err
}

func (s *Store) UpdateMonitorCFAction(id int64, action string, at int64) error {
	_, err := s.db.Exec(`UPDATE monitors SET cf_last_action=?, cf_last_action_at=? WHERE id=?`, action, at, id)
	return err
}

// CommitCheck saves a result atomically and updates the monitor state. It may open or close an incident.
func (s *Store) CommitCheck(m *model.Monitor, c *model.Check, openIncident, closeIncident bool) error {
	if c.At == 0 {
		c.At = nowUnix()
	}
	if len(c.Meta) == 0 {
		c.Meta = json.RawMessage(`{}`)
	}
	if c.Status == "" {
		c.Status = m.Status
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO checks(monitor_id,at,ok,status,latency_ms,status_code,error,meta)
		VALUES(?,?,?,?,?,?,?,?)`, m.ID, c.At, boolInt(c.OK), c.Status, c.LatencyMS, c.StatusCode, c.Error, string(c.Meta))
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()

	day := time.Unix(c.At, 0).UTC().Format("2006-01-02")
	if _, err := tx.Exec(`INSERT INTO monitor_daily(monitor_id,day,total,ok,sum_latency)
		VALUES(?,?,1,?,?)
		ON CONFLICT(monitor_id,day) DO UPDATE SET total=total+1, ok=ok+excluded.ok, sum_latency=sum_latency+excluded.sum_latency`,
		m.ID, day, boolInt(c.OK), c.LatencyMS); err != nil {
		return err
	}

	if _, err := tx.Exec(`UPDATE monitors SET status=?, last_checked_at=?, last_latency_ms=?, last_error=?,
		last_status_code=?, ssl_expires_at=CASE WHEN ?>0 THEN ? ELSE ssl_expires_at END,
		consecutive_fails=?, consecutive_oks=?, down_since=?, updated_at=? WHERE id=?`,
		m.Status, c.At, c.LatencyMS, c.Error, c.StatusCode, m.SSLExpiresAt, m.SSLExpiresAt,
		m.ConsecutiveFails, m.ConsecutiveOks, m.DownSince, c.At, m.ID); err != nil {
		return err
	}

	if openIncident {
		started := m.DownSince
		if started == 0 {
			started = c.At
		}
		if _, err := tx.Exec(`INSERT INTO incidents(monitor_id,started_at,ended_at,cause)
			VALUES(?,?,0,?)`, m.ID, started, c.Error); err != nil {
			return err
		}
	}
	if closeIncident {
		if _, err := tx.Exec(`UPDATE incidents SET ended_at=?, resolved_by='auto'
			WHERE monitor_id=? AND ended_at=0`, c.At, m.ID); err != nil {
			return err
		}
	} else if m.Status == "up" || m.Status == "degraded" {
		// Defensive cleanup: a healthy monitor cannot have an open incident.
		if _, err := tx.Exec(`UPDATE incidents SET ended_at=?, resolved_by='auto'
			WHERE monitor_id=? AND ended_at=0`, c.At, m.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type MonitorStat struct {
	Uptime     float64
	AvgLatency int64
	Spark      []int64
}

type TrendPoint struct {
	At         int64   `json:"at"`
	Total      int     `json:"total"`
	OK         int     `json:"ok"`
	AvgLatency int64   `json:"avg_latency_ms"`
	Uptime     float64 `json:"uptime"`
}

// GlobalTrend aggregates all checks into fixed-size buckets for the overview chart.
func (s *Store) GlobalTrend(since int64, bucketSec int64) ([]TrendPoint, error) {
	if bucketSec < 60 {
		bucketSec = 60
	}
	rows, err := s.db.Query(`SELECT (at / ?) * ? AS bucket, COUNT(*), SUM(ok), AVG(latency_ms)
		FROM checks WHERE at >= ? GROUP BY bucket ORDER BY bucket`, bucketSec, bucketSec, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrendPoint{}
	for rows.Next() {
		var p TrendPoint
		var avg sql.NullFloat64
		if err := rows.Scan(&p.At, &p.Total, &p.OK, &avg); err != nil {
			return nil, err
		}
		if p.Total > 0 {
			p.Uptime = float64(p.OK) / float64(p.Total) * 100
		}
		if avg.Valid {
			p.AvgLatency = int64(avg.Float64)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) MonitorStats(since int64) (map[int64]*MonitorStat, error) {
	out := map[int64]*MonitorStat{}
	rows, err := s.db.Query(`SELECT monitor_id, COUNT(*), SUM(ok), AVG(latency_ms)
		FROM checks WHERE at >= ? GROUP BY monitor_id`, since)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var total, ok int
		var avg sql.NullFloat64
		if err := rows.Scan(&id, &total, &ok, &avg); err != nil {
			rows.Close()
			return nil, err
		}
		st := &MonitorStat{}
		if total > 0 {
			st.Uptime = float64(ok) / float64(total) * 100
		}
		if avg.Valid {
			st.AvgLatency = int64(avg.Float64)
		}
		out[id] = st
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	srows, err := s.db.Query(`SELECT monitor_id, latency_ms FROM (
		SELECT monitor_id, latency_ms, ROW_NUMBER() OVER (PARTITION BY monitor_id ORDER BY at DESC) rn
		FROM checks WHERE at >= ? AND ok = 1) WHERE rn <= 24 ORDER BY monitor_id, rn DESC`, since)
	if err != nil {
		return nil, err
	}
	defer srows.Close()
	for srows.Next() {
		var id, latency int64
		if err := srows.Scan(&id, &latency); err != nil {
			return nil, err
		}
		if st := out[id]; st != nil {
			st.Spark = append(st.Spark, latency)
		}
	}
	return out, srows.Err()
}

func (s *Store) ListChecks(monitorID int64, limit int) ([]model.Check, error) {
	if limit <= 0 || limit > 2000 {
		limit = 120
	}
	rows, err := s.db.Query(`SELECT id,monitor_id,at,ok,status,latency_ms,status_code,error,meta
		FROM checks WHERE monitor_id=? ORDER BY at DESC LIMIT ?`, monitorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Check{}
	for rows.Next() {
		var c model.Check
		var ok int
		var meta string
		if err := rows.Scan(&c.ID, &c.MonitorID, &c.At, &ok, &c.Status, &c.LatencyMS, &c.StatusCode, &c.Error, &meta); err != nil {
			return nil, err
		}
		c.OK = ok == 1
		if meta == "" {
			meta = "{}"
		}
		c.Meta = json.RawMessage(meta)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) DailyStats(monitorID int64, sinceDay string) ([]model.DailyStat, error) {
	rows, err := s.db.Query(`SELECT day,total,ok,sum_latency FROM monitor_daily
		WHERE monitor_id=? AND day>=? ORDER BY day`, monitorID, sinceDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DailyStat{}
	for rows.Next() {
		var d model.DailyStat
		var sum int64
		if err := rows.Scan(&d.Day, &d.Total, &d.OK, &sum); err != nil {
			return nil, err
		}
		if d.Total > 0 {
			d.Uptime = float64(d.OK) / float64(d.Total) * 100
			d.AvgLatency = sum / int64(d.Total)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ListIncidents(limit int, activeOnly bool) ([]model.Incident, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT i.id,i.monitor_id,m.name,i.started_at,i.ended_at,i.cause,i.resolved_by
		FROM incidents i JOIN monitors m ON m.id=i.monitor_id`
	if activeOnly {
		q += ` WHERE i.ended_at=0`
	}
	q += ` ORDER BY i.started_at DESC LIMIT ?`
	rows, err := s.db.Query(q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Incident{}
	now := nowUnix()
	for rows.Next() {
		var i model.Incident
		if err := rows.Scan(&i.ID, &i.MonitorID, &i.MonitorName, &i.StartedAt, &i.EndedAt, &i.Cause, &i.ResolvedBy); err != nil {
			return nil, err
		}
		end := i.EndedAt
		if end == 0 {
			end = now
		}
		i.Duration = end - i.StartedAt
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) CountActiveIncidents() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM incidents WHERE ended_at=0`).Scan(&n)
	return n, err
}

func (s *Store) ResolveIncident(id int64, by string) error {
	_, err := s.db.Exec(`UPDATE incidents SET ended_at=?, resolved_by=? WHERE id=? AND ended_at=0`, nowUnix(), by, id)
	return err
}

const ruleColumns = `id,name,enabled,trigger_type,monitor_id,threshold,cooldown_sec,channels,last_fired_at,created_at`

func scanRule(sc rowScanner) (model.Rule, error) {
	var r model.Rule
	var enabled int
	var channels string
	err := sc.Scan(&r.ID, &r.Name, &enabled, &r.Trigger, &r.MonitorID, &r.Threshold, &r.CooldownSec, &channels, &r.LastFiredAt, &r.CreatedAt)
	if err != nil {
		return r, err
	}
	r.Enabled = enabled == 1
	r.Channels = []string{}
	if channels != "" {
		_ = json.Unmarshal([]byte(channels), &r.Channels)
	}
	if len(r.Channels) == 0 {
		r.Channels = []string{"all"}
	}
	return r, nil
}

func (s *Store) ListRules() ([]model.Rule, error) {
	rows, err := s.db.Query(`SELECT ` + ruleColumns + ` FROM rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Rule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetRule(id int64) (*model.Rule, error) {
	row := s.db.QueryRow(`SELECT `+ruleColumns+` FROM rules WHERE id=?`, id)
	r, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) CreateRule(r *model.Rule) error {
	channels, _ := json.Marshal(r.Channels)
	res, err := s.db.Exec(`INSERT INTO rules(name,enabled,trigger_type,monitor_id,threshold,cooldown_sec,channels,created_at)
		VALUES(?,?,?,?,?,?,?,?)`, r.Name, boolInt(r.Enabled), r.Trigger, r.MonitorID, r.Threshold, r.CooldownSec, string(channels), nowUnix())
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	r.CreatedAt = nowUnix()
	return nil
}

func (s *Store) UpdateRule(r *model.Rule) error {
	channels, _ := json.Marshal(r.Channels)
	_, err := s.db.Exec(`UPDATE rules SET name=?,enabled=?,trigger_type=?,monitor_id=?,threshold=?,cooldown_sec=?,channels=? WHERE id=?`,
		r.Name, boolInt(r.Enabled), r.Trigger, r.MonitorID, r.Threshold, r.CooldownSec, string(channels), r.ID)
	return err
}

func (s *Store) DeleteRule(id int64) error {
	_, err := s.db.Exec(`DELETE FROM rules WHERE id=?`, id)
	return err
}

func (s *Store) MarkRuleFired(id, at int64) error {
	_, err := s.db.Exec(`UPDATE rules SET last_fired_at=? WHERE id=?`, at, id)
	return err
}

func (s *Store) InsertNotification(n *model.NotificationLog) error {
	res, err := s.db.Exec(`INSERT INTO notification_log(at,channel,rule_name,event,title,body,ok,error)
		VALUES(?,?,?,?,?,?,?,?)`, n.At, n.Channel, n.RuleName, n.Event, n.Title, n.Body, boolInt(n.OK), n.Error)
	if err != nil {
		return err
	}
	n.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) ListNotifications(limit int) ([]model.NotificationLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id,at,channel,rule_name,event,title,body,ok,error FROM notification_log ORDER BY at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.NotificationLog{}
	for rows.Next() {
		var n model.NotificationLog
		var ok int
		if err := rows.Scan(&n.ID, &n.At, &n.Channel, &n.RuleName, &n.Event, &n.Title, &n.Body, &ok, &n.Error); err != nil {
			return nil, err
		}
		n.OK = ok == 1
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) AddAudit(actor, action, detail string) {
	_, _ = s.db.Exec(`INSERT INTO audit_log(at,actor,action,detail) VALUES(?,?,?,?)`, nowUnix(), actor, action, detail)
}

func (s *Store) ListAudit(limit int) ([]model.AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id,at,actor,action,detail FROM audit_log ORDER BY at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AuditEntry{}
	for rows.Next() {
		var e model.AuditEntry
		if err := rows.Scan(&e.ID, &e.At, &e.Actor, &e.Action, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) AllSettings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key,value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) SetSettings(values map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range values {
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Stats() (monitors int, checks int, agentsCount int, dbBytes int64, err error) {
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM monitors`).Scan(&monitors); err != nil {
		return
	}
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM checks`).Scan(&checks); err != nil {
		return
	}
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM agents`).Scan(&agentsCount); err != nil {
		return
	}
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(page_count),0)*page_size FROM pragma_page_count(), pragma_page_size()`).Scan(&dbBytes)
	return
}

func (s *Store) Cleanup(checkDays, metricDays int) {
	if checkDays > 0 {
		cut := time.Now().AddDate(0, 0, -checkDays).Unix()
		_, _ = s.db.Exec(`DELETE FROM checks WHERE at < ?`, cut)
		_, _ = s.db.Exec(`DELETE FROM monitor_daily WHERE day < ?`, time.Now().AddDate(0, 0, -90).UTC().Format("2006-01-02"))
	}
	if metricDays > 0 {
		cut := time.Now().AddDate(0, 0, -metricDays).Unix()
		_, _ = s.db.Exec(`DELETE FROM agent_metrics WHERE at < ?`, cut)
	}
	_, _ = s.db.Exec(`DELETE FROM notification_log WHERE at < ?`, time.Now().AddDate(0, 0, -60).Unix())
	_, _ = s.db.Exec(`DELETE FROM audit_log WHERE at < ?`, time.Now().AddDate(0, 0, -180).Unix())
	_, _ = s.db.Exec(`DELETE FROM incident`+`s WHERE ended_at>0 AND ended_at < ?`, time.Now().AddDate(0, 0, -365).Unix())
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
