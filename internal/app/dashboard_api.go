package app

import (
	"net/http"
	"time"

	"pulse/internal/model"
)

func (a *App) handleSummary(w http.ResponseWriter, r *http.Request) {
	monitors, err := a.db.ListMonitors()
	if err != nil {
		writeErr(w, 500, "دریافت خلاصه وضعیت ناموفق بود")
		return
	}
	a.enrichMonitors(monitors)

	agents, _ := a.db.ListAgents()
	conf := a.config()
	now := nowUnix()
	onlineAgents := 0
	for _, ag := range agents {
		if ag.LastSeen > 0 && now-ag.LastSeen <= int64(conf.AgentOfflineAfterSec) {
			onlineAgents++
		}
	}

	up, down, paused, unknown := 0, 0, 0, 0
	uptimeSum := 0.0
	uptimeCount := 0
	for _, m := range monitors {
		switch m.Status {
		case "up", "degraded":
			up++
		case "down":
			down++
		case "paused":
			paused++
		default:
			unknown++
		}
		if m.Status != "paused" && m.LastCheckedAt > 0 {
			uptimeSum += m.Uptime24
			uptimeCount++
		}
	}
	overallUptime := 0.0
	if uptimeCount > 0 {
		overallUptime = uptimeSum / float64(uptimeCount)
	}
	activeIncidents, _ := a.db.ListIncidents(20, true)
	recentIncidents, _ := a.db.ListIncidents(6, false)
	notifications, _ := a.db.ListNotifications(5)
	trend, _ := a.db.GlobalTrend(time.Now().Add(-24*time.Hour).Unix(), 900)

	overall := "healthy"
	if down > 0 {
		overall = "down"
	} else if unknown > 0 || uptimeCount == 0 {
		overall = "unknown"
	}

	writeData(w, map[string]any{
		"monitors": monitors,
		"agents":   agents,
		"stats": map[string]any{
			"total":            len(monitors),
			"up":               up,
			"down":             down,
			"paused":           paused,
			"unknown":          unknown,
			"uptime_24h":       overallUptime,
			"active_incidents": len(activeIncidents),
			"agents_total":     len(agents),
			"agents_online":    onlineAgents,
			"overall":          overall,
		},
		"incidents":         recentIncidents,
		"active_incidents":  firstNIncidents(activeIncidents, 8),
		"notifications":     notifications,
		"trend":             trend,
		"server_time":       now,
		"offline_after_sec": conf.AgentOfflineAfterSec,
		"site_title":        conf.SiteTitle,
		"version":           a.cfg.Version,
		"notify_repeat_min": conf.NotifyRepeatMin,
	})
}

func firstNIncidents(items []model.Incident, n int) []model.Incident {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

func (a *App) handleIncidents(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active") == "1"
	limit := queryInt(r, "limit", 100)
	incidents, err := a.db.ListIncidents(limit, activeOnly)
	if err != nil {
		writeErr(w, 500, "دریافت رخدادها ناموفق بود")
		return
	}
	writeData(w, map[string]any{
		"incidents":   incidents,
		"server_time": nowUnix(),
	})
}

func (a *App) handleResolveIncident(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := a.db.ResolveIncident(id, "manual"); err != nil {
		writeErr(w, 500, "بستن رخداد ناموفق بود")
		return
	}
	a.db.AddAudit("admin", "incident.resolve", "رخداد #"+r.PathValue("id")+" دستی بسته شد")
	a.hub.broadcast("incident.resolved", map[string]any{"id": id})
	writeData(w, map[string]any{"ok": true, "at": time.Now().Unix()})
}
