package app

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"time"

	"pulse/internal/config"
	"pulse/internal/model"
	"pulse/internal/notify"
	"pulse/internal/util"
)

type notifyEvent struct {
	Trigger string
	Monitor *model.Monitor
	Agent   *model.Agent
	Force   bool
	Value   float64
	Extra   map[string]any
}

func (a *App) dispatchEvent(ctx context.Context, ev notifyEvent) bool {
	if ev.Trigger == "" {
		return false
	}
	rules, err := a.db.ListRules()
	if err != nil {
		return false
	}
	conf := a.config()
	now := nowUnix()
	pushSender := a.pushSender()

	var pushSubs []model.PushSub
	pushLoaded := false
	loadPush := func() []model.PushSub {
		if !pushLoaded {
			pushSubs, _ = a.db.ListPushSubs()
			pushLoaded = true
		}
		return pushSubs
	}

	processedAny := false
	sentAny := false
	quiet := inQuietHours(conf, time.Now())
	critical := ev.Trigger == "down" || ev.Trigger == "agent_offline"
	if quiet && !(critical && conf.QuietAllowCritical) {
		return false
	}

	for _, rule := range rules {
		if !rule.Enabled || rule.Trigger != ev.Trigger {
			continue
		}
		if ev.Monitor != nil && rule.MonitorID != 0 && rule.MonitorID != ev.Monitor.ID {
			continue
		}
		if ev.Agent != nil && rule.MonitorID != 0 {
			continue
		}
		if !thresholdPass(rule, ev) {
			continue
		}
		entityKey := ruleEntityKey(rule, ev)
		if !ev.Force && rule.CooldownSec > 0 {
			a.notifyMu.Lock()
			last := a.notifyLast[entityKey]
			a.notifyMu.Unlock()
			if time.Since(last) < time.Duration(rule.CooldownSec)*time.Second {
				continue
			}
		}

		title, plain, htmlBody, link := a.buildMessage(ev, conf)
		title, plain, htmlBody = applyNotifyTemplate(conf, ev, title, plain, htmlBody)
		severity := "warning"
		switch ev.Trigger {
		case "down", "agent_offline":
			severity = "critical"
		case "up":
			severity = "success"
		}

		channels := a.expandChannels(rule.Channels, conf, len(loadPush()) > 0)
		if len(channels) == 0 {
			continue
		}
		processedAny = true
		sentThisRule := false

		for _, channel := range channels {
			var sendErr error
			switch channel {
			case "telegram":
				sendErr = notify.SendTelegram(ctx, conf.TelegramToken, conf.TelegramChatID, htmlBody)
				a.logNotification(&model.NotificationLog{
					At: now, Channel: "telegram", RuleName: rule.Name, Event: ev.Trigger,
					Title: title, Body: plain, OK: sendErr == nil, Error: errString(sendErr),
				})
			case "webpush":
				subs := loadPush()
				if len(subs) == 0 {
					sendErr = fmt.Errorf("هیچ مرورگری برای Web Push ثبت نشده است")
				} else {
					okCount := 0
					var lastErr error
					for _, sub := range subs {
						err := pushSender.SendPush(ctx, sub, title, pushBody(plain), "pulse-"+ev.Trigger+"-"+eventID(ev), link)
						if err == notify.ErrGone {
							_ = a.db.DeletePushSub(sub.Endpoint)
							continue
						}
						if err != nil {
							lastErr = err
						} else {
							okCount++
						}
					}
					if okCount == 0 {
						sendErr = lastErr
						if sendErr == nil {
							sendErr = fmt.Errorf("ارسال به هیچ مرورگری موفق نبود")
						}
					}
				}
				a.logNotification(&model.NotificationLog{
					At: now, Channel: "webpush", RuleName: rule.Name, Event: ev.Trigger,
					Title: title, Body: plain, OK: sendErr == nil, Error: errString(sendErr),
				})
			case "webhook":
				payload := map[string]any{
					"event": ev.Trigger, "severity": severity, "title": title, "body": plain,
					"at": now, "value": ev.Value, "monitor": ev.Monitor, "agent": ev.Agent,
				}
				sendErr = notify.SendWebhook(ctx, conf.WebhookURL, conf.WebhookSecret, payload)
				a.logNotification(&model.NotificationLog{
					At: now, Channel: "webhook", RuleName: rule.Name, Event: ev.Trigger,
					Title: title, Body: plain, OK: sendErr == nil, Error: errString(sendErr),
				})
			}
			if sendErr == nil {
				sentThisRule = true
			}
		}
		_ = a.db.MarkRuleFired(rule.ID, now)
		a.notifyMu.Lock()
		a.notifyLast[entityKey] = time.Unix(now, 0)
		a.notifyMu.Unlock()
		if sentThisRule {
			sentAny = true
		}
	}
	if processedAny {
		if ev.Monitor != nil {
			_ = a.db.UpdateMonitorNotified(ev.Monitor.ID, now)
		}
		a.hub.broadcast("notification.sent", map[string]any{"event": ev.Trigger, "sent": sentAny})
	}
	return sentAny
}

