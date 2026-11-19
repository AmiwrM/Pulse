package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"pulse/internal/model"
)

const Version = "1.2.0"

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

func Run(ctx context.Context, opts Options, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	interval := opts.Interval
	if interval < 10*time.Second {
		interval = 30 * time.Second
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
	coll := newCollector(opts.Services, opts.CollectWG)
	client := &http.Client{Timeout: 25 * time.Second}
	post := func() bool {
		snap := coll.collect()
		if snap.Hostname == "" {
			snap.Hostname, _ = os.Hostname()
		}
		report := model.AgentReport{
			Hostname:  snap.Hostname,
			OS:        snap.OS,
			Arch:      snap.Arch,
			Version:   opts.Version,
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
		body, _ := json.Marshal(report)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/api/agent/report", bytes.NewReader(body))
		if err != nil {
			logger.Error("agent request", "error", err)
			return false
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Agent-Token", opts.Token)
		req.Header.Set("User-Agent", "Pulse-Agent/"+opts.Version)
		resp, err := client.Do(req)
		if err != nil {
			logger.Warn("agent report failed", "error", err)
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(resp.Body)
			logger.Warn("agent report rejected", "status", resp.StatusCode, "body", strings.TrimSpace(buf.String()))
			return false
		}
		logger.Info("agent report sent", "cpu", snap.CPUPct, "ram", snap.RAMPct, "disk", snap.DiskPct)
		return true
	}

	if opts.Once {
		if post() {
			return nil
		}
		return fmt.Errorf("report failed")
	}
	logger.Info("pulse agent started", "server", server, "name", opts.Name, "interval", interval.String(), "services", opts.Services)
	if !post() {
		logger.Warn("first report failed; retrying in 5s")
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(5 * time.Second):
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			post()
		}
	}
}
