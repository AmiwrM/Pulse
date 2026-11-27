package xray

import (
	"encoding/json"
	"testing"
)

func TestBuildVLESSEncryption(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", "none"},
		{"mlkem768x25519plus.native.0rtt.testkey", "mlkem768x25519plus.native.0rtt.testkey"},
	} {
		raw, err := buildConfig(Node{
			Protocol: "vless", Address: "example.com", Port: 443,
			UUID:    "11111111-2222-3333-4444-555555555555",
			Network: "tcp", Security: "none", Encryption: tc.in,
		}, 1080)
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct {
			Outbounds []struct {
				Settings struct {
					Vnext []struct {
						Users []struct {
							Encryption string `json:"encryption"`
						} `json:"users"`
					} `json:"vnext"`
				} `json:"settings"`
			} `json:"outbounds"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatal(err)
		}
		got := ""
		if len(cfg.Outbounds) > 0 && len(cfg.Outbounds[0].Settings.Vnext) > 0 && len(cfg.Outbounds[0].Settings.Vnext[0].Users) > 0 {
			got = cfg.Outbounds[0].Settings.Vnext[0].Users[0].Encryption
		}
		if got != tc.want {
			t.Errorf("encryption %q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

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
