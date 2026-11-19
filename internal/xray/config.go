package xray

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Node is a complete client-side description of one proxy endpoint. It is
// intentionally independent from the checker package so the engine stays
// reusable and testable.
type Node struct {
	Name     string
	Protocol string
	Address  string
	Port     int

	UUID     string
	Password string
	Cipher   string
	AlterID  int
	Flow     string
	User     string

	// Transport and TLS.
	Network     string
	HeaderType  string
	Security    string // none, tls, reality
	SNI         string
	HostHeader  string
	Path        string
	ServiceName string
	Fingerprint string
	PublicKey   string
	ShortID     string
	SpiderX     string
	ALPN        []string

	// WireGuard outbound.
	WGSecretKey   string
	WGAddress     []string
	WGPeers       []WGPeer
	WGMTU         int
	WGNoKernelTun bool
}

type WGPeer struct {
	PublicKey    string
	PreSharedKey string
	Endpoint     string
	AllowedIPs   []string
	KeepAlive    int
}

// Supported reports whether Xray can natively dial this node.
func (n Node) Supported() bool {
	switch n.Protocol {
	case "vmess", "vless", "trojan", "shadowsocks", "socks", "wireguard":
		return n.Address != "" && n.Port > 0
	}
	return false
}

type vlessUser struct {
	ID         string `json:"id"`
	Encryption string `json:"encryption"`
	Flow       string `json:"flow,omitempty"`
}

type vnext struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Users   []any  `json:"users"`
}

func (n Node) outboundConfig() (map[string]any, error) {
	if !n.Supported() {
		return nil, fmt.Errorf("پروتکل %s برای تست واقعی Xray پشتیبانی نمی‌شود", n.Protocol)
	}
	settings := map[string]any{}
	var protocol string

	switch n.Protocol {
	case "vmess":
		if n.UUID == "" {
			return nil, fmt.Errorf("UUID در کانفیگ VMess وجود ندارد")
		}
		user := map[string]any{"id": n.UUID, "alterId": n.AlterID, "security": "auto"}
		settings["vnext"] = []vnext{{
			Address: n.Address, Port: n.Port, Users: []any{user},
		}}
		protocol = "vmess"
	case "vless":
		if n.UUID == "" {
			return nil, fmt.Errorf("UUID در کانفیگ VLESS وجود ندارد")
		}
		user := vlessUser{ID: n.UUID, Encryption: "none", Flow: n.Flow}
		settings["vnext"] = []vnext{{
			Address: n.Address, Port: n.Port, Users: []any{user},
		}}
		protocol = "vless"
	case "trojan":
		if n.Password == "" {
			return nil, fmt.Errorf("رمز در کانفیگ Trojan وجود ندارد")
		}
		server := map[string]any{"address": n.Address, "port": n.Port, "password": n.Password}
		if n.Flow != "" {
			server["flow"] = n.Flow
		}
		settings["servers"] = []any{server}
		protocol = "trojan"
	case "shadowsocks":
		if n.Cipher == "" || n.Password == "" {
			return nil, fmt.Errorf("متد یا رمز Shadowsocks ناقص است")
		}
		settings["servers"] = []any{map[string]any{
			"address": n.Address, "port": n.Port, "method": n.Cipher, "password": n.Password,
		}}
		protocol = "shadowsocks"
	case "socks":
		server := map[string]any{"address": n.Address, "port": n.Port}
		if n.User != "" || n.Password != "" {
			server["users"] = []any{map[string]any{"user": n.User, "pass": n.Password}}
		}
		settings["servers"] = []any{server}
		protocol = "socks"
	case "wireguard":
		if n.WGSecretKey == "" || len(n.WGPeers) == 0 {
			return nil, fmt.Errorf("کلید خصوصی یا Peer در کانفیگ WireGuard وجود ندارد")
		}
		peers := make([]map[string]any, 0, len(n.WGPeers))
		for _, p := range n.WGPeers {
			peer := map[string]any{
				"publicKey": p.PublicKey,
				"endpoint":  p.Endpoint,
			}
			if p.PreSharedKey != "" {
				peer["preSharedKey"] = p.PreSharedKey
			}
			if len(p.AllowedIPs) > 0 {
				peer["allowedIPs"] = p.AllowedIPs
			}
			if p.KeepAlive > 0 {
				peer["keepAlive"] = p.KeepAlive
			}
			peers = append(peers, peer)
		}
		mtu := n.WGMTU
		if mtu <= 0 {
			mtu = 1420
		}
		addresses := n.WGAddress
		if len(addresses) == 0 {
			addresses = []string{"10.66.66.2/32"}
		}
		settings["secretKey"] = n.WGSecretKey
		settings["address"] = addresses
		settings["mtu"] = mtu
		settings["peers"] = peers
		settings["remoteDNS"] = []string{"1.1.1.1", "8.8.8.8"}
		// The monitoring process usually runs unprivileged; Xray's userspace
		// netstack performs a real WireGuard handshake without creating a TUN.
		settings["noKernelTun"] = true
		protocol = "wireguard"
	}
	if protocol == "" {
		return nil, fmt.Errorf("پروتکل پشتیبانی نمی‌شود")
	}

	stream, err := streamSettings(n)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"tag":      "proxy",
		"protocol": protocol,
		"settings": settings,
	}
	if len(stream) > 0 {
		out["streamSettings"] = stream
	}
	return out, nil
}