func (a *App) expandChannels(ruleChannels []string, conf *config.Settings, hasPushSubs bool) []string {
	set := map[string]bool{}
	add := func(c string) {
		switch c {
		case "telegram":
			if conf.TelegramEnabled && conf.TelegramToken != "" && conf.TelegramChatID != "" {
				set[c] = true
			}
		case "webpush":
			if conf.WebPushEnabled && conf.VAPIDPublic != "" && hasPushSubs {
				set[c] = true
			}
		case "webhook":
			if strings.TrimSpace(conf.WebhookURL) != "" {
				set[c] = true
			}
		}
	}
	for _, ch := range ruleChannels {
		ch = strings.ToLower(strings.TrimSpace(ch))
		if ch == "all" {
			add("telegram")
			add("webpush")
			add("webhook")
			continue
		}
		add(ch)
	}
	out := []string{}
	for _, c := range []string{"telegram", "webpush", "webhook"} {
		if set[c] {
			out = append(out, c)
		}
	}
	return out
}

func ruleEntityKey(rule model.Rule, ev notifyEvent) string {
	entity := "global"
	if ev.Monitor != nil {
		entity = "m" + fmt.Sprint(ev.Monitor.ID)
	} else if ev.Agent != nil {
		entity = "a" + fmt.Sprint(ev.Agent.ID)
	}
	return fmt.Sprintf("%d:%s:%s", rule.ID, ev.Trigger, entity)
}

func thresholdPass(rule model.Rule, ev notifyEvent) bool {
	switch ev.Trigger {
	case "cert_expiring":
		if ev.Value <= 0 {
			return false
		}
		if rule.Threshold > 0 {
			return ev.Value <= rule.Threshold
		}
		return ev.Value <= 14
	case "cpu_high", "ram_high", "disk_high":
		th := rule.Threshold
		if th <= 0 {
			th = 90
		}
		return ev.Value >= th
	}
	return true
}

