package xray

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestRealHandshake is an optional integration test. Set XRAY_BIN to a real
// Xray-core binary to run it. It starts a local VLESS server, then asks the
// engine to establish a real VLESS handshake and fetch an HTTP endpoint.
func TestRealHandshake(t *testing.T) {
	bin := os.Getenv("XRAY_BIN")
	if bin == "" {
		t.Skip("set XRAY_BIN to run the real Xray integration test")
	}
	uuid := "11111111-2222-3333-4444-555555555555"
	serverPort := freePort(t)
	dir := t.TempDir()
	serverConfig := fmt.Sprintf(`{
  "log": {"loglevel": "warning"},
  "inbounds": [{
    "tag": "vless-in",
    "listen": "127.0.0.1",
    "port": %d,
    "protocol": "vless",
    "settings": {"clients": [{"id": "%s", "flow": ""}], "decryption": "none"}
  }],
  "outbounds": [{"tag": "direct", "protocol": "freedom"}]
}`, serverPort, uuid)
	configPath := filepath.Join(dir, "server.json")
	if err := os.WriteFile(configPath, []byte(serverConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "run", "-c", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	if err := waitTCP(fmt.Sprintf("127.0.0.1:%d", serverPort), 5*time.Second); err != nil {
		t.Fatal(err)
	}

	testURL := "https://www.gstatic.com/generate_204"

	engine := NewEngine(bin)
	status := engine.Detect(context.Background())
	if !status.Available {
		t.Fatalf("engine detection failed: %+v", status)
	}
	t.Logf("detected Xray %s", status.Version)

	node := Node{
		Name: "local-vless", Protocol: "vless", Address: "127.0.0.1", Port: serverPort,
		UUID: uuid, Network: "tcp", Security: "none",
	}
	results, err := engine.TestNodes(context.Background(), []Node{node}, testURL, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].OK {
		t.Fatalf("real handshake failed: %+v", results)
	}
	t.Logf("real handshake succeeded in %d ms", results[0].LatencyMS)

	bad := node
	bad.UUID = "99999999-2222-3333-4444-555555555555"
	badResults, err := engine.TestNodes(context.Background(), []Node{bad}, testURL, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(badResults) == 1 && badResults[0].OK {
		t.Fatalf("wrong UUID unexpectedly passed the real handshake")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitTCP(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}
