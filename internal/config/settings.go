package config

import (
	"crypto/rand"
	"encoding/base64"
	"runtime"
	"strconv"
	"strings"
)

// Settings is the application configuration persisted in SQLite.
type Settings struct {
	Timezone             string `json:"timezone"`
	PublicBaseURL        string `json:"public_base_url"`
	SiteTitle            string `json:"site_title"`
	DNSResolver          string `json:"dns_resolver"`
	CheckRetentionDays   int    `json:"check_retention_days"`
	MetricsRetentionDays int    `json:"metrics_retention_days"`
	NotifyRepeatMin      int    `json:"notify_repeat_min"`
	AgentOfflineAfterSec int    `json:"agent_offline_after_sec"`

	QuietHoursEnabled  bool   `json:"quiet_hours_enabled"`
	QuietStart         string `json:"quiet_start"`
	QuietEnd           string `json:"quiet_end"`
	QuietAllowCritical bool   `json:"quiet_allow_critical"`

	TelegramEnabled bool   `json:"telegram_enabled"`
	TelegramToken   string `json:"-"`
	TelegramChatID  string `json:"telegram_chat_id"`

	NotifyTitleTemplate string `json:"notify_title_template"`
	NotifyBodyTemplate  string `json:"notify_body_template"`

	WebPushEnabled bool `json:"webpush_enabled"`

	WebhookURL    string `json:"webhook_url"`
	WebhookSecret string `json:"-"`

	CloudflareEnabled bool   `json:"cloudflare_enabled"`
	CloudflareToken   string `json:"-"`

	AgentDownloadURL string `json:"agent_download_url"`

	XrayBinary  string `json:"xray_binary"`
	XrayTestURL string `json:"xray_test_url"`

	VAPIDPublic   string `json:"-"`
	VAPIDPrivate  string `json:"-"`
	SessionSecret string `json:"-"`
}

const defaultNotifyBodyTemplate = `{emoji} <b>{title}</b>

🖥 <b>{monitor}</b>
🧠 دلیل: {reason}
📋 جزئیات: {details}
🕒 {time}`

type reader interface {
	AllSettings() (map[string]string, error)
}

type writer interface {
	SetSettings(map[string]string) error
}

func Load(st reader, wr writer) (*Settings, error) {
	m, err := st.AllSettings()
	if err != nil {
		return nil, err
	}
	s := &Settings{
		Timezone:             "Asia/Tehran",
		SiteTitle:            "پالس",
		DNSResolver:          "1.1.1.1:53",
		CheckRetentionDays:   14,
		MetricsRetentionDays: 30,
		NotifyRepeatMin:      45,
		AgentOfflineAfterSec: 120,
		QuietStart:           "00:00",
		QuietEnd:             "07:00",
		QuietAllowCritical:   true,
		WebPushEnabled:       true,
		AgentDownloadURL:     defaultAgentDownloadURL(),
		XrayBinary:           "xray",
		XrayTestURL:          "https://www.gstatic.com/generate_204",
		NotifyTitleTemplate:  "{emoji} {title}",
		NotifyBodyTemplate:   defaultNotifyBodyTemplate,
	}
	s.Timezone = val(m, "timezone", s.Timezone)
	s.PublicBaseURL = val(m, "public_base_url", "")
	s.SiteTitle = val(m, "site_title", s.SiteTitle)
	s.DNSResolver = val(m, "dns_resolver", s.DNSResolver)
	s.CheckRetentionDays = valInt(m, "check_retention_days", s.CheckRetentionDays)
	s.MetricsRetentionDays = valInt(m, "metrics_retention_days", s.MetricsRetentionDays)
	s.NotifyRepeatMin = valInt(m, "notify_repeat_min", s.NotifyRepeatMin)
	s.AgentOfflineAfterSec = valInt(m, "agent_offline_after_sec", s.AgentOfflineAfterSec)
	s.QuietHoursEnabled = valBool(m, "quiet_hours_enabled", false)
	s.QuietStart = val(m, "quiet_start", s.QuietStart)
	s.QuietEnd = val(m, "quiet_end", s.QuietEnd)
	s.QuietAllowCritical = valBool(m, "quiet_allow_critical", true)
	s.TelegramEnabled = valBool(m, "telegram_enabled", false)
	s.TelegramToken = m["telegram_token"]
	s.TelegramChatID = m["telegram_chat_id"]
	s.NotifyTitleTemplate = m["notify_title_template"]
	s.NotifyBodyTemplate = m["notify_body_template"]
	if s.NotifyTitleTemplate == "" {
		s.NotifyTitleTemplate = "{emoji} {title}"
	}
	if s.NotifyBodyTemplate == "" {
		s.NotifyBodyTemplate = defaultNotifyBodyTemplate
	}
	s.WebPushEnabled = valBool(m, "webpush_enabled", true)
	s.WebhookURL = m["webhook_url"]
	s.WebhookSecret = m["webhook_secret"]
	s.CloudflareEnabled = valBool(m, "cloudflare_enabled", false)
	s.CloudflareToken = m["cloudflare_token"]
	if v, ok := m["agent_download_url"]; ok {
		s.AgentDownloadURL = v
	}
	s.XrayBinary = val(m, "xray_binary", s.XrayBinary)
	s.XrayTestURL = val(m, "xray_test_url", s.XrayTestURL)
	s.VAPIDPublic = m["vapid_public"]
	s.VAPIDPrivate = m["vapid_private"]
	s.SessionSecret = m["session_secret"]

	changed := map[string]string{}
	if s.SessionSecret == "" {
		s.SessionSecret = randomSecret(32)
		changed["session_secret"] = s.SessionSecret
	}
	if len(changed) > 0 {
		_ = wr.SetSettings(changed)
	}
	return s, nil
}

