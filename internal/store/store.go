package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store is the SQLite persistence layer.
type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create data directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=10000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS cf_accounts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	token TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS monitors (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	target TEXT NOT NULL,
	group_name TEXT NOT NULL DEFAULT '',
	interval_sec INTEGER NOT NULL DEFAULT 60,
	timeout_ms INTEGER NOT NULL DEFAULT 12000,
	enabled INTEGER NOT NULL DEFAULT 1,
	config TEXT NOT NULL DEFAULT '{}',
	probe_agent_id INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'unknown',
	last_checked_at INTEGER NOT NULL DEFAULT 0,
	last_latency_ms INTEGER NOT NULL DEFAULT 0,
	last_error TEXT NOT NULL DEFAULT '',
	last_status_code INTEGER NOT NULL DEFAULT 0,
	ssl_expires_at INTEGER NOT NULL DEFAULT 0,
	consecutive_fails INTEGER NOT NULL DEFAULT 0,
	consecutive_oks INTEGER NOT NULL DEFAULT 0,
	down_since INTEGER NOT NULL DEFAULT 0,
	last_notified_at INTEGER NOT NULL DEFAULT 0,
	fail_threshold INTEGER NOT NULL DEFAULT 2,
	recover_threshold INTEGER NOT NULL DEFAULT 1,
	cf_enabled INTEGER NOT NULL DEFAULT 0,
	cf_account_id INTEGER NOT NULL DEFAULT 0,
	cf_zone_id TEXT NOT NULL DEFAULT '',
	cf_record_id TEXT NOT NULL DEFAULT '',
	cf_record_name TEXT NOT NULL DEFAULT '',
	cf_primary TEXT NOT NULL DEFAULT '',
	cf_failover TEXT NOT NULL DEFAULT '',
	cf_last_action TEXT NOT NULL DEFAULT '',
	cf_last_action_at INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS checks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
	at INTEGER NOT NULL,
	ok INTEGER NOT NULL,
	status TEXT NOT NULL DEFAULT '',
	latency_ms INTEGER NOT NULL DEFAULT 0,
	status_code INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT '',
	meta TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_checks_monitor_at ON checks(monitor_id, at DESC);
CREATE INDEX IF NOT EXISTS idx_checks_at ON checks(at);

CREATE TABLE IF NOT EXISTS monitor_daily (
	monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
	day TEXT NOT NULL,
	total INTEGER NOT NULL DEFAULT 0,
	ok INTEGER NOT NULL DEFAULT 0,
	sum_latency INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (monitor_id, day)
);

CREATE TABLE IF NOT EXISTS incidents (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
	started_at INTEGER NOT NULL,
	ended_at INTEGER NOT NULL DEFAULT 0,
	cause TEXT NOT NULL DEFAULT '',
	resolved_by TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_incidents_monitor ON incidents(monitor_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_incidents_at ON incidents(started_at DESC);

CREATE TABLE IF NOT EXISTS rules (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	trigger_type TEXT NOT NULL,
	monitor_id INTEGER NOT NULL DEFAULT 0,
	threshold REAL NOT NULL DEFAULT 0,
	cooldown_sec INTEGER NOT NULL DEFAULT 1800,
	channels TEXT NOT NULL DEFAULT '["all"]',
	last_fired_at INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS agents (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	token_hash TEXT NOT NULL UNIQUE,
	hostname TEXT NOT NULL DEFAULT '',
	os TEXT NOT NULL DEFAULT '',
	arch TEXT NOT NULL DEFAULT '',
	version TEXT NOT NULL DEFAULT '',
	ip TEXT NOT NULL DEFAULT '',
	last_seen INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL,
	cpu_pct REAL NOT NULL DEFAULT 0,
	ram_pct REAL NOT NULL DEFAULT 0,
	disk_pct REAL NOT NULL DEFAULT 0,
	load1 REAL NOT NULL DEFAULT 0,
	uptime_sec INTEGER NOT NULL DEFAULT 0,
	net_rx_bps REAL NOT NULL DEFAULT 0,
	net_tx_bps REAL NOT NULL DEFAULT 0,
	ram_total INTEGER NOT NULL DEFAULT 0,
	disk_total INTEGER NOT NULL DEFAULT 0,
	services TEXT NOT NULL DEFAULT '[]',
	wireguard TEXT NOT NULL DEFAULT '[]',
	notes TEXT NOT NULL DEFAULT '',
	offline_notified INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS agent_metrics (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	agent_id INTEGER NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
	at INTEGER NOT NULL,
	cpu REAL NOT NULL DEFAULT 0,
	ram REAL NOT NULL DEFAULT 0,
	disk REAL NOT NULL DEFAULT 0,
	load1 REAL NOT NULL DEFAULT 0,
	net_rx_bps REAL NOT NULL DEFAULT 0,
	net_tx_bps REAL NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_agent_metrics ON agent_metrics(agent_id, at DESC);

CREATE TABLE IF NOT EXISTS push_subs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	endpoint TEXT NOT NULL UNIQUE,
	p256dh TEXT NOT NULL,
	auth TEXT NOT NULL,
	user_agent TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS notification_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	at INTEGER NOT NULL,
	channel TEXT NOT NULL,
	rule_name TEXT NOT NULL DEFAULT '',
	event TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL DEFAULT '',
	body TEXT NOT NULL DEFAULT '',
	ok INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_notification_at ON notification_log(at DESC);

CREATE TABLE IF NOT EXISTS audit_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	at INTEGER NOT NULL,
	actor TEXT NOT NULL DEFAULT '',
	action TEXT NOT NULL DEFAULT '',
	detail TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_audit_at ON audit_log(at DESC);

CREATE TABLE IF NOT EXISTS app_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	at INTEGER NOT NULL,
	level TEXT NOT NULL DEFAULT 'info',
	category TEXT NOT NULL DEFAULT 'panel',
	message TEXT NOT NULL,
	detail TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_app_logs_at ON app_logs(at DESC);
CREATE INDEX IF NOT EXISTS idx_app_logs_cat ON app_logs(category, at DESC);

CREATE TABLE IF NOT EXISTS sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	token_hash TEXT NOT NULL UNIQUE,
	user_id INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);
`

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// Lightweight upgrades for databases created by older releases.
	var hasOffline int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('agents') WHERE name='offline_notified'`).Scan(&hasOffline); err == nil && hasOffline == 0 {
		_, _ = s.db.Exec(`ALTER TABLE agents ADD COLUMN offline_notified INTEGER NOT NULL DEFAULT 0`)
	}
	var hasProbeAgent int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('monitors') WHERE name='probe_agent_id'`).Scan(&hasProbeAgent); err == nil && hasProbeAgent == 0 {
		_, _ = s.db.Exec(`ALTER TABLE monitors ADD COLUMN probe_agent_id INTEGER NOT NULL DEFAULT 0`)
	}
	var hasCFAccount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('monitors') WHERE name='cf_account_id'`).Scan(&hasCFAccount); err == nil && hasCFAccount == 0 {
		_, _ = s.db.Exec(`ALTER TABLE monitors ADD COLUMN cf_account_id INTEGER NOT NULL DEFAULT 0`)
	}
	now := nowUnix()
	// Seed the default rule set exactly once. User-deleted rules must stay deleted.
	var seeded string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key='rules_seeded'`).Scan(&seeded)
	if seeded == "" {
		var ruleCount int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM rules`).Scan(&ruleCount)
		seed := []struct {
			name, trigger, channels string
			threshold               float64
			cooldown                int
		}{
			{"🔴 هشدار قطعی سرویس", "down", `["all"]`, 0, 900},
			{"🟢 اطلاع بازیابی سرویس", "up", `["all"]`, 0, 300},
			{"📜 انقضای گواهی SSL", "cert_expiring", `["all"]`, 14, 86400},
			{"⚡ مصرف بالای CPU سرور", "cpu_high", `["all"]`, 90, 3600},
			{"🧠 مصرف بالای RAM سرور", "ram_high", `["all"]`, 90, 3600},
			{"💾 پر شدن فضای دیسک", "disk_high", `["all"]`, 90, 3600},
			{"📡 قطع ارتباط ایجنت", "agent_offline", `["all"]`, 0, 3600},
			{"🌐 اختلاف رکورد DNS", "dns_mismatch", `["all"]`, 0, 1800},
		}
		if ruleCount == 0 {
			for _, r := range seed {
				if _, err := s.db.Exec(`INSERT INTO rules(name, enabled, trigger_type, monitor_id, threshold, cooldown_sec, channels, created_at)
					VALUES(?, 1, ?, 0, ?, ?, ?, ?)`,
					r.name, r.trigger, r.threshold, r.cooldown, r.channels, now); err != nil {
					return fmt.Errorf("seed rule %s: %w", r.trigger, err)
				}
			}
		}
		_, _ = s.db.Exec(`INSERT INTO settings(key,value) VALUES('rules_seeded','1') ON CONFLICT(key) DO UPDATE SET value=excluded.value`)
	}
	return nil
}

// splitStatements is intentionally removed; schema is executed as one script.
