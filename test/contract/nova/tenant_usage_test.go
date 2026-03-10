package nova_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNovaTenantUsage_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNovaClient(t)

	// Get tenant usage via direct HTTP
	url := client.ServiceURL("os-simple-tenant-usage")
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
		TenantUsages []struct {
			TenantID      string  `json:"tenant_id"`
			TotalHours    float64 `json:"total_hours"`
			TotalInstances int    `json:"total_local_gb_usage"`
		} `json:"tenant_usages"`
	}

	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Should have at least one tenant usage entry
	assert.NotEmpty(t, result.TenantUsages)
}
