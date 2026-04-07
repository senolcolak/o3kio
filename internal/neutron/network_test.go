package neutron

import (
	"regexp"
	"strconv"
	"testing"
)

func TestGenerateMAC(t *testing.T) {
	t.Run("format validation", func(t *testing.T) {
		mac := generateMAC()
		pattern := regexp.MustCompile(`^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)
		if !pattern.MatchString(mac) {
			t.Errorf("MAC address %q does not match expected format XX:XX:XX:XX:XX:XX", mac)
		}
	})

	t.Run("local bit set and multicast cleared", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			mac := generateMAC()
			first, err := strconv.ParseUint(mac[0:2], 16, 8)
			if err != nil {
				t.Fatalf("failed to parse first octet of MAC %q: %v", mac, err)
			}
			if first&0x02 == 0 {
				t.Errorf("MAC %q: local bit not set in first octet 0x%02x", mac, first)
			}
			if first&0x01 != 0 {
				t.Errorf("MAC %q: multicast bit set in first octet 0x%02x", mac, first)
			}
		}
	})

	t.Run("uniqueness over 100 iterations", func(t *testing.T) {
		seen := make(map[string]struct{}, 100)
		for i := 0; i < 100; i++ {
			mac := generateMAC()
			if _, exists := seen[mac]; exists {
				t.Errorf("duplicate MAC address generated: %q", mac)
			}
			seen[mac] = struct{}{}
		}
	})
}
