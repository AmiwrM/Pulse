package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"pulse/internal/checker"
	"pulse/internal/model"
)

func (a *App) scheduler(ctx context.Context) {
	monitorTicker := time.NewTicker(2 * time.Second)
	agentTicker := time.NewTicker(5 * time.Second)
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
		a.logFor("scheduler").Error("list monitors", "error", err)
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

func (a *App) runMonitor(ctx context.Context, m model.Monitor) checker.Result {
	if m.ProbeAgentID > 0 {
		return a.runRemoteMonitor(ctx, m)
	}
	return a.probes.Run(ctx, m)
}

func (a *App) checkOne(ctx context.Context, m model.Monitor) {
	res := a.runMonitor(ctx, m)
	now := time.Now().Unix()
	if res.ProbeFailed {
		a.recordProbeFailure(m, res, now)
		return
	}
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
		a.logFor("monitor").Error("commit check", "monitor", m.ID, "error", err)
		return
	}
	if openIncident {
		a.logFor("monitor").Warn("monitor down", "monitor", m.Name, "target", m.Target, "error", res.Error, "latency_ms", res.LatencyMS)
		if m.Type == "proxy" {
			a.logFor("xray").Warn("proxy check failed", "monitor", m.Name)
		}
	}
	if closeIncident {
		a.logFor("monitor").Info("monitor recovered", "monitor", m.Name, "downtime_sec", downSec)
		if m.Type == "proxy" {
			a.logFor("xray").Info("proxy check recovered", "monitor", m.Name)
		}
	}
	if res.OK && res.Warn != "" && before != "degraded" {
		a.logFor("monitor").Warn("monitor warning", "monitor", m.Name, "warn", res.Warn)
	}

	a.hub.broadcast("check", map[string]any{
		"monitor_id": m.ID, "status": m.Status, "ok": res.OK,
		"latency_ms": res.LatencyMS, "error": res.Error, "at": now,
	})

	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer notifyCancel()
	notifyExtra := map[string]any{}
	switch list := meta["failed_configs"].(type) {
	case []string:
		if len(list) > 0 {
			notifyExtra["failed_configs"] = strings.Join(list, "\n")
		}
	case []any:
		failed := make([]string, 0, len(list))
		for _, item := range list {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				failed = append(failed, s)
			}
		}
		if len(failed) > 0 {
			notifyExtra["failed_configs"] = strings.Join(failed, "\n")
		}
	}
	if strings.TrimSpace(res.Warn) != "" {
		notifyExtra["warn"] = res.Warn
	}
	if openIncident {
		a.dispatchEvent(notifyCtx, notifyEvent{Trigger: "down", Monitor: &m, Force: true, Extra: notifyExtra})
		if dnsMismatch(meta) {
			a.dispatchEvent(notifyCtx, notifyEvent{Trigger: "dns_mismatch", Monitor: &m, Force: true, Extra: meta})
		}
		if m.CFEnabled {
			go a.applyCFFailover(m, "failover")
		}
	}
	if m.Status == "degraded" && before != "degraded" {
		a.dispatchEvent(notifyCtx, notifyEvent{Trigger: "degraded", Monitor: &m, Extra: notifyExtra})
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

// recordProbeFailure persists a check that could not run because its selected
// agent is unavailable. It never opens an incident: a broken probe is not proof
// that the target is down.
func (a *App) recordProbeFailure(m model.Monitor, res checker.Result, now int64) {
	if m.Status != "down" {
		m.Status = "degraded"
	}
	m.LastCheckedAt = now
	m.LastLatencyMS = 0
	m.LastStatusCode = 0
	m.LastError = res.Error
	meta := res.Meta
	if meta == nil {
		meta = map[string]any{}
	}
	meta["probe_infra"] = true
	meta["duration_ms"] = 0
	metaRaw, _ := json.Marshal(meta)
	check := model.Check{
		MonitorID: m.ID, At: now, OK: false, Status: m.Status,
		Error: res.Error, Meta: metaRaw,
	}
	if err := a.db.CommitCheck(&m, &check, false, false); err != nil {
		a.logFor("monitor").Error("commit probe failure", "monitor", m.ID, "error", err)
		return
	}
	a.logFor("monitor").Warn("probe agent unavailable", "monitor", m.Name, "error", res.Error)
	a.hub.broadcast("check", map[string]any{
		"monitor_id": m.ID, "status": m.Status, "ok": false,
		"latency_ms": 0, "error": res.Error, "at": now, "probe_failed": true,
	})
}

func dnsMismatch(meta map[string]any) bool {
	if meta == nil {
		return false
	}
	v, ok := meta["dns_mismatch"].(bool)
	return ok && v
}

func (a *App) tickAgents() {
	now := nowUnix()
	// Reports arrive every second; a live link with no protocol message for
	// 30 seconds is stale even if the TCP connection still looks open.
	for _, sess := range a.links.stale(now - 30) {
		if a.links.remove(sess) {
			a.onAgentLinkDown(sess)
			sess.shutdown(errors.New("agent link stopped sending telemetry"))
		}
	}
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
	// Keep the dedicated logs page useful without letting it grow forever.
	a.db.CleanupLogs(nowUnix()-14*86400, 20000)
}

func (a *App) handleAgentReport(w http.ResponseWriter, r *http.Request) {
	token := agentToken(r)
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
	if err := a.applyAgentReport(ag.ID, report, "REST"); err != nil {
		a.logger.Error("agent report", "agent", ag.ID, "error", err)
		writeErr(w, 500, "ذخیره گزارش ناموفق بود")
		return
	}
	writeData(w, map[string]any{"ok": true, "server_time": nowUnix()})
}

// applyAgentReport is shared by the REST endpoint and the persistent link so
// both transports produce identical database state, SSE updates and rules.
func (a *App) applyAgentReport(agentID int64, report model.AgentReport, transport string) error {
	ag, err := a.db.GetAgent(agentID)
	if err != nil {
		return err
	}
	if ag == nil {
		return errors.New("agent not found")
	}
	if report.Hostname == "" {
		report.Hostname = ag.Name
	}
	if err := a.db.RecordAgentReport(agentID, report); err != nil {
		a.logFor("agent").Error("store agent report", "agent", agentID, "error", err)
		return err
	}
	if transport == "" {
		transport = "REST"
	}
	if transport == "REST" {
		a.presenceMu.Lock()
		a.restSeen[agentID] = nowUnix()
		a.presenceMu.Unlock()
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
		"agent_id": ag.ID, "name": ag.Name, "hostname": ag.Hostname, "os": ag.OS, "ip": ag.IP,
		"cpu": ag.CPUPct, "ram": ag.RAMPct, "disk": ag.DiskPct, "load1": ag.Load1,
		"net_rx_bps": ag.NetRx, "net_tx_bps": ag.NetTx, "ram_total": ag.RAMTotal,
		"disk_total": ag.DiskTotal, "version": ag.Version, "last_seen": ag.LastSeen,
		"connected": true, "transport": transport,
	})
	// Rules can be slow (network sends); keep the live stream responsive.
	go a.dispatchAgentThresholds(ag, report)
	return nil
}

func (a *App) dispatchAgentThresholds(ag *model.Agent, report model.AgentReport) {
	notifyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, item := range []struct {
		trigger string
		value   float64
	}{
		{"cpu_high", report.CPUPct}, {"ram_high", report.RAMPct}, {"disk_high", report.DiskPct},
	} {
		a.dispatchEvent(notifyCtx, notifyEvent{Trigger: item.trigger, Agent: ag, Value: item.value})
	}
}
