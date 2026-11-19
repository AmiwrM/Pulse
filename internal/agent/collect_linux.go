//go:build linux

package agent

import (
	"context"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (c *collector) collect() Snapshot {
	s := Snapshot{
		OS: runtime.GOOS, Arch: runtime.GOARCH,
		Services: []Service{}, WireGuard: []WGInterface{},
	}
	s.Hostname, _ = os.Hostname()
	if pretty := osPrettyName(); pretty != "" {
		s.OS = pretty
	}
	s.CPUPct = c.cpuPercent()
	s.RAMPct, s.RAMTotal = memoryUsage()
	s.DiskPct, s.DiskTotal = diskUsage("/")
	s.Load1 = loadAverage()
	s.UptimeSec = uptimeSeconds()
	s.NetRxBps, s.NetTxBps = c.netRates()
	for _, name := range c.services {
		s.Services = append(s.Services, serviceStatus(name))
	}
	if c.collectWG {
		s.WireGuard = wireGuardStatus()
	}
	return s
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func (c *collector) cpuPercent() float64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	line := strings.SplitN(string(data), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return 0
	}
	var total, idle uint64
	for i, f := range fields[1:] {
		n, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			continue
		}
		total += n
		if i == 3 || i == 4 { // idle + iowait
			idle += n
		}
	}
	pct := 0.0
	if c.cpuTotal > 0 && total > c.cpuTotal {
		dt := float64(total - c.cpuTotal)
		di := float64(idle - c.cpuIdle)
		if dt > 0 {
			pct = (dt - di) / dt * 100
		}
	}
	c.cpuTotal, c.cpuIdle = total, idle
	return round2(pct)
}

func memoryUsage() (float64, uint64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	var total, avail uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = v * 1024
		case "MemAvailable:":
			avail = v * 1024
		}
	}
	if total == 0 {
		return 0, 0
	}
	return round2(float64(total-avail) / float64(total) * 100), total
}

func diskUsage(path string) (float64, uint64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0
	}
	total := st.Blocks * uint64(st.Bsize)
	avail := st.Bavail * uint64(st.Bsize)
	if total == 0 {
		return 0, 0
	}
	return round2(float64(total-avail) / float64(total) * 100), total
}

func loadAverage() float64 {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(fields[0], 64)
	return round2(v)
}

func uptimeSeconds() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(fields[0], 64)
	return int64(v)
}

func (c *collector) netRates() (float64, float64) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	var rx, tx uint64
	for i, line := range strings.Split(string(data), "\n") {
		if i < 2 {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(fields[0], 10, 64)
		t, _ := strconv.ParseUint(fields[8], 10, 64)
		rx += r
		tx += t
	}
	now := time.Now()
	rxBps, txBps := 0.0, 0.0
	if !c.netAt.IsZero() {
		elapsed := now.Sub(c.netAt).Seconds()
		if elapsed > 0 {
			if rx >= c.netRx {
				rxBps = float64(rx-c.netRx) / elapsed
			}
			if tx >= c.netTx {
				txBps = float64(tx-c.netTx) / elapsed
			}
		}
	}
	c.netRx, c.netTx, c.netAt = rx, tx, now
	return math.Round(rxBps), math.Round(txBps)
}

func serviceStatus(name string) Service {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "is-active", name).CombinedOutput()
	detail := strings.TrimSpace(string(out))
	if err != nil && detail == "" {
		detail = err.Error()
	}
	active := detail == "active"
	if !active && detail == "" {
		detail = "unknown"
	}
	return Service{Name: name, Active: active, Detail: detail}
}

func wireGuardStatus() []WGInterface {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "wg", "show", "all", "dump").Output()
	if err != nil {
		return []WGInterface{}
	}
	byName := map[string]*WGInterface{}
	order := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		switch len(fields) {
		case 5: // interface header
			name := fields[0]
			if _, ok := byName[name]; !ok {
				byName[name] = &WGInterface{Name: name, Peers: []WGPeer{}}
				order = append(order, name)
			}
		case 9: // peer
			name := fields[0]
			iface := byName[name]
			if iface == nil {
				iface = &WGInterface{Name: name, Peers: []WGPeer{}}
				byName[name] = iface
				order = append(order, name)
			}
			hs, _ := strconv.ParseInt(fields[5], 10, 64)
			rx, _ := strconv.ParseUint(fields[6], 10, 64)
			tx, _ := strconv.ParseUint(fields[7], 10, 64)
			endpoint := fields[3]
			if endpoint == "(none)" || endpoint == "none" {
				endpoint = ""
			}
			iface.Peers = append(iface.Peers, WGPeer{
				PublicKey: fields[1], Endpoint: endpoint, AllowedIPs: fields[4],
				LatestHandshake: hs, Rx: rx, Tx: tx,
			})
		}
	}
	result := make([]WGInterface, 0, len(order))
	for _, name := range order {
		result = append(result, *byName[name])
	}
	return result
}

func osPrettyName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			v := strings.TrimPrefix(line, "PRETTY_NAME=")
			v = strings.Trim(v, `"'`)
			if v != "" {
				return v
			}
		}
	}
	return ""
}
