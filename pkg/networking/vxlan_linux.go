// +build linux

package networking

import (
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// Linux-specific network constants
const (
	AF_BRIDGE     = unix.AF_BRIDGE
	NUD_PERMANENT = netlink.NUD_PERMANENT
	NTF_SELF      = netlink.NTF_SELF
)
