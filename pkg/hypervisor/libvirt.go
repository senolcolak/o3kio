package hypervisor

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	libvirt "github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket/dialers"
	"github.com/google/uuid"
)

// VMState represents the state of a virtual machine
type VMState string

const (
	VMStateNoState     VMState = "NOSTATE"
	VMStateRunning     VMState = "RUNNING"
	VMStateBlocked     VMState = "BLOCKED"
	VMStatePaused      VMState = "PAUSED"
	VMStateShutdown    VMState = "SHUTDOWN"
	VMStateShutoff     VMState = "SHUTOFF"
	VMStateCrashed     VMState = "CRASHED"
	VMStatePMSuspended VMState = "PMSUSPENDED"
)

// VMManager manages virtual machine operations
type VMManager struct {
	libvirtURI string
	mode       string // "stub" or "real"
	conn       *libvirt.Libvirt
	mu         sync.Mutex
	stubVMs    map[string]*stubVM // For stub mode
}

// stubVM represents a simulated VM in stub mode
type stubVM struct {
	uuid       string
	xml        string
	state      VMState
	powerState int
	createdAt  time.Time
}

// NewVMManager creates a new VM manager
func NewVMManager(libvirtURI, mode string) (*VMManager, error) {
	mgr := &VMManager{
		libvirtURI: libvirtURI,
		mode:       mode,
		stubVMs:    make(map[string]*stubVM),
	}

	// If real mode, connect to libvirt
	if mode == "real" {
		if err := mgr.connectLibvirt(); err != nil {
			return nil, fmt.Errorf("failed to connect to libvirt: %w", err)
		}
	}

	return mgr, nil
}

// connectLibvirt establishes a connection to libvirt
func (m *VMManager) connectLibvirt() error {
	// Parse URI to get connection type
	// For qemu:///system, connect to /var/run/libvirt/libvirt-sock
	socketPath := "/var/run/libvirt/libvirt-sock"

	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return fmt.Errorf("failed to dial libvirt socket: %w", err)
	}

	l := libvirt.NewWithDialer(dialers.NewAlreadyConnected(conn))
	if err := l.Connect(); err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to connect to libvirt: %w", err)
	}

	m.conn = l
	return nil
}

// CreateVM creates a virtual machine
func (m *VMManager) CreateVM(ctx context.Context, xml string) (string, error) {
	if m.mode == "stub" {
		return m.createVMStub(xml)
	}
	return m.createVMReal(ctx, xml)
}

// createVMStub simulates VM creation
func (m *VMManager) createVMStub(xml string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	vmUUID := uuid.New().String()
	m.stubVMs[vmUUID] = &stubVM{
		uuid:       vmUUID,
		xml:        xml,
		state:      VMStateRunning,
		powerState: 1, // Running
		createdAt:  time.Now(),
	}

	return vmUUID, nil
}

// createVMReal creates a real VM via libvirt
func (m *VMManager) createVMReal(ctx context.Context, xml string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return "", fmt.Errorf("not connected to libvirt")
	}

	// Define the domain
	domain, err := m.conn.DomainDefineXML(xml)
	if err != nil {
		return "", fmt.Errorf("failed to define domain: %w", err)
	}

	// Start the domain
	if err := m.conn.DomainCreate(domain); err != nil {
		// Try to undefine on failure
		_ = m.conn.DomainUndefine(domain)
		return "", fmt.Errorf("failed to start domain: %w", err)
	}

	// Get the UUID string (domain.UUID is already a [16]byte)
	vmUUID := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		domain.UUID[0:4], domain.UUID[4:6], domain.UUID[6:8], domain.UUID[8:10], domain.UUID[10:16])

	return vmUUID, nil
}

// DeleteVM deletes a virtual machine
func (m *VMManager) DeleteVM(ctx context.Context, vmUUID string) error {
	if m.mode == "stub" {
		return m.deleteVMStub(vmUUID)
	}
	return m.deleteVMReal(ctx, vmUUID)
}

