package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"pulse/internal/checker"
	"pulse/internal/model"
)

type monitorInput struct {
	Name             string          `json:"name"`
	Type             string          `json:"type"`
	Target           string          `json:"target"`
	Group            string          `json:"group"`
	IntervalSec      int             `json:"interval_sec"`
	TimeoutMS        int             `json:"timeout_ms"`
	Enabled          *bool           `json:"enabled"`
	Config           json.RawMessage `json:"config"`
	FailThreshold    int             `json:"fail_threshold"`
	RecoverThreshold int             `json:"recover_threshold"`
	CFEnabled        bool            `json:"cf_enabled"`
	CFZoneID         string          `json:"cf_zone_id"`
	CFRecordID       string          `json:"cf_record_id"`
	CFRecordName     string          `json:"cf_record_name"`
	CFPrimary        string          `json:"cf_primary"`
	CFFailover       string          `json:"cf_failover"`
}

var validMonitorTypes = map[string]bool{
	"http": true, "tcp": true, "ping": true, "dns": true, "ssl": true, "proxy": true,
}

func (in *monitorInput) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.ToLower(strings.TrimSpace(in.Type))
	in.Target = strings.TrimSpace(in.Target)
	in.Group = strings.TrimSpace(in.Group)
	if in.Name == "" {
		return fmt.Errorf("نام مانیتور الزامی است")
	}
	if len([]rune(in.Name)) > 120 {
		return fmt.Errorf("نام مانیتور طولانی است")
	}
	if !validMonitorTypes[in.Type] {
		return fmt.Errorf("نوع مانیتور نامعتبر است")
	}
	if in.IntervalSec == 0 {
		in.IntervalSec = 60
	}
	if in.IntervalSec < 10 {
		in.IntervalSec = 10
	}
	if in.IntervalSec > 86400 {
		in.IntervalSec = 86400
	}
	if in.TimeoutMS < 1000 {
		in.TimeoutMS = 12000
	}
	if in.TimeoutMS > 180000 {
		in.TimeoutMS = 180000
	}
	if in.FailThreshold <= 0 {
		in.FailThreshold = 2
	}
	if in.RecoverThreshold <= 0 {
		in.RecoverThreshold = 1
	}
	if in.FailThreshold > 20 {
		in.FailThreshold = 20
	}
	if in.RecoverThreshold > 20 {
		in.RecoverThreshold = 20
	}
	if len(in.Config) == 0 || string(in.Config) == "null" {
		in.Config = json.RawMessage(`{}`)
	}
	if err := checker.Validate(in.Type, in.Target, in.Config); err != nil {
		return err
	}
	if in.CFEnabled {
		if in.CFZoneID == "" || in.CFRecordID == "" {
			return fmt.Errorf("برای سوییچ خودکار DNS باید Zone و رکورد را انتخاب کنید")
		}
		if in.CFPrimary == "" || in.CFFailover == "" {
			return fmt.Errorf("IP اصلی و IP پشتیبان را وارد کنید")
		}
		if in.CFPrimary == in.CFFailover {
			return fmt.Errorf("IP اصلی و پشتیبان نباید یکسان باشند")
		}
	}
	return nil
}

func (in *monitorInput) apply(m *model.Monitor) {
	m.Name = in.Name
	m.Type = in.Type
	m.Target = in.Target
	m.Group = in.Group
	m.Interval = in.IntervalSec
	m.Timeout = in.TimeoutMS
	m.Config = in.Config
	m.FailThreshold = in.FailThreshold
	m.RecoverThreshold = in.RecoverThreshold
	m.CFEnabled = in.CFEnabled
	m.CFZoneID = in.CFZoneID
	m.CFRecordID = in.CFRecordID
	m.CFRecordName = in.CFRecordName
	m.CFPrimary = in.CFPrimary
	m.CFFailover = in.CFFailover
	if in.Enabled != nil {
		m.Enabled = *in.Enabled
	}
}

