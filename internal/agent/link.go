package agent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"

	"pulse/internal/agentproto"
	"pulse/internal/checker"
	"pulse/internal/model"
	"pulse/internal/xray"
)

const liveInterval = time.Second

type agentRunner struct {
	opts     Options
	logger   *slog.Logger
	server   string
	interval time.Duration
	coll     *collector
	probes   *checker.Manager
	xray     *xray.Engine
	checkSem chan struct{}
	ipMu     sync.Mutex
	ipText   string
	ipAt     time.Time
}

type agentLinkConn struct {
	send  func(*agentproto.AgentEvent) error
	recv  func() (*agentproto.PanelCommand, error)
	close func() error
}

func newAgentRunner(opts Options, logger *slog.Logger) *agentRunner {
	interval := opts.Interval
	if interval < time.Second {
		interval = time.Second
	}
	xrayBin := strings.TrimSpace(opts.XrayBin)
	if xrayBin == "" {
		xrayBin = "xray"
	}
	engine := xray.NewEngine(xrayBin)
	detectCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	status := engine.Detect(detectCtx)
	cancel()
	if status.Available {
		logger.Info("agent Xray detected", "version", status.Version, "path", status.Path)
	} else {
		logger.Warn("agent Xray not available; proxy real-handshake checks will be skipped", "error", status.Error)
	}
	probes := checker.New("")
	probes.Xray = engine
	return &agentRunner{
		opts:     opts,
		logger:   logger,
		server:   strings.TrimRight(strings.TrimSpace(opts.Server), "/"),
		interval: interval,
		coll:     newCollector(opts.Services, opts.CollectWG),
		probes:   probes,
		xray:     engine,
		checkSem: make(chan struct{}, 2),
	}
}

func (r *agentRunner) restClient() *restClient {
	return newRESTClient(r.server, r.opts.Token, r.opts.Version, r.logger)
}

// runLive forever cycles through the best available transport. gRPC is tried
// first; WebSocket is the fallback for reverse proxies without h2c; REST keeps
// metrics flowing if neither live transport is reachable.
func (r *agentRunner) runLive(ctx context.Context) error {
	failures := 0
	for {
		if ctx.Err() != nil {
			return nil
		}
		ran, err := r.runGRPC(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			return nil
		}
		if ran < 20*time.Second {
			failures++
		} else {
			failures = 0
		}
		r.logger.Warn("gRPC link unavailable; trying WebSocket", "error", err)

		ran, err = r.runWS(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			return nil
		}
		if ran < 20*time.Second {
			failures++
		} else {
			failures = 0
		}
		r.logger.Warn("WebSocket link unavailable; using REST reports for now", "error", err)

		restErr := r.runRESTWindow(ctx, 30*time.Second)
		if ctx.Err() != nil {
			return nil
		}
		if restErr != nil {
			failures++
			r.logger.Warn("REST fallback report failed", "error", restErr, "failures", failures)
		} else {
			failures = 0
		}
		if err := wait(ctx, retryDelay(failures)); err != nil {
			return nil
		}
	}
}

func (r *agentRunner) runGRPC(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	cc, err := r.dialGRPC(ctx)
	if err != nil {
		return 0, err
	}
	defer cc.Close()
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	mdCtx := metadata.AppendToOutgoingContext(streamCtx,
		"authorization", "Bearer "+r.opts.Token,
		"x-agent-token", r.opts.Token,
		"x-agent-protocol", strconv.Itoa(agentproto.ProtocolVersion),
	)
	stream, err := agentproto.NewClient(cc).Connect(mdCtx)
	if err != nil {
		return time.Since(start), err
	}
	link := agentLinkConn{
		send:  stream.Send,
		recv:  stream.Recv,
		close: func() error { _ = stream.CloseSend(); cancel(); return nil },
	}
	err = r.serveLink(streamCtx, link, "gRPC")
	return time.Since(start), err
}

