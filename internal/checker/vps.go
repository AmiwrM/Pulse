package checker

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pulse/internal/model"
)

type virtualizorConfig struct {
	PanelURL        string  `json:"panel_url"`
	APIKey          string  `json:"api_key"`
	APIPass         string  `json:"api_pass"`
	VPSID           int     `json:"vps_id"`
	SkipTLSVerify   bool    `json:"skip_tls_verify"`
	WarnPercent     float64 `json:"warn_percent"`
	CriticalPercent float64 `json:"critical_percent"`
}

type virtualizorVPS struct {
	VPSID         any `json:"vpsid"`
	Hostname      any `json:"hostname"`
	Status        any `json:"status"`
	Suspended     any `json:"suspended"`
	UsedBandwidth any `json:"used_bandwidth"`
	Bandwidth     any `json:"bandwidth"`
	NetIn         any `json:"net_in"`
	NetOut        any `json:"net_out"`
}

type virtualizorResponse struct {
	Title   string          `json:"title"`
	VPSData json.RawMessage `json:"vps_data"`
	Status  json.RawMessage `json:"status"`
	Error   any             `json:"error"`
	Done    any             `json:"done"`
}

func (m *Manager) checkVirtualizor(ctx context.Context, mon model.Monitor) Result {
	var c virtualizorConfig
	_ = decodeConfig(mon.Config, &c)
	c.PanelURL = strings.TrimRight(strings.TrimSpace(c.PanelURL), "/")
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.APIPass = strings.TrimSpace(c.APIPass)
	if c.PanelURL == "" || c.APIKey == "" || c.APIPass == "" || c.VPSID <= 0 {
		return Result{Error: "آدرس پنل، API Key، API Password و VPS ID الزامی است"}
	}
	if !strings.HasPrefix(c.PanelURL, "http://") && !strings.HasPrefix(c.PanelURL, "https://") {
		return Result{Error: "آدرس پنل Virtualizor باید با http:// یا https:// شروع شود"}
	}
	if c.WarnPercent <= 0 || c.WarnPercent >= 100 {
		c.WarnPercent = 80
	}
	if c.CriticalPercent <= 0 || c.CriticalPercent > 100 {
		c.CriticalPercent = 95
	}
	if c.CriticalPercent < c.WarnPercent {
		c.CriticalPercent = c.WarnPercent
	}

	start := time.Now()
	raw, err := fetchVirtualizor(ctx, c)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return Result{LatencyMS: latency, Error: err.Error()}
	}
	entry, err := pickVirtualizorVPS(raw, c.VPSID)
	if err != nil {
		return Result{LatencyMS: latency, Error: err.Error()}
	}
	status := strings.ToLower(strings.TrimSpace(anyString(entry.Status)))
	suspended := anyBool(entry.Suspended) || status == "suspended" || status == "suspend"
	running := status == "1" || status == "running" || status == "on" || status == "online"
	hostname := anyString(entry.Hostname)
	used := anyFloat(entry.UsedBandwidth)
	limit := anyFloat(entry.Bandwidth)
	percent := 0.0
	remaining := 0.0
	if limit > 0 {
		percent = used / limit * 100
		remaining = limit - used
		if remaining < 0 {
			remaining = 0
		}
	}
	meta := map[string]any{
		"provider": "virtualizor", "panel": c.PanelURL, "vps_id": c.VPSID,
		"hostname": hostname, "status": status, "suspended": suspended,
		"used_gb": round2(used), "limit_gb": round2(limit), "remaining_gb": round2(remaining),
		"percent": round1(percent), "net_in": anyFloat(entry.NetIn), "net_out": anyFloat(entry.NetOut),
	}
	res := Result{LatencyMS: latency, Meta: meta}
	switch {
	case suspended:
		res.Error = fmt.Sprintf("VPS %s suspended است", fallbackStr(hostname, fmt.Sprint(c.VPSID)))
	case !running:
		res.Error = fmt.Sprintf("VPS %s روشن نیست (status=%s)", fallbackStr(hostname, fmt.Sprint(c.VPSID)), status)
	case limit > 0 && percent >= c.CriticalPercent:
		res.Error = fmt.Sprintf("ترافیک VPS %s بحرانی است: %.1f%% از %.1f GB مصرف شده", fallbackStr(hostname, fmt.Sprint(c.VPSID)), percent, limit)
		res.Meta["traffic_state"] = "critical"
	case limit > 0 && percent >= c.WarnPercent:
		res.Warn = fmt.Sprintf("ترافیک VPS %s به %.1f%% رسیده است (%.1f از %.1f GB)", fallbackStr(hostname, fmt.Sprint(c.VPSID)), percent, used, limit)
		res.Meta["traffic_state"] = "warning"
	default:
		res.Meta["traffic_state"] = "normal"
	}
	res.OK = res.Error == ""
	return res
}

