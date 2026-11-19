package checker

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"pulse/internal/model"
	"pulse/internal/xray"
)

// A proxy monitor must remain useful when Xray is not installed, while clearly
// reporting that the requested real handshake could not be performed.
func TestProxyWithoutXrayFallsBackWithWarning(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	addr := ln.Addr().String()
	mon := model.Monitor{
		Type: "proxy", Target: "local", Timeout: 60000, Interval: 60,
		Config: []byte(`{"input":"vless://11111111-2222-3333-4444-555555555555@` + addr + `?security=none#Local","min_healthy":1,"real_test":true}`),
	}
	m := New("1.1.1.1:53")
	m.Xray = xray.NewEngine("pulse-xray-binary-that-does-not-exist")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res := m.Run(ctx, mon)
	if !res.OK {
		t.Fatalf("fallback reachability should keep the monitor healthy: %+v", res)
	}
	if !strings.Contains(res.Warn, "Xray") {
		t.Fatalf("expected a warning about unavailable Xray, got %q", res.Warn)
	}
	nodes, ok := res.Meta["nodes"].([]ProxyNode)
	if !ok || len(nodes) != 1 {
		t.Fatalf("unexpected nodes metadata: %#v", res.Meta["nodes"])
	}
	if nodes[0].Method != "tcp" {
		t.Fatalf("expected TCP method when real test is skipped, got %q", nodes[0].Method)
	}
}
