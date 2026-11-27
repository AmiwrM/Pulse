package model

import "encoding/json"

// Monitor is a single watch target (HTTP, TCP, ping, DNS, TLS or proxy config).
type Monitor struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Target   string `json:"target"`
	Group    string `json:"group"`
	Interval int    `json:"interval_sec"`
	Timeout  int    `json:"timeout_ms"`
	Enabled  bool   `json:"enabled"`

	Config json.RawMessage `json:"config"`

	ProbeAgentID int64 `json:"probe_agent_id"`

	Status         string `json:"status"`
	LastCheckedAt  int64  `json:"last_checked_at"`
	LastLatencyMS  int64  `json:"last_latency_ms"`
	LastError      string `json:"last_error"`
	LastStatusCode int    `json:"last_status_code"`
	SSLExpiresAt   int64  `json:"ssl_expires_at"`

	ConsecutiveFails int   `json:"consecutive_fails"`
	ConsecutiveOks   int   `json:"consecutive_oks"`
	DownSince        int64 `json:"down_since"`
	LastNotifiedAt   int64 `json:"last_notified_at"`

	FailThreshold    int `json:"fail_threshold"`
	RecoverThreshold int `json:"recover_threshold"`

	CFEnabled      bool   `json:"cf_enabled"`
	CFAccountID    int64  `json:"cf_account_id"`
	CFZoneID       string `json:"cf_zone_id"`
	CFRecordID     string `json:"cf_record_id"`
	CFRecordName   string `json:"cf_record_name"`
	CFPrimary      string `json:"cf_primary"`
	CFFailover     string `json:"cf_failover"`
	CFLastAction   string `json:"cf_last_action"`
	CFLastActionAt int64  `json:"cf_last_action_at"`

	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`

	// Computed fields, not persisted.
	ProbeAgentName string    `json:"probe_agent_name,omitempty"`
	Uptime24       float64   `json:"uptime_24h"`
	AvgLatency     int64     `json:"avg_latency_ms"`
	Spark          []int64   `json:"spark"`
	ActiveIncident *Incident `json:"active_incident,omitempty"`
}

// Check is one probe result.
type Check struct {
	ID         int64           `json:"id"`
	MonitorID  int64           `json:"monitor_id"`
	At         int64           `json:"at"`
	OK         bool            `json:"ok"`
	Status     string          `json:"status"`
	LatencyMS  int64           `json:"latency_ms"`
	StatusCode int             `json:"status_code"`
	Error      string          `json:"error"`
	Meta       json.RawMessage `json:"meta"`
}

type DailyStat struct {
	Day        string  `json:"day"`
	Total      int     `json:"total"`
	OK         int     `json:"ok"`
	AvgLatency int64   `json:"avg_latency_ms"`
	Uptime     float64 `json:"uptime"`
}

type Incident struct {
	ID          int64  `json:"id"`
	MonitorID   int64  `json:"monitor_id"`
	MonitorName string `json:"monitor_name"`
	StartedAt   int64  `json:"started_at"`
	EndedAt     int64  `json:"ended_at"`
	Cause       string `json:"cause"`
	ResolvedBy  string `json:"resolved_by"`
	Duration    int64  `json:"duration_sec"`
}

type Rule struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Trigger     string   `json:"trigger"`
	MonitorID   int64    `json:"monitor_id"`
	Threshold   float64  `json:"threshold"`
	CooldownSec int      `json:"cooldown_sec"`
	Channels    []string `json:"channels"`
	LastFiredAt int64    `json:"last_fired_at"`
	CreatedAt   int64    `json:"created_at"`
}

type Agent struct {
	ID              int64           `json:"id"`
	Name            string          `json:"name"`
	Hostname        string          `json:"hostname"`
	OS              string          `json:"os"`
	Arch            string          `json:"arch"`
	Version         string          `json:"version"`
	IP              string          `json:"ip"`
	LastSeen        int64           `json:"last_seen"`
	CreatedAt       int64           `json:"created_at"`
	CPUPct          float64         `json:"cpu_pct"`
	RAMPct          float64         `json:"ram_pct"`
	DiskPct         float64         `json:"disk_pct"`
	Load1           float64         `json:"load1"`
	UptimeSec       int64           `json:"uptime_sec"`
	NetRx           float64         `json:"net_rx_bps"`
	NetTx           float64         `json:"net_tx_bps"`
	RAMTotal        uint64          `json:"ram_total"`
	DiskTotal       uint64          `json:"disk_total"`
	Services        json.RawMessage `json:"services"`
	WireGuard       json.RawMessage `json:"wireguard"`
	Notes           string          `json:"notes"`
	OfflineNotified bool            `json:"-"`

	// Live-link fields, populated for API responses. They are not persisted
	// because the link state is inherently ephemeral.
	Connected     bool   `json:"connected"`
	Transport     string `json:"transport,omitempty"`
	Protocol      int    `json:"protocol,omitempty"`
	CanCheck      bool   `json:"can_check"`
	XrayAvailable bool   `json:"xray_available"`
	XrayVersion   string `json:"xray_version,omitempty"`
	ConnectedAt   int64  `json:"connected_at,omitempty"`
}

type AgentMetric struct {
	At    int64   `json:"at"`
	CPU   float64 `json:"cpu"`
	RAM   float64 `json:"ram"`
	Disk  float64 `json:"disk"`
	Load1 float64 `json:"load1"`
	NetRx float64 `json:"net_rx_bps"`
	NetTx float64 `json:"net_tx_bps"`
}

type AgentReport struct {
	Hostname  string          `json:"hostname"`
	OS        string          `json:"os"`
	Arch      string          `json:"arch"`
	Version   string          `json:"version"`
	IP        string          `json:"ip"`
	CPUPct    float64         `json:"cpu_pct"`
	RAMPct    float64         `json:"ram_pct"`
	DiskPct   float64         `json:"disk_pct"`
	Load1     float64         `json:"load1"`
	UptimeSec int64           `json:"uptime_sec"`
	NetRx     float64         `json:"net_rx_bps"`
	NetTx     float64         `json:"net_tx_bps"`
	RAMTotal  uint64          `json:"ram_total"`
	DiskTotal uint64          `json:"disk_total"`
	Services  json.RawMessage `json:"services"`
	WireGuard json.RawMessage `json:"wireguard"`
}

type PushSub struct {
	ID        int64  `json:"id"`
	Endpoint  string `json:"endpoint"`
	P256DH    string `json:"p256dh"`
	Auth      string `json:"auth"`
	UserAgent string `json:"user_agent"`
	CreatedAt int64  `json:"created_at"`
}

type NotificationLog struct {
	ID       int64  `json:"id"`
	At       int64  `json:"at"`
	Channel  string `json:"channel"`
	RuleName string `json:"rule_name"`
	Event    string `json:"event"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	OK       bool   `json:"ok"`
	Error    string `json:"error"`
}

type AuditEntry struct {
	ID     int64  `json:"id"`
	At     int64  `json:"at"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

// LogEntry is one structured application log. Category separates the panel,
// monitoring, Xray, Cloudflare, notification, agent, security and API streams.
type LogEntry struct {
	ID       int64  `json:"id"`
	At       int64  `json:"at"`
	Level    string `json:"level"`
	Category string `json:"category"`
	Message  string `json:"message"`
	Detail   string `json:"detail"`
}

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	CreatedAt    int64  `json:"created_at"`
}

type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type DNSRecord struct {
	ID       string `json:"id"`
	ZoneID   string `json:"zone_id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  bool   `json:"proxied"`
	Priority int    `json:"priority,omitempty"`
}

// CFAccount is one Cloudflare API token. The token is never returned in JSON.
type CFAccount struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Token     string `json:"-"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type PushSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}
