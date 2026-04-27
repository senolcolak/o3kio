package compat

import (
	"encoding/json"
	"fmt"
	"strings"
)

// EndpointResult holds the compat-check result for a single API endpoint.
type EndpointResult struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	Called     bool   `json:"called"`
	StatusCode int    `json:"status_code,omitempty"`
	Compatible bool   `json:"compatible"`
	Error      string `json:"error,omitempty"`
}

// Summary aggregates endpoint counts across a Report.
type Summary struct {
	Total        int `json:"total"`
	Compatible   int `json:"compatible"`
	Incompatible int `json:"incompatible"`
	Uncalled     int `json:"uncalled"`
}

// Report is the output of a compat-check run.
type Report struct {
	Compatible   bool             `json:"compatible"`
	OutputFormat string           `json:"-"`
	Endpoints    []EndpointResult `json:"endpoints"`
	Summary      Summary          `json:"summary"`
}

// String renders the report in the configured OutputFormat ("text" or JSON).
func (r Report) String() string {
	if r.OutputFormat == "text" {
		return r.toText()
	}
	b, _ := json.Marshal(r)
	return string(b)
}

func (r Report) toText() string {
	var sb strings.Builder
	verdict := "COMPATIBLE"
	if !r.Compatible {
		verdict = "INCOMPATIBLE"
	}
	fmt.Fprintf(&sb, "o3k compat-check: %s\n", verdict)
	fmt.Fprintf(&sb, "  Total: %d  Compatible: %d  Incompatible: %d  Uncalled: %d\n\n",
		r.Summary.Total, r.Summary.Compatible, r.Summary.Incompatible, r.Summary.Uncalled)
	for _, ep := range r.Endpoints {
		status := "OK"
		if !ep.Compatible {
			status = "FAIL"
		}
		fmt.Fprintf(&sb, "  [%s] %s %s", status, ep.Method, ep.Path)
		if ep.Error != "" {
			fmt.Fprintf(&sb, " — %s", ep.Error)
		}
		fmt.Fprintln(&sb)
	}
	return sb.String()
}
