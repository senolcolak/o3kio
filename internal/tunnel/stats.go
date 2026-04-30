package tunnel

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// HostStats holds capacity information about the local host.
type HostStats struct {
	VCPUTotal   int64
	RAMTotalMB  int64
	DiskTotalGB int64
}

// CollectHostStats returns host capacity. In stub mode it returns fixed safe
// values so the scheduler always sees the agent as available.
func CollectHostStats(mode string) HostStats {
	if mode == "stub" {
		return HostStats{
			VCPUTotal:   int64(runtime.NumCPU()),
			RAMTotalMB:  8192,
			DiskTotalGB: 100,
		}
	}
	return HostStats{
		VCPUTotal:   int64(runtime.NumCPU()),
		RAMTotalMB:  readMemTotalMB(),
		DiskTotalGB: 100,
	}
}

// readMemTotalMB reads MemTotal from /proc/meminfo and returns it in MiB.
// Falls back to 8192 on any error (non-Linux or permission issue).
func readMemTotalMB() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 8192
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseInt(fields[1], 10, 64)
				return kb / 1024
			}
		}
	}
	return 8192
}