func (r *agentRunner) dialGRPC(ctx context.Context) (*grpc.ClientConn, error) {
	u, err := url.Parse(r.server)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid server URL")
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	target := net.JoinHostPort(u.Hostname(), port)
	var creds credentials.TransportCredentials
	if u.Scheme == "https" {
		creds = credentials.NewTLS(&tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
	} else {
		creds = insecure.NewCredentials()
	}
	cc, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16<<20), grpc.MaxCallSendMsgSize(16<<20)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: 20 * time.Second, Timeout: 5 * time.Second, PermitWithoutStream: true}),
	)
	if err != nil {
		return nil, err
	}
	cc.Connect()
	deadline := time.Now().Add(5 * time.Second)
	for {
		state := cc.GetState()
		if state == connectivity.Ready {
			return cc, nil
		}
		if state == connectivity.Shutdown {
			cc.Close()
			return nil, fmt.Errorf("gRPC connection is shut down")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			cc.Close()
			return nil, fmt.Errorf("gRPC connection timed out")
		}
		waitCtx, waitCancel := context.WithTimeout(ctx, remaining)
		changed := cc.WaitForStateChange(waitCtx, state)
		waitCancel()
		if !changed {
			cc.Close()
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("gRPC connection timed out")
		}
	}
}

func (r *agentRunner) runWS(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	conn, err := r.dialWS(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	linkCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	link := agentLinkConn{
		send: func(ev *agentproto.AgentEvent) error {
			writeCtx, writeCancel := context.WithTimeout(linkCtx, 10*time.Second)
			defer writeCancel()
			return wsjson.Write(writeCtx, conn, ev)
		},
		recv: func() (*agentproto.PanelCommand, error) {
			var cmd agentproto.PanelCommand
			if err := wsjson.Read(linkCtx, conn, &cmd); err != nil {
				return nil, err
			}
			return &cmd, nil
		},
		close: func() error { cancel(); return conn.Close(websocket.StatusNormalClosure, "link closed") },
	}
	err = r.serveLink(linkCtx, link, "WebSocket")
	return time.Since(start), err
}

func wsURL(server string) string {
	u, err := url.Parse(strings.TrimSpace(server))
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/api/agent/link"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (r *agentRunner) dialWS(ctx context.Context) (*websocket.Conn, error) {
	target := wsURL(r.server)
	if target == "" {
		return nil, fmt.Errorf("invalid server URL")
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+r.opts.Token)
	headers.Set("X-Agent-Token", r.opts.Token)
	headers.Set("X-Agent-Protocol", strconv.Itoa(agentproto.ProtocolVersion))
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, target, &websocket.DialOptions{
		HTTPHeader:      headers,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(16 << 20)
	return conn, nil
}

func (r *agentRunner) cachedIP() string {
	r.ipMu.Lock()
	defer r.ipMu.Unlock()
	return r.ipText
}

// primePublicIP resolves the agent's public address in the background. The
// handshake must never wait for an external IP service: behind Caddy the peer
// address is loopback, but blocking Hello until ipify answers would delay the
// live link (and the first report) by several seconds.
func (r *agentRunner) primePublicIP() {
	if !r.shouldDetectPublicIP() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 14*time.Second)
		defer cancel()
		r.publicIP(ctx)
	}()
}

func (r *agentRunner) shouldDetectPublicIP() bool {
	u, err := url.Parse(strings.TrimSpace(r.server))
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return false
	}
	return true
}

