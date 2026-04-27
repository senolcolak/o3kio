package compat

import "fmt"

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

// Stubs so cmd/compat-check compiles (real impl in Tasks 2 & 3)
type Report struct {
	Compatible   bool
	OutputFormat string
}

func (r *Report) String() string {
	if r.Compatible {
		return `{"compatible":true}`
	}
	return `{"compatible":false}`
}

func (c *Checker) Run() (*Report, error) {
	return nil, fmt.Errorf("not yet implemented")
}
