package common

import (
	"regexp"
	"testing"
)

func TestGeneratePassword_Length(t *testing.T) {
	p := GeneratePassword(16)
	if len(p) != 16 {
		t.Errorf("expected length 16, got %d", len(p))
	}
}

func TestGeneratePassword_Alphanumeric(t *testing.T) {
	p := GeneratePassword(32)
	matched, err := regexp.MatchString(`^[a-zA-Z0-9]+$`, p)
	if err != nil {
		t.Fatalf("regexp error: %v", err)
	}
	if !matched {
		t.Errorf("password %q contains non-alphanumeric characters", p)
	}
}

func TestGeneratePassword_Uniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		p := GeneratePassword(16)
		if _, exists := seen[p]; exists {
			t.Errorf("duplicate password generated: %q", p)
		}
		seen[p] = struct{}{}
	}
}
