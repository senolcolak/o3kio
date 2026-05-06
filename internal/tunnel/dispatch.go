package tunnel

import (
	"fmt"

	pb "github.com/cobaltcore-dev/o3k/proto/tunnel"
)

// Dispatcher bridges API handlers to the Hub, sending tasks to available agents.
type Dispatcher struct {
	hub *Hub
}

// NewDispatcher creates a Dispatcher backed by the given Hub.
func NewDispatcher(hub *Hub) *Dispatcher {
	return &Dispatcher{hub: hub}
}

// Dispatch validates the task and sends it to an available agent via its gRPC stream.
func (d *Dispatcher) Dispatch(task Task) error {
	if err := task.Validate(); err != nil {
		return fmt.Errorf("invalid task: %w", err)
	}

	agent := d.hub.PickAgent()
	if agent == nil {
		return fmt.Errorf("no agents connected")
	}

	if agent.Stream == nil {
		return fmt.Errorf("agent %s has no active stream", agent.NodeID)
	}

	msg := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Task{
			Task: &pb.TaskMsg{
				TaskId:   task.ID,
				TaskType: task.Type,
				Payload:  task.Payload,
			},
		},
	}
	if err := agent.SafeSend(msg); err != nil {
		return fmt.Errorf("failed to send task to agent %s: %w", agent.NodeID, err)
	}
	return nil
}
