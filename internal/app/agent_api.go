package app

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// handleAgentHello is the REST handshake used by the agent before it starts
// reporting. It validates the token and tells the agent which panel/protocol it
// is talking to. It intentionally does not update last_seen: a successful report
// is the source of truth for online/offline status.
func (a *App) handleAgentHello(w http.ResponseWriter, r *http.Request) {
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
	conf := a.config()
	writeData(w, map[string]any{
		"agent": map[string]any{
			"id": ag.ID, "name": ag.Name,
		},
		"server_version":      a.cfg.Version,
		"protocol_version":    2,
		"report_interval_sec": 1,
		"offline_after_sec":   conf.AgentOfflineAfterSec,
		"server_time":         nowUnix(),
	})
}

func agentToken(r *http.Request) string {
	if token := strings.TrimSpace(r.Header.Get("X-Agent-Token")); token != "" {
		return token
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func (a *App) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := a.db.ListAgents()
	if err != nil {
		writeErr(w, 500, "دریافت سرورها ناموفق بود")
		return
	}
	conf := a.config()
	writeData(w, map[string]any{
		"agents":            agents,
		"server_time":       nowUnix(),
		"offline_after_sec": conf.AgentOfflineAfterSec,
	})
}

func (a *App) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeErr(w, 400, "نام سرور الزامی است")
		return
	}
	if len([]rune(body.Name)) > 80 {
		writeErr(w, 400, "نام سرور طولانی است")
		return
	}
	ag, token, err := a.db.CreateAgent(body.Name)
	if err != nil {
		writeErr(w, 500, "ساخت ایجنت ناموفق بود")
		return
	}
	server := a.publicBase(r)
	conf := a.config()
	downloadURL := conf.AgentDownloadURL
	command := fmt.Sprintf("sudo ./pulse --agent --server %s --token %s --name %s --interval 1s",
		shellQuote(server), shellQuote(token), shellQuote(body.Name))
	installCommand := agentInstallCommand(server, token, body.Name)
	unit := agentUnit(server, token, body.Name)
	a.db.AddAudit("admin", "agent.create", body.Name)
	a.hub.broadcast("agent.created", map[string]any{"id": ag.ID})
	writeData(w, map[string]any{
		"agent": ag, "token": token, "server_url": server,
		"command": command, "install_command": installCommand, "unit": unit,
		"download_url":     downloadURL,
		"download_command": downloadCommand(downloadURL),
	})
}

func (a *App) handleRotateAgent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ag, err := a.db.GetAgent(id)
	if err != nil || ag == nil {
		writeErr(w, 404, "سرور پیدا نشد")
		return
	}
	token, err := a.db.RotateAgentToken(id)
	if err != nil {
		writeErr(w, 500, "تولید توکن جدید ناموفق بود")
		return
	}
	server := a.publicBase(r)
	command := fmt.Sprintf("sudo ./pulse --agent --server %s --token %s --name %s --interval 1s",
		shellQuote(server), shellQuote(token), shellQuote(ag.Name))
	a.db.AddAudit("admin", "agent.rotate", ag.Name)
	writeData(w, map[string]any{
		"token": token, "command": command, "server_url": server,
		"install_command":  agentInstallCommand(server, token, ag.Name),
		"unit":             agentUnit(server, token, ag.Name),
		"download_url":     a.config().AgentDownloadURL,
		"download_command": downloadCommand(a.config().AgentDownloadURL),
	})
}

func (a *App) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var body struct {
		Name  string `json:"name"`
		Notes string `json:"notes"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeErr(w, 400, "نام سرور الزامی است")
		return
	}
	if err := a.db.UpdateAgentMeta(id, body.Name, strings.TrimSpace(body.Notes)); err != nil {
		writeErr(w, 500, "ذخیره ناموفق بود")
		return
	}
	a.hub.broadcast("agent.updated", map[string]any{"agent_id": id})
	writeData(w, map[string]any{"ok": true})
}

func (a *App) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ag, _ := a.db.GetAgent(id)
	if ag == nil {
		writeErr(w, 404, "سرور پیدا نشد")
		return
	}
	if err := a.db.DeleteAgent(id); err != nil {
		writeErr(w, 500, "حذف سرور ناموفق بود")
		return
	}
	a.db.AddAudit("admin", "agent.delete", ag.Name)
	a.hub.broadcast("agent.deleted", map[string]any{"agent_id": id})
	writeData(w, map[string]any{"ok": true})
}

func (a *App) handleAgentMetrics(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	hours := queryInt(r, "hours", 24)
	if hours < 1 {
		hours = 1
	}
	if hours > 168 {
		hours = 168
	}
	ag, err := a.db.GetAgent(id)
	if err != nil || ag == nil {
		writeErr(w, 404, "سرور پیدا نشد")
		return
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
	metrics, err := a.db.AgentMetrics(id, since)
	if err != nil {
		writeErr(w, 500, "دریافت نمودار ناموفق بود")
		return
	}
	writeData(w, map[string]any{"agent": ag, "metrics": metrics, "server_time": nowUnix()})
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

const agentInstallerURL = "https://raw.githubusercontent.com/AmiwrM/Pulse/main/deploy/install-agent.sh"

func agentInstallCommand(server, token, name string) string {
	return "curl -fsSL " + shellQuote(agentInstallerURL) + " | sudo bash -s -- " +
		"--server " + shellQuote(server) + " --token " + shellQuote(token) +
		" --name " + shellQuote(name) + " --interval 1s"
}

func downloadCommand(url string) string {
	if strings.TrimSpace(url) == "" {
		return ""
	}
	return "curl -fsSL " + shellQuote(strings.TrimSpace(url)) + " -o /tmp/pulse && chmod 0755 /tmp/pulse"
}

func agentUnit(server, token, name string) string {
	return fmt.Sprintf(`[Unit]
Description=Pulse server agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
# برای پایش سرویس‌ها، --services "xray,wireguard" را به ExecStart اضافه کنید.
ExecStart=/usr/local/bin/pulse --agent --server %s --token %s --name %s --interval 1s --wg
Restart=always
RestartSec=5
TimeoutStopSec=20
LimitNOFILE=65536
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true

[Install]
WantedBy=multi-user.target
`, server, token, shellQuote(name))
}
