package tunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/cobaltcore-dev/o3k/proto/tunnel"
)

// AgentClient manages the persistent gRPC stream from an agent node to the hub.
type AgentClient struct {
	serverAddr string
	nodeID     string
	tokenHash  string
	tlsConfig  *tls.Config
	executor   *Executor
}

// NewAgentClient creates an AgentClient that will connect to serverAddr.
func NewAgentClient(serverAddr, nodeID, tokenHash string) *AgentClient {
	return &AgentClient{
		serverAddr: serverAddr,
		nodeID:     nodeID,
		tokenHash:  tokenHash,
	}
}

// NewAgentClientWithExecutor creates an AgentClient with a real Executor for the given mode.
func NewAgentClientWithExecutor(serverAddr, nodeID, tokenHash, mode string) *AgentClient {
	return &AgentClient{
		serverAddr: serverAddr,
		nodeID:     nodeID,
		tokenHash:  tokenHash,
		executor:   NewExecutor(mode),
	}
}

// SetTLSConfig configures mTLS for the client. When nil (the default), the
// client dials without TLS, which is suitable for development/stub mode.
func (c *AgentClient) SetTLSConfig(cfg *tls.Config) {
	c.tlsConfig = cfg
}

// Connect runs the agent stream loop, reconnecting on error until ctx is done.
func (c *AgentClient) Connect(ctx context.Context) error {
	for {
		if err := c.runStream(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fmt.Printf("tunnel: stream error (%v) — reconnecting in 5s\n", err)
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (c *AgentClient) runStream(ctx context.Context) error {
	var dialOpts []grpc.DialOption
	if c.tlsConfig != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(c.tlsConfig)))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(c.serverAddr, dialOpts...)
	if err != nil {
		return fmt.Errorf("dial %s: %w", c.serverAddr, err)
	}
	defer conn.Close()

	client := pb.NewTunnelHubClient(conn)
	stream, err := client.AgentStream(ctx)
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}

	var sendMu sync.Mutex
	safeSend := func(msg *pb.AgentMessage) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return stream.Send(msg)
	}

	if err := safeSend(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Join{
			Join: &pb.JoinMsg{
				NodeId:    c.nodeID,
				Hostname:  "agent-hostname",
				TokenHash: c.tokenHash,
			},
		},
	}); err != nil {
		return fmt.Errorf("send join: %w", err)
	}

	// Start periodic heartbeat so the scheduler's stats_updated_at filter sees
	// this agent as active (requires a report within the last 30 seconds).
	// Use a child context so the heartbeat goroutine is cancelled when the stream
	// connection drops, preventing one leaked goroutine per reconnect attempt.
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		stats := CollectHostStats("stub") // TODO: use c.mode when available
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				_ = safeSend(&pb.AgentMessage{
					Payload: &pb.AgentMessage_Heartbeat{
						Heartbeat: &pb.HeartbeatMsg{
							TimestampUnix: time.Now().Unix(),
							VcpusFree:     int32(stats.VCPUTotal),
							MemoryFreeMb:  stats.RAMTotalMB,
						},
					},
				})
			}
		}
	}()

	for {
		msg, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("recv: %w", err)
		}
		if task := msg.GetTask(); task != nil {
			go c.executeTask(ctx, safeSend, task)
		}
	}
}

func (c *AgentClient) executeTask(ctx context.Context, safeSend func(*pb.AgentMessage) error, task *pb.TaskMsg) {
	var result []byte
	var errMsg string

	if c.executor != nil {
		var err error
		result, err = c.executor.Execute(ctx, task.GetTaskType(), task.GetPayload())
		if err != nil {
			errMsg = err.Error()
			result = nil
		}
	} else {
		fmt.Printf("agent: no executor, stub response for task %s\n", task.GetTaskId())
	}

	_ = safeSend(&pb.AgentMessage{
		Payload: &pb.AgentMessage_TaskResult{
			TaskResult: &pb.TaskResultMsg{
				TaskId:  task.GetTaskId(),
				Success: errMsg == "",
				Error:   errMsg,
				Result:  result,
			},
		},
	})
}
