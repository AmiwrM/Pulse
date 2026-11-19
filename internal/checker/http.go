package checker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"time"

	"pulse/internal/model"
)

type httpConfig struct {
	Method          string            `json:"method"`
	Headers         map[string]string `json:"headers"`
	Body            string            `json:"body"`
	ExpectedStatus  string            `json:"expected_status"`
	Keyword         string            `json:"keyword"`
	KeywordMode     string            `json:"keyword_mode"`
	FollowRedirects bool              `json:"follow_redirects"`
	SkipTLSVerify   bool              `json:"skip_tls_verify"`
	HostHeader      string            `json:"host_header"`
	SlowMS          int               `json:"slow_ms"`
	BasicUser       string            `json:"basic_user"`
	BasicPass       string            `json:"basic_pass"`
}

func (m *Manager) checkHTTP(ctx context.Context, mon model.Monitor) Result {
	start := time.Now()
	var c httpConfig
	_ = decodeConfig(mon.Config, &c)
	method := strings.ToUpper(strings.TrimSpace(c.Method))
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if c.Body != "" && method != http.MethodGet && method != http.MethodHead {
		body = strings.NewReader(c.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, mon.Target, body)
	if err != nil {
		return Result{Error: "ساخت درخواست ناموفق: " + err.Error()}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", m.UserAgent)
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	if c.HostHeader != "" {
		req.Host = c.HostHeader
	}
	if c.BasicUser != "" {
		req.SetBasicAuth(c.BasicUser, c.BasicPass)
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	var remote string
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			remote = info.Conn.RemoteAddr().String()
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	client := m.httpClient(c.FollowRedirects, c.SkipTLSVerify)
	resp, err := client.Do(req)
	if err != nil {
		return Result{Error: friendlyNetErr(err)}
	}
	defer resp.Body.Close()

	meta := map[string]any{
		"final_url": resp.Request.URL.String(),
	}
	if remote != "" {
		meta["remote_addr"] = remote
	}
	res := Result{StatusCode: resp.StatusCode, Meta: meta}

	if !statusMatches(c.ExpectedStatus, resp.StatusCode) {
		res.Error = fmt.Sprintf("کد وضعیت غیرمنتظره: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		res.SSLExpiresAt = cert.NotAfter.Unix()
		meta["tls_issuer"] = cert.Issuer.CommonName
		meta["tls_subject"] = cert.Subject.CommonName
		meta["tls_expires"] = cert.NotAfter.UTC().Format(time.RFC3339)
		meta["tls_days_left"] = int(time.Until(cert.NotAfter).Hours() / 24)
	}

	if c.Keyword != "" {
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		if readErr != nil {
			res.Error = "خواندن پاسخ ناموفق: " + readErr.Error()
			return res
		}
		has := strings.Contains(string(data), c.Keyword)
		meta["keyword_found"] = has
		if c.KeywordMode == "not_contains" {
			if has {
				res.Error = "کلیدواژه‌ای که نباید در پاسخ باشد، پیدا شد"
			}
		} else if !has {
			res.Error = "کلیدواژه در پاسخ پیدا نشد"
		}
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	}

	if c.SlowMS > 0 {
		elapsed := time.Since(start)
		if elapsed.Milliseconds() > int64(c.SlowMS) {
			res.Warn = fmt.Sprintf("پاسخ کند است (%d میلی‌ثانیه)", elapsed.Milliseconds())
			meta["slow"] = true
		}
	}
	res.OK = res.Error == ""
	return res
}

func statusMatches(rule string, code int) bool {
	rule = strings.TrimSpace(strings.ToLower(rule))
	if rule == "" {
		return code >= 200 && code < 400
	}
	for _, part := range strings.Split(rule, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasSuffix(part, "xx") && len(part) == 3 {
			base, err := strconv.Atoi(part[:1])
			if err == nil && code/100 == base {
				return true
			}
			continue
		}
		if strings.Contains(part, "-") {
			se := strings.SplitN(part, "-", 2)
			lo, e1 := strconv.Atoi(strings.TrimSpace(se[0]))
			hi, e2 := strconv.Atoi(strings.TrimSpace(se[1]))
			if e1 == nil && e2 == nil && code >= lo && code <= hi {
				return true
			}
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && code == n {
			return true
		}
	}
	return false
}

func friendlyNetErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "context deadline exceeded"):
		return "اتمام مهلت بررسی (timeout)"
	case strings.Contains(msg, "no such host"):
		return "دامنه پیدا نشد (DNS)"
	case strings.Contains(msg, "connection refused"):
		return "اتصال رد شد (connection refused)"
	case strings.Contains(msg, "certificate"):
		return "خطای گواهی TLS: " + msg
	}
	return msg
}

// startTimeFromContext is a small trick so timing helpers stay simple; the
// manager overwrites Result.LatencyMS with an accurate total anyway.
