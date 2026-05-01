package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cobaltcore-dev/o3k/pkg/hypervisor"
)

// Executor dispatches task types to libvirt VM lifecycle operations.
type Executor struct {
	vmManager *hypervisor.VMManager
}

// NewExecutor creates an Executor backed by a VMManager in the given mode
// ("stub" or "real").
func NewExecutor(mode string) *Executor {
	mgr, err := hypervisor.NewVMManager("qemu:///system", mode)
	if err != nil {
		// In stub mode this never fails; in real mode the caller should
		// handle nil gracefully — panic here is appropriate for wiring errors.
		panic(fmt.Sprintf("executor: failed to create VMManager: %v", err))
	}
	return &Executor{vmManager: mgr}
}

// Execute routes the task to the appropriate VM lifecycle handler.
func (e *Executor) Execute(ctx context.Context, taskType string, payload []byte) ([]byte, error) {
	switch strings.ToUpper(taskType) {
	case "VM_CREATE":
		return e.vmCreate(ctx, payload)
	case "VM_DELETE":
		return e.vmDelete(ctx, payload)
	case "VM_START":
		return e.vmStart(ctx, payload)
	case "VM_STOP":
		return e.vmStop(ctx, payload)
	case "VM_REBOOT":
		return e.vmReboot(ctx, payload)
	case "NET_ENSURE_NAMESPACE":
		return e.netEnsureNamespace(ctx, payload)
	case "NET_ADD_PORT":
		return e.netAddPort(ctx, payload)
	case "NET_REMOVE_PORT":
		return e.netRemovePort(ctx, payload)
	default:
		return nil, fmt.Errorf("unknown task type: %s", taskType)
	}
}

// vmCreatePayload is the JSON payload for VM_CREATE tasks.
type vmCreatePayload struct {
	InstanceID     string `json:"instance_id"`
	FlavorID       string `json:"flavor_id"`
	ImageLocalPath string `json:"image_local_path"`
	VCPU           int    `json:"vcpu"`
	RAMMB          int    `json:"ram_mb"`
	DiskGB         int    `json:"disk_gb"`
	NetworkID      string `json:"network_id"`
	MACAddress     string `json:"mac_address"`
}

// vmStatePayload is the JSON payload for VM state-change tasks (delete/start/stop/reboot).
type vmStatePayload struct {
	InstanceID string `json:"instance_id"`
	DomainName string `json:"domain_name"`
}

// vmCreateResult is the JSON result returned after a successful VM_CREATE.
type vmCreateResult struct {
	InstanceID string `json:"instance_id"`
	DomainUUID string `json:"domain_uuid"`
}

func (e *Executor) vmCreate(ctx context.Context, payload []byte) ([]byte, error) {
	var p vmCreatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("vm_create: invalid payload: %w", err)
	}

	spec := hypervisor.VMSpec{
		UUID:      p.InstanceID,
		Name:      fmt.Sprintf("instance-%s", p.InstanceID),
		VCPUs:     p.VCPU,
		MemoryMB:  p.RAMMB,
		DiskGB:    p.DiskGB,
		ImagePath: p.ImageLocalPath,
	}
	if p.NetworkID != "" || p.MACAddress != "" {
		spec.Networks = []hypervisor.NetworkConfig{
			{
				NetworkID:  p.NetworkID,
				MACAddress: p.MACAddress,
			},
		}
	}

	xml := hypervisor.GenerateVMXML(spec)
	domainUUID, err := e.vmManager.CreateVM(ctx, xml)
	if err != nil {
		return nil, fmt.Errorf("vm_create: CreateVM failed: %w", err)
	}

	out, err := json.Marshal(vmCreateResult{
		InstanceID: p.InstanceID,
		DomainUUID: domainUUID,
	})
	if err != nil {
		return nil, fmt.Errorf("vm_create: marshal result: %w", err)
	}
	return out, nil
}

func (e *Executor) vmDelete(ctx context.Context, payload []byte) ([]byte, error) {
	var p vmStatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("vm_delete: invalid payload: %w", err)
	}

	err := e.vmManager.DeleteVM(ctx, p.InstanceID)
	if err != nil {
		// Treat "VM not found" as an already-deleted condition — idempotent.
		if strings.Contains(err.Error(), "VM not found") || strings.Contains(err.Error(), "domain not found") {
			return json.Marshal(map[string]string{"status": "not_found"})
		}
		return nil, fmt.Errorf("vm_delete: DeleteVM failed: %w", err)
	}

	return json.Marshal(map[string]string{"status": "deleted"})
}

func (e *Executor) vmStart(ctx context.Context, payload []byte) ([]byte, error) {
	var p vmStatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("vm_start: invalid payload: %w", err)
	}

	if err := e.vmManager.StartVM(ctx, p.InstanceID); err != nil {
		return nil, fmt.Errorf("vm_start: StartVM failed: %w", err)
	}

	return json.Marshal(map[string]string{"status": "started"})
}

func (e *Executor) vmStop(ctx context.Context, payload []byte) ([]byte, error) {
	var p vmStatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("vm_stop: invalid payload: %w", err)
	}

	if err := e.vmManager.StopVM(ctx, p.InstanceID); err != nil {
		return nil, fmt.Errorf("vm_stop: StopVM failed: %w", err)
	}

	return json.Marshal(map[string]string{"status": "stopped"})
}

func (e *Executor) vmReboot(ctx context.Context, payload []byte) ([]byte, error) {
	var p vmStatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("vm_reboot: invalid payload: %w", err)
	}

	if err := e.vmManager.RebootVM(ctx, p.InstanceID); err != nil {
		return nil, fmt.Errorf("vm_reboot: RebootVM failed: %w", err)
	}

	return json.Marshal(map[string]string{"status": "rebooted"})
}

// netNamespacePayload is the JSON payload for NET_ENSURE_NAMESPACE tasks.
type netNamespacePayload struct {
	NetworkID string `json:"network_id"`
	ProjectID string `json:"project_id"`
}

// netPortPayload is the JSON payload for NET_ADD_PORT and NET_REMOVE_PORT tasks.
type netPortPayload struct {
	PortID     string `json:"port_id"`
	NetworkID  string `json:"network_id"`
	MACAddress string `json:"mac_address"`
	IPAddress  string `json:"ip_address"`
	InstanceID string `json:"instance_id"`
}

func (e *Executor) netEnsureNamespace(_ context.Context, payload []byte) ([]byte, error) {
	var p netNamespacePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("net_ensure_namespace: invalid payload: %w", err)
	}

	return json.Marshal(map[string]string{
		"network_id": p.NetworkID,
		"status":     "ensured",
	})
}

func (e *Executor) netAddPort(_ context.Context, payload []byte) ([]byte, error) {
	var p netPortPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("net_add_port: invalid payload: %w", err)
	}

	return json.Marshal(map[string]string{
		"port_id": p.PortID,
		"status":  "added",
	})
}

func (e *Executor) netRemovePort(_ context.Context, payload []byte) ([]byte, error) {
	var p netPortPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("net_remove_port: invalid payload: %w", err)
	}

	return json.Marshal(map[string]string{
		"port_id": p.PortID,
		"status":  "removed",
	})
}
