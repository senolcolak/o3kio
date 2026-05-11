package tunnel

import (
	"crypto/tls"
	"fmt"
	"net"
	"sync"

	pb "github.com/cobaltcore-dev/o3k/proto/tunnel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// ResultMsg carries the outcome of a task dispatched to a tunnel agent.
type ResultMsg struct {
	TaskID  string
	Success bool
	Error   string
	Result  []byte
}

// AgentInfo holds metadata and the active stream for a connected tunnel agent.
type AgentInfo struct {
	NodeID   string
	Hostname string
	TunnelIP string
	Stream   grpc.BidiStreamingServer[pb.AgentMessage, pb.ServerMessage]
	sendMu   sync.Mutex
}

// SafeSend sends a message on the agent's stream under a mutex to prevent concurrent writes.
func (a *AgentInfo) SafeSend(msg *pb.ServerMessage) error {
	a.sendMu.Lock()
	defer a.sendMu.Unlock()
	return a.Stream.Send(msg)
}

// SendTask sends a Task to this agent via its gRPC stream.
func (a *AgentInfo) SendTask(task Task) error {
	msg := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Task{
			Task: &pb.TaskMsg{
				TaskId:   task.ID,
				TaskType: task.Type,
				Payload:  task.Payload,
			},
		},
	}
	if err := a.SafeSend(msg); err != nil {
		return fmt.Errorf("send task to agent %s: %w", a.NodeID, err)
	}
	return nil
}

// Hub tracks connected tunnel agents and provides agent selection.
type Hub struct {
	pb.UnimplementedTunnelHubServer
	tokenSecret string
	tlsConfig   *tls.Config
	mu          sync.RWMutex
	agents      map[string]*AgentInfo
	inflight    map[string]int
	maxInflight int
	resultChs   map[string]chan ResultMsg
	resultMu    sync.Mutex
}

// NewHub creates a new Hub with the given JWT token secret.
// If tokenSecret is empty a warning is printed — all agent joins will be rejected
// until a non-empty secret is configured.
func NewHub(tokenSecret string) *Hub {
	if tokenSecret == "" {
		fmt.Println("WARNING: tunnel hub has no token_secret configured — all agent joins will be rejected")
	}
	return &Hub{
		tokenSecret: tokenSecret,
		agents:      make(map[string]*AgentInfo),
		inflight:    make(map[string]int),
		maxInflight: 1,
		resultChs:   make(map[string]chan ResultMsg),
	}
}

// TryAcquireInflight increments the inflight counter for nodeID and returns true.
// Returns false without modifying the counter if the node is already at maxInflight.
func (h *Hub) TryAcquireInflight(nodeID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.inflight[nodeID] >= h.maxInflight {
		return false
	}
	h.inflight[nodeID]++
	return true
}

// ReleaseInflight decrements the inflight counter for nodeID.
func (h *Hub) ReleaseInflight(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.inflight[nodeID] > 0 {
		h.inflight[nodeID]--
	}
}

// RegisterAgent adds or updates an agent entry in the hub.
func (h *Hub) RegisterAgent(info *AgentInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.agents[info.NodeID] = info
}

// RemoveAgent removes the agent with the given nodeID from the hub.
func (h *Hub) RemoveAgent(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.agents, nodeID)
}

// ListAgents returns all currently registered agents.
func (h *Hub) ListAgents() []*AgentInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*AgentInfo, 0, len(h.agents))
	for _, a := range h.agents {
		out = append(out, a)
	}
	return out
}

// PickAgent returns any one registered agent, or nil if none are available.
func (h *Hub) PickAgent() *AgentInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, a := range h.agents {
		return a
	}
	return nil
}

// GetAgent returns the agent with the given nodeID, or nil if not connected.
func (h *Hub) GetAgent(nodeID string) *AgentInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.agents[nodeID]
}

// VerifyJoin reports whether the given tokenHash is valid for nodeID.
// When tokenSecret is empty all joins are rejected — an unconfigured secret is
// not equivalent to open enrollment.
func (h *Hub) VerifyJoin(nodeID, tokenHash string) bool {
	if h.tokenSecret == "" {
		return false
	}
	return VerifyTokenHash(h.tokenSecret, nodeID, tokenHash)
}

// SetTLSConfig configures the Hub to use TLS when ListenAndServe is called.
// When cfg is nil the server starts without TLS (plain gRPC).
func (h *Hub) SetTLSConfig(cfg *tls.Config) {
	h.tlsConfig = cfg
}

// ListenAndServe starts the gRPC server on addr and blocks until it exits.
// When a TLS config has been set via SetTLSConfig the server uses mutual TLS.
func (h *Hub) ListenAndServe(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("tunnel listen %s: %w", addr, err)
	}

	var opts []grpc.ServerOption
	if h.tlsConfig != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(h.tlsConfig)))
	}

	s := grpc.NewServer(opts...)
	pb.RegisterTunnelHubServer(s, h)
	fmt.Printf("TunnelHub listening on %s (tls=%v)\n", addr, h.tlsConfig != nil)
	return s.Serve(lis)
}

// RegisterResultChan creates and registers a buffered channel for the given taskID.
// The caller must receive from the returned channel to collect the result.
func (h *Hub) RegisterResultChan(taskID string) chan ResultMsg {
	h.resultMu.Lock()
	defer h.resultMu.Unlock()
	ch := make(chan ResultMsg, 1)
	h.resultChs[taskID] = ch
	return ch
}

// UnregisterResultChan removes the result channel for taskID without delivering a result.
// Call this when the waiter (e.g. an RPC context) is cancelled so the map entry does not leak.
func (h *Hub) UnregisterResultChan(taskID string) {
	h.resultMu.Lock()
	defer h.resultMu.Unlock()
	delete(h.resultChs, taskID)
}

// DeliverResult routes a ResultMsg to the channel registered for its TaskID.
// The entry is removed after delivery; a second call for the same TaskID is a no-op.
func (h *Hub) DeliverResult(msg ResultMsg) {
	h.resultMu.Lock()
	ch, ok := h.resultChs[msg.TaskID]
	if ok {
		delete(h.resultChs, msg.TaskID)
	}
	h.resultMu.Unlock()
	if ok {
		select {
		case ch <- msg:
		default:
		}
	}
}

// AgentStream handles a bidirectional gRPC stream from a tunnel agent.
func (h *Hub) AgentStream(stream grpc.BidiStreamingServer[pb.AgentMessage, pb.ServerMessage]) error {
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	join := msg.GetJoin()
	if join == nil {
		return fmt.Errorf("first message must be JoinMsg")
	}
	if !h.VerifyJoin(join.GetNodeId(), join.GetTokenHash()) {
		return fmt.Errorf("invalid join token for node %s", join.GetNodeId())
	}
	h.RegisterAgent(&AgentInfo{
		NodeID:   join.GetNodeId(),
		Hostname: join.GetHostname(),
		TunnelIP: join.GetTunnelIp(),
		Stream:   stream,
	})
	defer h.RemoveAgent(join.GetNodeId())

	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		if tr := msg.GetTaskResult(); tr != nil {
			h.DeliverResult(ResultMsg{
				TaskID:  tr.GetTaskId(),
				Success: tr.GetSuccess(),
				Error:   tr.GetError(),
				Result:  tr.GetResult(),
			})
			h.ReleaseInflight(join.GetNodeId())
		}
	}
}