func (a *App) buildMessage(ev notifyEvent, conf *config.Settings) (title, plain, htmlBody, link string) {
	loc := loadLocation(conf.Timezone)
	stamp := util.FormatDateTime(nowUnix(), loc)
	link = "/#/dashboard"

	if ev.Monitor != nil {
		m := ev.Monitor
		link = "/#/monitors"
		name := html.EscapeString(m.Name)
		target := html.EscapeString(m.Target)
		errText := html.EscapeString(truncateRunes(m.LastError, 240))
		typeLabel := monitorTypeLabel(m.Type)
		statusLabel := statusLabel(m.Status)
		switch ev.Trigger {
		case "down":
			title = "🔴 قطع سرویس: " + m.Name
			plain = fmt.Sprintf("%s (%s) از دسترس خارج شد.\nهدف: %s\nوضعیت: %s\nخطا: %s\nزمان: %s",
				m.Name, typeLabel, m.Target, statusLabel, fallback(m.LastError, "بدون جزئیات"), stamp)
			htmlBody = fmt.Sprintf("🔴 <b>قطع سرویس</b>\n\n<b>%s</b>\n🎯 نوع: %s\n🌐 هدف: <code>%s</code>\n📄 خطا: %s\n🕒 %s",
				name, typeLabel, target, errText, util.FaDigits(stamp))
		case "up":
			dur := ""
			if ev.Extra != nil {
				if sec, ok := ev.Extra["down_sec"].(int64); ok && sec > 0 {
					dur = "\n⏳ مدت قطعی: " + util.HumanDuration(sec)
				}
			}
			title = "🟢 بازیابی سرویس: " + m.Name
			plain = fmt.Sprintf("%s دوباره در دسترس است.\nهدف: %s%s\nزمان: %s", m.Name, m.Target, stripTags(dur), stamp)
			htmlBody = fmt.Sprintf("🟢 <b>بازیابی سرویس</b>\n\n<b>%s</b>\n🌐 هدف: <code>%s</code>%s\n🕒 %s",
				name, target, strings.ReplaceAll(dur, "\n", "<br>"), util.FaDigits(stamp))
			link = "/#/monitors"
		case "degraded":
			warn := ""
			if ev.Extra != nil {
				if v, ok := ev.Extra["warn"].(string); ok {
					warn = v
				}
			}
			if warn == "" {
				warn = m.LastError
			}
			title = "🟡 هشدار سرویس: " + m.Name
			plain = fmt.Sprintf("%s در وضعیت هشدار است.\nهدف: %s\nدلیل: %s\nزمان: %s",
				m.Name, m.Target, fallback(warn, "بدون جزئیات"), stamp)
			htmlBody = fmt.Sprintf("🟡 <b>هشدار سرویس</b>\n\n<b>%s</b>\n🌐 هدف: <code>%s</code>\n📄 دلیل: %s\n🕒 %s",
				name, target, html.EscapeString(truncateRunes(warn, 500)), util.FaDigits(stamp))
			link = "/#/monitors"
		case "cert_expiring":
			days := int(ev.Value)
			title = fmt.Sprintf("📜 گواهی SSL %s رو به انقضاست", m.Name)
			plain = fmt.Sprintf("گواهی SSL %s تا %s روز دیگر منقضی می‌شود.\nهدف: %s\nزمان: %s", m.Name, util.FaDigits(fmt.Sprint(days)), m.Target, stamp)
			htmlBody = fmt.Sprintf("📜 <b>هشدار انقضای گواهی</b>\n\n<b>%s</b>\n🌐 %s\n⏳ <b>%s روز</b> تا انقضا\n🕒 %s",
				name, target, util.FaDigits(fmt.Sprint(days)), util.FaDigits(stamp))
			link = "/#/monitors"
		case "dns_mismatch":
			missing := ""
			if ev.Extra != nil {
				if v, ok := ev.Extra["missing"]; ok {
					if b, err := json.Marshal(v); err == nil {
						missing = string(b)
					}
				}
			}
			title = "🌐 تغییر رکورد DNS: " + m.Name
			plain = fmt.Sprintf("رکورد DNS %s با مقدار انتظار مطابقت ندارد.\nمقدار ناموجود: %s\nزمان: %s", m.Target, missing, stamp)
			htmlBody = fmt.Sprintf("🌐 <b>اختلاف رکورد DNS</b>\n\n<b>%s</b>\n🌐 %s\n📄 مقدار ناموجود: <code>%s</code>\n🕒 %s",
				name, target, html.EscapeString(missing), util.FaDigits(stamp))
			link = "/#/monitors"
		case "cpu_high", "ram_high", "disk_high":
			label := map[string]string{"cpu_high": "CPU", "ram_high": "RAM", "disk_high": "دیسک"}[ev.Trigger]
			title = fmt.Sprintf("⚠️ مصرف بالای %s: %s", label, m.Name)
			plain = fmt.Sprintf("مصرف %s در %s به %s درصد رسیده است.\nزمان: %s", label, m.Name, util.FaDigits(fmt.Sprintf("%.1f", ev.Value)), stamp)
			htmlBody = fmt.Sprintf("⚠️ <b>مصرف بالای %s</b>\n\n<b>%s</b>\n📈 مقدار: <b>%s٪</b>\n🕒 %s",
				label, name, util.FaDigits(fmt.Sprintf("%.1f", ev.Value)), util.FaDigits(stamp))
			link = "/#/servers"
		default:
			title = "رویداد " + m.Name
			plain = fmt.Sprintf("%s — %s", m.Name, m.Target)
			htmlBody = fmt.Sprintf("<b>%s</b>\n%s", name, target)
		}
		return
	}

	if ev.Agent != nil {
		ag := ev.Agent
		link = "/#/servers"
		name := html.EscapeString(ag.Name)
		lastSeen := util.FormatDateTime(ag.LastSeen, loc)
		switch ev.Trigger {
		case "agent_offline":
			title = "📡 ایجنت آفلاین شد: " + ag.Name
			plain = fmt.Sprintf("ارتباط سرور %s قطع شده است.\nآخرین گزارش: %s\nهاست: %s", ag.Name, lastSeen, ag.Hostname)
			htmlBody = fmt.Sprintf("📡 <b>قطع ارتباط ایجنت</b>\n\n<b>%s</b>\n🖥 %s\n🕒 آخرین گزارش: %s",
				name, html.EscapeString(ag.Hostname), util.FaDigits(lastSeen))
			link = "/#/servers"
		case "cpu_high", "ram_high", "disk_high":
			label := map[string]string{"cpu_high": "CPU", "ram_high": "RAM", "disk_high": "دیسک"}[ev.Trigger]
			title = fmt.Sprintf("⚠️ مصرف بالای %s: %s", label, ag.Name)
			plain = fmt.Sprintf("مصرف %s سرور %s به %s درصد رسید.", label, ag.Name, util.FaDigits(fmt.Sprintf("%.1f", ev.Value)))
			htmlBody = fmt.Sprintf("⚠️ <b>مصرف بالای %s</b>\n\n<b>%s</b>\n🖥 %s\n📈 <b>%s٪</b>\n🕒 %s",
				label, name, html.EscapeString(ag.Hostname), util.FaDigits(fmt.Sprintf("%.1f", ev.Value)), util.FaDigits(stamp))
			link = "/#/servers"
		default:
			title = "رویداد سرور " + ag.Name
			plain = ag.Name + " — " + ag.Hostname
			htmlBody = html.EscapeString(plain)
		}
	}
	return
}

