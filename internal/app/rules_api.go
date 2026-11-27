package app

import (
	"net/http"
	"strings"

	"pulse/internal/model"
)

var validTriggers = map[string]bool{
	"down": true, "up": true, "degraded": true, "cert_expiring": true,
	"cpu_high": true, "ram_high": true, "disk_high": true,
	"agent_offline": true, "dns_mismatch": true,
}

type ruleInput struct {
	Name        string   `json:"name"`
	Enabled     *bool    `json:"enabled"`
	Trigger     string   `json:"trigger"`
	MonitorID   int64    `json:"monitor_id"`
	Threshold   float64  `json:"threshold"`
	CooldownSec int      `json:"cooldown_sec"`
	Channels    []string `json:"channels"`
}

func (in *ruleInput) normalize(a *App) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Trigger = strings.ToLower(strings.TrimSpace(in.Trigger))
	if in.Name == "" {
		return errText("نام قانون الزامی است")
	}
	if !validTriggers[in.Trigger] {
		return errText("نوع رویداد قانون نامعتبر است")
	}
	if in.MonitorID < 0 {
		in.MonitorID = 0
	}
	if in.MonitorID > 0 {
		m, err := a.db.GetMonitor(in.MonitorID)
		if err != nil || m == nil {
			return errText("مانیتور انتخاب‌شده پیدا نشد")
		}
	}
	switch in.Trigger {
	case "cert_expiring":
		if in.Threshold <= 0 || in.Threshold > 365 {
			in.Threshold = 14
		}
	case "cpu_high", "ram_high", "disk_high":
		if in.Threshold <= 0 || in.Threshold > 100 {
			in.Threshold = 90
		}
		in.MonitorID = 0
	case "agent_offline":
		in.MonitorID = 0
		in.Threshold = 0
	default:
		in.Threshold = 0
	}
	if in.CooldownSec < 0 {
		in.CooldownSec = 0
	}
	if in.CooldownSec > 604800 {
		in.CooldownSec = 604800
	}
	cleaned := []string{}
	seen := map[string]bool{}
	for _, ch := range in.Channels {
		ch = strings.ToLower(strings.TrimSpace(ch))
		switch ch {
		case "all", "telegram", "webpush", "webhook":
			if !seen[ch] {
				seen[ch] = true
				cleaned = append(cleaned, ch)
			}
		}
	}
	if len(cleaned) == 0 {
		cleaned = []string{"all"}
	}
	in.Channels = cleaned
	return nil
}

func (a *App) handleListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := a.db.ListRules()
	if err != nil {
		writeErr(w, 500, "دریافت قوانین ناموفق بود")
		return
	}
	writeData(w, map[string]any{"rules": rules})
}

func (a *App) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	var in ruleInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := in.normalize(a); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	rule := &model.Rule{
		Name: in.Name, Enabled: enabled, Trigger: in.Trigger, MonitorID: in.MonitorID,
		Threshold: in.Threshold, CooldownSec: in.CooldownSec, Channels: in.Channels,
	}
	if err := a.db.CreateRule(rule); err != nil {
		writeErr(w, 500, "ساخت قانون ناموفق بود")
		return
	}
	a.db.AddAudit("admin", "rule.create", rule.Name)
	a.hub.broadcast("rule.changed", map[string]any{"id": rule.ID})
	writeData(w, map[string]any{"rule": rule})
}

func (a *App) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	old, err := a.db.GetRule(id)
	if err != nil || old == nil {
		writeErr(w, 404, "قانون پیدا نشد")
		return
	}
	var in ruleInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := in.normalize(a); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	old.Name = in.Name
	old.Trigger = in.Trigger
	old.MonitorID = in.MonitorID
	old.Threshold = in.Threshold
	old.CooldownSec = in.CooldownSec
	old.Channels = in.Channels
	if in.Enabled != nil {
		old.Enabled = *in.Enabled
	}
	if err := a.db.UpdateRule(old); err != nil {
		writeErr(w, 500, "ذخیره قانون ناموفق بود")
		return
	}
	a.db.AddAudit("admin", "rule.update", old.Name)
	a.hub.broadcast("rule.changed", map[string]any{"id": old.ID})
	writeData(w, map[string]any{"rule": old})
}

func (a *App) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	rule, _ := a.db.GetRule(id)
	if rule == nil {
		writeErr(w, 404, "قانون پیدا نشد")
		return
	}
	if err := a.db.DeleteRule(id); err != nil {
		writeErr(w, 500, "حذف قانون ناموفق بود")
		return
	}
	a.db.AddAudit("admin", "rule.delete", rule.Name)
	a.hub.broadcast("rule.changed", map[string]any{"id": id})
	writeData(w, map[string]any{"ok": true})
}
