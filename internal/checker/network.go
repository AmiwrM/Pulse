package checker

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"pulse/internal/model"
)

func decodeConfig(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}

func tlsConfig(skip bool, serverName string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: skip, //nolint:gosec // explicit user choice for self-signed endpoints
		ServerName:         serverName,
		MinVersion:         tls.VersionTLS12,
	}
}

func (m *Manager) checkTCP(ctx context.Context, mon model.Monitor) Result {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", mon.Target)
	if err != nil {
		return Result{Error: friendlyNetErr(err)}
	}
	defer conn.Close()
	meta := map[string]any{
		"remote_addr": conn.RemoteAddr().String(),
	}
	return Result{OK: true, Meta: meta}
}

var pingTimeRe = regexp.MustCompile(`(?i)time[=<]\s*([0-9]+(?:[.,][0-9]+)?)\s*ms`)
var pingFallbackRe = regexp.MustCompile(`(?i)([0-9]+(?:[.,][0-9]+)?)\s*ms`)

func (m *Manager) checkPing(ctx context.Context, mon model.Monitor) Result {
	bin, err := exec.LookPath("ping")
	if err != nil {
		return Result{Error: "ابزار ping روی میزبان نصب نیست؛ برای بررسی ICMP آن را نصب کنید"}
	}
	target := strings.TrimSpace(mon.Target)
	timeout := time.Duration(mon.Timeout) * time.Millisecond
	if timeout < time.Second {
		timeout = time.Second
	}
	var args []string
	switch runtime.GOOS {
	case "windows":
		args = []string{"-n", "1", "-w", strconv.Itoa(int(timeout.Milliseconds())), target}
	case "darwin":
		args = []string{"-c", "1", "-W", strconv.Itoa(int(timeout.Milliseconds())), target}
	default:
		sec := int(timeout.Seconds())
		if sec < 1 {
			sec = 1
		}
		args = []string{"-c", "1", "-W", strconv.Itoa(sec), target}
	}
	start := time.Now()
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start).Milliseconds()
	text := strings.TrimSpace(string(out))
	if err != nil {
		msg := "پینگ ناموفق بود"
		lower := strings.ToLower(text)
		switch {
		case strings.Contains(lower, "permission") || strings.Contains(lower, "not permitted") || strings.Contains(lower, "operation not permitted"):
			msg = "دسترسی ICMP کافی نیست (در کانتینر cap_add NET_RAW لازم است)"
		case strings.Contains(lower, "100% packet loss") || strings.Contains(lower, "100% loss"):
			msg = "پکت‌ها به مقصد نرسیدند (packet loss 100%)"
		case ctx.Err() == context.DeadlineExceeded:
			msg = "اتمام مهلت بررسی پینگ"
		}
		if text != "" {
			first := strings.Split(text, "\n")[0]
			msg += " — " + strings.TrimSpace(first)
		}
		return Result{Error: msg, Meta: map[string]any{"output": truncate(text, 500)}}
	}
	latency := elapsed
	if mm := pingTimeRe.FindStringSubmatch(text); len(mm) == 2 {
		if v, e := strconv.ParseFloat(strings.ReplaceAll(mm[1], ",", "."), 64); e == nil {
			latency = int64(v)
		}
	} else if mm := pingFallbackRe.FindStringSubmatch(text); len(mm) == 2 {
		if v, e := strconv.ParseFloat(strings.ReplaceAll(mm[1], ",", "."), 64); e == nil {
			latency = int64(v)
		}
	}
	return Result{OK: true, LatencyMS: latency, Meta: map[string]any{"output": truncate(text, 500)}}
}

type dnsConfig struct {
	Record string `json:"record"`
	Expect string `json:"expect"`
	Server string `json:"server"`
}

