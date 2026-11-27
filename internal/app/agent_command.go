package app

import (
	"net/http"

	"pulse/internal/agentproto"
)

func (a *App) handleAgentCommand(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var command string
	switch body.Action {
	case "restart":
		command = agentproto.CommandRestart
	case "update":
		command = agentproto.CommandUpdate
	default:
		writeErr(w, http.StatusBadRequest, "عملیات ایجنت معتبر نیست")
		return
	}
	sess := a.links.get(id)
	if sess == nil {
		writeErr(w, http.StatusConflict, "اتصال زندهٔ ایجنت برقرار نیست؛ ابتدا اتصال را بررسی کنید")
		return
	}
	if err := sess.sendCommand(&agentproto.PanelCommand{Type: command}); err != nil {
		writeErr(w, http.StatusBadGateway, "ارسال فرمان به ایجنت ناموفق بود: "+err.Error())
		return
	}
	a.db.AddAudit("admin", "agent."+body.Action, "فرمان "+body.Action+" برای ایجنت ارسال شد")
	a.logFor("agent").Info("agent command sent", "agent_id", id, "action", body.Action)
	writeData(w, map[string]any{"sent": true, "action": body.Action})
}
