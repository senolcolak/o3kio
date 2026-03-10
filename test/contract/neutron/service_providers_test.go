package neutron_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeutronServiceProvidersList_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// List service providers using direct HTTP
	url := client.ServiceURL("service-providers")
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
		ServiceProviders []struct {
			Name         string `json:"name"`
			ServiceType  string `json:"service_type"`
			Default      bool   `json:"default"`
		} `json:"service_providers"`
	}

	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Should have at least one service provider
	assert.NotEmpty(t, result.ServiceProviders)
}
