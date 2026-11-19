package xray

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Optional integration test for VMess and Shadowsocks outbound JSON generation.
func TestProtocolConfigs(t *testing.T) {
	bin := os.Getenv("XRAY_BIN")
	if bin == "" {
		t.Skip("set XRAY_BIN to run the real Xray integration test")
	}
	vmessPort := freePort(t)
	ssPort := freePort(t)
	uuid := "11111111-2222-3333-4444-555555555555"
	serverConfig := fmt.Sprintf(`{
  "log": {"loglevel": "warning"},
  "inbounds": [
    {"port": %d, "listen": "127.0.0.1", "protocol": "vmess", "settings": {"clients": [{"id": "%s", "alterId": 0}]}},
    {"port": %d, "listen": "127.0.0.1", "protocol": "shadowsocks", "settings": {"method": "aes-256-gcm", "password": "test-pass", "network": "tcp"}}
  ],
  "outbounds": [{"protocol": "freedom"}]
}`, vmessPort, uuid, ssPort)
	dir := t.TempDir()
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
	if err := waitTCP(fmt.Sprintf("127.0.0.1:%d", vmessPort), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := waitTCP(fmt.Sprintf("127.0.0.1:%d", ssPort), 5*time.Second); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(bin)
	if st := engine.Detect(context.Background()); !st.Available {
		t.Fatalf("engine detection failed: %+v", st)
	}
	nodes := []Node{
		{Name: "vmess", Protocol: "vmess", Address: "127.0.0.1", Port: vmessPort, UUID: uuid, AlterID: 0, Network: "tcp", Security: "none"},
		{Name: "ss", Protocol: "shadowsocks", Address: "127.0.0.1", Port: ssPort, Cipher: "aes-256-gcm", Password: "test-pass", Security: "none"},
	}
	results, err := engine.TestNodes(context.Background(), nodes, DefaultTestURL, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range results {
		if !r.OK {
			t.Fatalf("node %s failed: %+v", nodes[i].Name, r)
		}
		t.Logf("%s real test passed in %dms", nodes[i].Name, r.LatencyMS)
	}
}
