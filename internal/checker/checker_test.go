package checker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"pulse/internal/model"
)

func TestStatusMatches(t *testing.T) {
	cases := []struct {
		rule string
		code int
		want bool
	}{
		{"", 200, true},
		{"", 302, true},
		{"", 500, false},
		{"2xx", 204, true},
		{"2xx", 301, false},
		{"200-299", 250, true},
		{"200,204,301", 301, true},
		{"200,204,301", 302, false},
		{"4xx", 404, true},
	}
	for _, c := range cases {
		if got := statusMatches(c.rule, c.code); got != c.want {
			t.Errorf("statusMatches(%q, %d) = %v, want %v", c.rule, c.code, got, c.want)
		}
	}
}

func TestValidateMonitor(t *testing.T) {
	cases := []struct {
		typ    string
		target string
		cfg    string
		ok     bool
	}{
		{"http", "https://example.com/health", `{"expected_status":"2xx"}`, true},
		{"http", "example.com", `{}`, false},
		{"tcp", "example.com:443", `{}`, true},
		{"tcp", "example.com", `{}`, false},
		{"ping", "1.1.1.1", `{}`, true},
		{"ping", "http://1.1.1.1", `{}`, false},
		{"dns", "example.com", `{"record":"A"}`, true},
		{"dns", "example.com", `{"record":"BAD"}`, false},
		{"ssl", "example.com", `{}`, true},
		{"proxy", "sub", `{"input":"vless://id@example.com:443?security=tls"}`, true},
		{"proxy", "sub", `{}`, false},
	}
	for _, c := range cases {
		err := Validate(c.typ, c.target, json.RawMessage(c.cfg))
		if (err == nil) != c.ok {
			t.Errorf("Validate(%s, %s, %s) error=%v, want ok=%v", c.typ, c.target, c.cfg, err, c.ok)
		}
	}
}

func TestParseProxyNodes(t *testing.T) {
	vmessObj := map[string]any{"add": "vmess.example.com", "port": "443", "ps": "VMess Node", "tls": "tls", "sni": "vmess.example.com", "id": "11111111-2222-3333-4444-555555555555", "aid": "0", "net": "tcp"}
	raw, _ := json.Marshal(vmessObj)
	vmess := "vmess://" + base64.RawStdEncoding.EncodeToString(raw)
	ssUser := base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:password"))
	input := strings.Join([]string{
		vmess,
		"vless://11111111-2222-3333-4444-555555555555@vless.example.com:443?security=reality&sni=cdn.example.com&pbk=abcdef&sid=1234&fp=chrome#VLESS%20Node",
		"trojan://pass@trojan.example.com:8443?security=tls&sni=trojan.example.com#Trojan",
		"ss://" + ssUser + "@ss.example.com:8388#SS",
		"hysteria2://auth@hy.example.com:443#Hysteria",
	}, "\n")
	nodes := parseProxyNodes(input)
	if len(nodes) != 5 {
		t.Fatalf("expected 5 nodes, got %d: %+v", len(nodes), nodes)
	}
	if nodes[0].Name != "VMess Node" || !nodes[0].TLS {
		t.Errorf("unexpected vmess node: %+v", nodes[0])
	}
	if nodes[0].XrayNode == nil || nodes[0].XrayNode.UUID == "" || nodes[0].XrayNode.Network != "tcp" {
		t.Errorf("vmess real-test spec missing: %+v", nodes[0].XrayNode)
	}
	if nodes[1].Protocol != "vless" || nodes[1].TLS {
		t.Errorf("vless reality should use a real Xray test, got %+v", nodes[1])
	}
	if nodes[1].XrayNode == nil || nodes[1].XrayNode.Security != "reality" || nodes[1].XrayNode.PublicKey == "" {
		t.Errorf("reality real-test spec missing: %+v", nodes[1].XrayNode)
	}
	if nodes[2].Address != "trojan.example.com:8443" || !nodes[2].TLS {
		t.Errorf("unexpected trojan node: %+v", nodes[2])
	}
	if nodes[2].XrayNode == nil || nodes[2].XrayNode.Password != "pass" {
		t.Errorf("trojan real-test spec missing: %+v", nodes[2].XrayNode)
	}
	if nodes[3].Address != "ss.example.com:8388" {
		t.Errorf("unexpected ss node: %+v", nodes[3])
	}
	if nodes[3].XrayNode == nil || nodes[3].XrayNode.Cipher != "aes-256-gcm" {
		t.Errorf("ss real-test spec missing: %+v", nodes[3].XrayNode)
	}
	if !nodes[4].UDP {
		t.Errorf("hysteria2 should use UDP probe: %+v", nodes[4])
	}
}

func TestParseWireGuardNodes(t *testing.T) {
	conf := `[Interface]
PrivateKey = xxx
Address = 10.0.0.2/32

[Peer]
PublicKey = yyy
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0

[Peer]
PublicKey = zzz
Endpoint = 198.51.100.8:51821
`
	nodes := parseWireGuardNodes(conf)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Address != "vpn.example.com:51820" || !nodes[0].UDP {
		t.Errorf("unexpected node: %+v", nodes[0])
	}
	if nodes[0].XrayNode == nil || nodes[0].XrayNode.Protocol != "wireguard" || len(nodes[0].XrayNode.WGPeers) != 1 {
		t.Errorf("wireguard real-test spec missing: %+v", nodes[0].XrayNode)
	}
}

func TestBase64Subscription(t *testing.T) {
	raw := "vless://id@example.com:443?security=tls#Node"
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	got := maybeDecodeBase64(encoded)
	if got != raw {
		t.Fatalf("base64 decode failed: %q", got)
	}
}

func TestProxyConfigValidation(t *testing.T) {
	mon := model.Monitor{Type: "proxy", Config: json.RawMessage(`{"input":"vmess://bad"}`)}
	// Parsing an invalid vmess must produce a useful error rather than a panic.
	m := New("1.1.1.1:53")
	res := m.checkProxy(context.Background(), mon)
	if res.Error == "" {
		t.Fatalf("expected an error for invalid config")
	}
}
