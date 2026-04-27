package tunnel

import (
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
