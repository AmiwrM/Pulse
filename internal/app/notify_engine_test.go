package app

import (
	"strings"
	"testing"

	"pulse/internal/config"
	"pulse/internal/model"
)

func TestApplyNotifyTemplate(t *testing.T) {
	conf := &config.Settings{
		Timezone:            "UTC",
		NotifyTitleTemplate: "{emoji} {title} · {status}",
		NotifyBodyTemplate:  "<b>{monitor}</b>\n{reason}\n{failed_configs}\n{time}",
	}
	mon := &model.Monitor{Name: "Main & Backup", Type: "proxy", Status: "down", LastError: "bad <script> config"}
	title, plain, htmlBody := applyNotifyTemplate(conf, notifyEvent{
		Trigger: "down", Monitor: mon,
		Extra: map[string]any{"failed_configs": "DE-1: timeout\nUS-2: refused"},
	}, "Pulse Alert", "Pulse Alert\nMain", "<b>Pulse Alert</b>")
	if !strings.HasPrefix(title, "🔴 ") || !strings.Contains(title, "Pulse Alert") || !strings.Contains(title, "قطع") {
		t.Fatalf("unexpected title: %q", title)
	}
	if strings.Contains(htmlBody, "<script>") || !strings.Contains(htmlBody, "bad &lt;script&gt; config") {
		t.Fatalf("placeholder HTML was not escaped: %q", htmlBody)
	}
	if !strings.Contains(htmlBody, "\n") {
		t.Fatalf("newlines should be preserved for Telegram HTML: %q", htmlBody)
	}
	if !strings.Contains(plain, "DE-1: timeout") || !strings.Contains(plain, "bad <script> config") {
		t.Fatalf("unexpected plain body: %q", plain)
	}
	if strings.Contains(plain, "<b>") {
		t.Fatalf("plain body should have HTML tags removed: %q", plain)
	}
}

func TestApplyNotifyTemplateRecoveryFallback(t *testing.T) {
	conf := &config.Settings{Timezone: "UTC", NotifyBodyTemplate: "reason={reason}\ndetails={details}"}
	mon := &model.Monitor{Name: "Main", Type: "http", Status: "up"}
	_, plain, _ := applyNotifyTemplate(conf, notifyEvent{Trigger: "up", Monitor: mon}, "Recovered", "Main is reachable again", "")
	if !strings.Contains(plain, "reason=سرویس دوباره سالم شد") || !strings.Contains(plain, "details=Main is reachable again") {
		t.Fatalf("unexpected recovery fallback: %q", plain)
	}
}

func TestApplyNotifyTemplateDefaults(t *testing.T) {
	conf := &config.Settings{Timezone: "UTC"}
	title, plain, htmlBody := applyNotifyTemplate(conf, notifyEvent{Trigger: "up"}, "Title", "Plain", "<b>HTML</b>")
	if title != "Title" || plain != "Plain" || htmlBody != "<b>HTML</b>" {
		t.Fatalf("empty templates must keep the original message: %q / %q / %q", title, plain, htmlBody)
	}
}
