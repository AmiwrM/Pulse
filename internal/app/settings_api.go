package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"pulse/internal/config"
)

func (a *App) settingsView() map[string]any {
	conf := a.config()
	pushCount, _ := a.db.CountPushSubs()
	monitors, checks, agents, dbBytes, _ := a.db.Stats()
	return map[string]any{
		"settings":              conf,
		"telegram_configured":   conf.TelegramToken != "",
		"cloudflare_configured": conf.CloudflareToken != "",
		"webhook_configured":    conf.WebhookURL != "",
		"vapid_public":          conf.VAPIDPublic,
		"push_subscriptions":    pushCount,
		"xray":                  a.xray.Status(),
		"system": map[string]any{
			"version":     a.cfg.Version,
			"go_version":  runtime.Version(),
			"os":          runtime.GOOS,
			"arch":        runtime.GOARCH,
			"started_at":  a.start.Unix(),
			"uptime_sec":  int64(time.Since(a.start).Seconds()),
			"db_bytes":    dbBytes,
			"monitors":    monitors,
			"checks":      checks,
			"agents":      agents,
			"server_time": nowUnix(),
		},
	}
}

func (a *App) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeData(w, a.settingsView())
}

func (a *App) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	current := a.config()
	next := *current

	if v, ok := rawString(body, "timezone"); ok {
		next.Timezone = strings.TrimSpace(v)
	}
	if v, ok := rawString(body, "site_title"); ok {
		next.SiteTitle = strings.TrimSpace(v)
	}
	if v, ok := rawString(body, "public_base_url"); ok {
		next.PublicBaseURL = strings.TrimRight(strings.TrimSpace(v), "/")
	}
	if v, ok := rawString(body, "dns_resolver"); ok {
		next.DNSResolver = strings.TrimSpace(v)
	}
	if n, ok := rawInt(body, "check_retention_days"); ok {
		next.CheckRetentionDays = n
	}
	if n, ok := rawInt(body, "metrics_retention_days"); ok {
		next.MetricsRetentionDays = n
	}
	if n, ok := rawInt(body, "notify_repeat_min"); ok {
		next.NotifyRepeatMin = n
	}
	if n, ok := rawInt(body, "agent_offline_after_sec"); ok {
		next.AgentOfflineAfterSec = n
	}
	if b, ok := rawBool(body, "quiet_hours_enabled"); ok {
		next.QuietHoursEnabled = b
	}
	if v, ok := rawString(body, "quiet_start"); ok {
		next.QuietStart = strings.TrimSpace(v)
	}
	if v, ok := rawString(body, "quiet_end"); ok {
		next.QuietEnd = strings.TrimSpace(v)
	}
	if b, ok := rawBool(body, "quiet_allow_critical"); ok {
		next.QuietAllowCritical = b
	}
	if b, ok := rawBool(body, "telegram_enabled"); ok {
		next.TelegramEnabled = b
	}
	if v, ok := rawString(body, "telegram_chat_id"); ok {
		next.TelegramChatID = strings.TrimSpace(v)
	}
	if b, ok := rawBool(body, "webpush_enabled"); ok {
		next.WebPushEnabled = b
	}
	if v, ok := rawString(body, "webhook_url"); ok {
		next.WebhookURL = strings.TrimSpace(v)
	}
	if b, ok := rawBool(body, "cloudflare_enabled"); ok {
		next.CloudflareEnabled = b
	}
	if v, ok := rawString(body, "agent_download_url"); ok {
		next.AgentDownloadURL = strings.TrimSpace(v)
	}
	if v, ok := rawString(body, "xray_binary"); ok {
		next.XrayBinary = strings.TrimSpace(v)
	}
	if v, ok := rawString(body, "xray_test_url"); ok {
		next.XrayTestURL = strings.TrimSpace(v)
	}
	applySecret(body, "telegram_token", &next.TelegramToken)
	applySecret(body, "webhook_secret", &next.WebhookSecret)
	applySecret(body, "cloudflare_token", &next.CloudflareToken)

	if err := validateSettings(&next); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := next.Persist(a.db); err != nil {
		writeErr(w, 500, "ذخیره تنظیمات ناموفق بود")
		return
	}
	a.setConfig(&next)
	a.probes.SetDNS(next.DNSResolver)
	a.probes.SetProxyTestURL(next.XrayTestURL)
	if next.XrayBinary != current.XrayBinary {
		a.xray.SetBinary(next.XrayBinary)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			status := a.xray.Detect(ctx)
			a.hub.broadcast("xray.status", status)
		}()
	}
	a.db.AddAudit("admin", "settings.update", "تنظیمات سامانه به‌روزرسانی شد")
	a.logFor("panel").Info("settings updated", "ip", clientIP(r))
	a.hub.broadcast("settings.updated", map[string]any{})
	writeData(w, a.settingsView())
}

