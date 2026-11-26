package app

import (
	"net/http"

	"pulse/internal/store"
)

var logCategories = []string{"panel", "scheduler", "monitor", "xray", "cloudflare", "notify", "agent", "security", "api"}
var logLevels = []string{"debug", "info", "warn", "error"}

func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	filter := store.LogFilter{
		Category: r.URL.Query().Get("category"),
		Level:    r.URL.Query().Get("level"),
		Query:    r.URL.Query().Get("q"),
		AfterID:  int64(queryInt(r, "after", 0)),
		BeforeID: int64(queryInt(r, "before", 0)),
		Limit:    queryInt(r, "limit", 200),
	}
	entries, err := a.db.ListLogs(filter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "دریافت لاگ‌ها ناموفق بود")
		return
	}
	writeData(w, map[string]any{
		"logs":        entries,
		"categories":  logCategories,
		"levels":      logLevels,
		"server_time": nowUnix(),
	})
}
