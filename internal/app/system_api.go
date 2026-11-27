package app

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	panelPortRequestPath = "/var/lib/pulse/port-request"
	panelPortRequestTmp  = "/var/lib/pulse/port-request.tmp"
	panelPortHelperPath  = "/etc/systemd/system/pulse-port.path"
)

func (a *App) handleChangePanelPort(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Port int `json:"port"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if body.Port < 1 || body.Port > 65535 {
		writeErr(w, 400, "پورت باید بین 1 و 65535 باشد")
		return
	}
	current := 0
	if _, port, err := net.SplitHostPort(strings.TrimSpace(a.cfg.Addr)); err == nil {
		current, _ = strconv.Atoi(port)
	}
	if body.Port == current {
		writeErr(w, 400, "پورت جدید با پورت فعلی یکسان است")
		return
	}
	if _, err := os.Stat(panelPortHelperPath); err != nil {
		writeErr(w, 400, fmt.Sprintf(
			"تغییر پورت از داخل پنل روی این نصب فعال نیست. یک‌بار install.sh را دوباره اجرا کنید یا دستور «sudo pulse port %d» را بزنید.",
			body.Port,
		))
		return
	}
	if err := os.WriteFile(panelPortRequestTmp, []byte(strconv.Itoa(body.Port)), 0640); err != nil {
		writeErr(w, 400, "نوشتن درخواست تغییر پورت ناموفق بود: "+err.Error())
		return
	}
	a.db.AddAudit("admin", "system.port", fmt.Sprintf("requested %d -> %d", current, body.Port))
	a.logFor("panel").Info("panel port change requested", "from", current, "to", body.Port)
	writeData(w, map[string]any{
		"ok":      true,
		"pending": true,
		"port":    body.Port,
		"message": "درخواست ثبت شد؛ پنل چند لحظه بعد روی پورت جدید راه‌اندازی می‌شود.",
	})
	// Let the HTTP response flush before the root path unit restarts the panel.
	go func() {
		time.Sleep(800 * time.Millisecond)
		_ = os.Rename(panelPortRequestTmp, panelPortRequestPath)
	}()
}