func streamSettings(n Node) (map[string]any, error) {
	switch n.Protocol {
	case "wireguard", "shadowsocks", "socks":
		return nil, nil
	}
	network := strings.ToLower(strings.TrimSpace(n.Network))
	if network == "" {
		network = "tcp"
	}
	if network == "h2c" {
		network = "http"
	}
	ss := map[string]any{}

	switch network {
	case "tcp", "raw":
		ss["network"] = "tcp"
		if strings.EqualFold(n.HeaderType, "http") {
			host := n.HostHeader
			if host == "" {
				host = n.SNI
			}
			req := map[string]any{"version": "1.1", "method": "GET", "path": []string{"/"}}
			if host != "" {
				req["headers"] = map[string]any{"Host": []string{host}}
			}
			ss["tcpSettings"] = map[string]any{"header": map[string]any{"type": "http", "request": req}}
		}
	case "ws":
		ws := map[string]any{}
		if n.Path != "" {
			ws["path"] = n.Path
		}
		if n.HostHeader != "" {
			ws["headers"] = map[string]any{"Host": n.HostHeader}
		}
		ss["network"] = "ws"
		ss["wsSettings"] = ws
	case "grpc", "gun":
		grpc := map[string]any{}
		service := n.ServiceName
		if service == "" {
			service = n.Path
		}
		if service != "" {
			grpc["serviceName"] = service
		}
		if n.HostHeader != "" {
			grpc["authority"] = n.HostHeader
		}
		ss["network"] = "grpc"
		ss["grpcSettings"] = grpc
	case "http", "h2":
		httpSettings := map[string]any{}
		if n.Path != "" {
			httpSettings["path"] = n.Path
		}
		if n.HostHeader != "" {
			httpSettings["host"] = []string{n.HostHeader}
		}
		ss["network"] = "http"
		ss["httpSettings"] = httpSettings
	case "httpupgrade":
		hu := map[string]any{}
		if n.Path != "" {
			hu["path"] = n.Path
		}
		if n.HostHeader != "" {
			hu["host"] = n.HostHeader
		}
		ss["network"] = "httpupgrade"
		ss["httpupgradeSettings"] = hu
	case "kcp", "mkcp":
		header := n.HeaderType
		if header == "" {
			header = "none"
		}
		ss["network"] = "kcp"
		ss["kcpSettings"] = map[string]any{"header": map[string]any{"type": header}}
	case "quic":
		ss["network"] = "quic"
		ss["quicSettings"] = map[string]any{
			"security": "none",
			"key":      "",
			"header":   map[string]any{"type": "none"},
		}
	default:
		return nil, fmt.Errorf("شبکه %s برای تست واقعی پشتیبانی نمی‌شود", network)
	}

	security := strings.ToLower(strings.TrimSpace(n.Security))
	if security == "xtls" {
		security = "tls"
	}
	switch security {
	case "tls":
		tls := map[string]any{"allowInsecure": false}
		if n.SNI != "" {
			tls["serverName"] = n.SNI
		}
		if n.Fingerprint != "" {
			tls["fingerprint"] = n.Fingerprint
		}
		if len(n.ALPN) > 0 {
			tls["alpn"] = n.ALPN
		}
		ss["security"] = "tls"
		ss["tlsSettings"] = tls
	case "reality":
		if n.PublicKey == "" {
			return nil, fmt.Errorf("کلید عمومی Reality (pbk) وجود ندارد")
		}
		fp := n.Fingerprint
		if fp == "" {
			fp = "chrome"
		}
		reality := map[string]any{
			"serverName":  n.SNI,
			"fingerprint": fp,
			"publicKey":   n.PublicKey,
			"shortId":     n.ShortID,
			"spiderX":     n.SpiderX,
		}
		ss["security"] = "reality"
		ss["realitySettings"] = reality
	case "", "none":
		ss["security"] = "none"
	default:
		return nil, fmt.Errorf("امنیت %s پشتیبانی نمی‌شود", security)
	}
	return ss, nil
}

func buildConfig(node Node, port int) ([]byte, error) {
	out, err := node.outboundConfig()
	if err != nil {
		return nil, err
	}
	inbound := map[string]any{
		"tag":      "socks-in",
		"listen":   "127.0.0.1",
		"port":     port,
		"protocol": "socks",
		"settings": map[string]any{"auth": "noauth", "udp": false},
	}
	cfg := map[string]any{
		"log": map[string]any{
			"loglevel": "warning",
			"access":   "none",
			"error":    "",
		},
		"inbounds":  []any{inbound},
		"outbounds": []any{out},
		"routing": map[string]any{
			"domainStrategy": "AsIs",
			"rules": []any{map[string]any{
				"type": "field", "inboundTag": []string{"socks-in"}, "outboundTag": "proxy",
			}},
		},
	}
	return json.Marshal(cfg)
}
