package compat

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
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

	ctx := context.Background()
	srv, err := StartEmbeddedServer(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start embedded server: %w", err)
	}
	defer srv.Shutdown(ctx)

	cmd := exec.CommandContext(ctx, "terraform", "plan", "-no-color")
	cmd.Dir = c.TerraformDir
	cmd.Env = append(cmd.Environ(),
		fmt.Sprintf("OS_AUTH_URL=http://%s/v3", srv.Addr()),
		"OS_USERNAME=admin",
		"OS_PASSWORD=secret",
		"OS_PROJECT_NAME=default",
		"OS_REGION_NAME=RegionOne",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := string(out)
		if isProviderError(outStr) {
			return nil, fmt.Errorf("terraform plan provider error: %s", outStr)
		}
		if len(outStr) == 0 || strings.Contains(outStr, "no such file or directory") || strings.Contains(outStr, "no configuration files") {
			return nil, fmt.Errorf("terraform plan failed: %w: %s", err, outStr)
		}
	}

	results := srv.Recorder.Results()
	summary := buildSummary(results)
	return &Report{
		Compatible:   summary.Incompatible == 0,
		OutputFormat: c.OutputFormat,
		Endpoints:    results,
		Summary:      summary,
	}, nil
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
