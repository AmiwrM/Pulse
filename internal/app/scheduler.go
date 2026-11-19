package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"pulse/internal/model"
)

func (a *App) scheduler(ctx context.Context) {
	monitorTicker := time.NewTicker(2 * time.Second)
	agentTicker := time.NewTicker(20 * time.Second)
	houseTicker := time.NewTicker(time.Hour)
	defer monitorTicker.Stop()
	defer agentTicker.Stop()
	defer houseTicker.Stop()

	a.runCleanup()
	for {
		select {
		case <-ctx.Done():
			return
		case <-monitorTicker.C:
			a.tickMonitors()
		case <-agentTicker.C:
			a.tickAgents()
		case <-houseTicker.C:
			a.runCleanup()
		}
	}
}

func (a *App) tickMonitors() {
	monitors, err := a.db.ListEnabledMonitors()
	if err != nil {
		a.logger.Error("list monitors", "error", err)
		return
	}
	now := nowUnix()
	for _, m := range monitors {
		interval := int64(m.Interval)
		if interval < 10 {
			interval = 10
		}
		if m.LastCheckedAt > 0 && now-m.LastCheckedAt < interval {
			continue
		}
		a.startCheck(m)
	}
}

func (a *App) checkOne(ctx context.Context, m model.Monitor) {
	res := a.probes.Run(ctx, m)
	now := time.Now().Unix()
	before := m.Status
	if m.FailThreshold <= 0 {
		m.FailThreshold = 2
	}
	if m.RecoverThreshold <= 0 {
		m.RecoverThreshold = 1
	}
	openIncident := false
	closeIncident := false
	downSec := int64(0)

	if res.OK {
		m.ConsecutiveOks++
		m.ConsecutiveFails = 0
		if before == "down" {
			if m.ConsecutiveOks >= m.RecoverThreshold {
				m.Status = "up"
				closeIncident = true
				downSec = now - m.DownSince
				if downSec < 0 {
					downSec = 0
				}
				m.DownSince = 0
			} else {
				m.Status = "down"
			}
		} else {
			m.Status = "up"
		}
	} else {
		m.ConsecutiveFails++
		m.ConsecutiveOks = 0
		if before == "down" {
			m.Status = "down"
		} else if m.ConsecutiveFails >= m.FailThreshold {
			m.Status = "down"
			m.DownSince = now
			openIncident = true
		} else if before == "up" || before == "degraded" {
			// Keep the previous healthy/warning state until the failure threshold is reached.
			m.Status = before
		} else {
			m.Status = "unknown"
		}
	}

	if res.OK && res.Warn != "" && m.Status == "up" {
		m.Status = "degraded"
	}

	m.LastCheckedAt = now
	m.LastLatencyMS = res.LatencyMS
	m.LastError = res.Error
	m.LastStatusCode = res.StatusCode
	if res.SSLExpiresAt > 0 {
		m.SSLExpiresAt = res.SSLExpiresAt
	}

	meta := res.Meta
	if meta == nil {
		meta = map[string]any{}
	}
	meta["warn"] = res.Warn
	meta["duration_ms"] = res.LatencyMS
	meta["consecutive_fails"] = m.ConsecutiveFails
	meta["consecutive_oks"] = m.ConsecutiveOks
	metaRaw, _ := json.Marshal(meta)

	check := model.Check{
		MonitorID: m.ID, At: now, OK: res.OK, Status: m.Status,
		LatencyMS: res.LatencyMS, StatusCode: res.StatusCode, Error: res.Error, Meta: metaRaw,
	}
	if err := a.db.CommitCheck(&m, &check, openIncident, closeIncident); err != nil {
		a.logger.Error("commit check", "monitor", m.ID, "error", err)
		return
	}

	a.hub.broadcast("check", map[string]any{
		"monitor_id": m.ID, "status": m.Status, "ok": res.OK,
		"latency_ms": res.LatencyMS, "error": res.Error, "at": now,
	})

	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer notifyCancel()
	if openIncident {
		a.dispatchEvent(notifyCtx, notifyEvent{Trigger: "down", Monitor: &m, Force: true})
		if dnsMismatch(meta) {
			a.dispatchEvent(notifyCtx, notifyEvent{Trigger: "dns_mismatch", Monitor: &m, Force: true, Extra: meta})
		}
		if m.CFEnabled {
			go a.applyCFFailover(m, "failover")
		}
	}
	if closeIncident {
		if downSec > 0 {
			a.dispatchEvent(notifyCtx, notifyEvent{Trigger: "up", Monitor: &m, Force: true, Extra: map[string]any{"down_sec": downSec}})
		}
		if m.CFEnabled {
			go a.applyCFFailover(m, "restore")
		}
	}

	conf := a.config()
	if m.Status == "down" && !openIncident && conf.NotifyRepeatMin > 0 {
		if m.LastNotifiedAt == 0 || now-m.LastNotifiedAt >= int64(conf.NotifyRepeatMin)*60 {
			a.dispatchEvent(notifyCtx, notifyEvent{Trigger: "down", Monitor: &m})
		}
	}
	if res.OK && m.SSLExpiresAt > 0 {
		days := (m.SSLExpiresAt - now) / 86400
		if days >= 0 {
			mCopy := m
			a.dispatchEvent(notifyCtx, notifyEvent{Trigger: "cert_expiring", Monitor: &mCopy, Value: float64(days)})
		}
	}
}