func (a *App) handleListMonitors(w http.ResponseWriter, r *http.Request) {
	monitors, err := a.db.ListMonitors()
	if err != nil {
		writeErr(w, 500, "دریافت مانیتورها ناموفق بود")
		return
	}
	a.enrichMonitors(monitors)
	writeData(w, map[string]any{
		"monitors":    monitors,
		"server_time": nowUnix(),
		"groups":      collectGroups(monitors),
	})
}

func collectGroups(monitors []model.Monitor) []string {
	set := map[string]bool{}
	for _, m := range monitors {
		if m.Group != "" {
			set[m.Group] = true
		}
	}
	out := make([]string, 0, len(set))
	for g := range set {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

func (a *App) enrichMonitors(monitors []model.Monitor) {
	stats, err := a.db.MonitorStats(time.Now().Add(-24 * time.Hour).Unix())
	if err == nil {
		for i := range monitors {
			if st, ok := stats[monitors[i].ID]; ok {
				monitors[i].Uptime24 = st.Uptime
				monitors[i].AvgLatency = st.AvgLatency
				if len(st.Spark) > 0 {
					monitors[i].Spark = st.Spark
				}
			}
		}
	}
	active, err := a.db.ListIncidents(500, true)
	if err == nil {
		byMonitor := map[int64]*model.Incident{}
		for i := range active {
			inc := active[i]
			byMonitor[inc.MonitorID] = &inc
		}
		for i := range monitors {
			if inc, ok := byMonitor[monitors[i].ID]; ok {
				monitors[i].ActiveIncident = inc
			}
		}
	}
}

func (a *App) handleGetMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	m, err := a.db.GetMonitor(id)
	if err != nil {
		writeErr(w, 500, "خطا در دریافت مانیتور")
		return
	}
	if m == nil {
		writeErr(w, 404, "مانیتور پیدا نشد")
		return
	}
	monitors := []model.Monitor{*m}
	a.enrichMonitors(monitors)
	m = &monitors[0]

	checks, err := a.db.ListChecks(id, 150)
	if err != nil {
		writeErr(w, 500, "دریافت تاریخچه ناموفق بود")
		return
	}
	sinceDay := time.Now().AddDate(0, 0, -29).UTC().Format("2006-01-02")
	daily, err := a.db.DailyStats(id, sinceDay)
	if err != nil {
		writeErr(w, 500, "دریافت آپتایم ناموفق بود")
		return
	}
	allIncidents, err := a.db.ListIncidents(300, false)
	if err != nil {
		writeErr(w, 500, "دریافت رخدادها ناموفق بود")
		return
	}
	incidents := make([]model.Incident, 0, 10)
	for _, inc := range allIncidents {
		if inc.MonitorID == id {
			incidents = append(incidents, inc)
			if len(incidents) >= 20 {
				break
			}
		}
	}
	writeData(w, map[string]any{
		"monitor":     m,
		"checks":      checks,
		"daily":       daily,
		"incidents":   incidents,
		"server_time": nowUnix(),
	})
}

func (a *App) handleCreateMonitor(w http.ResponseWriter, r *http.Request) {
	var in monitorInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := in.normalize(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	m := &model.Monitor{
		Name: in.Name, Type: in.Type, Target: in.Target, Group: in.Group,
		Interval: in.IntervalSec, Timeout: in.TimeoutMS, Enabled: enabled,
		Config: in.Config, FailThreshold: in.FailThreshold, RecoverThreshold: in.RecoverThreshold,
	}
	in.apply(m)
	if err := a.db.CreateMonitor(m); err != nil {
		writeErr(w, 500, "ساخت مانیتور ناموفق بود")
		return
	}
	a.db.AddAudit("admin", "monitor.create", m.Name+" ("+m.Target+")")
	a.hub.broadcast("monitor.created", map[string]any{"id": m.ID})
	if m.Enabled {
		a.triggerCheck(m.ID)
	}
	writeData(w, map[string]any{"monitor": m})
}

func (a *App) handleUpdateMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	old, err := a.db.GetMonitor(id)
	if err != nil || old == nil {
		writeErr(w, 404, "مانیتور پیدا نشد")
		return
	}
	var in monitorInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := in.normalize(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	wasEnabled := old.Enabled
	oldType, oldTarget := old.Type, old.Target
	in.apply(old)
	if err := a.db.UpdateMonitor(old); err != nil {
		writeErr(w, 500, "ذخیره مانیتور ناموفق بود")
		return
	}
	enabledChanged := old.Enabled != wasEnabled
	if enabledChanged {
		if err := a.db.SetMonitorEnabled(id, old.Enabled); err != nil {
			writeErr(w, 500, "ذخیره وضعیت ناموفق بود")
			return
		}
	}
	if old.Enabled && (enabledChanged || old.Type != oldType || old.Target != oldTarget) {
		_ = a.db.ResetMonitorState(id)
		a.triggerCheck(id)
	}
	a.db.AddAudit("admin", "monitor.update", in.Name)
	a.hub.broadcast("monitor.updated", map[string]any{"id": id})
	fresh, _ := a.db.GetMonitor(id)
	writeData(w, map[string]any{"monitor": fresh})
}

func (a *App) handleToggleMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	m, err := a.db.GetMonitor(id)
	if err != nil || m == nil {
		writeErr(w, 404, "مانیتور پیدا نشد")
		return
	}
	enabled := !m.Enabled
	if err := a.db.SetMonitorEnabled(id, enabled); err != nil {
		writeErr(w, 500, "تغییر وضعیت ناموفق بود")
		return
	}
	a.db.AddAudit("admin", "monitor.toggle", map[bool]string{true: "فعال", false: "غیرفعال"}[enabled]+" — "+m.Name)
	a.hub.broadcast("monitor.updated", map[string]any{"id": id})
	if enabled {
		a.triggerCheck(id)
	}
	writeData(w, map[string]any{"enabled": enabled})
}

func (a *App) handleDeleteMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	m, _ := a.db.GetMonitor(id)
	if m == nil {
		writeErr(w, 404, "مانیتور پیدا نشد")
		return
	}
	if err := a.db.DeleteMonitor(id); err != nil {
		writeErr(w, 500, "حذف مانیتور ناموفق بود")
		return
	}
	a.db.AddAudit("admin", "monitor.delete", m.Name)
	a.hub.broadcast("monitor.deleted", map[string]any{"id": id})
	writeData(w, map[string]any{"ok": true})
}

