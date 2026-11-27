package xray

import (
	"encoding/json"
	"testing"
)

func TestBuildXHTTPConfig(t *testing.T) {
	for _, network := range []string{"xhttp", "splithttp"} {
		raw, err := buildConfig(Node{
			Protocol: "vless", Address: "cdn.example.com", Port: 443,
			UUID:    "11111111-2222-3333-4444-555555555555",
			Network: network, Security: "tls", SNI: "cdn.example.com",
			HostHeader: "cdn.example.com", Path: "/x", Mode: "auto",
		}, 1080)
		if err != nil {
			t.Fatalf("%s: buildConfig: %v", network, err)
		}
		var cfg struct {
			Outbounds []struct {
				StreamSettings struct {
					Network string `json:"network"`
					XHTTP   struct {
						Host string `json:"host"`
						Path string `json:"path"`
						Mode string `json:"mode"`
					} `json:"xhttpSettings"`
				} `json:"streamSettings"`
			} `json:"outbounds"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("%s: unmarshal: %v", network, err)
		}
		if len(cfg.Outbounds) != 1 {
			t.Fatalf("%s: expected one outbound", network)
		}
		ss := cfg.Outbounds[0].StreamSettings
		if ss.Network != "xhttp" {
			t.Errorf("%s: network = %q, want xhttp", network, ss.Network)
		}
		if ss.XHTTP.Host != "cdn.example.com" || ss.XHTTP.Path != "/x" || ss.XHTTP.Mode != "auto" {
			t.Errorf("%s: unexpected xhttpSettings: %+v", network, ss.XHTTP)
		}
	}
}
