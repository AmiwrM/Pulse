package app

import (
	"path/filepath"
	"testing"
	"time"

	"pulse/internal/config"
	"pulse/internal/model"
	"pulse/internal/store"
)

func TestIsPublicReportIP(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8": true, "2001:4860:4860::8888": true,
		"192.168.1.10": false, "10.0.0.1": false, "127.0.0.1": false,
		"::1": false, "fe80::1": false, "": false,
	}
	for ip, want := range cases {
		if got := isPublicReportIP(ip); got != want {
			t.Errorf("isPublicReportIP(%q) = %v, want %v", ip, got, want)
		}
	}
}

func TestMigrateCFAccounts(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "pulse.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := &App{db: db}
	a.migrateCFAccounts(&config.Settings{CloudflareToken: "legacy-token"})
	accounts, err := db.ListCFAccounts()
	if err != nil || len(accounts) != 1 {
		t.Fatalf("expected one migrated account, got %+v, %v", accounts, err)
	}
	if accounts[0].Token != "legacy-token" || accounts[0].Name != "Default" {
		t.Fatalf("unexpected migrated account: %+v", accounts[0])
	}
	if marker, _ := db.GetSetting("cf_accounts_migrated"); marker != "1" {
		t.Fatalf("migration marker not set: %q", marker)
	}
	if err := db.DeleteCFAccount(accounts[0].ID); err != nil {
		t.Fatal(err)
	}
	a.migrateCFAccounts(&config.Settings{CloudflareToken: "legacy-token"})
	accounts, _ = db.ListCFAccounts()
	if len(accounts) != 0 {
		t.Fatalf("migration must not resurrect a deleted account: %+v", accounts)
	}
}

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
