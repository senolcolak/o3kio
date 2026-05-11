package tunnel

import (
	"fmt"

	"github.com/google/uuid"
)

// Task type constants for known tunnel task operations.
const (
	TaskCreateVM   = "VM_CREATE"
	TaskDeleteVM   = "VM_DELETE"
	TaskCreatePort = "NET_ADD_PORT"
	TaskDeletePort = "NET_REMOVE_PORT"
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
