package tunnel

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/cobaltcore-dev/o3k/proto/tunnel"
)

// AgentClient manages the persistent gRPC stream from an agent node to the hub.
type AgentClient struct {
	serverAddr string
	nodeID     string
	tokenHash  string
}

// NewAgentClient creates an AgentClient that will connect to serverAddr.
func NewAgentClient(serverAddr, nodeID, tokenHash string) *AgentClient {
	return &AgentClient{
		serverAddr: serverAddr,
		nodeID:     nodeID,
		tokenHash:  tokenHash,
	}
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
	conn, err := grpc.NewClient(c.serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("dial %s: %w", c.serverAddr, err)
	}
	defer conn.Close()

	client := pb.NewTunnelHubClient(conn)
	stream, err := client.AgentStream(ctx)
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}

	if err := stream.Send(&pb.AgentMessage{
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

	for {
		msg, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("recv: %w", err)
		}
		if task := msg.GetTask(); task != nil {
			go c.executeTask(ctx, stream, task)
		}
	}
}

func (c *AgentClient) executeTask(ctx context.Context, stream pb.TunnelHub_AgentStreamClient, task *pb.TaskMsg) {
	fmt.Printf("agent: received task %s type=%s\n", task.GetTaskId(), task.GetTaskType())
	_ = stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_TaskResult{
			TaskResult: &pb.TaskResultMsg{
				TaskId:  task.GetTaskId(),
				Success: true,
			},
		},
	})
}
