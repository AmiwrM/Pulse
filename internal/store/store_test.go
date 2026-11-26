package store

import (
	"path/filepath"
	"testing"

	"pulse/internal/model"
)

func TestMigrateAndSeed(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "pulse.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rules, err := db.ListRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) < 8 {
		t.Fatalf("expected seeded rules, got %d", len(rules))
	}

	m := &model.Monitor{
		Name: "Sample", Type: "http", Target: "https://example.com", Enabled: true,
		Interval: 60, Timeout: 10000, FailThreshold: 2, RecoverThreshold: 1,
		Config: []byte(`{"expected_status":"2xx"}`),
	}
	if err := db.CreateMonitor(m); err != nil {
		t.Fatal(err)
	}
	if m.ID == 0 {
		t.Fatal("monitor id was not assigned")
	}

	check := &model.Check{At: nowUnix(), OK: false, Status: "down", Error: "boom", Meta: []byte(`{}`)}
	m.Status = "down"
	m.DownSince = check.At
	if err := db.CommitCheck(m, check, true, false); err != nil {
		t.Fatal(err)
	}

	incidents, err := db.ListIncidents(10, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(incidents) != 1 {
		t.Fatalf("expected 1 active incident, got %d", len(incidents))
	}

	if err := db.SetMonitorEnabled(m.ID, false); err != nil {
		t.Fatal(err)
	}
	incidents, _ = db.ListIncidents(10, true)
	if len(incidents) != 0 {
		t.Fatalf("pausing should close active incidents, got %d", len(incidents))
	}
}

func TestMonitorProbeAgentAndDelete(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "pulse.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ag, _, err := db.CreateAgent("iran-probe")
	if err != nil {
		t.Fatal(err)
	}
	m := &model.Monitor{
		Name: "probe target", Type: "tcp", Target: "127.0.0.1:80", Enabled: true,
		Interval: 60, Timeout: 3000, ProbeAgentID: ag.ID,
		Config: []byte(`{}`), FailThreshold: 2, RecoverThreshold: 1,
	}
	if err := db.CreateMonitor(m); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetMonitor(m.ID)
	if err != nil || got == nil {
		t.Fatalf("get monitor: %v", err)
	}
	if got.ProbeAgentID != ag.ID {
		t.Fatalf("probe agent was not persisted: got %d want %d", got.ProbeAgentID, ag.ID)
	}
	if err := db.DeleteAgent(ag.ID); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetMonitor(m.ID)
	if err != nil || got == nil {
		t.Fatalf("get monitor after delete: %v", err)
	}
	if got.ProbeAgentID != 0 {
		t.Fatalf("deleting an agent must clear its monitors, got %d", got.ProbeAgentID)
	}
}

func TestApplicationLogs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "pulse.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.AddLog(model.LogEntry{At: nowUnix(), Level: "info", Category: "panel", Message: "started"}); err != nil {
		t.Fatal(err)
	}
	if err := db.AddLog(model.LogEntry{At: nowUnix(), Level: "error", Category: "xray", Message: "core failed", Detail: `{"code":1}`}); err != nil {
		t.Fatal(err)
	}
	all, err := db.ListLogs(LogFilter{Limit: 10})
	if err != nil || len(all) != 2 {
		t.Fatalf("list logs: %d entries, %v", len(all), err)
	}
	xray, err := db.ListLogs(LogFilter{Category: "xray", Level: "error", Query: "core", Limit: 10})
	if err != nil || len(xray) != 1 || xray[0].Message != "core failed" {
		t.Fatalf("filtered logs: %+v, %v", xray, err)
	}
	db.CleanupLogs(nowUnix()+1, 1)
	remaining, _ := db.ListLogs(LogFilter{Limit: 10})
	if len(remaining) != 0 {
		t.Fatalf("expected cleanup to remove old logs, got %d", len(remaining))
	}
}

func TestUsersAndPushSubs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "pulse.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.CreateUser("admin", "hash"); err != nil {
		t.Fatal(err)
	}
	user, err := db.GetUser("admin")
	if err != nil || user == nil {
		t.Fatalf("user lookup failed: %v", err)
	}
	if err := db.SavePushSub(model.PushSub{Endpoint: "https://push.example.com/1", P256DH: "a", Auth: "b"}); err != nil {
		t.Fatal(err)
	}
	subs, err := db.ListPushSubs()
	if err != nil || len(subs) != 1 {
		t.Fatalf("expected one push subscription, got %d (%v)", len(subs), err)
	}
	if err := db.DeletePushSub("https://push.example.com/1"); err != nil {
		t.Fatal(err)
	}
}
