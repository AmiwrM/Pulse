package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"pulse/internal/model"
)

const (
	Version         = "1.4.3"
	ProtocolVersion = 2
)

type Options struct {
	Server    string
	Token     string
	Name      string
	Interval  time.Duration
	Services  []string
	CollectWG bool
	Once      bool
	Version   string
}

type Service struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
	Detail string `json:"detail,omitempty"`
}

type WGPeer struct {
	PublicKey       string `json:"public_key"`
	Endpoint        string `json:"endpoint"`
	AllowedIPs      string `json:"allowed_ips"`
	LatestHandshake int64  `json:"latest_handshake"`
	Rx              uint64 `json:"rx"`
	Tx              uint64 `json:"tx"`
}

type WGInterface struct {
	Name  string   `json:"name"`
	Peers []WGPeer `json:"peers"`
}

type Snapshot struct {
	Hostname  string        `json:"hostname"`
	OS        string        `json:"os"`
	Arch      string        `json:"arch"`
	CPUPct    float64       `json:"cpu_pct"`
	RAMPct    float64       `json:"ram_pct"`
	DiskPct   float64       `json:"disk_pct"`
	Load1     float64       `json:"load1"`
	UptimeSec int64         `json:"uptime_sec"`
	NetRxBps  float64       `json:"net_rx_bps"`
	NetTxBps  float64       `json:"net_tx_bps"`
	RAMTotal  uint64        `json:"ram_total"`
	DiskTotal uint64        `json:"disk_total"`
	Services  []Service     `json:"services"`
	WireGuard []WGInterface `json:"wireguard"`
}

type collector struct {
	services  []string
	collectWG bool
	cpuTotal  uint64
	cpuIdle   uint64
	netRx     uint64
	netTx     uint64
	netAt     time.Time
}

type helloResponse struct {
	OK   bool `json:"ok"`
	Data struct {
		ServerVersion     string `json:"server_version"`
		ReportIntervalSec int    `json:"report_interval_sec"`
		OfflineAfterSec   int    `json:"offline_after_sec"`
	} `json:"data"`
}

type restClient struct {
	base    string
	token   string
	version string
	http    *http.Client
	logger  *slog.Logger
}

func newRESTClient(server, token, version string, logger *slog.Logger) *restClient {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          8,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &restClient{
		base:    strings.TrimRight(strings.TrimSpace(server), "/"),
		token:   token,
		version: version,
		http:    &http.Client{Timeout: 25 * time.Second, Transport: transport},
		logger:  logger,
	}
}