// ApplyEnv lets deployment tools provide secrets without writing them to the UI.
func (s *Settings) ApplyEnv(telegramToken, telegramChat, cfToken, webhookURL string) {
	if telegramToken != "" {
		s.TelegramToken = telegramToken
	}
	if telegramChat != "" {
		s.TelegramChatID = telegramChat
	}
	if cfToken != "" {
		s.CloudflareToken = cfToken
	}
	if webhookURL != "" {
		s.WebhookURL = webhookURL
	}
}

func (s *Settings) Persist(wr writer) error {
	return wr.SetSettings(map[string]string{
		"timezone":                s.Timezone,
		"public_base_url":         s.PublicBaseURL,
		"site_title":              s.SiteTitle,
		"dns_resolver":            s.DNSResolver,
		"check_retention_days":    itoa(s.CheckRetentionDays),
		"metrics_retention_days":  itoa(s.MetricsRetentionDays),
		"notify_repeat_min":       itoa(s.NotifyRepeatMin),
		"agent_offline_after_sec": itoa(s.AgentOfflineAfterSec),
		"quiet_hours_enabled":     boolStr(s.QuietHoursEnabled),
		"quiet_start":             s.QuietStart,
		"quiet_end":               s.QuietEnd,
		"quiet_allow_critical":    boolStr(s.QuietAllowCritical),
		"telegram_enabled":        boolStr(s.TelegramEnabled),
		"telegram_token":          s.TelegramToken,
		"telegram_chat_id":        s.TelegramChatID,
		"notify_title_template":   s.NotifyTitleTemplate,
		"notify_body_template":    s.NotifyBodyTemplate,
		"webpush_enabled":         boolStr(s.WebPushEnabled),
		"webhook_url":             s.WebhookURL,
		"webhook_secret":          s.WebhookSecret,
		"cloudflare_enabled":      boolStr(s.CloudflareEnabled),
		"cloudflare_token":        s.CloudflareToken,
		"agent_download_url":      s.AgentDownloadURL,
		"xray_binary":             s.XrayBinary,
		"xray_test_url":           s.XrayTestURL,
		"vapid_public":            s.VAPIDPublic,
		"vapid_private":           s.VAPIDPrivate,
		"session_secret":          s.SessionSecret,
	})
}

func defaultAgentDownloadURL() string {
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return "https://github.com/AmiwrM/Pulse/releases/latest/download/pulse-linux-" + runtime.GOARCH
	default:
		return ""
	}
}

func randomSecret(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func val(m map[string]string, k, def string) string {
	if v, ok := m[k]; ok && v != "" {
		return v
	}
	return def
}

func valInt(m map[string]string, k string, def int) int {
	v, ok := m[k]
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func valBool(m map[string]string, k string, def bool) bool {
	v, ok := m[k]
	if !ok {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func itoa(n int) string { return strconv.Itoa(n) }