func (a *App) handleCheckNow(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	m, err := a.db.GetMonitor(id)
	if err != nil || m == nil {
		writeErr(w, 404, "مانیتور پیدا نشد")
		return
	}
	if !m.Enabled {
		writeErr(w, 400, "این مانیتور غیرفعال است")
		return
	}
	a.startCheck(*m)
	writeData(w, map[string]any{"started": true})
}

func (a *App) handleMonitorChecks(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	limit := queryInt(r, "limit", 120)
	checks, err := a.db.ListChecks(id, limit)
	if err != nil {
		writeErr(w, 500, "دریافت تاریخچه ناموفق بود")
		return
	}
	writeData(w, map[string]any{"checks": checks})
}

func (a *App) triggerCheck(id int64) {
	go func() {
		m, err := a.db.GetMonitor(id)
		if err != nil || m == nil || !m.Enabled {
			return
		}
		a.startCheck(*m)
	}()
}

func (a *App) startCheck(m model.Monitor) bool {
	a.runMu.Lock()
	if a.running[m.ID] {
		a.runMu.Unlock()
		return false
	}
	a.runMu.Unlock()
	select {
	case a.slots <- struct{}{}:
	default:
		return false
	}
	a.runMu.Lock()
	if a.running[m.ID] {
		a.runMu.Unlock()
		<-a.slots
		return false
	}
	a.running[m.ID] = true
	a.runMu.Unlock()
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				a.logger.Error("monitor check panic", "monitor", m.ID, "error", rec)
			}
			<-a.slots
			a.runMu.Lock()
			delete(a.running, m.ID)
			a.runMu.Unlock()
		}()
		timeout := time.Duration(m.Timeout)*time.Millisecond + 5*time.Second
		if timeout < 15*time.Second {
			timeout = 15 * time.Second
		}
		if m.Type == "proxy" && timeout < 50*time.Second {
			timeout = 50 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		a.checkOne(ctx, m)
	}()
	return true
}