func (r *agentRunner) publicIP(ctx context.Context) string {
	r.ipMu.Lock()
	if r.ipText != "" || (!r.ipAt.IsZero() && time.Since(r.ipAt) < 5*time.Minute) {
		value := r.ipText
		r.ipMu.Unlock()
		return value
	}
	r.ipAt = time.Now()
	r.ipMu.Unlock()

	client := &http.Client{Timeout: 4 * time.Second}
	// Prefer explicit IPv4 endpoints: a dual-stack resolver often returns an
	// IPv6 egress address, which is not the primary server address shown by
	// most VPS panels and confused operators in the UI.
	for _, endpoint := range []string{
		"https://api.ipify.org",
		"https://ipv4.icanhazip.com",
		"https://ipv4.ifconfig.co",
		"https://checkip.amazonaws.com",
	} {
		reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			cancel()
			continue
		}
		req.Header.Set("User-Agent", "Pulse-Agent")
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 128))
		resp.Body.Close()
		cancel()
		value := strings.Trim(strings.TrimSpace(string(data)), "[]")
		ip := net.ParseIP(value)
		if ip == nil || ip.To4() == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() {
			continue
		}
		r.ipMu.Lock()
		r.ipText = ip.String()
		r.ipMu.Unlock()
		return ip.String()
	}
	return ""
}

func (r *agentRunner) serveLink(parent context.Context, link agentLinkConn, transport string) error {
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	defer link.close()

	var sendMu sync.Mutex
	send := func(ev *agentproto.AgentEvent) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		if err := link.send(ev); err != nil {
			cancel(err)
			return err
		}
		return nil
	}

	r.primePublicIP()
	snap, firstReport := r.coll.snapshot()
	status := r.xray.Status()
	hello := &agentproto.Hello{
		Version: r.opts.Version, Hostname: snap.Hostname, OS: snap.OS, Arch: snap.Arch,
		IP: r.cachedIP(), CanCheck: true, XrayAvailable: status.Available, XrayVersion: status.Version,
		IntervalSec: int(liveInterval / time.Second),
	}
	if err := send(&agentproto.AgentEvent{Type: agentproto.EventHello, Hello: hello}); err != nil {
		return err
	}

	recvErr := make(chan error, 1)
	go func() {
		for {
			cmd, err := link.recv()
			if err != nil {
				recvErr <- err
				return
			}
			r.handleCommand(ctx, cmd, send)
		}
	}()

	r.logger.Info("agent live link established", "transport", transport, "xray", status.Available)
	// Use the same snapshot for hello and the first report: a second immediate
	// snapshot would calculate network rates over a near-zero interval.
	firstReport.Version = r.opts.Version
	firstReport.IP = r.cachedIP()
	if err := send(&agentproto.AgentEvent{Type: agentproto.EventReport, Report: &firstReport}); err != nil {
		return err
	}
	ticker := time.NewTicker(liveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if parent.Err() != nil {
				return nil
			}
			return context.Cause(ctx)
		case err := <-recvErr:
			cancel(err)
			return err
		case <-ticker.C:
			if err := r.sendReport(send); err != nil {
				return err
			}
		}
	}
}

func (r *agentRunner) sendReport(send func(*agentproto.AgentEvent) error) error {
	_, report := r.coll.snapshot()
	report.Version = r.opts.Version
	report.IP = r.cachedIP()
	if err := send(&agentproto.AgentEvent{Type: agentproto.EventReport, Report: &report, SentAt: time.Now().Unix()}); err != nil {
		return err
	}
	return nil
}

func (r *agentRunner) handleCommand(ctx context.Context, cmd *agentproto.PanelCommand, send func(*agentproto.AgentEvent) error) {
	if cmd == nil {
		return
	}
	switch cmd.Type {
	case agentproto.CommandHello:
		if strings.TrimSpace(cmd.TestURL) != "" {
			r.probes.SetProxyTestURL(cmd.TestURL)
		}
		r.logger.Debug("agent link hello acknowledged", "server_version", cmd.ServerVersion, "transport_interval", cmd.IntervalSec)
	case agentproto.CommandCheck:
		if cmd.Check == nil {
			return
		}
		select {
		case r.checkSem <- struct{}{}:
			go func() {
				defer func() { <-r.checkSem }()
				r.runRemoteCheck(ctx, *cmd.Check, send)
			}()
		default:
			go func() {
				result := &agentproto.CheckResult{
					ID: cmd.Check.ID, MonitorID: cmd.Check.MonitorID, ProbeFailed: true,
					Error: "ایجنت در حال اجرای دو تست دیگر است؛ چند لحظه بعد دوباره تلاش کنید",
				}
				_ = send(&agentproto.AgentEvent{Type: agentproto.EventCheck, Check: result})
			}()
		}
	case agentproto.CommandError:
		r.logger.Warn("panel reported a link error", "message", cmd.Message)
	case agentproto.CommandRestart:
		r.logger.Info("panel requested an agent restart")
		go func() {
			time.Sleep(250 * time.Millisecond)
			os.Exit(0)
		}()
	case agentproto.CommandUpdate:
		r.logger.Info("panel requested an agent update")
		go r.runSelfUpdate()
	}
}

