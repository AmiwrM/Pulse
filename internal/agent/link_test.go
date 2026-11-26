package agent

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"pulse/internal/agentproto"
)

func TestRunnerWebSocketLinkAndRemoteCheck(t *testing.T) {
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	go func() {
		for {
			conn, err := tcp.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	reportSeen := make(chan struct{}, 1)
	checkSeen := make(chan agentproto.CheckResult, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Agent-Token") != "test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		if err := wsjson.Write(ctx, conn, &agentproto.PanelCommand{Type: agentproto.CommandHello, ServerVersion: "test"}); err != nil {
			return
		}
		sentCheck := false
		for {
			var ev agentproto.AgentEvent
			if err := wsjson.Read(ctx, conn, &ev); err != nil {
				return
			}
			switch ev.Type {
			case agentproto.EventReport:
				select {
				case reportSeen <- struct{}{}:
				default:
				}
				if !sentCheck {
					sentCheck = true
					if err := wsjson.Write(ctx, conn, &agentproto.PanelCommand{Type: agentproto.CommandCheck, Check: &agentproto.CheckRequest{
						ID: "check-1", MonitorID: 7, Type: "tcp", Target: tcp.Addr().String(), TimeoutMS: 3000,
					}}); err != nil {
						return
					}
				}
			case agentproto.EventCheck:
				if ev.Check != nil {
					select {
					case checkSeen <- *ev.Check:
					default:
					}
				}
			}
		}
	}))
	defer srv.Close()

	opts := Options{
		Server: srv.URL, Token: "test-token", Name: "ws-agent", Version: "1.6.0-test",
		Interval: time.Second, XrayBin: "xray-not-installed",
	}
	runner := newAgentRunner(opts, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := runner.runWS(ctx); done <- err }()

	select {
	case <-reportSeen:
	case <-time.After(4 * time.Second):
		t.Fatal("agent did not send a live report")
	}
	select {
	case res := <-checkSeen:
		if !res.OK || res.ID != "check-1" || res.MonitorID != 7 {
			t.Fatalf("unexpected remote check result: %+v", res)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("agent did not execute the remote check")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("agent WebSocket runner did not stop")
	}
}

func TestWSUrl(t *testing.T) {
	if got := wsURL("https://status.example.com/base"); got != "wss://status.example.com/api/agent/link" {
		t.Fatalf("unexpected wss URL: %s", got)
	}
	if got := wsURL("http://127.0.0.1:8080"); got != "ws://127.0.0.1:8080/api/agent/link" {
		t.Fatalf("unexpected ws URL: %s", got)
	}
	if !strings.HasPrefix(wsURL("http://[::1]:8080"), "ws://[::1]:8080") {
		t.Fatalf("IPv6 URL was mangled: %s", wsURL("http://[::1]:8080"))
	}
}
