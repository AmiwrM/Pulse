package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"pulse/internal/agentproto"
	"pulse/internal/checker"
	"pulse/internal/model"
)

var errAgentLinkClosed = errors.New("agent link is closed")

// agentLinkHub keeps the newest live link for each agent. A reconnecting agent
// replaces its old stream so a half-open TCP connection cannot black-hole
// remote checks.
type agentLinkHub struct {
	mu      sync.RWMutex
	byAgent map[int64]*agentSession
}

func newAgentLinkHub() *agentLinkHub {
	return &agentLinkHub{byAgent: map[int64]*agentSession{}}
}

func (h *agentLinkHub) get(id int64) *agentSession {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.byAgent[id]
}

func (h *agentLinkHub) register(s *agentSession) *agentSession {
	h.mu.Lock()
	old := h.byAgent[s.agentID]
	h.byAgent[s.agentID] = s
	h.mu.Unlock()
	return old
}

// remove deletes s only when it is still the current session. It returns true
// for the transport that actually owned the slot, so replaced connections do
// not emit a misleading "offline" event.
func (h *agentLinkHub) remove(s *agentSession) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.byAgent[s.agentID] != s {
		return false
	}
	delete(h.byAgent, s.agentID)
	return true
}

func (h *agentLinkHub) close(id int64, reason string) *agentSession {
	h.mu.Lock()
	s := h.byAgent[id]
	delete(h.byAgent, id)
	h.mu.Unlock()
	if s != nil {
		s.shutdown(errors.New(reason))
	}
	return s
}

func (h *agentLinkHub) live(id int64) bool { return h.get(id) != nil }

// stale returns links that have not produced any protocol message within the
// cutoff. A dead TCP connection may take a keepalive cycle to notice; this
// keeps the UI honest when an agent process is stuck but its socket is alive.
func (h *agentLinkHub) stale(cutoff int64) []*agentSession {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*agentSession, 0)
	for _, s := range h.byAgent {
		if s.lastEvent.Load() < cutoff {
			out = append(out, s)
		}
	}
	return out
}

type checkOutcome struct {
	result agentproto.CheckResult
	err    error
}

// agentSession is transport agnostic. gRPC and WebSocket only provide the
// send/receive plumbing; commands and reports share one implementation.
type agentSession struct {
	app         *App
	agentID     int64
	agentName   string
	transport   string
	ip          string
	connectedAt int64

	ctx    context.Context
	cancel context.CancelFunc

	sendMu sync.Mutex
	sendFn func(*agentproto.PanelCommand) error
	close  func() error

	pendingMu sync.Mutex
	pending   map[string]chan checkOutcome

	helloMu sync.RWMutex
	hello   agentproto.Hello

	errMu     sync.Mutex
	errText   string
	seq       atomic.Uint64
	lastEvent atomic.Int64
	stop      sync.Once
}

func (a *App) newAgentSession(parent context.Context, ag *model.Agent, transport, ip string, send func(*agentproto.PanelCommand) error, closeFn func() error) *agentSession {
	ctx, cancel := context.WithCancel(parent)
	if closeFn == nil {
		closeFn = func() error { return nil }
	}
	return &agentSession{
		app:         a,
		agentID:     ag.ID,
		agentName:   ag.Name,
		transport:   transport,
		ip:          ip,
		connectedAt: nowUnix(),
		ctx:         ctx,
		cancel:      cancel,
		sendFn:      send,
		close:       closeFn,
		pending:     map[string]chan checkOutcome{},
	}
}

func (s *agentSession) setError(err error) {
	if err == nil {
		return
	}
	s.errMu.Lock()
	if s.errText == "" {
		s.errText = err.Error()
	}
	s.errMu.Unlock()
}

func (s *agentSession) linkError() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	if s.errText != "" {
		return errors.New(s.errText)
	}
	return errAgentLinkClosed
}

func (s *agentSession) shutdown(err error) {
	s.stop.Do(func() {
		s.setError(err)
		s.cancel()
		_ = s.close()
		s.failPending(s.linkError())
	})
}

func (s *agentSession) failPending(err error) {
	s.pendingMu.Lock()
	pending := s.pending
	s.pending = map[string]chan checkOutcome{}
	s.pendingMu.Unlock()
	for _, ch := range pending {
		select {
		case ch <- checkOutcome{err: err}:
		default:
		}
	}
}

func (s *agentSession) sendCommand(cmd *agentproto.PanelCommand) error {
	if err := s.ctx.Err(); err != nil {
		return s.linkError()
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if err := s.sendFn(cmd); err != nil {
		s.setError(err)
		s.shutdown(err)
		return err
	}
	return nil
}

func (s *agentSession) handleEvent(ev *agentproto.AgentEvent) {
	if ev == nil {
		return
	}
	s.lastEvent.Store(nowUnix())
	// A replaced session must not keep writing reports or check results into
	// the panel while its old TCP/TLS stream is still winding down.
	if s.app.links.get(s.agentID) != s {
		return
	}
	switch ev.Type {
	case agentproto.EventHello:
		if ev.Hello == nil {
			return
		}
		s.helloMu.Lock()
		s.hello = *ev.Hello
		s.helloMu.Unlock()
		_ = s.app.db.TouchAgent(s.agentID, nowUnix())
		s.app.broadcastAgentLink(s, true)
	case agentproto.EventReport:
		if ev.Report == nil {
			return
		}
		report := *ev.Report
		if strings.TrimSpace(report.IP) == "" {
			report.IP = s.ip
		}
		if err := s.app.applyAgentReport(s.agentID, report, s.transport); err != nil {
			s.app.logFor("agent").Warn("agent link report", "agent", s.agentID, "transport", s.transport, "error", err)
		}
	case agentproto.EventCheck:
		if ev.Check != nil {
			s.resolve(*ev.Check)
		}
	}
}

func (s *agentSession) resolve(result agentproto.CheckResult) {
	if strings.TrimSpace(result.ID) == "" {
		return
	}
	s.pendingMu.Lock()
	ch := s.pending[result.ID]
	delete(s.pending, result.ID)
	s.pendingMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- checkOutcome{result: result}:
	default:
	}
}

func (s *agentSession) runCheck(ctx context.Context, req agentproto.CheckRequest) (agentproto.CheckResult, error) {
	id := fmt.Sprintf("%d-%d", s.agentID, s.seq.Add(1))
	req.ID = id
	ch := make(chan checkOutcome, 1)
	s.pendingMu.Lock()
	if s.ctx.Err() != nil {
		s.pendingMu.Unlock()
		return agentproto.CheckResult{}, s.linkError()
	}
	s.pending[id] = ch
	s.pendingMu.Unlock()
	defer func() {
		s.pendingMu.Lock()
		delete(s.pending, id)
		s.pendingMu.Unlock()
	}()
	if err := s.sendCommand(&agentproto.PanelCommand{Type: agentproto.CommandCheck, Check: &req}); err != nil {
		return agentproto.CheckResult{}, err
	}
	select {
	case out := <-ch:
		if out.err != nil {
			return agentproto.CheckResult{}, out.err
		}
		return out.result, nil
	case <-ctx.Done():
		return agentproto.CheckResult{}, ctx.Err()
	case <-s.ctx.Done():
		return agentproto.CheckResult{}, s.linkError()
	}
}

func (s *agentSession) capabilities() (bool, bool, string) {
	s.helloMu.RLock()
	defer s.helloMu.RUnlock()
	return s.hello.CanCheck, s.hello.XrayAvailable, s.hello.XrayVersion
}

func (a *App) broadcastAgentLink(s *agentSession, connected bool) {
	if s == nil {
		return
	}
	payload := map[string]any{
		"agent_id":     s.agentID,
		"connected":    connected,
		"transport":    s.transport,
		"last_seen":    nowUnix(),
		"protocol":     agentproto.ProtocolVersion,
		"connected_at": s.connectedAt,
	}
	if connected {
		canCheck, xray, xrayVersion := s.capabilities()
		payload["can_check"] = canCheck
		payload["xray_available"] = xray
		payload["xray_version"] = xrayVersion
		s.helloMu.RLock()
		if s.hello.Version != "" {
			payload["version"] = s.hello.Version
		}
		if s.hello.Hostname != "" {
			payload["hostname"] = s.hello.Hostname
		}
		s.helloMu.RUnlock()
	}
	a.hub.broadcast("agent.link", payload)
}

func (a *App) onAgentLinkUp(s *agentSession) {
	a.presenceMu.Lock()
	a.linkSeen[s.agentID] = true
	a.presenceMu.Unlock()
	if err := a.db.TouchAgent(s.agentID, nowUnix()); err != nil {
		a.logFor("agent").Warn("touch agent", "agent", s.agentID, "error", err)
	}
	a.logFor("agent").Info("agent link connected", "agent", s.agentName, "agent_id", s.agentID, "transport", s.transport, "ip", s.ip)
	a.broadcastAgentLink(s, true)
}

func (a *App) onAgentLinkDown(s *agentSession) {
	a.logFor("agent").Info("agent link disconnected", "agent", s.agentName, "agent_id", s.agentID, "transport", s.transport, "reason", s.linkError().Error())
	a.hub.broadcast("agent.link", map[string]any{
		"agent_id": s.agentID, "connected": false, "transport": "",
	})
}

// enrichAgents adds ephemeral link state to persisted agent rows. A gRPC or
// WebSocket session counts as connected immediately; agents still on the old
// REST protocol use the last report age, preserving backward compatibility.
func (a *App) enrichAgents(agents []model.Agent) []model.Agent {
	now := nowUnix()
	after := a.config().AgentOfflineAfterSec
	if after <= 0 {
		after = 120
	}
	for i := range agents {
		agents[i].Connected = false
		agents[i].Transport = ""
		agents[i].Protocol = 0
		agents[i].CanCheck = false
		agents[i].XrayAvailable = false
		agents[i].XrayVersion = ""
		agents[i].ConnectedAt = 0
		if link := a.links.get(agents[i].ID); link != nil {
			agents[i].Connected = true
			agents[i].Transport = link.transport
			agents[i].Protocol = agentproto.ProtocolVersion
			agents[i].ConnectedAt = link.connectedAt
			canCheck, xray, version := link.capabilities()
			agents[i].CanCheck = canCheck
			agents[i].XrayAvailable = xray
			agents[i].XrayVersion = version
			continue
		}
		a.presenceMu.Lock()
		live := a.linkSeen[agents[i].ID]
		restAt := a.restSeen[agents[i].ID]
		a.presenceMu.Unlock()
		if restAt > 0 && now-restAt <= 10 {
			agents[i].Connected = true
			agents[i].Transport = "REST"
			agents[i].Protocol = 2
			continue
		}
		if live {
			// This process has seen a live link for the agent. A missing link
			// means "offline now", not "REST online for the next two minutes".
			continue
		}
		if agents[i].LastSeen > 0 && now-agents[i].LastSeen <= int64(after) {
			agents[i].Connected = true
			agents[i].Transport = "REST"
			agents[i].Protocol = 2
		}
	}
	return agents
}

func agentTokenFromMetadata(md metadata.MD) string {
	for _, key := range []string{"x-agent-token", "authorization"} {
		for _, value := range md.Get(key) {
			value = strings.TrimSpace(value)
			if strings.HasPrefix(strings.ToLower(value), "bearer ") {
				value = strings.TrimSpace(value[7:])
			}
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func (a *App) agentFromToken(token string) (*model.Agent, error) {
	if strings.TrimSpace(token) == "" {
		return nil, status.Error(codes.Unauthenticated, "توکن ایجنت ارسال نشده است")
	}
	ag, err := a.db.GetAgentByToken(token)
	if err != nil {
		return nil, status.Error(codes.Internal, "خطا در بررسی توکن ایجنت")
	}
	if ag == nil {
		return nil, status.Error(codes.Unauthenticated, "توکن ایجنت معتبر نیست")
	}
	return ag, nil
}

// Connect is the gRPC implementation of the live link.
func (a *App) Connect(stream grpc.ServerStream) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	ag, err := a.agentFromToken(agentTokenFromMetadata(md))
	if err != nil {
		a.logFor("agent").Warn("gRPC link rejected", "error", err)
		return err
	}
	ip := ""
	if p, ok := peer.FromContext(stream.Context()); ok && p.Addr != nil {
		ip = hostOnly(p.Addr.String())
	}
	sess := a.newAgentSession(stream.Context(), ag, "gRPC", ip, func(cmd *agentproto.PanelCommand) error {
		return stream.SendMsg(cmd)
	}, nil)
	sess.lastEvent.Store(nowUnix())
	if old := a.links.register(sess); old != nil {
		old.shutdown(errors.New("agent opened a newer link"))
	}
	a.onAgentLinkUp(sess)
	defer func() {
		if a.links.remove(sess) {
			a.onAgentLinkDown(sess)
		}
		sess.shutdown(errors.New("agent link closed"))
	}()

	if err := sess.sendCommand(&agentproto.PanelCommand{
		Type: agentproto.CommandHello, ServerVersion: a.cfg.Version,
		IntervalSec: 1, AgentID: ag.ID, AgentName: ag.Name,
		TestURL: a.probes.ProxyTestURL(),
	}); err != nil {
		return err
	}
	for {
		ev := new(agentproto.AgentEvent)
		if err := stream.RecvMsg(ev); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if a.links.get(ag.ID) != sess {
			return status.Error(codes.Aborted, "agent link was replaced")
		}
		sess.handleEvent(ev)
	}
}

// handleAgentLinkWS is the HTTP/1.1-compatible fallback for reverse proxies
// that do not forward gRPC/h2c.
func (a *App) handleAgentLinkWS(w http.ResponseWriter, r *http.Request) {
	ag, err := a.agentFromToken(agentToken(r))
	if err != nil {
		a.logFor("agent").Warn("WebSocket link rejected", "error", err)
		writeErr(w, http.StatusUnauthorized, "توکن ایجنت معتبر نیست")
		return
	}
	// The HTTP server has a 30s ReadTimeout for REST requests. A WebSocket is
	// long-lived, so clear that deadline before hijacking the connection.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	conn.SetReadLimit(16 << 20)
	sess := a.newAgentSession(r.Context(), ag, "WebSocket", clientIP(r), func(cmd *agentproto.PanelCommand) error {
		writeCtx, writeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer writeCancel()
		return wsjson.Write(writeCtx, conn, cmd)
	}, func() error {
		return conn.Close(websocket.StatusNormalClosure, "link closed")
	})
	sess.lastEvent.Store(nowUnix())
	if old := a.links.register(sess); old != nil {
		old.shutdown(errors.New("agent opened a newer link"))
	}
	a.onAgentLinkUp(sess)
	defer func() {
		if a.links.remove(sess) {
			a.onAgentLinkDown(sess)
		}
		sess.shutdown(errors.New("agent link closed"))
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()

	if err := sess.sendCommand(&agentproto.PanelCommand{
		Type: agentproto.CommandHello, ServerVersion: a.cfg.Version,
		IntervalSec: 1, AgentID: ag.ID, AgentName: ag.Name,
		TestURL: a.probes.ProxyTestURL(),
	}); err != nil {
		return
	}
	for {
		var ev agentproto.AgentEvent
		if err := wsjson.Read(r.Context(), conn, &ev); err != nil {
			return
		}
		if a.links.get(ag.ID) != sess {
			return
		}
		sess.handleEvent(&ev)
	}
}

// runRemoteMonitor executes one monitor on its selected agent.
func (a *App) runRemoteMonitor(ctx context.Context, m model.Monitor) checker.Result {
	ag, err := a.db.GetAgent(m.ProbeAgentID)
	if err != nil || ag == nil {
		return remoteFailure(m.ProbeAgentID, "", m.ProbeAgentName, "سرور تست انتخاب‌شده پیدا نشد")
	}
	name := ag.Name
	if name == "" {
		name = m.ProbeAgentName
	}
	sess := a.links.get(m.ProbeAgentID)
	if sess == nil {
		a.logFor("agent").Warn("remote check without live link", "agent", name, "agent_id", m.ProbeAgentID, "monitor", m.Name)
		return remoteFailure(m.ProbeAgentID, name, m.ProbeAgentName, "اتصال زندهٔ ایجنت برقرار نیست؛ ایجنت را به‌روزرسانی و ری‌استارت کنید")
	}
	testURL := a.probes.ProxyTestURL()
	res, err := sess.runCheck(ctx, agentproto.CheckRequest{
		MonitorID: m.ID, Type: m.Type, Target: m.Target, TimeoutMS: m.Timeout,
		Config: m.Config, TestURL: testURL,
	})
	if err != nil {
		a.logFor("agent").Warn("remote check failed", "agent", name, "agent_id", m.ProbeAgentID, "monitor", m.Name, "error", err)
		return remoteFailure(m.ProbeAgentID, name, m.ProbeAgentName, "اجرای تست روی ایجنت ناموفق بود: "+err.Error())
	}
	out := checker.Result{
		OK: res.OK, LatencyMS: res.LatencyMS, StatusCode: res.StatusCode,
		Error: res.Error, Warn: res.Warn, SSLExpiresAt: res.SSLExpiresAt,
		ProbeFailed: res.ProbeFailed,
	}
	if len(res.Meta) > 0 {
		_ = json.Unmarshal(res.Meta, &out.Meta)
	}
	if out.Meta == nil {
		out.Meta = map[string]any{}
	}
	out.Meta["probe_agent_id"] = ag.ID
	out.Meta["probe_agent_name"] = name
	out.Meta["probe_transport"] = sess.transport
	return out
}

func remoteFailure(agentID int64, agentName, fallbackName, msg string) checker.Result {
	if agentName == "" {
		agentName = fallbackName
	}
	return checker.Result{
		Error:       msg,
		ProbeFailed: true,
		Meta: map[string]any{
			"probe_agent_id":   agentID,
			"probe_agent_name": agentName,
			"probe_infra":      true,
		},
	}
}

func hostOnly(addr string) string {
	host := addr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return strings.Trim(host, "[]")
}