// runSelfUpdate starts the updater in a transient systemd unit so the current
// agent process (and its cgroup) can be restarted without killing the updater
// halfway through replacing the binary.
func (r *agentRunner) runSelfUpdate() {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		r.logger.Warn("agent self-update failed: executable path is unavailable")
		return
	}
	runner, err := exec.LookPath("systemd-run")
	if err != nil {
		r.logger.Warn("agent self-update needs systemd-run", "error", err)
		return
	}
	cmd := exec.Command(runner,
		"--unit=pulse-agent-update", "--collect", "--quiet",
		"--property=Type=oneshot", "--", exe, "agent", "update")
	cmd.Env = append(os.Environ(), "PULSE_AGENT_UPDATE=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		r.logger.Warn("agent self-update could not start", "error", err, "output", strings.TrimSpace(string(out)))
		return
	}
	r.logger.Info("agent self-update started")
}

func (r *agentRunner) runRemoteCheck(ctx context.Context, req agentproto.CheckRequest, send func(*agentproto.AgentEvent) error) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("remote check panic", "monitor", req.MonitorID, "error", rec)
			_ = send(&agentproto.AgentEvent{Type: agentproto.EventCheck, Check: &agentproto.CheckResult{
				ID: req.ID, MonitorID: req.MonitorID, ProbeFailed: true,
				Error: fmt.Sprintf("اجرای تست روی ایجنت با خطای داخلی متوقف شد: %v", rec),
			}})
		}
	}()
	timeout := time.Duration(req.TimeoutMS) * time.Millisecond
	if timeout < 2*time.Second {
		timeout = 2 * time.Second
	}
	if req.Type == "proxy" && timeout < 3*time.Minute {
		timeout = 3 * time.Minute
	}
	if timeout > 3*time.Minute+30*time.Second {
		timeout = 3*time.Minute + 30*time.Second
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	mon := model.Monitor{
		ID: req.MonitorID, Type: req.Type, Target: req.Target,
		Timeout: req.TimeoutMS, Config: req.Config, Enabled: true,
	}
	res := r.probes.Run(checkCtx, mon)
	meta, _ := json.Marshal(res.Meta)
	result := &agentproto.CheckResult{
		ID: req.ID, MonitorID: req.MonitorID, OK: res.OK, LatencyMS: res.LatencyMS,
		StatusCode: res.StatusCode, Error: res.Error, Warn: res.Warn,
		SSLExpiresAt: res.SSLExpiresAt, ProbeFailed: res.ProbeFailed, Meta: meta,
	}
	if err := send(&agentproto.AgentEvent{Type: agentproto.EventCheck, Check: result}); err != nil {
		r.logger.Debug("sending remote check result failed", "error", err)
	}
}

func (r *agentRunner) runRESTWindow(ctx context.Context, window time.Duration) error {
	client := r.restClient()
	if err := client.hello(ctx); err != nil {
		r.logger.Debug("agent REST handshake failed", "error", err)
	}
	deadline := time.Now().Add(window)
	r.primePublicIP()
	for {
		if ctx.Err() != nil {
			return nil
		}
		_, report := r.coll.snapshot()
		report.Version = r.opts.Version
		report.IP = r.cachedIP()
		if err := client.report(ctx, report); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return nil
		}
		if err := wait(ctx, r.interval); err != nil {
			return nil
		}
	}
}
