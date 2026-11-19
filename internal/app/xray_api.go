package app

import (
	"context"
	"net/http"
	"time"
)

func (a *App) handleXrayStatus(w http.ResponseWriter, r *http.Request) {
	writeData(w, map[string]any{
		"status":    a.xray.Status(),
		"binary":    a.config().XrayBinary,
		"test_url":  a.config().XrayTestURL,
		"protocols": []string{"vmess", "vless", "trojan", "shadowsocks", "wireguard", "socks"},
		"test_mode": "real HTTP request through a temporary Xray core",
	})
}

func (a *App) handleXrayDetect(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	status := a.xray.Detect(ctx)
	a.hub.broadcast("xray.status", status)
	writeData(w, map[string]any{
		"status":   status,
		"binary":   a.xray.Binary(),
		"test_url": a.config().XrayTestURL,
	})
}
