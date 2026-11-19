package app

import (
	"net/http"
	"strings"

	"pulse/internal/model"
	"pulse/internal/notify"
)

func (a *App) handlePushConfig(w http.ResponseWriter, r *http.Request) {
	conf := a.config()
	count, _ := a.db.CountPushSubs()
	writeData(w, map[string]any{
		"public_key":    conf.VAPIDPublic,
		"enabled":       conf.WebPushEnabled,
		"configured":    conf.VAPIDPublic != "" && conf.VAPIDPrivate != "",
		"subscriptions": count,
	})
}

func (a *App) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	var body model.PushSubscription
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	body.Endpoint = strings.TrimSpace(body.Endpoint)
	if !strings.HasPrefix(body.Endpoint, "https://") {
		writeErr(w, 400, "آدرس اشتراک پوش معتبر نیست")
		return
	}
	if body.Keys.P256DH == "" || body.Keys.Auth == "" {
		writeErr(w, 400, "کلیدهای اشتراک پوش ناقص هستند")
		return
	}
	err := a.db.SavePushSub(model.PushSub{
		Endpoint:  body.Endpoint,
		P256DH:    body.Keys.P256DH,
		Auth:      body.Keys.Auth,
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeErr(w, 500, "ذخیره اشتراک پوش ناموفق بود")
		return
	}
	count, _ := a.db.CountPushSubs()
	writeData(w, map[string]any{"subscriptions": count})
}

func (a *App) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if body.Endpoint != "" {
		_ = a.db.DeletePushSub(body.Endpoint)
	}
	count, _ := a.db.CountPushSubs()
	writeData(w, map[string]any{"subscriptions": count})
}

func (a *App) handlePushTest(w http.ResponseWriter, r *http.Request) {
	conf := a.config()
	if !conf.WebPushEnabled {
		writeErr(w, 400, "اعلان Web Push غیرفعال است")
		return
	}
	subs, err := a.db.ListPushSubs()
	if err != nil || len(subs) == 0 {
		writeErr(w, 400, "هیچ مرورگری برای Web Push ثبت نشده است. ابتدا فعال‌سازی را انجام دهید.")
		return
	}
	sender := a.pushSender()
	sent, failed := 0, 0
	var lastErr string
	for _, sub := range subs {
		err := sender.SendPush(r.Context(), sub, "🔔 تست اعلان پالس", "اگر این پیام را می‌بینید، اعلان‌های مرورگر درست کار می‌کنند.", "pulse-test", "/#/notifications")
		if err == notify.ErrGone {
			_ = a.db.DeletePushSub(sub.Endpoint)
			failed++
			continue
		}
		if err != nil {
			failed++
			lastErr = err.Error()
		} else {
			sent++
		}
	}
	ok := sent > 0
	a.logNotification(&model.NotificationLog{
		At: nowUnix(), Channel: "webpush", RuleName: "تست دستی", Event: "test",
		Title: "تست Web Push", Body: "ارسال پیام آزمایشی", OK: ok, Error: lastErr,
	})
	if !ok {
		writeErr(w, 400, "ارسال ناموفق بود: "+fallback(lastErr, "اشتراک‌ها منقضی شده‌اند"))
		return
	}
	writeData(w, map[string]any{"sent": sent, "failed": failed})
}
