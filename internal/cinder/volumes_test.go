package cinder

import (
	"strings"
	"testing"
)

func TestJoinConditions(t *testing.T) {
	tests := []struct {
		name       string
		conditions []string
		want       string
	}{
		{"empty", nil, ""},
		{"single", []string{"a = $1"}, "a = $1"},
		{"two", []string{"a = $1", "b = $2"}, "a = $1 AND b = $2"},
		{"three", []string{"x", "y", "z"}, "x AND y AND z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinConditions(tt.conditions)
			if got != tt.want {
				t.Errorf("joinConditions(%v) = %q, want %q", tt.conditions, got, tt.want)
			}
		})
	}
}

func TestBuildVolumeFilterConditions_Placeholders(t *testing.T) {
	// Verify that placeholder indices increment correctly regardless of which
	// filters are applied, and that the number of returned conditions matches
	// the number of extra args appended.
	tests := []struct {
		name          string
		queryParams   map[string]string
		startArgIdx   int
		wantCondCount int
	}{
		{
			name:          "no filters",
			queryParams:   map[string]string{},
			startArgIdx:   2,
			wantCondCount: 0,
		},
		{
			name:          "status only",
			queryParams:   map[string]string{"status": "available"},
			startArgIdx:   2,
			wantCondCount: 1,
		},
		{
			name:          "all filters",
			queryParams:   map[string]string{"status": "available", "name": "vol1", "bootable": "true", "availability_zone": "nova"},
			startArgIdx:   2,
			wantCondCount: 4,
		},
		{
			name:          "bootable false",
			queryParams:   map[string]string{"bootable": "false"},
			startArgIdx:   3,
			wantCondCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newFakeGinContext(tt.queryParams)
			initialArgs := make([]interface{}, tt.startArgIdx-1)

			extra, resultArgs, nextArgIdx := buildVolumeFilterConditions(c, initialArgs, tt.startArgIdx)

			if len(extra) != tt.wantCondCount {
				t.Errorf("got %d conditions, want %d", len(extra), tt.wantCondCount)
			}
			addedArgs := len(resultArgs) - len(initialArgs)
			if addedArgs != tt.wantCondCount {
				t.Errorf("got %d extra args, want %d", addedArgs, tt.wantCondCount)
			}
			wantNextIdx := tt.startArgIdx + tt.wantCondCount
			if nextArgIdx != wantNextIdx {
				t.Errorf("nextArgIdx = %d, want %d", nextArgIdx, wantNextIdx)
			}

			// Each condition must reference the correct placeholder
			for i, cond := range extra {
				expectedPlaceholder := "$" + itoa(tt.startArgIdx+i)
				if !strings.Contains(cond, expectedPlaceholder) {
					t.Errorf("condition %q does not contain placeholder %s", cond, expectedPlaceholder)
				}
			}
		})
	}
}

// itoa is a tiny helper to avoid importing strconv just for this test.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