func applyNotifyTemplate(conf *config.Settings, ev notifyEvent, title, plain, htmlBody string) (string, string, string) {
	if conf.NotifyTitleTemplate == "" && conf.NotifyBodyTemplate == "" {
		return title, plain, htmlBody
	}
	loc := loadLocation(conf.Timezone)
	name, target, hostname := "", "", ""
	typeLabel, status := "", ""
	reason, details, failedConfigs := "", "", ""
	emoji := triggerEmoji(ev.Trigger)
	if ev.Monitor != nil {
		m := ev.Monitor
		name, target = m.Name, m.Target
		typeLabel = monitorTypeLabel(m.Type)
		status = statusLabel(m.Status)
		reason = m.LastError
		details = m.LastError
	}
	if ev.Agent != nil {
		ag := ev.Agent
		name, hostname = ag.Name, ag.Hostname
		status = "آفلاین"
		reason = "آخرین گزارش: " + util.FormatDateTime(ag.LastSeen, loc)
		details = reason
	}
	if ev.Extra != nil {
		if v, ok := ev.Extra["warn"].(string); ok && strings.TrimSpace(v) != "" {
			reason = v
			details = v
		}
		if v, ok := ev.Extra["reason"].(string); ok && strings.TrimSpace(v) != "" {
			reason = v
		}
		if v, ok := ev.Extra["details"].(string); ok && strings.TrimSpace(v) != "" {
			details = v
		}
		if v, ok := ev.Extra["failed_configs"].(string); ok {
			failedConfigs = v
			if details == "" {
				details = v
			}
		}
	}
	if strings.TrimSpace(reason) == "" {
		switch ev.Trigger {
		case "up":
			reason = "سرویس دوباره سالم شد"
		case "cert_expiring":
			reason = fmt.Sprintf("%d روز تا انقضای گواهی", int(ev.Value))
		case "dns_mismatch":
			reason = "مقدار DNS با مقدار مورد انتظار تفاوت دارد"
		case "cpu_high":
			reason = fmt.Sprintf("مصرف CPU به %.1f%% رسیده است", ev.Value)
		case "ram_high":
			reason = fmt.Sprintf("مصرف RAM به %.1f%% رسیده است", ev.Value)
		case "disk_high":
			reason = fmt.Sprintf("مصرف دیسک به %.1f%% رسیده است", ev.Value)
		default:
			reason = strings.TrimSpace(plain)
		}
	}
	if strings.TrimSpace(details) == "" {
		details = strings.TrimSpace(plain)
	}
	values := map[string]string{
		"{event}": ev.Trigger, "{title}": title, "{emoji}": emoji,
		"{monitor}": name, "{agent}": name, "{hostname}": hostname,
		"{type}": typeLabel, "{target}": target, "{status}": status,
		"{reason}": reason, "{details}": details, "{failed_configs}": failedConfigs,
		"{message}": plain,
		"{value}":   fmt.Sprintf("%.1f", ev.Value), "{time}": util.FormatDateTime(nowUnix(), loc),
	}
	pairs := make([]string, 0, len(values)*2)
	pairsHTML := make([]string, 0, len(values)*2)
	for k, v := range values {
		pairs = append(pairs, k, v)
		pairsHTML = append(pairsHTML, k, html.EscapeString(v))
	}
	render := func(tpl string) string { return strings.NewReplacer(pairs...).Replace(tpl) }
	renderHTML := func(tpl string) string { return strings.NewReplacer(pairsHTML...).Replace(tpl) }
	if strings.TrimSpace(conf.NotifyTitleTemplate) != "" {
		title = render(conf.NotifyTitleTemplate)
	}
	if strings.TrimSpace(conf.NotifyBodyTemplate) != "" {
		htmlBody = renderHTML(conf.NotifyBodyTemplate)
		plain = html.UnescapeString(htmlTagRE.ReplaceAllString(htmlBody, ""))
	}
	return title, plain, htmlBody
}

