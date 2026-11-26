package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"pulse/internal/agentproto"
	"pulse/internal/checker"
	"pulse/internal/model"
)

func newLinkTestApp(t *testing.T) (*App, *model.Agent, string) {
	t.Helper()
	a, err := New(Config{
		Addr: "127.0.0.1:0", DBPath: filepath.Join(t.TempDir(), "pulse.db"), Version: "1.5.0-test",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	ag, token, err := a.db.CreateAgent("iran-probe")
	if err != nil {
		t.Fatal(err)
	}
	return a, ag, token
}

func testReport() *model.AgentReport {
	return &model.AgentReport{
		Hostname: "iran-probe", OS: "linux", Arch: "amd64", Version: "1.5.0-test",
		CPUPct: 42.5, RAMPct: 51, DiskPct: 20, Load1: 0.7,
		NetRx: 1500, NetTx: 500, RAMTotal: 1 << 30, DiskTotal: 10 << 30,
		Services: json.RawMessage("[]"), WireGuard: json.RawMessage("[]"),
	}
}

func waitAgentMetric(t *testing.T, a *App, id int64, want float64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ag, err := a.db.GetAgent(id)
		if err == nil && ag != nil && ag.CPUPct == want && a.links.get(id) != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("agent metric %v was not persisted", want)
}

func remoteCheckRoundTrip(t *testing.T, a *App, ag *model.Agent, recv func(context.Context) (*agentproto.PanelCommand, error), send func(*agentproto.AgentEvent) error) checker.Result {
	t.Helper()
	m := &model.Monitor{
		Name: "remote-tcp", Type: "tcp", Target: "127.0.0.1:9", Enabled: true,
		Interval: 60, Timeout: 2000, ProbeAgentID: ag.ID,
		Config: json.RawMessage(`{}`), FailThreshold: 2, RecoverThreshold: 1,
	}
	if err := a.db.CreateMonitor(m); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan checker.Result, 1)
	go func() { done <- a.runRemoteMonitor(ctx, *m) }()

	cmd, err := recv(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Type != agentproto.CommandCheck || cmd.Check == nil {
		t.Fatalf("unexpected command: %+v", cmd)
	}
	if cmd.Check.Target != m.Target || cmd.Check.MonitorID != m.ID {
		t.Fatalf("unexpected check command: %+v", cmd.Check)
	}
	if err := send(&agentproto.AgentEvent{Type: agentproto.EventCheck, Check: &agentproto.CheckResult{
		ID: cmd.Check.ID, MonitorID: cmd.Check.MonitorID, OK: true, LatencyMS: 17,
		Meta: json.RawMessage(`{"method":"tcp"}`),
	}}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		return got
	case <-ctx.Done():
		t.Fatal("remote check timed out")
		return checker.Result{}
	}
}

func TestGRPCAgentLinkReportsAndRunsRemoteCheck(t *testing.T) {
	a, ag, token := newLinkTestApp(t)
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	agentproto.RegisterServer(gs, a)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-agent-token", token)
	stream, err := agentproto.NewClient(conn).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	helloAck, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if helloAck.Type != agentproto.CommandHello {
		t.Fatalf("expected hello ack, got %+v", helloAck)
	}
	if err := stream.Send(&agentproto.AgentEvent{Type: agentproto.EventHello, Hello: &agentproto.Hello{
		Version: "1.5.0-test", Hostname: "iran-probe", CanCheck: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&agentproto.AgentEvent{Type: agentproto.EventReport, Report: testReport()}); err != nil {
		t.Fatal(err)
	}
	waitAgentMetric(t, a, ag.ID, 42.5)

	got := remoteCheckRoundTrip(t, a, ag,
		func(ctx context.Context) (*agentproto.PanelCommand, error) { return stream.Recv() },
		stream.Send,
	)
	if !got.OK || got.LatencyMS != 17 {
		t.Fatalf("unexpected remote result: %+v", got)
	}
	if got.Meta["probe_agent_name"] != "iran-probe" || got.Meta["probe_transport"] != "gRPC" {
		t.Fatalf("missing probe metadata: %+v", got.Meta)
	}
}

func TestWebSocketAgentLinkReportsAndRunsRemoteCheck(t *testing.T) {
	a, ag, token := newLinkTestApp(t)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/agent/link"
	headers := http.Header{}
	headers.Set("X-Agent-Token", token)
	headers.Set("Authorization", "Bearer "+token)
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(1 << 20)

	var helloAck agentproto.PanelCommand
	if err := wsjson.Read(ctx, conn, &helloAck); err != nil {
		t.Fatal(err)
	}
	if helloAck.Type != agentproto.CommandHello {
		t.Fatalf("expected hello ack, got %+v", helloAck)
	}
	if err := wsjson.Write(ctx, conn, &agentproto.AgentEvent{Type: agentproto.EventHello, Hello: &agentproto.Hello{
		Version: "1.5.0-test", Hostname: "iran-probe", CanCheck: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(ctx, conn, &agentproto.AgentEvent{Type: agentproto.EventReport, Report: testReport()}); err != nil {
		t.Fatal(err)
	}
	waitAgentMetric(t, a, ag.ID, 42.5)

	got := remoteCheckRoundTrip(t, a, ag,
		func(ctx context.Context) (*agentproto.PanelCommand, error) {
			var cmd agentproto.PanelCommand
			if err := wsjson.Read(ctx, conn, &cmd); err != nil {
				return nil, err
			}
			return &cmd, nil
		},
		func(ev *agentproto.AgentEvent) error { return wsjson.Write(ctx, conn, ev) },
	)
	if !got.OK || got.LatencyMS != 17 {
		t.Fatalf("unexpected remote result: %+v", got)
	}
	if got.Meta["probe_transport"] != "WebSocket" {
		t.Fatalf("missing probe metadata: %+v", got.Meta)
	}
}
