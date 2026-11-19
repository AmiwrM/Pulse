//go:build !linux

package agent

import (
	"os"
	"runtime"
)

// The agent's full resource collection targets Linux servers. Other platforms
// still report basic information so the panel can be developed cross-platform.
func (c *collector) collect() Snapshot {
	hostname, _ := os.Hostname()
	services := make([]Service, 0, len(c.services))
	for _, name := range c.services {
		services = append(services, Service{Name: name, Detail: "unsupported platform"})
	}
	return Snapshot{
		Hostname:  hostname,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		CPUPct:    0,
		RAMPct:    0,
		DiskPct:   0,
		Load1:     0,
		UptimeSec: 0,
		Services:  services,
		WireGuard: []WGInterface{},
	}
}
