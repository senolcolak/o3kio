package glance

import (
	"strings"
	"testing"
)

func TestValidateSCSProperty(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   any
		wantErr string // substring of expected error; empty = expect success
	}{
		// replace_frequency enum
		{"replace_frequency monthly", "replace_frequency", "monthly", ""},
		{"replace_frequency never", "replace_frequency", "never", ""},
		{"replace_frequency invalid", "replace_frequency", "annually", "must be one of"},
		{"replace_frequency wrong type", "replace_frequency", 5, "must be a string"},

		// os_purpose enum
		{"os_purpose generic", "os_purpose", "generic", ""},
		{"os_purpose k8snode", "os_purpose", "k8snode", ""},
		{"os_purpose invalid", "os_purpose", "database", "must be one of"},

		// os_hash_algo enum
		{"os_hash_algo sha256", "os_hash_algo", "sha256", ""},
		{"os_hash_algo md5", "os_hash_algo", "md5", "must be one of"},

		// provided_until: date or sentinels
		{"provided_until date", "provided_until", "2027-12-31", ""},
		{"provided_until none", "provided_until", "none", ""},
		{"provided_until notice", "provided_until", "notice", ""},
		{"provided_until garbage", "provided_until", "soon", "must be YYYY-MM-DD"},
		{"provided_until bad format", "provided_until", "31-12-2027", "must be YYYY-MM-DD"},

		// uuid_validity: date, last-N, or sentinels
		{"uuid_validity date", "uuid_validity", "2030-01-01", ""},
		{"uuid_validity last-3", "uuid_validity", "last-3", ""},
		{"uuid_validity forever", "uuid_validity", "forever", ""},
		{"uuid_validity none", "uuid_validity", "none", ""},
		{"uuid_validity invalid", "uuid_validity", "always", "must be YYYY-MM-DD"},

		// image_build_date: date or datetime
		{"image_build_date date only", "image_build_date", "2026-01-15", ""},
		{"image_build_date with time", "image_build_date", "2026-01-15 14:30", ""},
		{"image_build_date with seconds", "image_build_date", "2026-01-15 14:30:00", ""},
		{"image_build_date invalid", "image_build_date", "yesterday", "must be YYYY-MM-DD"},

		// boolean fields
		{"license_included true", "license_included", true, ""},
		{"license_required false", "license_required", false, ""},
		{"os_secure_boot bool", "os_secure_boot", true, ""},
		{"license_included not bool", "license_included", "yes", "must be a boolean"},

		// numeric fields
		{"hotfix_hours int", "hotfix_hours", 24, ""},
		{"hotfix_hours float (JSON)", "hotfix_hours", float64(24), ""},
		{"hotfix_hours string", "hotfix_hours", "24", "must be a number"},
		{"hw_video_ram int", "hw_video_ram", 16, ""},

		// unknown SCS property — accepted (bag is open)
		{"custom property", "x_my_custom", "anything", ""},
		{"os_distro free text", "os_distro", "ubuntu", ""},
		{"architecture free text", "architecture", "x86_64", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSCSProperty(tt.key, tt.value)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error to contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestValidateSCSPropertiesLicensePair(t *testing.T) {
	// SCS-0102: license_included and license_required cannot both be true.
	err := validateSCSProperties(map[string]any{
		"license_included": true,
		"license_required": true,
	})
	if err == nil {
		t.Fatal("expected error for license_included + license_required both true")
	}
	if !strings.Contains(err.Error(), "cannot both be true") {
		t.Fatalf("expected license-pair error, got %q", err.Error())
	}

	// One of them being true is fine.
	if err := validateSCSProperties(map[string]any{
		"license_included": true,
		"license_required": false,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSCSPropertiesEmptyBagAllowed(t *testing.T) {
	// SCS validation is policy on values, not presence. An empty bag passes.
	if err := validateSCSProperties(map[string]any{}); err != nil {
		t.Fatalf("empty bag should be valid, got %v", err)
	}
	if err := validateSCSProperties(nil); err != nil {
		t.Fatalf("nil bag should be valid, got %v", err)
	}
}

func TestValidateSCSPropertiesFullExample(t *testing.T) {
	// A realistic SCS-conformant property set should validate cleanly.
	props := map[string]any{
		"architecture":        "x86_64",
		"os_distro":           "ubuntu",
		"os_version":          "24.04",
		"hw_disk_bus":         "scsi",
		"hw_scsi_model":       "virtio-scsi",
		"replace_frequency":   "monthly",
		"provided_until":      "2029-04-30",
		"uuid_validity":       "last-2",
		"image_source":        "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
		"image_description":   "https://cloud-images.ubuntu.com/noble/current/",
		"image_build_date":    "2026-01-15",
		"image_original_user": "ubuntu",
		"os_purpose":          "generic",
		"os_hash_algo":        "sha512",
		"os_hash_value":       "abc123",
		"hotfix_hours":        float64(72),
		"license_included":    false,
		"license_required":    false,
	}
	if err := validateSCSProperties(props); err != nil {
		t.Fatalf("realistic SCS property set failed validation: %v", err)
	}
}