func fetchVirtualizor(ctx context.Context, c virtualizorConfig) ([]byte, error) {
	key := randomVZKey(8)
	sum := md5.Sum([]byte(c.APIPass + key))
	base := c.PanelURL
	if strings.HasSuffix(strings.ToLower(base), "/index.php") {
		base = strings.TrimSuffix(base, "/index.php")
	}
	endpoint := strings.TrimRight(base, "/") + "/index.php"
	q := url.Values{}
	q.Set("act", "vps_stats")
	q.Set("live", "1")
	q.Set("vpsid", fmt.Sprint(c.VPSID))
	q.Set("reslen", "50")
	q.Set("api", "json")
	q.Set("adminapikey", c.APIKey)
	q.Set("adminapipass", c.APIPass)
	q.Set("apikey", key+hex.EncodeToString(sum[:]))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	if c.SkipTLSVerify {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("اتصال به Virtualizor ناموفق بود: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Virtualizor کد %d برگرداند", resp.StatusCode)
	}
	var body json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("پاسخ Virtualizor JSON معتبر نبود (آیا API=json فعال است؟): %v", err)
	}
	return body, nil
}

func pickVirtualizorVPS(raw []byte, vpsID int) (virtualizorVPS, error) {
	var resp virtualizorResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return virtualizorVPS{}, fmt.Errorf("پاسخ Virtualizor قابل خواندن نبود: %v", err)
	}
	if msg := strings.TrimSpace(anyString(resp.Error)); msg != "" && msg != "0" {
		return virtualizorVPS{}, fmt.Errorf("Virtualizor: %s", msg)
	}
	if len(resp.VPSData) == 0 || string(resp.VPSData) == "null" {
		return virtualizorVPS{}, fmt.Errorf("اطلاعات VPS در پاسخ Virtualizor پیدا نشد")
	}
	var byID map[string]virtualizorVPS
	if err := json.Unmarshal(resp.VPSData, &byID); err == nil {
		for key, entry := range byID {
			if int(anyFloat(entry.VPSID)) == vpsID || key == fmt.Sprintf("K_%d", vpsID) || key == fmt.Sprint(vpsID) {
				return entry, nil
			}
		}
	}
	var list []virtualizorVPS
	if err := json.Unmarshal(resp.VPSData, &list); err == nil {
		for _, entry := range list {
			if int(anyFloat(entry.VPSID)) == vpsID {
				return entry, nil
			}
		}
	}
	var single virtualizorVPS
	if err := json.Unmarshal(resp.VPSData, &single); err == nil && int(anyFloat(single.VPSID)) == vpsID {
		return single, nil
	}
	return virtualizorVPS{}, fmt.Errorf("VPS با شناسه %d در پاسخ Virtualizor پیدا نشد", vpsID)
}

func randomVZKey(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	out := make([]byte, n)
	for i := range out {
		v, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			out[i] = 'a'
			continue
		}
		out[i] = chars[v.Int64()]
	}
	return string(out)
}

func anyString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case float64:
		return fmt.Sprintf("%.0f", value)
	case json.Number:
		return value.String()
	case bool:
		if value {
			return "1"
		}
		return "0"
	}
	return ""
}

func anyFloat(v any) float64 {
	switch value := v.(type) {
	case float64:
		return value
	case string:
		var f float64
		_, _ = fmt.Sscanf(strings.TrimSpace(value), "%f", &f)
		return f
	case json.Number:
		f, _ := value.Float64()
		return f
	case bool:
		if value {
			return 1
		}
	}
	return 0
}

func anyBool(v any) bool {
	switch value := v.(type) {
	case bool:
		return value
	case float64:
		return value != 0
	case string:
		value = strings.ToLower(strings.TrimSpace(value))
		return value == "1" || value == "true" || value == "yes"
	}
	return false
}

func fallbackStr(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }
func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }
