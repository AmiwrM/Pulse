// Package agentproto defines the persistent agent <-> panel link protocol.
//
// The link is a bidirectional gRPC stream. JSON is used as the wire codec so
// the protocol stays readable and the project does not need generated
// protobuf code. A WebSocket transport carries the exact same messages when
// HTTP/2 (gRPC) is not available through a reverse proxy.
package agentproto

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"

	"pulse/internal/model"
)

const (
	// ProtocolVersion is incremented whenever the link message contract changes.
	ProtocolVersion = 3

	// ServiceName is the fully qualified gRPC service name.
	ServiceName = "pulse.agent.v1.AgentLink"
	// MethodConnect is the bidirectional stream method path.
	MethodConnect = "/" + ServiceName + "/Connect"
	// ContentSubtype selects the JSON codec on the gRPC wire.
	ContentSubtype = "json"
)

// Agent hello event.
const (
	EventHello  = "hello"
	EventReport = "report"
	EventCheck  = "check_result"
)

// Panel commands.
const (
	CommandHello = "hello_ack"
	CommandCheck = "run_check"
	CommandError = "error"
)

// Hello is sent once when an agent opens a live link.
type Hello struct {
	Version       string `json:"version"`
	Hostname      string `json:"hostname,omitempty"`
	OS            string `json:"os,omitempty"`
	Arch          string `json:"arch,omitempty"`
	CanCheck      bool   `json:"can_check"`
	XrayAvailable bool   `json:"xray_available"`
	XrayVersion   string `json:"xray_version,omitempty"`
	IntervalSec   int    `json:"interval_sec,omitempty"`
}

// AgentEvent is the only message sent by an agent over the link.
type AgentEvent struct {
	Type   string             `json:"type"`
	SentAt int64              `json:"sent_at,omitempty"`
	Hello  *Hello             `json:"hello,omitempty"`
	Report *model.AgentReport `json:"report,omitempty"`
	Check  *CheckResult       `json:"check,omitempty"`
}

// CheckRequest asks an agent to execute one monitor probe locally.
type CheckRequest struct {
	ID        string          `json:"id"`
	MonitorID int64           `json:"monitor_id"`
	Type      string          `json:"type"`
	Target    string          `json:"target"`
	TimeoutMS int             `json:"timeout_ms"`
	TestURL   string          `json:"test_url,omitempty"`
	Config    json.RawMessage `json:"config,omitempty"`
}

// CheckResult is an agent-side probe result. Meta mirrors checker.Result.Meta.
type CheckResult struct {
	ID           string          `json:"id"`
	MonitorID    int64           `json:"monitor_id"`
	OK           bool            `json:"ok"`
	LatencyMS    int64           `json:"latency_ms"`
	StatusCode   int             `json:"status_code,omitempty"`
	Error        string          `json:"error,omitempty"`
	Warn         string          `json:"warn,omitempty"`
	SSLExpiresAt int64           `json:"ssl_expires_at,omitempty"`
	ProbeFailed  bool            `json:"probe_failed,omitempty"`
	Meta         json.RawMessage `json:"meta,omitempty"`
}

// PanelCommand is the only message sent by the panel over the link.
type PanelCommand struct {
	Type          string        `json:"type"`
	ServerVersion string        `json:"server_version,omitempty"`
	IntervalSec   int           `json:"interval_sec,omitempty"`
	AgentID       int64         `json:"agent_id,omitempty"`
	AgentName     string        `json:"agent_name,omitempty"`
	Message       string        `json:"message,omitempty"`
	TestURL       string        `json:"test_url,omitempty"`
	Check         *CheckRequest `json:"check,omitempty"`
}

// AgentLinkServer is implemented by the panel.
type AgentLinkServer interface {
	Connect(grpc.ServerStream) error
}

// ServiceDesc exposes the link as a bidirectional gRPC stream without
// generated protobuf code.
var ServiceDesc = grpc.ServiceDesc{
	ServiceName: ServiceName,
	HandlerType: (*AgentLinkServer)(nil),
	Streams: []grpc.StreamDesc{{
		StreamName:    "Connect",
		Handler:       connectHandler,
		ServerStreams: true,
		ClientStreams: true,
	}},
	Metadata: "agentlink.json",
}

func connectHandler(srv any, stream grpc.ServerStream) error {
	return srv.(AgentLinkServer).Connect(stream)
}

// RegisterServer registers the link service on a gRPC server.
func RegisterServer(s grpc.ServiceRegistrar, srv AgentLinkServer) {
	s.RegisterService(&ServiceDesc, srv)
}

// ConnectClient is the agent side of the stream.
type ConnectClient interface {
	Send(*AgentEvent) error
	Recv() (*PanelCommand, error)
	CloseSend() error
}

// AgentLinkClient creates the bidirectional stream.
type AgentLinkClient interface {
	Connect(ctx context.Context, opts ...grpc.CallOption) (ConnectClient, error)
}

type linkClient struct {
	cc grpc.ClientConnInterface
}

// NewClient returns a link client for an existing gRPC connection.
func NewClient(cc grpc.ClientConnInterface) AgentLinkClient {
	return &linkClient{cc: cc}
}

func (c *linkClient) Connect(ctx context.Context, opts ...grpc.CallOption) (ConnectClient, error) {
	opts = append(opts, grpc.CallContentSubtype(ContentSubtype))
	stream, err := c.cc.NewStream(ctx, &ServiceDesc.Streams[0], MethodConnect, opts...)
	if err != nil {
		return nil, err
	}
	return &connectClient{ClientStream: stream}, nil
}

type connectClient struct {
	grpc.ClientStream
}

func (c *connectClient) Send(ev *AgentEvent) error {
	return c.ClientStream.SendMsg(ev)
}

func (c *connectClient) Recv() (*PanelCommand, error) {
	cmd := new(PanelCommand)
	if err := c.ClientStream.RecvMsg(cmd); err != nil {
		return nil, err
	}
	return cmd, nil
}

type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func (jsonCodec) Name() string { return ContentSubtype }

func init() {
	encoding.RegisterCodec(jsonCodec{})
}