func (m *Manager) checkDNS(ctx context.Context, mon model.Monitor) Result {
	var c dnsConfig
	_ = decodeConfig(mon.Config, &c)
	record := strings.ToUpper(strings.TrimSpace(c.Record))
	if record == "" {
		record = "A"
	}
	server := strings.TrimSpace(c.Server)
	if server == "" {
		server = m.DNS()
	}
	resolver := net.DefaultResolver
	if server != "" {
		if _, _, err := net.SplitHostPort(server); err != nil {
			server = net.JoinHostPort(server, "53")
		}
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, network, server)
			},
		}
	}
	host := mon.Target
	var values []string
	var err error
	switch record {
	case "A", "AAAA":
		network := "ip4"
		if record == "AAAA" {
			network = "ip6"
		}
		ips, e := resolver.LookupIP(ctx, network, host)
		err = e
		for _, ip := range ips {
			values = append(values, ip.String())
		}
	case "CNAME":
		var cname string
		cname, err = resolver.LookupCNAME(ctx, host)
		if err == nil {
			values = append(values, strings.TrimSuffix(cname, "."))
		}
	case "MX":
		var mxs []*net.MX
		mxs, err = resolver.LookupMX(ctx, host)
		for _, mx := range mxs {
			values = append(values, fmt.Sprintf("%d %s", mx.Pref, strings.TrimSuffix(mx.Host, ".")))
		}
	case "TXT":
		values, err = resolver.LookupTXT(ctx, host)
	case "NS":
		var nss []*net.NS
		nss, err = resolver.LookupNS(ctx, host)
		for _, ns := range nss {
			values = append(values, strings.TrimSuffix(ns.Host, "."))
		}
	default:
		return Result{Error: "نوع رکورد پشتیبانی نمی‌شود"}
	}
	meta := map[string]any{
		"record":   record,
		"resolved": values,
		"server":   server,
	}
	if err != nil {
		msg := err.Error()
		if dnsErr, ok := err.(*net.DNSError); ok {
			switch {
			case dnsErr.IsNotFound:
				msg = "رکورد/دامنه پیدا نشد (NXDOMAIN)"
			case dnsErr.IsTimeout:
				msg = "پاسخ DNS در مهلت مقرر دریافت نشد"
			}
		}
		return Result{Error: msg, Meta: meta}
	}
	res := Result{OK: true, Meta: meta}
	expected := splitExpected(c.Expect)
	if len(expected) > 0 {
		ok, missing := matchExpected(values, expected)
		if !ok {
			res.OK = false
			res.Error = "مقدار DNS با مقدار مورد انتظار مطابقت ندارد"
			meta["dns_mismatch"] = true
			meta["missing"] = missing
		}
	}
	return res
}

func splitExpected(expect string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(expect, func(r rune) bool {
		return r == ',' || r == '\n' || r == ' ' || r == ';'
	}) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func matchExpected(actual, expected []string) (bool, []string) {
	norm := func(s string) string {
		return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
	}
	missing := []string{}
	for _, want := range expected {
		found := false
		for _, got := range actual {
			if norm(got) == norm(want) || strings.Contains(norm(got), norm(want)) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, want)
		}
	}
	return len(missing) == 0, missing
}

type sslConfig struct {
	ServerName string `json:"server_name"`
	WarnDays   int    `json:"warn_days"`
	SkipVerify bool   `json:"skip_verify"`
}

func (m *Manager) checkSSL(ctx context.Context, mon model.Monitor) Result {
	var c sslConfig
	_ = decodeConfig(mon.Config, &c)
	host := strings.TrimSpace(mon.Target)
	if strings.Contains(host, "://") {
		if u, err := url.Parse(host); err == nil {
			host = u.Host
		}
	}
	hostname := host
	port := "443"
	if h, p, err := net.SplitHostPort(host); err == nil {
		hostname, port = h, p
	} else {
		hostname = strings.Trim(host, "[]")
	}
	sni := strings.TrimSpace(c.ServerName)
	if sni == "" {
		sni = hostname
	}
	warnDays := c.WarnDays
	if warnDays <= 0 {
		warnDays = 14
	}
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(hostname, port))
	if err != nil {
		return Result{Error: friendlyNetErr(err)}
	}
	defer conn.Close()
	tlsConn := tls.Client(conn, tlsConfig(c.SkipVerify, sni))
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return Result{Error: "خطای دست‌دهی TLS: " + err.Error()}
	}
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return Result{Error: "گواهی SSL دریافت نشد"}
	}
	cert := state.PeerCertificates[0]
	days := int(time.Until(cert.NotAfter).Hours() / 24)
	meta := map[string]any{
		"subject":    cert.Subject.CommonName,
		"issuer":     cert.Issuer.CommonName,
		"not_before": cert.NotBefore.UTC().Format(time.RFC3339),
		"not_after":  cert.NotAfter.UTC().Format(time.RFC3339),
		"days_left":  days,
		"dns_names":  cert.DNSNames,
		"sni":        sni,
	}
	res := Result{OK: true, SSLExpiresAt: cert.NotAfter.Unix(), Meta: meta}
	if days < 0 {
		res.OK = false
		res.Error = "گواهی SSL منقضی شده است"
	} else if days <= warnDays {
		res.Warn = fmt.Sprintf("گواهی SSL تا %d روز دیگر منقضی می‌شود", days)
	}
	return res
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
