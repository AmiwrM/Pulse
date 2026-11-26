package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"pulse/internal/model"
)

func (a *App) handleCFStatus(w http.ResponseWriter, r *http.Request) {
	conf := a.config()
	out := map[string]any{
		"configured": conf.CloudflareToken != "",
		"enabled":    conf.CloudflareEnabled,
		"connected":  false,
	}
	if conf.CloudflareToken == "" {
		writeData(w, out)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	verify, err := a.cf().Verify(ctx)
	if err != nil {
		out["error"] = err.Error()
		writeData(w, out)
		return
	}
	out["connected"] = verify.Status == "active"
	out["token_status"] = verify.Status
	writeData(w, out)
}

func (a *App) handleCFZones(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	zones, err := a.cf().ListZones(ctx)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeData(w, map[string]any{"zones": zones})
}

func (a *App) handleCFRecords(w http.ResponseWriter, r *http.Request) {
	zoneID := strings.TrimSpace(r.URL.Query().Get("zone_id"))
	if zoneID == "" {
		writeErr(w, 400, "شناسه Zone الزامی است")
		return
	}
	recordType := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("type")))
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	records, err := a.cf().ListRecords(ctx, zoneID, recordType, search)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeData(w, map[string]any{"records": records})
}

func (a *App) handleCFUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ZoneID   string `json:"zone_id"`
		RecordID string `json:"record_id"`
		Content  string `json:"content"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	body.Content = strings.TrimSpace(body.Content)
	if body.ZoneID == "" || body.RecordID == "" || body.Content == "" {
		writeErr(w, 400, "شناسه Zone، رکورد و مقدار جدید الزامی است")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rec, err := a.cf().UpdateRecordContent(ctx, body.ZoneID, body.RecordID, body.Content)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.db.AddAudit("admin", "cloudflare.update", rec.Name+" → "+rec.Content)
	a.logFor("cloudflare").Info("cloudflare record updated manually", "record", rec.Name, "content", rec.Content)
	a.hub.broadcast("cloudflare.updated", map[string]any{"record": rec})
	writeData(w, map[string]any{"record": rec})
}

func (a *App) applyCFFailover(m model.Monitor, mode string) {
	conf := a.config()
	if !conf.CloudflareEnabled || conf.CloudflareToken == "" {
		return
	}
	if !m.CFEnabled || m.CFZoneID == "" || m.CFRecordID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	client := a.cf()
	record, err := client.GetRecord(ctx, m.CFZoneID, m.CFRecordID)
	if err != nil {
		a.logCFAction(m, false, "دریافت رکورد Cloudflare ناموفق بود: "+err.Error())
		return
	}
	current := strings.TrimSpace(record.Content)
	var target, label string
	switch mode {
	case "failover":
		target = strings.TrimSpace(m.CFFailover)
		label = "IP پشتیبان"
	case "restore":
		target = strings.TrimSpace(m.CFPrimary)
		label = "IP اصلی"
	default:
		return
	}
	if target == "" {
		return
	}
	if sameContent(current, target) {
		a.logCFAction(m, true, "رکورد DNS از قبل روی "+label+" تنظیم شده است")
		return
	}
	safeSource := m.CFPrimary
	if mode == "restore" {
		safeSource = m.CFFailover
	}
	if safeSource != "" && !sameContent(current, safeSource) {
		msg := "مقدار فعلی رکورد (" + current + ") با مقدار مورد انتظار (" + safeSource + ") هم‌خوانی ندارد؛ برای جلوگیری از تغییر ناخواسته، سوییچ انجام نشد"
		a.logCFAction(m, false, msg)
		return
	}
	updated, err := client.UpdateRecordContent(ctx, m.CFZoneID, m.CFRecordID, target)
	if err != nil {
		a.logCFAction(m, false, "تغییر رکورد Cloudflare ناموفق بود: "+err.Error())
		return
	}
	a.logCFAction(m, true, "رکورد "+updated.Name+" با موفقیت به "+target+" ("+label+") تغییر کرد")
}

func (a *App) logCFAction(m model.Monitor, ok bool, message string) {
	if ok {
		a.logFor("cloudflare").Info("cloudflare DNS action", "monitor", m.Name, "message", message)
	} else {
		a.logFor("cloudflare").Warn("cloudflare DNS action failed", "monitor", m.Name, "message", message)
	}
	now := nowUnix()
	_ = a.db.UpdateMonitorCFAction(m.ID, message, now)
	_ = a.db.InsertNotification(&model.NotificationLog{
		At: now, Channel: "cloudflare", RuleName: "سوییچ خودکار DNS", Event: "cf_failover",
		Title: m.Name, Body: message, OK: ok, Error: map[bool]string{true: "", false: message}[ok],
	})
	a.db.AddAudit("system", "cloudflare.failover", m.Name+": "+message)
	a.hub.broadcast("cf.action", map[string]any{"monitor_id": m.ID, "ok": ok, "message": message})
}

func sameContent(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(strings.TrimSuffix(a, ".")), strings.TrimSpace(strings.TrimSuffix(b, ".")))
}
