package compat_test

import (
	"testing"

	"github.com/cobaltcore-dev/o3k/internal/compat"
	"github.com/stretchr/testify/assert"
)

func TestNewCheckerDefaults(t *testing.T) {
	c := compat.NewChecker(compat.CheckerOptions{})
	assert.Equal(t, "json", c.OutputFormat)
	assert.Equal(t, compat.DefaultListenAddr, c.ListenAddr)
}

func TestReportJSON(t *testing.T) {
	r := compat.Report{
		Compatible: true,
		Endpoints: []compat.EndpointResult{
			{Method: "POST", Path: "/v3/auth/tokens", Called: true, StatusCode: 201, Compatible: true},
			{Method: "GET", Path: "/v2.1/servers", Called: true, StatusCode: 200, Compatible: true},
		},
		Summary: compat.Summary{Total: 2, Compatible: 2, Incompatible: 0},
	}
	out := r.String()
	assert.Contains(t, out, `"compatible":true`)
	assert.Contains(t, out, `"total":2`)
}

func TestReportText(t *testing.T) {
	r := compat.Report{
		Compatible:   false,
		OutputFormat: "text",
		Endpoints: []compat.EndpointResult{
			{Method: "DELETE", Path: "/v2.1/servers/:id", Called: true, StatusCode: 404, Compatible: false,
				Error: "unexpected 404, expected 204"},
		},
		Summary: compat.Summary{Total: 1, Compatible: 0, Incompatible: 1},
	}
	out := r.String()
	assert.Contains(t, out, "INCOMPATIBLE")
	assert.Contains(t, out, "DELETE /v2.1/servers/:id")
}
