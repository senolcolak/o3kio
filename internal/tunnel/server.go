package tunnel

import (
	"fmt"
	"net"
	"sync"

	pb "github.com/cobaltcore-dev/o3k/proto/tunnel"
	"google.golang.org/grpc"
)

// AgentInfo holds metadata and the active stream for a connected tunnel agent.
type AgentInfo struct {
	NodeID   string
	Hostname string
	TunnelIP string
	Stream   grpc.BidiStreamingServer[pb.AgentMessage, pb.ServerMessage]
}

// Hub tracks connected tunnel agents and provides agent selection.
type Hub struct {
	pb.UnimplementedTunnelHubServer
	tokenSecret string
	mu          sync.RWMutex
	agents      map[string]*AgentInfo
}

// NewHub creates a new Hub with the given JWT token secret.
func NewHub(tokenSecret string) *Hub {
	return &Hub{
		tokenSecret: tokenSecret,
		agents:      make(map[string]*AgentInfo),
	}
}

// RegisterAgent adds or updates an agent entry in the hub.
func (h *Hub) RegisterAgent(info AgentInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.agents[info.NodeID] = &info
}

// RemoveAgent removes the agent with the given nodeID from the hub.
func (h *Hub) RemoveAgent(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.agents, nodeID)
}

// ListAgents returns a snapshot of all currently registered agents.
func (h *Hub) ListAgents() []AgentInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]AgentInfo, 0, len(h.agents))
	for _, a := range h.agents {
		out = append(out, *a)
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

// VerifyJoin reports whether the given tokenHash is valid for nodeID.
// When tokenSecret is empty the hub is in open enrollment mode and all joins are accepted.
func (h *Hub) VerifyJoin(nodeID, tokenHash string) bool {
	if h.tokenSecret == "" {
		return true
	}
	return VerifyTokenHash(h.tokenSecret, nodeID, tokenHash)
}

// ListenAndServe starts the gRPC server on addr and blocks until it exits.
func (h *Hub) ListenAndServe(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("tunnel listen %s: %w", addr, err)
	}
	s := grpc.NewServer()
	pb.RegisterTunnelHubServer(s, h)
	fmt.Printf("TunnelHub listening on %s\n", addr)
	return s.Serve(lis)
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
	h.RegisterAgent(AgentInfo{
		NodeID:   join.GetNodeId(),
		Hostname: join.GetHostname(),
		TunnelIP: join.GetTunnelIp(),
		Stream:   stream,
	})
	defer h.RemoveAgent(join.GetNodeId())

	for {
		if _, err := stream.Recv(); err != nil {
			return err
		}
	}
}