// deleteVMStub simulates VM deletion
func (m *VMManager) deleteVMStub(vmUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.stubVMs[vmUUID]; !exists {
		return fmt.Errorf("VM not found: %s", vmUUID)
	}

	delete(m.stubVMs, vmUUID)
	return nil
}

// deleteVMReal deletes a real VM via libvirt
func (m *VMManager) deleteVMReal(ctx context.Context, vmUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return fmt.Errorf("not connected to libvirt")
	}

	// Parse UUID
	uuidBytes, err := parseUUID(vmUUID)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}

	// Lookup domain
	domain, err := m.conn.DomainLookupByUUID(uuidBytes)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	// Get domain state
	state, _, err := m.conn.DomainGetState(domain, 0)
	if err != nil {
		return fmt.Errorf("failed to get domain state: %w", err)
	}

	// Destroy if running
	if state == 1 { // Running
		if err := m.conn.DomainDestroy(domain); err != nil {
			return fmt.Errorf("failed to destroy domain: %w", err)
		}
	}

	// Undefine domain
	if err := m.conn.DomainUndefine(domain); err != nil {
		return fmt.Errorf("failed to undefine domain: %w", err)
	}

	return nil
}

// RebootVM reboots a virtual machine
func (m *VMManager) RebootVM(ctx context.Context, vmUUID string) error {
	if m.mode == "stub" {
		return m.rebootVMStub(vmUUID)
	}
	return m.rebootVMReal(ctx, vmUUID)
}

// rebootVMStub simulates VM reboot
func (m *VMManager) rebootVMStub(vmUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.stubVMs[vmUUID]
	if !exists {
		return fmt.Errorf("VM not found: %s", vmUUID)
	}

	// Simulate reboot by keeping state as running
	vm.state = VMStateRunning
	vm.powerState = 1
	return nil
}

// rebootVMReal reboots a real VM via libvirt
func (m *VMManager) rebootVMReal(ctx context.Context, vmUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return fmt.Errorf("not connected to libvirt")
	}

	uuidBytes, err := parseUUID(vmUUID)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}

	domain, err := m.conn.DomainLookupByUUID(uuidBytes)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	if err := m.conn.DomainReboot(domain, 0); err != nil {
		return fmt.Errorf("failed to reboot domain: %w", err)
	}

	return nil
}

// StopVM stops a virtual machine
func (m *VMManager) StopVM(ctx context.Context, vmUUID string) error {
	if m.mode == "stub" {
		return m.stopVMStub(vmUUID)
	}
	return m.stopVMReal(ctx, vmUUID)
}

// stopVMStub simulates VM stop
func (m *VMManager) stopVMStub(vmUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.stubVMs[vmUUID]
	if !exists {
		return fmt.Errorf("VM not found: %s", vmUUID)
	}

	vm.state = VMStateShutoff
	vm.powerState = 4 // Shutoff
	return nil
}

// stopVMReal stops a real VM via libvirt
func (m *VMManager) stopVMReal(ctx context.Context, vmUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return fmt.Errorf("not connected to libvirt")
	}

	uuidBytes, err := parseUUID(vmUUID)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}

	domain, err := m.conn.DomainLookupByUUID(uuidBytes)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	if err := m.conn.DomainShutdown(domain); err != nil {
		return fmt.Errorf("failed to shutdown domain: %w", err)
	}

	return nil
}

// StartVM starts a virtual machine
func (m *VMManager) StartVM(ctx context.Context, vmUUID string) error {
	if m.mode == "stub" {
		return m.startVMStub(vmUUID)
	}
	return m.startVMReal(ctx, vmUUID)
}

// startVMStub simulates VM start
func (m *VMManager) startVMStub(vmUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.stubVMs[vmUUID]
	if !exists {
		return fmt.Errorf("VM not found: %s", vmUUID)
	}

	vm.state = VMStateRunning
	vm.powerState = 1 // Running
	return nil
}

