package glance

import (
	"fmt"
	"regexp"
)

// SCS-0102-v1 image metadata properties.
//
// See: https://docs.scs.community/standards/scs-0102-v1-image-metadata
//
// O3K accepts these properties on image create / update and round-trips them
// through GetImage. The standard distinguishes Mandatory, Recommended, and
// Optional properties — we don't enforce that mandatory properties are
// *present* (that's a federation-level policy decision), but when a property
// IS supplied we validate its shape so an SCS-conformance test will not see
// invalid values stored against a property name it knows.

// scsMandatoryProps lists the SCS-0102 mandatory property names. Used by the
// scs-alignment doc and as the canonical reference; not used to reject create
// requests that omit them (see comment above).
var scsMandatoryProps = []string{
	"architecture",
	"min_disk",
	"min_ram",
	"os_version",
	"os_distro",
	"hw_disk_bus",
	"replace_frequency",
	"provided_until",
	"uuid_validity",
	"image_source",
	"image_description",
	"image_build_date",
	"image_original_user",
}

// scsReplaceFrequencyValues is the closed enum from SCS-0102 §replace_frequency.
var scsReplaceFrequencyValues = map[string]struct{}{
	"yearly": {}, "quarterly": {}, "monthly": {}, "weekly": {},
	"daily": {}, "critical_bug": {}, "never": {},
}

// scsOSPurposeValues is the closed enum from SCS-0102 §os_purpose.
var scsOSPurposeValues = map[string]struct{}{
	"generic": {}, "minimal": {}, "k8snode": {},
	"gpu": {}, "network": {}, "custom": {},
}

// scsHashAlgoValues is the closed enum from SCS-0102 §os_hash_algo.
var scsHashAlgoValues = map[string]struct{}{
	"sha256": {}, "sha512": {},
}

// dateOnlyRE matches YYYY-MM-DD.
var dateOnlyRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// dateTimeRE matches YYYY-MM-DD hh:mm[:ss], used by image_build_date.
var dateTimeRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}(:\d{2})?$`)

// lastNRE matches the "last-N" form for uuid_validity.
var lastNRE = regexp.MustCompile(`^last-\d+$`)

// validateSCSProperty checks a single property name/value pair against
// SCS-0102 rules. It returns nil if the property is unknown to SCS (we don't
// reject custom properties — Glance is a metadata bag) or if the value
// conforms. It returns an error describing the violation otherwise.
//
// Only constrained-vocabulary fields are validated. Free-text fields like
// `os_distro` or `image_description` are accepted as-is because the SCS
// standard defers their values to OpenStack documentation, which is itself
// open-ended.
func validateSCSProperty(key string, value any) error {
	switch key {
	case "replace_frequency":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("replace_frequency must be a string")
		}
		if _, ok := scsReplaceFrequencyValues[s]; !ok {
			return fmt.Errorf("replace_frequency must be one of yearly|quarterly|monthly|weekly|daily|critical_bug|never, got %q", s)
		}
	case "os_purpose":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("os_purpose must be a string")
		}
		if _, ok := scsOSPurposeValues[s]; !ok {
			return fmt.Errorf("os_purpose must be one of generic|minimal|k8snode|gpu|network|custom, got %q", s)
		}
	case "os_hash_algo":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("os_hash_algo must be a string")
		}
		if _, ok := scsHashAlgoValues[s]; !ok {
			return fmt.Errorf("os_hash_algo must be one of sha256|sha512, got %q", s)
		}
	case "provided_until", "maintained_until":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", key)
		}
		if s == "none" || s == "notice" {
			return nil
		}
		if !dateOnlyRE.MatchString(s) {
			return fmt.Errorf("%s must be YYYY-MM-DD or one of none|notice, got %q", key, s)
		}
	case "uuid_validity":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("uuid_validity must be a string")
		}
		if s == "none" || s == "notice" || s == "forever" {
			return nil
		}
		if dateOnlyRE.MatchString(s) || lastNRE.MatchString(s) {
			return nil
		}
		return fmt.Errorf("uuid_validity must be YYYY-MM-DD, last-N, or one of none|notice|forever, got %q", s)
	case "image_build_date":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("image_build_date must be a string")
		}
		if !dateOnlyRE.MatchString(s) && !dateTimeRE.MatchString(s) {
			return fmt.Errorf("image_build_date must be YYYY-MM-DD or YYYY-MM-DD hh:mm[:ss], got %q", s)
		}
	case "license_included", "license_required",
		"subscription_included", "subscription_required",
		"os_secure_boot", "hw_mem_encryption", "hw_pmu",
		"hw_vif_multiqueue_enabled":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", key)
		}
	case "hotfix_hours", "hw_video_ram":
		// JSON numbers decode as float64 — accept any numeric.
		switch value.(type) {
		case float64, int, int64:
		default:
			return fmt.Errorf("%s must be a number", key)
		}
	}
	return nil
}

// validateSCSProperties validates every property in the bag. License-pair
// constraint (`license_included` and `license_required` cannot both be true)
// is enforced here because it's cross-field.
func validateSCSProperties(props map[string]any) error {
	for k, v := range props {
		if err := validateSCSProperty(k, v); err != nil {
			return err
		}
	}
	li, _ := props["license_included"].(bool)
	lr, _ := props["license_required"].(bool)
	if li && lr {
		return fmt.Errorf("license_included and license_required cannot both be true")
	}
	return nil
}
