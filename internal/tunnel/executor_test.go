package tunnel_test

import (
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/tunnel"
	"github.com/stretchr/testify/assert"
)

func TestExecutorVMCreate(t *testing.T) {
	exec := tunnel.NewExecutor("stub")

	payload := []byte(`{"instance_id":"inst-12345678","flavor_id":"m1.small","image_local_path":"/images/cirros.qcow2","vcpu":1,"ram_mb":512,"disk_gb":10}`)
	result, err := exec.Execute(t.Context(), "VM_CREATE", payload)
	assert.NoError(t, err)
	assert.Contains(t, string(result), "instance_id")
}

func TestExecutorVMDelete(t *testing.T) {
	exec := tunnel.NewExecutor("stub")

	payload := []byte(`{"instance_id":"inst-12345678","domain_name":"instance-inst1234"}`)
	result, err := exec.Execute(t.Context(), "VM_DELETE", payload)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestExecutorUnknownType(t *testing.T) {
	exec := tunnel.NewExecutor("stub")

	_, err := exec.Execute(t.Context(), "UNKNOWN_TYPE", []byte(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown task type")
}

func TestExecutorNetEnsureNamespace(t *testing.T) {
	exec := tunnel.NewExecutor("stub")
	payload := []byte(`{"network_id":"net-12345678","project_id":"proj-1"}`)
	result, err := exec.Execute(t.Context(), "NET_ENSURE_NAMESPACE", payload)
	assert.NoError(t, err)
	assert.Contains(t, string(result), "network_id")
}

func TestExecutorNetAddPort(t *testing.T) {
	exec := tunnel.NewExecutor("stub")
	payload := []byte(`{"port_id":"port-1","network_id":"net-12345678","mac_address":"fa:16:3e:aa:bb:cc","ip_address":"192.168.1.10","instance_id":"inst-1"}`)
	result, err := exec.Execute(t.Context(), "NET_ADD_PORT", payload)
	assert.NoError(t, err)
	assert.Contains(t, string(result), "port_id")
}

func TestExecutorNetRemovePort(t *testing.T) {
	exec := tunnel.NewExecutor("stub")
	payload := []byte(`{"port_id":"port-1","network_id":"net-12345678","mac_address":"fa:16:3e:aa:bb:cc","ip_address":"192.168.1.10","instance_id":"inst-1"}`)
	result, err := exec.Execute(t.Context(), "NET_REMOVE_PORT", payload)
	assert.NoError(t, err)
	assert.Contains(t, string(result), "removed")
}
