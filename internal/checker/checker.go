package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"pulse/internal/model"
	"pulse/internal/xray"
)

type Result struct {
	OK           bool           `json:"ok"`
	LatencyMS    int64          `json:"latency_ms"`
	StatusCode   int            `json:"status_code"`
	Error        string         `json:"error"`
	Warn         string         `json:"warn"`
	SSLExpiresAt int64          `json:"ssl_expires_at"`
	Meta         map[string]any `json:"meta"`
}

type Manager struct {
	mu                  sync.RWMutex
	DefaultDNS          string
	UserAgent           string
	Xray                *xray.Engine
	defaultProxyTestURL string
}

func New(defaultDNS string) *Manager {
	return &Manager{
		DefaultDNS:          defaultDNS,
		UserAgent:           "Pulse/1.4 (+https://github.com/AmiwrM/Pulse)",
		defaultProxyTestURL: xray.DefaultTestURL,
	}
}

func (m *Manager) SetProxyTestURL(u string) {
	m.mu.Lock()
	m.defaultProxyTestURL = u
	m.mu.Unlock()
}

func (m *Manager) ProxyTestURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.defaultProxyTestURL == "" {
		return xray.DefaultTestURL
	}
	return m.defaultProxyTestURL
}

func (m *Manager) SetDNS(server string) {
	m.mu.Lock()
	m.DefaultDNS = server
	m.mu.Unlock()
}

func (m *Manager) DNS() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.DefaultDNS
}

func (m *Manager) Run(parent context.Context, mon model.Monitor) Result {
	timeout := time.Duration(mon.Timeout) * time.Millisecond
	if timeout < 2*time.Second {
		timeout = 2 * time.Second
	}
	// Real Xray handshakes need time to start temporary cores and perform an
	// actual request through each node.
	if mon.Type == "proxy" && timeout < 45*time.Second {
		timeout = 45 * time.Second
	}
	if timeout > 3*time.Minute {
		timeout = 3 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	start := time.Now()
	var res Result
	switch mon.Type {
	case "http":
		res = m.checkHTTP(ctx, mon)
	case "tcp":
		res = m.checkTCP(ctx, mon)
	case "ping":
		res = m.checkPing(ctx, mon)
	case "dns":
		res = m.checkDNS(ctx, mon)
	case "ssl":
		res = m.checkSSL(ctx, mon)
	case "proxy":
		res = m.checkProxy(ctx, mon)
	default:
		res = Result{Error: "نوع مانیتور ناشناخته است: " + mon.Type}
	}
	if res.LatencyMS <= 0 {
		res.LatencyMS = time.Since(start).Milliseconds()
	}
	if res.LatencyMS == 0 {
		res.LatencyMS = 1
	}
	if res.Meta == nil {
		res.Meta = map[string]any{}
	}
	if ctx.Err() == context.DeadlineExceeded && !res.OK {
		if res.Error == "" {
			res.Error = "اتمام مهلت بررسی (timeout)"
		}
	}
	return res
}

func Validate(monType, target string, raw json.RawMessage) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("آدرس هدف الزامی است")
	}
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	switch monType {
	case "http":
		u, err := url.Parse(target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("آدرس HTTP باید با http:// یا https:// شروع شود")
		}
		var c httpConfig
		if err := json.Unmarshal(raw, &c); err != nil {
			return fmt.Errorf("تنظیمات HTTP نامعتبر است")
		}
		if c.Keyword != "" && c.KeywordMode != "" && c.KeywordMode != "contains" && c.KeywordMode != "not_contains" {
			return fmt.Errorf("حالت کلیدواژه نامعتبر است")
		}
	case "tcp":
		if _, _, err := net.SplitHostPort(target); err != nil {
			return fmt.Errorf("آدرس TCP باید به شکل host:port باشد")
		}
	case "ping":
		if strings.ContainsAny(target, " /") {
			return fmt.Errorf("آدرس پینگ باید فقط نام دامنه یا IP باشد")
		}
	case "dns":
		var c dnsConfig
		if err := json.Unmarshal(raw, &c); err != nil {
			return fmt.Errorf("تنظیمات DNS نامعتبر است")
		}
		switch strings.ToUpper(c.Record) {
		case "", "A", "AAAA", "CNAME", "MX", "TXT", "NS":
		default:
			return fmt.Errorf("نوع رکورد DNS پشتیبانی نمی‌شود")
		}
	case "ssl":
		host := target
		if u, err := url.Parse(target); err == nil && u.Host != "" {
			host = u.Host
		}
		if _, _, err := net.SplitHostPort(host); err != nil {
			host = net.JoinHostPort(host, "443")
		}
		if _, _, err := net.SplitHostPort(host); err != nil {
			return fmt.Errorf("آدرس SSL نامعتبر است")
		}
	case "proxy":
		var c proxyConfig
		if err := json.Unmarshal(raw, &c); err != nil {
			return fmt.Errorf("تنظیمات کانفیگ نامعتبر است")
		}
		if strings.TrimSpace(c.Input) == "" {
			return fmt.Errorf("متن کانفیگ یا لینک سابسکریپشن الزامی است")
		}
		if c.RealTest == nil || *c.RealTest {
			if u := strings.TrimSpace(c.TestURL); u != "" {
				parsed, err := url.Parse(u)
				if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
					return fmt.Errorf("آدرس تست واقعی باید یک URL معتبر HTTP/HTTPS باشد")
				}
			}
		}
		if c.MaxRealNodes < 0 || c.MaxRealNodes > 30 {
			return fmt.Errorf("تعداد نودهای تست واقعی باید بین ۱ تا ۳۰ باشد")
		}
	default:
		return fmt.Errorf("نوع مانیتور ناشناخته است")
	}
	return nil
}

func (m *Manager) httpClient(followRedirects bool, skipTLS bool) *http.Client {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          5,
		IdleConnTimeout:       20 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 0,
		TLSClientConfig:       tlsConfig(skipTLS, ""),
	}
	client := &http.Client{Transport: tr}
	if !followRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client
}