var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

func triggerEmoji(trigger string) string {
	switch trigger {
	case "down":
		return "🔴"
	case "up":
		return "🟢"
	case "degraded":
		return "🟡"
	case "cert_expiring":
		return "📜"
	case "agent_offline":
		return "📡"
	case "cpu_high", "ram_high", "disk_high":
		return "⚠️"
	case "dns_mismatch":
		return "🌐"
	}
	return "🔔"
}

func (a *App) logNotification(n *model.NotificationLog) {
	_ = a.db.InsertNotification(n)
	if n.OK {
		a.logFor("notify").Info("notification sent", "channel", n.Channel, "event", n.Event, "rule", n.RuleName)
	} else {
		a.logFor("notify").Warn("notification failed", "channel", n.Channel, "event", n.Event, "rule", n.RuleName, "error", n.Error)
	}
}

func monitorTypeLabel(t string) string {
	switch t {
	case "http":
		return "HTTP / وب‌سایت"
	case "tcp":
		return "پورت TCP"
	case "ping":
		return "پینگ ICMP"
	case "dns":
		return "رکورد DNS"
	case "ssl":
		return "گواهی SSL"
	case "proxy":
		return "کانفیگ پروکسی"
	}
	return t
}

func statusLabel(s string) string {
	switch s {
	case "up":
		return "سالم"
	case "down":
		return "قطع"
	case "paused":
		return "متوقف"
	case "degraded":
		return "هشدار"
	}
	return "نامشخص"
}

