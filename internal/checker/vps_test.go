package checker

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"pulse/internal/model"
)

func TestPickVirtualizorVPS(t *testing.T) {
	raw := []byte(`{
		"title":"VPS Statistics",
		"vps_data":{
			"K_4":{"vpsid":4,"hostname":"vm1.example.com","status":1,"used_bandwidth":24.1,"bandwidth":500},
			"K_5":{"vpsid":5,"hostname":"vm2.example.com","status":0,"used_bandwidth":10,"bandwidth":100}
		}
	}`)
	entry, err := pickVirtualizorVPS(raw, 4)
	if err != nil {
		t.Fatalf("pickVirtualizorVPS: %v", err)
	}
	if anyString(entry.Hostname) != "vm1.example.com" || anyFloat(entry.UsedBandwidth) != 24.1 {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if _, err := pickVirtualizorVPS(raw, 9); err == nil {
		t.Fatal("expected missing VPS error")
	}
	apiErr := []byte(`{"title":"Error","error":"Invalid API key"}`)
	if _, err := pickVirtualizorVPS(apiErr, 4); err == nil {
		t.Fatal("expected API error")
	}
}

func TestCheckVirtualizorRemainingThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"VPS Statistics","vps_data":{"K_4":{"vpsid":4,"hostname":"vm1.example.com","status":1,"used_bandwidth":490,"bandwidth":500}}}`))
	}))
	defer srv.Close()
	cfg, _ := json.Marshal(map[string]any{
		"panel_url": srv.URL, "api_key": "key", "api_pass": "secret", "vps_id": 4,
		"threshold_mode": "remaining", "warn_remaining_gb": 20, "critical_remaining_gb": 5,
	})
	res := (&Manager{}).checkVirtualizor(context.Background(), model.Monitor{Type: "vps", Config: cfg})
	if res.Meta["traffic_state"] != "warning" || res.Warn == "" {
		t.Fatalf("expected remaining warning, got %+v", res)
	}
	cfg, _ = json.Marshal(map[string]any{
		"panel_url": srv.URL, "api_key": "key", "api_pass": "secret", "vps_id": 4,
		"threshold_mode": "remaining", "warn_remaining_gb": 20, "critical_remaining_gb": 12,
	})
	res = (&Manager{}).checkVirtualizor(context.Background(), model.Monitor{Type: "vps", Config: cfg})
	if res.Meta["traffic_state"] != "critical" || res.Error == "" {
		t.Fatalf("expected remaining critical, got %+v", res)
	}
}

func TestCheckVirtualizor(t *testing.T) {
	var authErr error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("act") != "vps_stats" || q.Get("vpsid") != "4" || q.Get("api") != "json" {
			authErr = fmt.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		if q.Get("adminapikey") != "key" || q.Get("adminapipass") != "secret" {
			authErr = fmt.Errorf("missing admin credentials: %s", r.URL.RawQuery)
		}
		sig := q.Get("apikey")
		if len(sig) != 40 {
			authErr = fmt.Errorf("invalid signature length: %q", sig)
		} else {
			sum := md5.Sum([]byte("secret" + sig[:8]))
			if sig[8:] != hex.EncodeToString(sum[:]) {
				authErr = fmt.Errorf("invalid signature: %q", sig)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"VPS Statistics","vps_data":{"K_4":{"vpsid":4,"hostname":"vm1.example.com","status":1,"used_bandwidth":480,"bandwidth":500,"net_in":1024,"net_out":2048}}}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(map[string]any{
		"panel_url": srv.URL, "api_key": "key", "api_pass": "secret",
		"vps_id": 4, "warn_percent": 80, "critical_percent": 95,
	})
	res := (&Manager{}).checkVirtualizor(context.Background(), model.Monitor{Type: "vps", Config: cfg})
	if authErr != nil {
		t.Fatal(authErr)
	}
	if res.OK || res.Error == "" {
		t.Fatalf("expected critical VPS, got %+v", res)
	}
	if res.Meta["used_gb"] != 480.0 || res.Meta["limit_gb"] != 500.0 || res.Meta["remaining_gb"] != 20.0 {
		t.Fatalf("unexpected traffic meta: %+v", res.Meta)
	}
	if res.Meta["traffic_state"] != "critical" {
		t.Fatalf("expected critical traffic state, got %+v", res.Meta)
	}
}
