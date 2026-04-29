package compat

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const DefaultListenAddr = "127.0.0.1:35357"

type CheckerOptions struct {
	TerraformDir string
	OutputFormat string // "json" or "text"
	ListenAddr   string
}

type Checker struct {
	TerraformDir string
	OutputFormat string
	ListenAddr   string
}

func NewChecker(opts CheckerOptions) *Checker {
	if opts.OutputFormat == "" {
		opts.OutputFormat = "json"
	}
	if opts.ListenAddr == "" {
		opts.ListenAddr = DefaultListenAddr
	}
	return &Checker{
		TerraformDir: opts.TerraformDir,
		OutputFormat: opts.OutputFormat,
		ListenAddr:   opts.ListenAddr,
	}
}

func (c *Checker) Run() (*Report, error) {
	if _, err := exec.LookPath("terraform"); err != nil {
		return nil, fmt.Errorf("terraform not found in PATH: %w", err)
	}
	if c.TerraformDir == "" {
		return nil, fmt.Errorf("TerraformDir must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	srv, err := StartEmbeddedServer(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start embedded server: %w", err)
	}
	defer srv.Shutdown(ctx)

	authURL := fmt.Sprintf("http://%s/v3", srv.Addr())
	baseURL := fmt.Sprintf("http://%s", srv.Addr())
	env := append(os.Environ(),
		"OS_AUTH_URL="+authURL,
		"OS_USERNAME=admin",
		"OS_PASSWORD=secret",
		"OS_PROJECT_NAME=default",
		"OS_USER_DOMAIN_NAME=Default",
		"OS_PROJECT_DOMAIN_NAME=Default",
		"OS_REGION_NAME=RegionOne",
		"OS_IDENTITY_API_VERSION=3",
		"OS_ENDPOINT_TYPE=public",
		"OS_COMPUTE_ENDPOINT_OVERRIDE="+baseURL+"/v2.1/",
		"OS_NETWORK_ENDPOINT_OVERRIDE="+baseURL+"/v2.0/",
		"OS_BLOCKSTORAGE_ENDPOINT_OVERRIDE="+baseURL+"/v3/",
		"OS_IMAGESERVICE_ENDPOINT_OVERRIDE="+baseURL+"/",
	)

	// terraform init
	initCmd := exec.CommandContext(ctx, "terraform", "init", "-no-color", "-input=false")
	initCmd.Dir = c.TerraformDir
	initCmd.Env = env
	if out, err := initCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("terraform init failed: %w\n%s", err, out)
	}

	// terraform plan
	planCmd := exec.CommandContext(ctx, "terraform", "plan", "-no-color", "-input=false")
	planCmd.Dir = c.TerraformDir
	planCmd.Env = env
	planOut, planErr := planCmd.CombinedOutput()

	results := srv.Recorder.Results()
	summary := buildSummary(results)

	report := &Report{
		Compatible:   summary.Incompatible == 0 && planErr == nil,
		OutputFormat: c.OutputFormat,
		Endpoints:    results,
		Summary:      summary,
	}

	if planErr != nil && isProviderError(string(planOut)) {
		report.Compatible = false
	}

	return report, nil
}

func isProviderError(output string) bool {
	return strings.Contains(output, "Error: Provider configuration")
}

func buildSummary(results []EndpointResult) Summary {
	s := Summary{Total: len(results)}
	for _, r := range results {
		if r.Compatible {
			s.Compatible++
		} else {
			s.Incompatible++
		}
	}
	return s
}