func validateSettings(s *config.Settings) error {
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("منطقه زمانی نامعتبر است")
	}
	s.SiteTitle = strings.TrimSpace(s.SiteTitle)
	if s.SiteTitle == "" {
		s.SiteTitle = "پالس"
	}
	if len([]rune(s.SiteTitle)) > 50 {
		return fmt.Errorf("عنوان سامانه طولانی است")
	}
	if s.PublicBaseURL != "" {
		u, err := url.Parse(s.PublicBaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("آدرس عمومی سامانه معتبر نیست")
		}
	}
	if s.WebhookURL != "" {
		u, err := url.Parse(s.WebhookURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("آدرس وب‌هوک معتبر نیست")
		}
	}
	if s.DNSResolver != "" {
		host := s.DNSResolver
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if strings.ContainsAny(host, " /") || host == "" {
			return fmt.Errorf("آدرس DNS Resolver معتبر نیست")
		}
	}
	if s.CheckRetentionDays < 1 {
		s.CheckRetentionDays = 1
	}
	if s.CheckRetentionDays > 365 {
		s.CheckRetentionDays = 365
	}
	if s.MetricsRetentionDays < 1 {
		s.MetricsRetentionDays = 1
	}
	if s.MetricsRetentionDays > 365 {
		s.MetricsRetentionDays = 365
	}
	if s.NotifyRepeatMin < 0 {
		s.NotifyRepeatMin = 0
	}
	if s.NotifyRepeatMin > 10080 {
		s.NotifyRepeatMin = 10080
	}
	if s.AgentOfflineAfterSec < 60 {
		s.AgentOfflineAfterSec = 60
	}
	if s.AgentOfflineAfterSec > 3600 {
		s.AgentOfflineAfterSec = 3600
	}
	if s.QuietHoursEnabled {
		if _, ok := parseClock(s.QuietStart); !ok {
			return fmt.Errorf("ساعت شروع سکوت معتبر نیست (مثال: 23:30)")
		}
		if _, ok := parseClock(s.QuietEnd); !ok {
			return fmt.Errorf("ساعت پایان سکوت معتبر نیست (مثال: 07:00)")
		}
	}
	s.XrayBinary = strings.TrimSpace(s.XrayBinary)
	if s.XrayBinary == "" {
		s.XrayBinary = "xray"
	}
	s.XrayTestURL = strings.TrimSpace(s.XrayTestURL)
	if s.XrayTestURL != "" {
		u, err := url.Parse(s.XrayTestURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("آدرس تست واقعی Xray معتبر نیست")
		}
	}
	return nil
}

func applySecret(body map[string]json.RawMessage, key string, target *string) {
	if _, ok := body[key+"_clear"]; ok {
		if b, ok := rawBool(body, key+"_clear"); ok && b {
			*target = ""
			return
		}
	}
	if v, ok := rawString(body, key); ok && strings.TrimSpace(v) != "" {
		*target = strings.TrimSpace(v)
	}
}

func rawString(m map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := m[key]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func rawBool(m map[string]json.RawMessage, key string) (bool, bool) {
	raw, ok := m[key]
	if !ok {
		return false, false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, false
	}
	return b, true
}

func rawInt(m map[string]json.RawMessage, key string) (int, bool) {
	raw, ok := m[key]
	if !ok {
		return 0, false
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		var s string
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return 0, false
		}
		return 0, false
	}
	return int(f), true
}

func (a *App) publicBase(r *http.Request) string {
	if base := strings.TrimSpace(a.config().PublicBaseURL); base != "" {
		return strings.TrimRight(base, "/")
	}
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	host := r.Host
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		host = strings.TrimSpace(strings.Split(fh, ",")[0])
	}
	return scheme + "://" + host
}

func (a *App) handleBackup(w http.ResponseWriter, r *http.Request) {
	monitors, _ := a.db.ListMonitors()
	rules, _ := a.db.ListRules()
	agents, _ := a.db.ListAgents()
	incidents, _ := a.db.ListIncidents(500, false)
	payload := map[string]any{
		"app":         "pulse",
		"version":     a.cfg.Version,
		"exported_at": nowUnix(),
		"settings":    a.config(),
		"monitors":    monitors,
		"rules":       rules,
		"agents":      agents,
		"incidents":   incidents,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pulse-backup-`+time.Now().Format("20060102-150405")+`.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
	a.db.AddAudit("admin", "backup.download", "تهیه نسخه پشتیبان JSON")
}
