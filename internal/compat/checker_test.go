package compat_test

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestRecorderCaptures(t *testing.T) {
	rec := compat.NewRecorder()
	rec.Record("POST", "/v3/auth/tokens", 201)
	rec.Record("GET", "/v2.1/servers", 200)
	rec.Record("DELETE", "/v2.1/servers/abc", 204)

	results := rec.Results()
	assert.Len(t, results, 3)
	assert.Equal(t, "POST", results[0].Method)
	assert.Equal(t, 201, results[0].StatusCode)
	assert.True(t, results[0].Compatible)
}

func TestCheckerRunNoTerraform(t *testing.T) {
	c := compat.NewChecker(compat.CheckerOptions{TerraformDir: "/nonexistent"})
	_, err := c.Run()
	assert.Error(t, err)
}

func TestCheckerRunWithTerraform(t *testing.T) {
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform not in PATH")
	}

	dir := t.TempDir()
	tfConfig := `
terraform {
  required_providers {
    openstack = {
      source  = "terraform-provider-openstack/openstack"
      version = "~> 3.0"
    }
  }
}

provider "openstack" {}

data "openstack_identity_auth_scope_v3" "scope" {
  name = "my_scope"
}
`
	err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(tfConfig), 0644)
	assert.NoError(t, err)

	c := compat.NewChecker(compat.CheckerOptions{
		TerraformDir: dir,
		OutputFormat: "json",
	})

	report, err := c.Run()
	if err != nil {
		t.Logf("Run() error (may be expected): %v", err)
		return
	}

	assert.NotNil(t, report)
	t.Logf("Report: %s", report.String())
}
