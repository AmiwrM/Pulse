package checker

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"pulse/internal/model"
	"pulse/internal/xray"
)

type proxyConfig struct {
	Input         string `json:"input"`
	MinHealthy    int    `json:"min_healthy"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
	SlowMS        int    `json:"slow_ms"`
	RealTest      *bool  `json:"real_test"`
	TestURL       string `json:"test_url"`
	MaxRealNodes  int    `json:"max_real_nodes"`
}

type ProxyNode struct {
	Name        string `json:"name"`
	Protocol    string `json:"protocol"`
	Address     string `json:"address"`
	SNI         string `json:"sni,omitempty"`
	TLS         bool   `json:"tls"`
	UDP         bool   `json:"udp"`
	OK          bool   `json:"ok"`
	LatencyMS   int64  `json:"latency_ms"`
	Error       string `json:"error,omitempty"`
	Method      string `json:"method,omitempty"`
	RealTested  bool   `json:"real_tested,omitempty"`
	RealSkipped bool   `json:"real_skipped,omitempty"`
	RealNote    string `json:"real_note,omitempty"`

	XrayNode *xray.Node `json:"-"`
}

func (m *Manager) checkProxy(ctx context.Context, mon model.Monitor) Result {
	var c proxyConfig
	_ = decodeConfig(mon.Config, &c)
	input := strings.TrimSpace(c.Input)
	if input == "" {
		return Result{Error: "متن کانفیگ یا لینک سابسکریپشن خالی است"}
	}
	minHealthy := c.MinHealthy
	if minHealthy <= 0 {
		minHealthy = 1
	}
	meta := map[string]any{"source": "manual"}
	nodes := []ProxyNode{}
	rawInput := input

	if isURLOnly(input) {
		content, err := fetchSubscription(ctx, input, c.SkipTLSVerify)
		if err != nil {
			return Result{Error: "دریافت سابسکریپشن ناموفق: " + err.Error()}
		}
		input = content
		meta["source"] = "subscription"
		meta["subscription_url"] = rawInput
	}

	if looksLikeWireGuard(input) {
		nodes = parseWireGuardNodes(input)
		meta["source"] = "wireguard"
	} else {
		text := maybeDecodeBase64(input)
		if looksLikeClash(text) {
			nodes = parseClashNodes(text)
			meta["source"] = "clash"
		} else {
			nodes = parseProxyNodes(text)
		}
	}

	meta["total"] = len(nodes)
	if len(nodes) == 0 {
		return Result{Error: "هیچ کانفیگ قابل‌پشتیبانی پیدا نشد (vmess/vless/trojan/ss/wireguard)", Meta: meta}
	}

	// Fast transport-level probe first. It removes dead endpoints before we pay
	// the cost of spawning a real Xray core for every node.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := range nodes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			nodes[i] = testNode(ctx, nodes[i], c.SkipTLSVerify)
		}(i)
	}
	wg.Wait()

	warnings := []string{}
	realRequested := c.RealTest == nil || *c.RealTest
	meta["real_test_requested"] = realRequested
	if realRequested {
		for i := range nodes {
			if nodes[i].XrayNode == nil {
				nodes[i].RealSkipped = true
				if nodes[i].RealNote == "" {
					nodes[i].RealNote = "تست واقعی Xray برای این نوع کانفیگ در دسترس نیست"
				}
			}
		}
		engine := m.Xray
		if engine == nil {
			meta["real_test_error"] = "هستهٔ Xray در دسترس نیست"
			warnings = append(warnings, "تست واقعی انجام نشد: هستهٔ Xray پیکربندی نشده است")
		} else {
			status := engine.Status()
			meta["xray_available"] = status.Available
			meta["xray_version"] = status.Version
			if !status.Available {
				errText := status.Error
				if strings.TrimSpace(errText) == "" {
					errText = "هستهٔ Xray آماده نیست"
				}
				meta["real_test_error"] = errText
				warnings = append(warnings, "تست واقعی Xray انجام نشد: "+errText)
			} else {
				// Run the authoritative Xray test for nodes that passed the fast
				// probe first, then for nodes that failed it but still have enough
				// protocol data for Xray. A TCP/TLS probe can fail on valid Reality,
				// CDN, or WebSocket configurations; the real tunnel test is the
				// source of truth, not the preliminary reachability probe.
				candidates := []int{}
				recovery := []int{}
				for i := range nodes {
					if nodes[i].XrayNode == nil {
						continue
					}
					if nodes[i].OK {
						candidates = append(candidates, i)
					} else {
						recovery = append(recovery, i)
					}
				}
				candidates = append(candidates, recovery...)
				maxReal := c.MaxRealNodes
				if maxReal <= 0 {
					maxReal = 10
				}
				if maxReal > 30 {
					maxReal = 30
				}
				testURL := strings.TrimSpace(c.TestURL)
				if testURL == "" {
					testURL = m.ProxyTestURL()
				}
				meta["test_url"] = testURL
				if len(candidates) == 0 {
					meta["real_test_skipped"] = "هیچ کانفیگی برای تست واقعی Xray وجود ندارد"
				} else {
					selected := candidates
					if len(selected) > maxReal {
						selected = selected[:maxReal]
						for _, idx := range candidates[maxReal:] {
							nodes[idx].RealSkipped = true
							nodes[idx].RealNote = "محدودیت تعداد تست واقعی Xray"
						}
						warnings = append(warnings, fmt.Sprintf("تست واقعی فقط برای %d کانفیگ انجام شد", maxReal))
					}
					specs := make([]xray.Node, 0, len(selected))
					for _, idx := range selected {
						specs = append(specs, *nodes[idx].XrayNode)
					}
					results, err := engine.TestNodes(ctx, specs, testURL, 4)
					if err != nil {
						meta["real_test_error"] = err.Error()
						warnings = append(warnings, "تست واقعی ناموفق ماند: "+err.Error())
					} else {
						realOK, realDone, infraErrors := 0, 0, 0
						for j, r := range results {
							idx := selected[j]
							if r.InfraError {
								infraErrors++
								nodes[idx].RealSkipped = true
								nodes[idx].RealNote = "تست واقعی Xray ممکن نشد: " + r.Error
								continue
							}
							realDone++
							nodes[idx].RealTested = true
							nodes[idx].Method = "xray"
							nodes[idx].LatencyMS = r.LatencyMS
							if r.OK {
								realOK++
								nodes[idx].OK = true
								nodes[idx].Error = ""
							} else {
								nodes[idx].OK = false
								nodes[idx].Error = "هندشیک واقعی ناموفق: " + r.Error
							}
						}
						meta["real_tested"] = realDone
						meta["real_passed"] = realOK
						if infraErrors > 0 {
							warnings = append(warnings, fmt.Sprintf("تست واقعی برای %d کانفیگ اجرا نشد (خطای موتور Xray)", infraErrors))
						}
						if realDone > 0 && realOK < realDone {
							warnings = append(warnings, fmt.Sprintf("%d کانفیگ هندشیک واقعی را رد نکردند", realDone-realOK))
						}
					}
				}
			}
		}
	}

	healthy := 0
	minLatency := int64(0)
	for i := range nodes {
		if nodes[i].OK {
			healthy++
			if minLatency == 0 || nodes[i].LatencyMS < minLatency {
				minLatency = nodes[i].LatencyMS
			}
		}
	}
	meta["healthy"] = healthy
	meta["min_healthy"] = minHealthy
	meta["nodes"] = nodes

	res := Result{OK: healthy >= minHealthy, LatencyMS: minLatency, Meta: meta}
	if !res.OK {
		res.Error = fmt.Sprintf("تنها %d از %d کانفیگ در دسترس است (حداقل %d لازم است)", healthy, len(nodes), minHealthy)
	} else if healthy < len(nodes) {
		warnings = append(warnings, fmt.Sprintf("%d از %d کانفیگ در دسترس نیست", len(nodes)-healthy, len(nodes)))
	}
	if len(warnings) > 0 {
		res.Warn = strings.Join(warnings, "؛ ")
	}
	return res
}

func isURLOnly(s string) bool {
	s = strings.TrimSpace(s)
	return (strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")) &&
		!strings.ContainsAny(s, "\n\r")
}

func fetchSubscription(ctx context.Context, raw string, skipTLS bool) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Pulse/1.5")
	client := &http.Client{
		Timeout: 25 * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: tlsConfig(skipTLS, ""),
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("کد وضعیت %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func looksLikeWireGuard(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "[interface]") && strings.Contains(l, "endpoint")
}

func looksLikeClash(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "proxies:") || strings.Contains(t, "\nproxies:")
}

func maybeDecodeBase64(s string) string {
	if strings.Contains(s, "://") {
		return s
	}
	compact := strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', ' ', '\t':
			return -1
		}
		return r
	}, s)
	if compact == "" {
		return s
	}
	encodings := []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding,
	}
	for _, enc := range encodings {
		if decoded, err := enc.DecodeString(compact); err == nil {
			out := string(decoded)
			if strings.Contains(out, "://") || looksLikeWireGuard(out) || looksLikeClash(out) {
				return out
			}
		}
	}
	return s
}

func parseProxyNodes(text string) []ProxyNode {
	var out []ProxyNode
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		node, ok := parseProxyURI(line)
		if ok {
			out = append(out, node)
			continue
		}
		if isHostPort(line) {
			host, port, _ := net.SplitHostPort(line)
			out = append(out, ProxyNode{
				Name: "TCP " + line, Protocol: "tcp", Address: net.JoinHostPort(host, port),
			})
		}
	}
	return out
}

func parseProxyURI(line string) (ProxyNode, bool) {
	scheme := strings.ToLower(line)
	if i := strings.Index(scheme, "://"); i > 0 {
		scheme = scheme[:i]
	} else {
		return ProxyNode{}, false
	}
	switch scheme {
	case "vmess":
		return parseVMess(line)
	case "vless":
		return parseVLESS(line)
	case "trojan":
		return parseTrojan(line)
	case "ss":
		return parseShadowsocks(line)
	case "hysteria", "hy2", "hysteria2", "tuic":
		return parseUDPURI(line, scheme)
	case "socks", "socks5":
		return parseSocks(line)
	default:
		return ProxyNode{}, false
	}
}

func parseVMess(line string) (ProxyNode, bool) {
	raw := strings.TrimPrefix(line, "vmess://")
	raw = strings.SplitN(raw, "#", 2)[0]
	raw = strings.SplitN(raw, "?", 2)[0]
	decoded, err := decodeBase64Any(raw)
	if err != nil {
		return ProxyNode{}, false
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(decoded), &obj); err != nil {
		return ProxyNode{}, false
	}
	host := strings.Trim(mapStr(obj, "add"), "[]")
	port := mapPort(obj, "port")
	if host == "" || port == 0 {
		return ProxyNode{}, false
	}
	name := mapStr(obj, "ps")
	if name == "" {
		name = "VMess " + net.JoinHostPort(host, strconv.Itoa(port))
	}
	network := strings.ToLower(mapStr(obj, "net"))
	if network == "" {
		network = "tcp"
	}
	headerType := strings.ToLower(mapStr(obj, "type"))
	hostHeader := mapStr(obj, "host")
	path := mapStr(obj, "path")
	fp := mapStr(obj, "fp")
	alpn := splitList(mapStr(obj, "alpn"))
	tlsOn := strings.EqualFold(mapStr(obj, "tls"), "tls") || strings.EqualFold(mapStr(obj, "tls"), "reality")
	sni := mapStr(obj, "sni")
	if sni == "" {
		sni = hostHeader
	}
	if sni == "" {
		sni = host
	}
	udp := network == "kcp" || network == "mkcp" || network == "quic"
	node := ProxyNode{
		Name: name, Protocol: "vmess", Address: net.JoinHostPort(host, strconv.Itoa(port)),
		TLS: tlsOn, SNI: sni, UDP: udp,
	}
	id := mapStr(obj, "id")
	if id != "" && xrayNetworkOK(network) {
		x := &xray.Node{
			Name: name, Protocol: "vmess", Address: host, Port: port,
			UUID: id, AlterID: mapInt(obj, "aid"), Network: network, HeaderType: headerType,
			Security: "none", SNI: sni, HostHeader: hostHeader, Path: path,
			Fingerprint: fp, ALPN: alpn,
		}
		if tlsOn {
			x.Security = "tls"
		}
		node.XrayNode = x
	} else if id == "" {
		node.RealNote = "UUID برای تست واقعی موجود نیست"
	} else {
		node.RealNote = "شبکهٔ " + network + " برای تست واقعی Xray پشتیبانی نمی‌شود"
	}
	return node, true
}

func parseVLESS(line string) (ProxyNode, bool) {
	u, err := url.Parse(line)
	if err != nil || u.Hostname() == "" || u.User == nil {
		return ProxyNode{}, false
	}
	port := parsePort(u.Port(), 443)
	if port == 0 {
		return ProxyNode{}, false
	}
	q := u.Query()
	security := strings.ToLower(q.Get("security"))
	if security == "" {
		security = "none"
	}
	network := strings.ToLower(firstNonEmpty(q.Get("type"), q.Get("net"), "tcp"))
	sni := firstNonEmpty(q.Get("sni"), q.Get("peer"), q.Get("host"), u.Hostname())
	hostHeader := q.Get("host")
	path := q.Get("path")
	serviceName := firstNonEmpty(q.Get("serviceName"), q.Get("service_name"))
	flow := q.Get("flow")
	fp := q.Get("fp")
	pbk := firstNonEmpty(q.Get("pbk"), q.Get("publicKey"))
	sid := firstNonEmpty(q.Get("sid"), q.Get("shortId"))
	spx := firstNonEmpty(q.Get("spx"), q.Get("spiderX"))
	name := fragmentName(u)
	if name == "" {
		name = "VLESS " + net.JoinHostPort(u.Hostname(), strconv.Itoa(port))
	}
	tlsOn := security == "tls" || security == "xtls"
	node := ProxyNode{
		Name: name, Protocol: "vless", Address: net.JoinHostPort(u.Hostname(), strconv.Itoa(port)),
		TLS: tlsOn, SNI: sni, UDP: network == "kcp" || network == "mkcp" || network == "quic",
	}
	if xrayNetworkOK(network) && (security != "reality" || pbk != "") {
		x := &xray.Node{
			Name: name, Protocol: "vless", Address: u.Hostname(), Port: port,
			UUID: u.User.Username(), Flow: flow, Network: network, HeaderType: q.Get("headerType"),
			Security: security, SNI: sni, HostHeader: hostHeader, Path: path, ServiceName: serviceName,
			Fingerprint: fp, PublicKey: pbk, ShortID: sid, SpiderX: spx, ALPN: splitList(q.Get("alpn")),
		}
		if security == "xtls" {
			x.Security = "tls"
		}
		node.XrayNode = x
	} else if security == "reality" {
		node.RealNote = "کلید عمومی Reality (pbk) وجود ندارد"
	} else {
		node.RealNote = "شبکهٔ " + network + " برای تست واقعی Xray پشتیبانی نمی‌شود"
	}
	return node, true
}

func parseTrojan(line string) (ProxyNode, bool) {
	u, err := url.Parse(line)
	if err != nil || u.Hostname() == "" || u.User == nil {
		return ProxyNode{}, false
	}
	port := parsePort(u.Port(), 443)
	if port == 0 {
		return ProxyNode{}, false
	}
	q := u.Query()
	security := strings.ToLower(q.Get("security"))
	if security == "" {
		security = "tls"
	}
	network := strings.ToLower(firstNonEmpty(q.Get("type"), q.Get("net"), "tcp"))
	sni := firstNonEmpty(q.Get("sni"), q.Get("peer"), q.Get("host"), u.Hostname())
	name := fragmentName(u)
	if name == "" {
		name = "Trojan " + net.JoinHostPort(u.Hostname(), strconv.Itoa(port))
	}
	tlsOn := security == "tls" || security == "xtls"
	node := ProxyNode{
		Name: name, Protocol: "trojan", Address: net.JoinHostPort(u.Hostname(), strconv.Itoa(port)),
		TLS: tlsOn, SNI: sni, UDP: network == "kcp" || network == "mkcp" || network == "quic",
	}
	if xrayNetworkOK(network) {
		x := &xray.Node{
			Name: name, Protocol: "trojan", Address: u.Hostname(), Port: port,
			Password: u.User.Username(), Network: network, HeaderType: q.Get("headerType"),
			Security: security, SNI: sni, HostHeader: q.Get("host"), Path: q.Get("path"),
			ServiceName: q.Get("serviceName"), Fingerprint: q.Get("fp"), ALPN: splitList(q.Get("alpn")),
		}
		if security == "xtls" {
			x.Security = "tls"
		}
		node.XrayNode = x
	} else {
		node.RealNote = "شبکهٔ " + network + " برای تست واقعی Xray پشتیبانی نمی‌شود"
	}
	return node, true
}

func parseSocks(line string) (ProxyNode, bool) {
	u, err := url.Parse(line)
	if err != nil || u.Hostname() == "" {
		return ProxyNode{}, false
	}
	port := parsePort(u.Port(), 1080)
	if port == 0 {
		return ProxyNode{}, false
	}
	name := fragmentName(u)
	if name == "" {
		name = "SOCKS " + net.JoinHostPort(u.Hostname(), strconv.Itoa(port))
	}
	node := ProxyNode{
		Name: name, Protocol: "socks", Address: net.JoinHostPort(u.Hostname(), strconv.Itoa(port)),
	}
	x := &xray.Node{Name: name, Protocol: "socks", Address: u.Hostname(), Port: port}
	if u.User != nil {
		x.User = u.User.Username()
		x.Password, _ = u.User.Password()
	}
	node.XrayNode = x
	return node, true
}

func parseShadowsocks(line string) (ProxyNode, bool) {
	raw := strings.TrimPrefix(line, "ss://")
	name := ""
	if i := strings.Index(raw, "#"); i >= 0 {
		name, _ = url.PathUnescape(raw[i+1:])
		raw = raw[:i]
	}
	query := ""
	if i := strings.Index(raw, "?"); i >= 0 {
		query = raw[i+1:]
		raw = raw[:i]
	}
	body := raw
	method, password := "", ""
	plugin := ""
	if q, err := url.ParseQuery(query); err == nil {
		plugin = q.Get("plugin")
	}
	if !strings.Contains(body, "@") {
		if decoded, err := decodeBase64Any(body); err == nil {
			body = decoded
		}
	}
	at := strings.LastIndex(body, "@")
	if at < 0 {
		return ProxyNode{}, false
	}
	userinfo := body[:at]
	hostport := body[at+1:]
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return ProxyNode{}, false
	}
	decodedUserinfo, err := decodeBase64Any(userinfo)
	if err == nil && strings.Contains(decodedUserinfo, ":") {
		userinfo = decodedUserinfo
	} else if unescaped, e := url.PathUnescape(userinfo); e == nil {
		userinfo = unescaped
	}
	method, password, _ = strings.Cut(userinfo, ":")
	if name == "" {
		name = "SS " + hostport
	}
	node := ProxyNode{
		Name: name, Protocol: "ss", Address: net.JoinHostPort(host, port),
	}
	if plugin != "" {
		node.RealNote = "پلاگین Shadowsocks برای تست واقعی Xray پشتیبانی نمی‌شود"
		return node, true
	}
	if method != "" && password != "" {
		node.XrayNode = &xray.Node{
			Name: name, Protocol: "shadowsocks", Address: host, Port: parsePort(port, 0),
			Cipher: method, Password: password, Security: "none",
		}
	} else {
		node.RealNote = "نام کاربری/رمز Shadowsocks کامل نیست"
	}
	return node, true
}

func parseUDPURI(line, proto string) (ProxyNode, bool) {
	u, err := url.Parse(line)
	if err != nil || u.Hostname() == "" {
		return ProxyNode{}, false
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	name := proto + " " + net.JoinHostPort(u.Hostname(), port)
	if u.Fragment != "" {
		if n, err := url.PathUnescape(u.Fragment); err == nil && n != "" {
			name = n
		}
	}
	return ProxyNode{
		Name: name, Protocol: proto, Address: net.JoinHostPort(u.Hostname(), port), UDP: true,
	}, true
}

func parseWireGuardNodes(text string) []ProxyNode {
	type wgPeer struct {
		publicKey, presharedKey, endpoint string
		allowedIPs                        []string
		keepAlive                         int
	}
	var secret string
	var addresses []string
	mtu := 0
	peers := []wgPeer{}
	var current *wgPeer
	section := ""
	flush := func() {
		if current != nil && current.publicKey != "" {
			peers = append(peers, *current)
		}
		current = nil
	}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.EqualFold(line, "[Interface]") {
			flush()
			section = "interface"
			continue
		}
		if strings.EqualFold(line, "[Peer]") {
			flush()
			section = "peer"
			current = &wgPeer{}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch section {
		case "interface":
			switch {
			case strings.EqualFold(key, "PrivateKey"):
				secret = value
			case strings.EqualFold(key, "Address"):
				addresses = splitList(value)
			case strings.EqualFold(key, "MTU"):
				mtu, _ = strconv.Atoi(value)
			}
		case "peer":
			if current == nil {
				continue
			}
			switch {
			case strings.EqualFold(key, "PublicKey"):
				current.publicKey = value
			case strings.EqualFold(key, "PresharedKey"):
				current.presharedKey = value
			case strings.EqualFold(key, "Endpoint"):
				current.endpoint = value
			case strings.EqualFold(key, "AllowedIPs"):
				current.allowedIPs = splitList(value)
			case strings.EqualFold(key, "PersistentKeepalive"):
				current.keepAlive, _ = strconv.Atoi(value)
			}
		}
	}
	flush()

	out := []ProxyNode{}
	for i, p := range peers {
		host, port, ok := normalizeEndpoint(p.endpoint)
		if !ok {
			continue
		}
		name := fmt.Sprintf("WireGuard #%d", i+1)
		node := ProxyNode{
			Name: name, Protocol: "wireguard", Address: net.JoinHostPort(host, strconv.Itoa(port)),
			UDP: true,
		}
		if secret != "" && len(addresses) > 0 {
			allowed := p.allowedIPs
			if len(allowed) == 0 {
				allowed = []string{"0.0.0.0/0", "::/0"}
			}
			node.XrayNode = &xray.Node{
				Name: name, Protocol: "wireguard", Address: host, Port: port,
				WGSecretKey: secret, WGAddress: addresses, WGMTU: mtu, WGNoKernelTun: true,
				WGPeers: []xray.WGPeer{{
					PublicKey: p.publicKey, PreSharedKey: p.presharedKey,
					Endpoint: net.JoinHostPort(host, strconv.Itoa(port)), AllowedIPs: allowed,
					KeepAlive: p.keepAlive,
				}},
			}
		} else {
			node.RealNote = "PrivateKey یا Address در Interface وجود ندارد"
		}
		out = append(out, node)
	}
	return out
}

func normalizeEndpoint(value string) (string, int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", 0, false
	}
	if host, port, err := net.SplitHostPort(value); err == nil {
		p := parsePort(port, 0)
		if p == 0 {
			return "", 0, false
		}
		return host, p, true
	}
	value = strings.Trim(value, "[]")
	if ip := net.ParseIP(value); ip != nil {
		return value, 51820, true
	}
	if !strings.Contains(value, ":") {
		return value, 51820, true
	}
	return "", 0, false
}

func parsePort(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 65535 {
		return 0
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func fragmentName(u *url.URL) string {
	if u.Fragment == "" {
		return ""
	}
	name, err := url.PathUnescape(u.Fragment)
	if err != nil {
		return u.Fragment
	}
	return name
}

func splitList(s string) []string {
	out := []string{}
	for _, p := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	}) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mapInt(m map[string]any, key string) int {
	s := mapStr(m, key)
	n, _ := strconv.Atoi(s)
	return n
}

func xrayNetworkOK(network string) bool {
	switch strings.ToLower(network) {
	case "", "tcp", "raw", "ws", "grpc", "gun", "http", "h2", "h2c", "httpupgrade", "kcp", "mkcp", "quic":
		return true
	}
	return false
}

func parseClashNodes(text string) []ProxyNode {
	var out []ProxyNode
	var cur map[string]string
	flush := func() {
		if cur == nil {
			return
		}
		host := cur["server"]
		port := cur["port"]
		if host == "" || port == "" {
			cur = nil
			return
		}
		name := cur["name"]
		if name == "" {
			name = cur["type"] + " " + host + ":" + port
		}
		tlsOn := strings.EqualFold(cur["tls"], "true")
		sni := cur["servername"]
		if sni == "" {
			sni = host
		}
		out = append(out, ProxyNode{
			Name: name, Protocol: cur["type"], Address: net.JoinHostPort(strings.Trim(host, "[]"), port),
			TLS: tlsOn, SNI: sni,
		})
		cur = nil
	}
	inProxies := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, "\r")
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if trim == "proxies:" {
			inProxies = true
			continue
		}
		if inProxies && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(trim, "-") {
			flush()
			break
		}
		if !inProxies {
			continue
		}
		if strings.HasPrefix(trim, "- ") {
			flush()
			cur = map[string]string{}
			trim = strings.TrimSpace(strings.TrimPrefix(trim, "- "))
		}
		if cur == nil || trim == "" {
			continue
		}
		k, v, ok := strings.Cut(trim, ":")
		if !ok {
			continue
		}
		cur[strings.ToLower(strings.TrimSpace(k))] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	flush()
	return out
}

func testNode(ctx context.Context, n ProxyNode, skipTLS bool) ProxyNode {
	start := time.Now()
	if n.UDP {
		n.Method = "udp"
		err := udpProbe(ctx, n.Address)
		n.LatencyMS = time.Since(start).Milliseconds()
		if err != nil {
			n.Error = err.Error()
			return n
		}
		n.OK = true
		return n
	}
	n.Method = "tcp"
	if n.TLS {
		n.Method = "tls"
	}
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", n.Address)
	if err != nil {
		n.Error = friendlyNetErr(err)
		return n
	}
	defer conn.Close()
	if n.TLS {
		t := tls.Client(conn, tlsConfig(true, n.SNI))
		if err := t.HandshakeContext(ctx); err != nil {
			n.Error = "TLS handshake: " + truncate(err.Error(), 160)
			return n
		}
	}
	n.LatencyMS = time.Since(start).Milliseconds()
	n.OK = true
	return n
}

func udpProbe(ctx context.Context, address string) error {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "udp", address)
	if err != nil {
		return fmt.Errorf("%s", friendlyNetErr(err))
	}
	defer conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	buf := make([]byte, 32)
	_, _ = conn.Write(buf)
	_, err = conn.Read(buf)
	if err == nil {
		return nil
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		// UDP is connectionless; a timeout means the port did not actively reject us.
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "refused") {
		return fmt.Errorf("پورت UDP بسته است")
	}
	return err
}

func decodeBase64Any(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', ' ', '\t':
			return -1
		}
		return r
	}, s)
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("invalid base64")
}

func isHostPort(s string) bool {
	if strings.ContainsAny(s, " /") || strings.HasPrefix(s, "//") {
		return false
	}
	_, _, err := net.SplitHostPort(s)
	return err == nil
}

func mapStr(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case string:
			return t
		case float64:
			return strconv.FormatInt(int64(t), 10)
		}
	}
	return ""
}

func mapPort(m map[string]any, key string) int {
	s := mapStr(m, key)
	n, _ := strconv.Atoi(s)
	if n < 1 || n > 65535 {
		return 0
	}
	return n
}
