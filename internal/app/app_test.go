package app

import (
	"testing"
	"time"

	"pulse/internal/config"
	"pulse/internal/model"
)

func TestThresholdPass(t *testing.T) {
	if !thresholdPass(model.Rule{Threshold: 14}, notifyEvent{Trigger: "cert_expiring", Value: 10}) {
		t.Error("cert rule should fire at 10 days with threshold 14")
	}
	if thresholdPass(model.Rule{Threshold: 14}, notifyEvent{Trigger: "cert_expiring", Value: 30}) {
		t.Error("cert rule should not fire at 30 days with threshold 14")
	}
	if !thresholdPass(model.Rule{Threshold: 90}, notifyEvent{Trigger: "cpu_high", Value: 95}) {
		t.Error("cpu rule should fire above threshold")
	}
	if thresholdPass(model.Rule{Threshold: 90}, notifyEvent{Trigger: "cpu_high", Value: 50}) {
		t.Error("cpu rule should not fire below threshold")
	}
}

func TestQuietHours(t *testing.T) {
	conf := &config.Settings{QuietHoursEnabled: true, Timezone: "UTC", QuietStart: "23:00", QuietEnd: "07:00"}
	if !inQuietHours(conf, time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC)) {
		t.Error("02:00 should be quiet")
	}
	if inQuietHours(conf, time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)) {
		t.Error("12:00 should not be quiet")
	}
	conf.QuietStart = "01:00"
	conf.QuietEnd = "03:00"
	if !inQuietHours(conf, time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC)) {
		t.Error("02:00 should be inside 01:00-03:00")
	}
}