// startVMReal starts a real VM via libvirt
func (m *VMManager) startVMReal(ctx context.Context, vmUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return fmt.Errorf("not connected to libvirt")
	}

	uuidBytes, err := parseUUID(vmUUID)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}

	domain, err := m.conn.DomainLookupByUUID(uuidBytes)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	if err := m.conn.DomainCreate(domain); err != nil {
		return fmt.Errorf("failed to start domain: %w", err)
	}

	return nil
}

// GetVMState returns the state of a virtual machine
func (m *VMManager) GetVMState(ctx context.Context, vmUUID string) (string, int, error) {
	if m.mode == "stub" {
		return m.getVMStateStub(vmUUID)
	}
	return m.getVMStateReal(ctx, vmUUID)
}

// getVMStateStub returns simulated VM state
func (m *VMManager) getVMStateStub(vmUUID string) (string, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.stubVMs[vmUUID]
	if !exists {
		return string(VMStateNoState), 0, fmt.Errorf("VM not found: %s", vmUUID)
	}

	return string(vm.state), vm.powerState, nil
}

// getVMStateReal returns real VM state via libvirt
func (m *VMManager) getVMStateReal(ctx context.Context, vmUUID string) (string, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return string(VMStateNoState), 0, fmt.Errorf("not connected to libvirt")
	}

	uuidBytes, err := parseUUID(vmUUID)
	if err != nil {
		return string(VMStateNoState), 0, fmt.Errorf("invalid UUID: %w", err)
	}

	domain, err := m.conn.DomainLookupByUUID(uuidBytes)
	if err != nil {
		return string(VMStateNoState), 0, fmt.Errorf("domain not found: %w", err)
	}

	state, _, err := m.conn.DomainGetState(domain, 0)
	if err != nil {
		return string(VMStateNoState), 0, fmt.Errorf("failed to get domain state: %w", err)
	}

	// Convert libvirt state to OpenStack state
	stateStr := libvirtStateToString(int32(state))
	return stateStr, int(state), nil
}

// AttachDevice attaches a device (disk/network) to a VM
func (m *VMManager) AttachDevice(ctx context.Context, vmUUID, deviceXML string) error {
	if m.mode == "stub" {
		return m.attachDeviceStub(vmUUID, deviceXML)
	}
	return m.attachDeviceReal(ctx, vmUUID, deviceXML)
}

// attachDeviceStub simulates device attachment
func (m *VMManager) attachDeviceStub(vmUUID, deviceXML string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.stubVMs[vmUUID]; !exists {
		return fmt.Errorf("VM not found: %s", vmUUID)
	}

	// In stub mode, we just verify the VM exists
	return nil
}

// attachDeviceReal attaches a device to a real VM via libvirt
func (m *VMManager) attachDeviceReal(ctx context.Context, vmUUID, deviceXML string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return fmt.Errorf("not connected to libvirt")
	}

	uuidBytes, err := parseUUID(vmUUID)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}

	domain, err := m.conn.DomainLookupByUUID(uuidBytes)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	// Attach device (libvirt flags: 0 for default, 1 for persistent, 2 for live)
	flags := uint32(3) // Both persistent and live (1 | 2)
	if err := m.conn.DomainAttachDeviceFlags(domain, deviceXML, flags); err != nil {
		return fmt.Errorf("failed to attach device: %w", err)
	}

	return nil
}

// DetachDevice detaches a device from a VM
func (m *VMManager) DetachDevice(ctx context.Context, vmUUID, deviceXML string) error {
	if m.mode == "stub" {
		return m.detachDeviceStub(vmUUID, deviceXML)
	}
	return m.detachDeviceReal(ctx, vmUUID, deviceXML)
}

// detachDeviceStub simulates device detachment
func (m *VMManager) detachDeviceStub(vmUUID, deviceXML string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.stubVMs[vmUUID]; !exists {
		return fmt.Errorf("VM not found: %s", vmUUID)
	}

	// In stub mode, we just verify the VM exists
	return nil
}

