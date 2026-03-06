package hypervisor

import (
	"context"
	"testing"
)

func TestNewVMManagerStubMode(t *testing.T) {
	manager, err := NewVMManager("test:///default", "stub")
	if err != nil {
		t.Fatalf("Failed to create VM manager: %v", err)
	}

	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	if manager.libvirtURI != "test:///default" {
		t.Errorf("Expected URI test:///default, got %s", manager.libvirtURI)
	}

	if manager.mode != "stub" {
		t.Errorf("Expected mode stub, got %s", manager.mode)
	}
}

func TestCreateVMStub(t *testing.T) {
	manager, _ := NewVMManager("test:///default", "stub")
	ctx := context.Background()

	// In stub mode, CreateVM should succeed and return a UUID
	vmUUID, err := manager.CreateVM(ctx, "<domain></domain>")
	if err != nil {
		t.Errorf("Expected no error from stub implementation, got: %v", err)
	}

	if vmUUID == "" {
		t.Error("Expected non-empty UUID from stub implementation")
	}

	// Verify VM is tracked in stub mode
	_, powerState, err := manager.GetVMState(ctx, vmUUID)
	if err != nil {
		t.Errorf("Failed to get VM state: %v", err)
	}

	if powerState != 1 {
		t.Errorf("Expected power state 1 (running), got %d", powerState)
	}
}

func TestDeleteVMStub(t *testing.T) {
	manager, _ := NewVMManager("test:///default", "stub")
	ctx := context.Background()

	// Create a VM first
	vmUUID, _ := manager.CreateVM(ctx, "<domain></domain>")

	// Delete should succeed
	err := manager.DeleteVM(ctx, vmUUID)
	if err != nil {
		t.Errorf("Expected no error from stub delete, got: %v", err)
	}

	// Verify VM is gone
	_, _, err = manager.GetVMState(ctx, vmUUID)
	if err == nil {
		t.Error("Expected error when getting state of deleted VM")
	}
}

func TestRebootVMStub(t *testing.T) {
	manager, _ := NewVMManager("test:///default", "stub")
	ctx := context.Background()

	// Create a VM first
	vmUUID, _ := manager.CreateVM(ctx, "<domain></domain>")

	// Reboot should succeed
	err := manager.RebootVM(ctx, vmUUID)
	if err != nil {
		t.Errorf("Expected no error from stub reboot, got: %v", err)
	}
}

func TestStopVMStub(t *testing.T) {
	manager, _ := NewVMManager("test:///default", "stub")
	ctx := context.Background()

	// Create a VM first
	vmUUID, _ := manager.CreateVM(ctx, "<domain></domain>")

	// Stop should succeed
	err := manager.StopVM(ctx, vmUUID)
	if err != nil {
		t.Errorf("Expected no error from stub stop, got: %v", err)
	}

	// Verify state changed
	_, powerState, _ := manager.GetVMState(ctx, vmUUID)
	if powerState != 4 {
		t.Errorf("Expected power state 4 (shutoff), got %d", powerState)
	}
}

func TestStartVMStub(t *testing.T) {
	manager, _ := NewVMManager("test:///default", "stub")
	ctx := context.Background()

	// Create a VM first
	vmUUID, _ := manager.CreateVM(ctx, "<domain></domain>")

	// Stop it
	manager.StopVM(ctx, vmUUID)

	// Start should succeed
	err := manager.StartVM(ctx, vmUUID)
	if err != nil {
		t.Errorf("Expected no error from stub start, got: %v", err)
	}

	// Verify state changed back to running
	_, powerState, _ := manager.GetVMState(ctx, vmUUID)
	if powerState != 1 {
		t.Errorf("Expected power state 1 (running), got %d", powerState)
	}
}

func TestGetVMStateStub(t *testing.T) {
	manager, _ := NewVMManager("test:///default", "stub")
	ctx := context.Background()

	// Create a VM
	vmUUID, _ := manager.CreateVM(ctx, "<domain></domain>")

	// Get state should succeed
	state, powerState, err := manager.GetVMState(ctx, vmUUID)
	if err != nil {
		t.Errorf("Expected no error from stub state, got: %v", err)
	}

	if state != "RUNNING" {
		t.Errorf("Expected state RUNNING, got %s", state)
	}

	if powerState != 1 {
		t.Errorf("Expected power state 1, got %d", powerState)
	}
}

func TestVMLifecycleStub(t *testing.T) {
	manager, _ := NewVMManager("test:///default", "stub")
	ctx := context.Background()

	// Create
	vmUUID, err := manager.CreateVM(ctx, "<domain></domain>")
	if err != nil {
		t.Fatalf("Failed to create VM: %v", err)
	}

	// Verify running
	state, _, _ := manager.GetVMState(ctx, vmUUID)
	if state != "RUNNING" {
		t.Errorf("Expected RUNNING state after create, got %s", state)
	}

	// Stop
	manager.StopVM(ctx, vmUUID)
	state, _, _ = manager.GetVMState(ctx, vmUUID)
	if state != "SHUTOFF" {
		t.Errorf("Expected SHUTOFF state after stop, got %s", state)
	}

	// Start
	manager.StartVM(ctx, vmUUID)
	state, _, _ = manager.GetVMState(ctx, vmUUID)
	if state != "RUNNING" {
		t.Errorf("Expected RUNNING state after start, got %s", state)
	}

	// Reboot
	manager.RebootVM(ctx, vmUUID)
	state, _, _ = manager.GetVMState(ctx, vmUUID)
	if state != "RUNNING" {
		t.Errorf("Expected RUNNING state after reboot, got %s", state)
	}

	// Delete
	err = manager.DeleteVM(ctx, vmUUID)
	if err != nil {
		t.Fatalf("Failed to delete VM: %v", err)
	}

	// Verify gone
	_, _, err = manager.GetVMState(ctx, vmUUID)
	if err == nil {
		t.Error("Expected error when getting state of deleted VM")
	}
}
