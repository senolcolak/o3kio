// +build darwin

package networking

// Darwin (macOS) stub constants for VXLAN
// These match the Linux values but won't be used for actual networking operations
const (
	AF_BRIDGE     = 7    // Linux: unix.AF_BRIDGE
	NUD_PERMANENT = 0x80 // Linux: netlink.NUD_PERMANENT
	NTF_SELF      = 0x02 // Linux: netlink.NTF_SELF
)