func fallback(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func stripTags(s string) string {
	return strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<b>", "", "</b>", "").Replace(s)
}

func pushBody(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, 3)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	body := strings.Join(out, " · ")
	return truncateRunes(body, 180)
}

func eventID(ev notifyEvent) string {
	if ev.Monitor != nil {
		return fmt.Sprint(ev.Monitor.ID)
	}
	if ev.Agent != nil {
		return fmt.Sprint(ev.Agent.ID)
	}
	return "0"
}

func inQuietHours(conf *config.Settings, now time.Time) bool {
	if !conf.QuietHoursEnabled {
		return false
	}
	loc := loadLocation(conf.Timezone)
	t := now.In(loc)
	start, ok1 := parseClock(conf.QuietStart)
	end, ok2 := parseClock(conf.QuietEnd)
	if !ok1 || !ok2 || start == end {
		return false
	}
	cur := t.Hour()*60 + t.Minute()
	if start < end {
		return cur >= start && cur < end
	}
	return cur >= start || cur < end
}

func parseClock(s string) (int, bool) {
	var h, m int
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &h, &m); err != nil {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// handleTestTelegram sends a test message with the current settings.
func (a *App) handleTestTelegram(w http.ResponseWriter, r *http.Request) {
	conf := a.config()
	text := fmt.Sprintf("✅ <b>اتصال پالس برقرار است</b>\n\nاین یک پیام آزمایشی از سامانه پایش پالس است.\n🕒 %s",
		util.FaDigits(util.FormatDateTime(nowUnix(), loadLocation(conf.Timezone))))
	if err := notify.SendTelegram(r.Context(), conf.TelegramToken, conf.TelegramChatID, text); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.logNotification(&model.NotificationLog{At: nowUnix(), Channel: "telegram", RuleName: "تست دستی", Event: "test", Title: "پیام آزمایشی", Body: "اتصال تلگرام برقرار است", OK: true})
	writeData(w, map[string]any{"ok": true})
}

func (a *App) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	conf := a.config()
	err := notify.SendWebhook(r.Context(), conf.WebhookURL, conf.WebhookSecret, map[string]any{
		"event": "test", "title": "تست وب‌هوک پالس", "body": "ارتباط وب‌هوک برقرار است", "at": nowUnix(),
	})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.logNotification(&model.NotificationLog{At: nowUnix(), Channel: "webhook", RuleName: "تست دستی", Event: "test", Title: "پیام آزمایشی", Body: "اتصال وب‌هوک برقرار است", OK: true})
	writeData(w, map[string]any{"ok": true})
}

func (a *App) handleNotifications(w http.ResponseWriter, r *http.Request) {
	items, err := a.db.ListNotifications(queryInt(r, "limit", 50))
	if err != nil {
		writeErr(w, 500, "دریافت اعلان‌ها ناموفق بود")
		return
	}
	writeData(w, map[string]any{"notifications": items})
}

func (a *App) handleAudit(w http.ResponseWriter, r *http.Request) {
	items, err := a.db.ListAudit(queryInt(r, "limit", 50))
	if err != nil {
		writeErr(w, 500, "دریافت لاگ ناموفق بود")
		return
	}
	writeData(w, map[string]any{"audit": items})
}