func (c *restClient) request(ctx context.Context, method, path string, payload any, out any) (int, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Agent-Token", c.token)
	req.Header.Set("User-Agent", "Pulse-Agent/"+c.version)
	req.Header.Set("X-Agent-Protocol", fmt.Sprint(ProtocolVersion))
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if readErr != nil {
		return resp.StatusCode, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		text := strings.TrimSpace(string(raw))
		if len(text) > 300 {
			text = text[:300] + "…"
		}
		if text == "" {
			text = resp.Status
		}
		return resp.StatusCode, fmt.Errorf("server returned %s: %s", resp.Status, text)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("invalid server response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

func (c *restClient) hello(ctx context.Context) error {
	var out helloResponse
	status, err := c.request(ctx, http.MethodGet, "/api/agent/hello", nil, &out)
	if err != nil {
		if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
			// Older panels do not expose the handshake endpoint. Reports still work.
			c.logger.Debug("agent hello endpoint is not available on this panel")
			return nil
		}
		return err
	}
	if !out.OK {
		return errors.New("agent hello was rejected")
	}
	if out.Data.ServerVersion != "" {
		c.logger.Info("agent REST handshake", "server_version", out.Data.ServerVersion, "protocol", ProtocolVersion)
	}
	return nil
}

func (c *restClient) report(ctx context.Context, report model.AgentReport) error {
	var out struct {
		OK bool `json:"ok"`
	}
	if _, err := c.request(ctx, http.MethodPost, "/api/agent/report", report, &out); err != nil {
		return err
	}
	if !out.OK {
		return errors.New("agent report was rejected")
	}
	return nil
}

func newCollector(services []string, collectWG bool) *collector {
	clean := []string{}
	seen := map[string]bool{}
	for _, s := range services {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		clean = append(clean, s)
	}
	return &collector{services: clean, collectWG: collectWG}
}

func (c *collector) snapshot() (Snapshot, model.AgentReport) {
	snap := c.collect()
	if snap.Hostname == "" {
		snap.Hostname, _ = os.Hostname()
	}
	report := model.AgentReport{
		Hostname:  snap.Hostname,
		OS:        snap.OS,
		Arch:      snap.Arch,
		CPUPct:    snap.CPUPct,
		RAMPct:    snap.RAMPct,
		DiskPct:   snap.DiskPct,
		Load1:     snap.Load1,
		UptimeSec: snap.UptimeSec,
		NetRx:     snap.NetRxBps,
		NetTx:     snap.NetTxBps,
		RAMTotal:  snap.RAMTotal,
		DiskTotal: snap.DiskTotal,
	}
	report.Services, _ = json.Marshal(snap.Services)
	report.WireGuard, _ = json.Marshal(snap.WireGuard)
	return snap, report
}

func wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryDelay(failures int) time.Duration {
	if failures <= 0 {
		return 0
	}
	delay := 5 * time.Second
	for i := 1; i < failures && delay < 2*time.Minute; i++ {
		delay *= 2
	}
	if delay > 2*time.Minute {
		delay = 2 * time.Minute
	}
	// Small jitter avoids all agents retrying in the same second after a panel outage.
	jitter := time.Duration(time.Now().UnixNano()%int64(delay/10+1)) * time.Nanosecond
	return delay + jitter
}

func Run(ctx context.Context, opts Options, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	interval := opts.Interval
	if interval < time.Second {
		interval = time.Second
	}
	server := strings.TrimRight(strings.TrimSpace(opts.Server), "/")
	if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		return fmt.Errorf("agent server must start with http:// or https://")
	}
	if opts.Token == "" {
		return fmt.Errorf("agent token is required (--token)")
	}
	if opts.Version == "" {
		opts.Version = Version
	}
	client := newRESTClient(server, opts.Token, opts.Version, logger)
	coll := newCollector(opts.Services, opts.CollectWG)

	if opts.Once {
		if err := client.hello(ctx); err != nil {
			logger.Warn("agent handshake failed", "error", err)
		}
		_, report := coll.snapshot()
		report.Version = opts.Version
		if err := client.report(ctx, report); err != nil {
			return err
		}
		return nil
	}

	logger.Info("pulse agent started", "server", server, "name", opts.Name, "interval", interval.String(), "services", opts.Services, "transport", "REST")
	// A best-effort handshake gives operators an early token/server diagnostic.
	// It must not prevent reporting if an older panel does not expose the endpoint.
	if err := client.hello(ctx); err != nil {
		logger.Warn("agent handshake failed; reports will still be attempted", "error", err)
	}

	failures := 0
	delay := time.Duration(0)
	for {
		if err := wait(ctx, delay); err != nil {
			return nil
		}
		start := time.Now()
		_, report := coll.snapshot()
		report.Version = opts.Version
		err := client.report(ctx, report)
		elapsed := time.Since(start)
		if err != nil {
			failures++
			delay = retryDelay(failures)
			logger.Warn("agent report failed; retrying", "error", err, "failures", failures, "retry_in", delay.String())
			continue
		}
		failures = 0
		delay = interval - elapsed
		if delay < time.Second {
			delay = time.Second
		}
		logger.Info("agent report sent", "rtt_ms", elapsed.Milliseconds(), "cpu", report.CPUPct, "ram", report.RAMPct, "disk", report.DiskPct)
	}
}
