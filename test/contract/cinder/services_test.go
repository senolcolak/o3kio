package cinder_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCinderListServices_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupCinderClient(t)

	// Get services using direct HTTP call
	url := client.ServiceURL("os-services")
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	req.Header.Set("X-Auth-Token", client.TokenID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result struct {
		Services []struct {
			Binary string `json:"binary"`
			Host   string `json:"host"`
			Zone   string `json:"zone"`
			Status string `json:"status"`
			State  string `json:"state"`
		} `json:"services"`
	}

	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Should have at least one service (cinder-volume)
	assert.NotEmpty(t, result.Services)

	if len(result.Services) > 0 {
		svc := result.Services[0]
		assert.NotEmpty(t, svc.Binary)
		assert.NotEmpty(t, svc.Host)
		assert.NotEmpty(t, svc.State)
		assert.NotEmpty(t, svc.Status)
	}
}
