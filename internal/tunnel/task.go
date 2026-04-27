package tunnel

import (
	"fmt"

	"github.com/google/uuid"
)

// Task type constants for known tunnel task operations.
const (
	TaskCreateVM   = "create_vm"
	TaskDeleteVM   = "delete_vm"
	TaskCreatePort = "create_port"
	TaskDeletePort = "delete_port"
)

// Task represents a unit of work dispatched to a tunnel agent.
type Task struct {
	ID      string
	Type    string
	Payload []byte
}

// Validate checks that the task is well-formed, auto-assigning an ID if absent.
func (t *Task) Validate() error {
	if t.Type == "" {
		return fmt.Errorf("task type is required")
	}
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return nil
}