func dnsMismatch(meta map[string]any) bool {
	if meta == nil {
		return false
	}
	v, ok := meta["dns_mismatch"].(bool)
	return ok && v
}

func (a *App) tickAgents() {
	conf := a.config()
	after := conf.AgentOfflineAfterSec
	if after < 60 {
		after = 60
	}
	cutoff := nowUnix() - int64(after)
	agents, err := a.db.OfflineAgents(cutoff)
	if err != nil {
		return
	}
	for _, ag := range agents {
		if !a.db.ClaimAgentOffline(ag.ID) {
			continue
		}
		copyAgent := ag
		notifyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		a.dispatchEvent(notifyCtx, notifyEvent{Trigger: "agent_offline", Agent: &copyAgent, Force: true})
		cancel()
		a.hub.broadcast("agent.offline", map[string]any{"agent_id": ag.ID, "last_seen": ag.LastSeen})
	}
}

func (a *App) runCleanup() {
	conf := a.config()
	a.db.Cleanup(conf.CheckRetentionDays, conf.MetricsRetentionDays)
}

func (a *App) handleAgentReport(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-Agent-Token")
	if token == "" {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token = strings.TrimSpace(auth[7:])
		}
	}
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "توکن ایجنت ارسال نشده است")
		return
	}
	ag, err := a.db.GetAgentByToken(token)
	if err != nil || ag == nil {
		writeErr(w, http.StatusUnauthorized, "توکن ایجنت معتبر نیست")
		return
	}
	var report model.AgentReport
	if err := decodeJSON(r, &report); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	report.IP = clientIP(r)
	if report.Hostname == "" {
		report.Hostname = ag.Name
	}
	if err := a.db.RecordAgentReport(ag.ID, report); err != nil {
		a.logger.Error("agent report", "agent", ag.ID, "error", err)
		writeErr(w, 500, "ذخیره گزارش ناموفق بود")
		return
	}

	ag.Hostname = report.Hostname
	ag.OS = report.OS
	ag.Arch = report.Arch
	ag.Version = report.Version
	ag.IP = report.IP
	ag.LastSeen = nowUnix()
	ag.CPUPct = report.CPUPct
	ag.RAMPct = report.RAMPct
	ag.DiskPct = report.DiskPct
	ag.Load1 = report.Load1
	ag.UptimeSec = report.UptimeSec
	ag.NetRx = report.NetRx
	ag.NetTx = report.NetTx
	ag.RAMTotal = report.RAMTotal
	ag.DiskTotal = report.DiskTotal
	ag.Services = report.Services
	ag.WireGuard = report.WireGuard

	a.hub.broadcast("agent.updated", map[string]any{
		"agent_id": ag.ID, "cpu": ag.CPUPct, "ram": ag.RAMPct, "disk": ag.DiskPct, "last_seen": ag.LastSeen,
	})
	// Threshold rules are evaluated on every report. The rule engine applies the
	// per-rule threshold and cooldown, so lower thresholds than 80% still work.
	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer notifyCancel()
	for _, item := range []struct {
		trigger string
		value   float64
	}{
		{"cpu_high", report.CPUPct}, {"ram_high", report.RAMPct}, {"disk_high", report.DiskPct},
	} {
		v := item.value
		copyAgent := ag
		a.dispatchEvent(notifyCtx, notifyEvent{Trigger: item.trigger, Agent: copyAgent, Value: v})
	}
	writeData(w, map[string]any{"ok": true, "server_time": nowUnix()})
}
