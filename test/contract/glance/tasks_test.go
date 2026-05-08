package glance_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGlanceCreateTask_Contract tests POST /v2/tasks
func TestGlanceCreateTask_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Test: Create import task
	task := map[string]interface{}{
		"type": "import",
		"input": map[string]interface{}{
			"import_from":        "http://example.com/image.qcow2",
			"import_from_format": "qcow2",
			"image_properties": map[string]string{
				"name": "imported-image",
			},
		},
	}

	body, _ := json.Marshal(task)
	url := client.ServiceURL("tasks")
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result["id"])
	assert.Equal(t, "import", result["type"])
	assert.Contains(t, []string{"pending", "processing"}, result["status"])
}

// TestGlanceListTasks_Contract tests GET /v2/tasks
func TestGlanceListTasks_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Create a task first
	task := map[string]interface{}{
		"type": "import",
		"input": map[string]interface{}{
			"import_from":        "http://example.com/image.qcow2",
			"import_from_format": "qcow2",
		},
	}
	taskBody, _ := json.Marshal(task)
	createURL := client.ServiceURL("tasks")
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(taskBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(createReq)

	// Test: List tasks
	url := client.ServiceURL("tasks")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Tasks []map[string]interface{} `json:"tasks"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Tasks)
}

// TestGlanceGetTask_Contract tests GET /v2/tasks/:id
func TestGlanceGetTask_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	// Create a task
	task := map[string]interface{}{
		"type": "import",
		"input": map[string]interface{}{
			"import_from": "http://example.com/test.qcow2",
		},
	}
	taskBody, _ := json.Marshal(task)
	createURL := client.ServiceURL("tasks")
	createReq, _ := http.NewRequest("POST", createURL, bytes.NewReader(taskBody))
	createReq.Header.Set("X-Auth-Token", client.TokenID)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()

	createBody, _ := io.ReadAll(createResp.Body)
	t.Logf("Create task status: %d, body: %s", createResp.StatusCode, string(createBody))

	if createResp.StatusCode != http.StatusCreated {
		t.Logf("Create task failed: %d, body: %s", createResp.StatusCode, string(createBody))
		t.FailNow()
	}

	var createResult map[string]interface{}
	json.Unmarshal(createBody, &createResult)
	taskID := createResult["id"].(string)

	// Test: Get task
	url := client.ServiceURL("tasks", taskID)
	t.Logf("GET URL: %s", url)
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Logf("GET response status: %d, body: %s", resp.StatusCode, string(respBody))
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, taskID, result["id"])
	assert.Equal(t, "import", result["type"])
}

// TestGlanceListStores_Contract tests GET /v2/stores
func TestGlanceListStores_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupGlanceClient(t)

	url := client.ServiceURL("stores")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Stores []map[string]interface{} `json:"stores"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	// Should have at least one store
	assert.NotEmpty(t, result.Stores)
}