// detachDeviceReal detaches a device from a real VM via libvirt
func (m *VMManager) detachDeviceReal(ctx context.Context, vmUUID, deviceXML string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return fmt.Errorf("not connected to libvirt")
	}

	uuidBytes, err := parseUUID(vmUUID)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}

	domain, err := m.conn.DomainLookupByUUID(uuidBytes)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	// Detach device (libvirt flags: 0 for default, 1 for persistent, 2 for live)
	flags := uint32(3) // Both persistent and live (1 | 2)
	if err := m.conn.DomainDetachDeviceFlags(domain, deviceXML, flags); err != nil {
		return fmt.Errorf("failed to detach device: %w", err)
	}

	return nil
}

// SuspendVM suspends a virtual machine (saves RAM to disk)
func (m *VMManager) SuspendVM(ctx context.Context, vmUUID string) error {
	if m.mode == "stub" {
		return m.suspendVMStub(vmUUID)
	}
	return m.suspendVMReal(ctx, vmUUID)
}

// suspendVMStub simulates VM suspend
func (m *VMManager) suspendVMStub(vmUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.stubVMs[vmUUID]
	if !exists {
		return fmt.Errorf("VM not found: %s", vmUUID)
	}

	vm.state = "SUSPENDED"
	vm.powerState = 4 // SUSPENDED
	return nil
}

// suspendVMReal suspends a real VM via libvirt
func (m *VMManager) suspendVMReal(ctx context.Context, vmUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return fmt.Errorf("not connected to libvirt")
	}

	uuidBytes, err := parseUUID(vmUUID)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}

	domain, err := m.conn.DomainLookupByUUID(uuidBytes)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	// Suspend domain (saves to disk)
	if err := m.conn.DomainManagedSave(domain, 0); err != nil {
		return fmt.Errorf("failed to suspend domain: %w", err)
	}

	return nil
}

// ResumeVM resumes a suspended virtual machine
func (m *VMManager) ResumeVM(ctx context.Context, vmUUID string) error {
	if m.mode == "stub" {
		return m.resumeVMStub(vmUUID)
	}
	return m.resumeVMReal(ctx, vmUUID)
}

// resumeVMStub simulates VM resume
func (m *VMManager) resumeVMStub(vmUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.stubVMs[vmUUID]
	if !exists {
		return fmt.Errorf("VM not found: %s", vmUUID)
	}

	vm.state = VMStateRunning
	vm.powerState = 1 // Running
	return nil
}

// resumeVMReal resumes a real VM via libvirt
func (m *VMManager) resumeVMReal(ctx context.Context, vmUUID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return fmt.Errorf("not connected to libvirt")
	}

	uuidBytes, err := parseUUID(vmUUID)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}

	domain, err := m.conn.DomainLookupByUUID(uuidBytes)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	// Resume domain (starts from managed save)
	if err := m.conn.DomainCreate(domain); err != nil {
		return fmt.Errorf("failed to resume domain: %w", err)
	}

	return nil
}

// Close closes the VM manager
func (m *VMManager) Close() {
	if m.conn != nil {
		_ = m.conn.Disconnect()
	}
}

// parseUUID converts a UUID string to bytes
func parseUUID(uuidStr string) (libvirt.UUID, error) {
	var result libvirt.UUID
	parsed, err := uuid.Parse(uuidStr)
	if err != nil {
		return result, err
	}
	copy(result[:], parsed[:])
	return result, nil
}

// libvirtStateToString converts libvirt state to string
func libvirtStateToString(state int32) string {
	switch state {
	case 0:
		return string(VMStateNoState)
	case 1:
		return string(VMStateRunning)
	case 2:
		return string(VMStateBlocked)
	case 3:
		return string(VMStatePaused)
	case 4:
		return string(VMStateShutdown)
	case 5:
		return string(VMStateShutoff)
	case 6:
		return string(VMStateCrashed)
	case 7:
		return string(VMStatePMSuspended)
	default:
		return string(VMStateNoState)
	}
}
