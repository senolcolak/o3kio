package neutron_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create test port in database (bypasses bridge issues)
func createDBPort(t *testing.T, name string) string {
	portID := uuid.New().String()
	cmd := fmt.Sprintf(`docker exec o3k-postgres psql -U lightstack -d lightstack -c "INSERT INTO ports (id, name, network_id, project_id, mac_address, admin_state_up, status, fixed_ips, created_at, updated_at) SELECT '%s', '%s', id, (SELECT id FROM projects WHERE name='default' LIMIT 1), '00:00:00:00:00:00', true, 'ACTIVE', '[]', NOW(), NOW() FROM networks LIMIT 1 ON CONFLICT DO NOTHING;"`, portID, name)
	exec.Command("sh", "-c", cmd).Run()
	return portID
}

// TestTrunkBasicCRUD tests trunk CRUD operations
func TestTrunkBasicCRUD(t *testing.T) {
	skipIfO3KNotRunning(t)
	client := setupNeutronClient(t)

	portID := createDBPort(t, "test-trunk-port")

	// CREATE
	createReq := map[string]interface{}{
		"trunk": map[string]interface{}{
			"name":    "test-trunk",
			"port_id": portID,
		},
	}
	body, _ := json.Marshal(createReq)
	req, _ := http.NewRequest("POST", client.ServiceURL("trunks"), bytes.NewReader(body))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var createResult struct {
		Trunk map[string]interface{} `json:"trunk"`
	}
	json.Unmarshal(respBody, &createResult)
	trunkID := createResult.Trunk["id"].(string)
	assert.NotEmpty(t, trunkID)

	// LIST
	req, _ = http.NewRequest("GET", client.ServiceURL("trunks"), nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// GET
	req, _ = http.NewRequest("GET", client.ServiceURL("trunks", trunkID), nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// UPDATE
	updateReq := map[string]interface{}{
		"trunk": map[string]interface{}{
			"name": "updated-trunk",
		},
	}
	body, _ = json.Marshal(updateReq)
	req, _ = http.NewRequest("PUT", client.ServiceURL("trunks", trunkID), bytes.NewReader(body))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// DELETE
	req, _ = http.NewRequest("DELETE", client.ServiceURL("trunks", trunkID), nil)
	req.Header.Set("X-Auth-Token", client.TokenID)
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Cleanup
	exec.Command("sh", "-c", fmt.Sprintf(`docker exec o3k-postgres psql -U lightstack -d lightstack -c "DELETE FROM ports WHERE id='%s';"`, portID)).Run()
}

// TestTrunkSubports tests adding subports
func TestTrunkSubports(t *testing.T) {
	skipIfO3KNotRunning(t)
	client := setupNeutronClient(t)

	parentPort := createDBPort(t, "trunk-parent")
	subPort := createDBPort(t, "trunk-sub")

	// Create trunk
	createReq := map[string]interface{}{
		"trunk": map[string]interface{}{
			"name":    "test-trunk-subport",
			"port_id": parentPort,
		},
	}
	body, _ := json.Marshal(createReq)
	req, _ := http.NewRequest("POST", client.ServiceURL("trunks"), bytes.NewReader(body))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var createResult struct {
		Trunk map[string]interface{} `json:"trunk"`
	}
	json.Unmarshal(respBody, &createResult)
	trunkID := createResult.Trunk["id"].(string)

	// Add subport
	addReq := map[string]interface{}{
		"sub_ports": []map[string]interface{}{
			{
				"port_id":           subPort,
				"segmentation_type": "vlan",
				"segmentation_id":   100,
			},
		},
	}
	body, _ = json.Marshal(addReq)
	req, _ = http.NewRequest("PUT", client.ServiceURL("trunks", trunkID, "add_subports"), bytes.NewReader(body))
	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Cleanup
	http.NewRequest("DELETE", client.ServiceURL("trunks", trunkID), nil)
	exec.Command("sh", "-c", fmt.Sprintf(`docker exec o3k-postgres psql -U lightstack -d lightstack -c "DELETE FROM ports WHERE id IN ('%s', '%s');"`, parentPort, subPort)).Run()
}
