package neutron_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeutronAvailabilityZonesList_Contract(t *testing.T) {
	skipIfO3KNotRunning(t)

	client := setupNeutronClient(t)

	// List availability zones using direct HTTP
	url := client.ServiceURL("availability_zones")
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
		AvailabilityZones []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"availability_zones"`
	}

	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Should have at least one zone
	assert.NotEmpty(t, result.AvailabilityZones)

	// Verify zone structure
	if len(result.AvailabilityZones) > 0 {
		zone := result.AvailabilityZones[0]
		assert.NotEmpty(t, zone.Name)
		assert.NotEmpty(t, zone.State)
	}
}
