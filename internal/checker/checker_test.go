package checker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

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
		{"vps", "VPS 1", `{"panel_url":"https://panel.example.com:4085","api_key":"k","api_pass":"p","vps_id":1}`, true},
		{"vps", "VPS 1", `{"panel_url":"panel.example.com","api_key":"k","api_pass":"p","vps_id":1}`, false},
		{"vps", "VPS 1", `{"panel_url":"https://panel.example.com:4085","api_key":"k","api_pass":"p"}`, false},
		{"vps", "VPS 1", `{"panel_url":"https://panel.example.com:4085","api_key":"k","api_pass":"p","vps_id":1,"threshold_mode":"remaining","warn_remaining_gb":10,"critical_remaining_gb":2}`, true},
		{"vps", "VPS 1", `{"panel_url":"https://panel.example.com:4085","api_key":"k","api_pass":"p","vps_id":1,"threshold_mode":"bogus"}`, false},
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
		"vless://11111111-2222-3333-4444-555555555555@vless.example.com:443?security=reality&sni=cdn.example.com&pbk=abcdef&sid=1234&fp=chrome&encryption=mlkem768x25519plus.native.0rtt.testkey#VLESS%20Node",
		"trojan://pass@trojan.example.com:8443?security=tls&sni=trojan.example.com#Trojan",
		"ss://" + ssUser + "@ss.example.com:8388#SS",
		"hysteria2://auth@hy.example.com:443#Hysteria",
		"vless://11111111-2222-3333-4444-555555555555@cdn.example.com:443?security=tls&type=xhttp&sni=cdn.example.com&host=cdn.example.com&path=%2Fx&mode=auto#XHTTP",
	}, "\n")
	nodes := parseProxyNodes(input)
	if len(nodes) != 6 {
		t.Fatalf("expected 6 nodes, got %d: %+v", len(nodes), nodes)
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
	if nodes[1].XrayNode == nil || nodes[1].XrayNode.Encryption != "mlkem768x25519plus.native.0rtt.testkey" {
		t.Errorf("vless encryption missing: %+v", nodes[1].XrayNode)
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
	if nodes[5].XrayNode == nil || nodes[5].XrayNode.Network != "xhttp" || nodes[5].XrayNode.Mode != "auto" {
		t.Errorf("xhttp real-test spec missing: %+v", nodes[5].XrayNode)
	}
}

func TestProxyCheckTimeoutFloor(t *testing.T) {
	cases := []struct {
		maxReal int
		want    time.Duration
	}{
		{0, 60 * time.Second},
		{1, 30 * time.Second},
		{4, 30 * time.Second},
		{5, 45 * time.Second},
		{10, 60 * time.Second},
		{30, 135 * time.Second},
	}
	for _, c := range cases {
		raw, _ := json.Marshal(map[string]any{"max_real_nodes": c.maxReal})
		if got := proxyCheckTimeoutFloor(raw); got != c.want {
			t.Errorf("proxyCheckTimeoutFloor(max=%d) = %s, want %s", c.maxReal, got, c.want)
		}
	}
}

func TestFastProbeTimeoutIsBounded(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	start := time.Now()
	done := make(chan ProxyNode, 1)
	go func() {
		done <- testNodeWithTimeout(context.Background(), ProxyNode{
			Protocol: "vless", Address: ln.Addr().String(), TLS: true, SNI: "example.com",
		}, true, 150*time.Millisecond)
	}()
	select {
	case node := <-done:
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("fast probe took %s, expected the per-node cap", elapsed)
		}
		if node.OK {
			t.Fatalf("expected the stalled TLS probe to fail: %+v", node)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fast probe did not honor its timeout")
	}
	select {
	case conn := <-accepted:
		_ = conn.Close()
	case <-time.After(time.Second):
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
