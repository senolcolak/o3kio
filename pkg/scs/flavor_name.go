// Package scs implements parsing and validation for the Sovereign Cloud Stack
// flavor naming standard SCS-0100-v3.
//
// A flavor name has the form
//
//	SCS-<vCPUs><cpu-type>[i]-<RAM_GiB>[u][o][-[M×]<disk_GB>[<disk-type>]]
//
// followed by optional underscore-prefixed extension fields (hypervisor,
// nested-virt, CPU arch, GPU, infiniband). The mandatory prefix and the
// extensions we surface are documented in the package types below.
package scs

import (
	"fmt"
	"strconv"
	"strings"
)

// CPUType is the scheduling guarantee on the vCPU.
type CPUType string

const (
	CPUTypeCrowded         CPUType = "crowded-core"   // L: heavily oversubscribed
	CPUTypeShared          CPUType = "shared-core"    // V: shared vCPU (default)
	CPUTypeDedicatedThread CPUType = "dedicated-thread" // T: dedicated SMT thread
	CPUTypeDedicatedCore   CPUType = "dedicated-core"   // C: dedicated physical core
)

// DiskType is the storage medium for the optional pre-attached root disk.
type DiskType string

const (
	DiskTypeUnspecified DiskType = ""
	DiskTypeNetwork     DiskType = "network" // n
	DiskTypeHDD         DiskType = "hdd"     // h
	DiskTypeSSD         DiskType = "ssd"     // s
	DiskTypeNVMe        DiskType = "nvme"    // p
)

// FlavorName is the parsed form of an SCS-0100-v3 flavor name.
type FlavorName struct {
	VCPUs             int
	CPUType           CPUType
	CPUInsecure       bool // `i` suffix on cpu-type letter

	RAMGiB            float64
	RAMNoECC          bool // `u` suffix on RAM
	RAMOversubscribed bool // `o` suffix on RAM

	HasDisk   bool
	DiskCount int // 1 unless an `M×N` multiplier is present
	DiskGB    int
	DiskType  DiskType

	Hypervisor string // from optional _kvm/_xen/_chy/_vmw/_hyv/_bms extension
}

// ParseFlavorName parses an SCS-0100-v3 flavor name. It rejects anything that
// doesn't begin with the mandatory `SCS-` prefix, and returns a descriptive
// error pointing at the offending token for malformed names.
func ParseFlavorName(name string) (FlavorName, error) {
	var fn FlavorName

	if name == "" {
		return fn, fmt.Errorf("empty flavor name")
	}
	if !strings.HasPrefix(name, "SCS-") {
		return fn, fmt.Errorf("missing SCS- prefix: %q", name)
	}
	rest := strings.TrimPrefix(name, "SCS-")
	if rest == "" {
		return fn, fmt.Errorf("missing vCPU segment after SCS- prefix")
	}

	// Split off underscore-prefixed extensions before slicing on '-', so that
	// extension fields with their own dashes (none today, but be defensive)
	// don't confuse the mandatory-prefix splitter.
	var extensions string
	if i := strings.Index(rest, "_"); i >= 0 {
		extensions = rest[i:]
		rest = rest[:i]
	}

	segs := strings.Split(rest, "-")
	if len(segs) < 2 {
		return fn, fmt.Errorf("flavor name needs at least vCPU and RAM segments: %q", name)
	}

	// Segment 1: <vCPUs><cpu-type>[i]
	if err := parseCPUSegment(segs[0], &fn); err != nil {
		return fn, err
	}

	// Segment 2: <RAM_GiB>[u][o]
	if err := parseRAMSegment(segs[1], &fn); err != nil {
		return fn, err
	}

	// Segment 3 (optional): [M×]<disk_GB>[<disk-type>]
	if len(segs) >= 3 {
		if err := parseDiskSegment(segs[2], &fn); err != nil {
			return fn, err
		}
	}
	if len(segs) > 3 {
		return fn, fmt.Errorf("unexpected extra segments in flavor name: %q", name)
	}

	if extensions != "" {
		if err := parseExtensions(extensions, &fn); err != nil {
			return fn, err
		}
	}

	return fn, nil
}

// parseCPUSegment parses the leading `<N><letter>[i]` token.
func parseCPUSegment(seg string, fn *FlavorName) error {
	if seg == "" {
		return fmt.Errorf("empty vCPU segment")
	}

	// Trim trailing `i` (insecure) before the letter.
	insecure := false
	if strings.HasSuffix(seg, "i") && len(seg) >= 3 {
		insecure = true
		seg = seg[:len(seg)-1]
	}

	// Last char is the cpu-type letter.
	letter := seg[len(seg)-1]
	digits := seg[:len(seg)-1]
	if digits == "" {
		return fmt.Errorf("missing vCPU count before CPU-type letter %q", string(letter))
	}

	n, err := strconv.Atoi(digits)
	if err != nil {
		return fmt.Errorf("invalid vCPU count %q: %w", digits, err)
	}
	if n <= 0 {
		return fmt.Errorf("vCPU count must be > 0, got %d", n)
	}

	var cpuType CPUType
	switch letter {
	case 'L':
		cpuType = CPUTypeCrowded
	case 'V':
		cpuType = CPUTypeShared
	case 'T':
		cpuType = CPUTypeDedicatedThread
	case 'C':
		cpuType = CPUTypeDedicatedCore
	default:
		return fmt.Errorf("unknown CPU type letter %q (want L/V/T/C)", string(letter))
	}

	fn.VCPUs = n
	fn.CPUType = cpuType
	fn.CPUInsecure = insecure
	return nil
}

// parseRAMSegment parses `<N>[.<M>][u][o]`.
func parseRAMSegment(seg string, fn *FlavorName) error {
	if seg == "" {
		return fmt.Errorf("empty RAM segment")
	}

	// Find where the numeric portion ends and suffixes begin. Numeric chars
	// are 0-9 and one optional `.`. After that, suffixes must appear in
	// order: `u` first (no ECC), then `o` (oversubscribed). Anything else,
	// or out-of-order suffixes, is rejected.
	numEnd := 0
	for numEnd < len(seg) {
		c := seg[numEnd]
		if (c >= '0' && c <= '9') || c == '.' {
			numEnd++
			continue
		}
		break
	}
	num := seg[:numEnd]
	suffix := seg[numEnd:]

	noECC := false
	oversub := false
	for i := 0; i < len(suffix); i++ {
		switch suffix[i] {
		case 'u':
			if noECC {
				return fmt.Errorf("duplicate RAM suffix `u`")
			}
			if oversub {
				return fmt.Errorf("invalid RAM suffix order: `u` must come before `o`")
			}
			noECC = true
		case 'o':
			if oversub {
				return fmt.Errorf("duplicate RAM suffix `o`")
			}
			oversub = true
		default:
			return fmt.Errorf("unknown RAM suffix %q (want u/o)", string(suffix[i]))
		}
	}

	if num == "" {
		return fmt.Errorf("missing RAM size")
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return fmt.Errorf("invalid RAM value %q: %w", num, err)
	}
	if v <= 0 {
		return fmt.Errorf("RAM must be > 0, got %v", v)
	}

	fn.RAMGiB = v
	fn.RAMNoECC = noECC
	fn.RAMOversubscribed = oversub
	return nil
}

// parseDiskSegment parses `[M×]<disk_GB>[<disk-type>]`. The multiplier
// separator can be either the unicode `×` (U+00D7) or ASCII `x`.
func parseDiskSegment(seg string, fn *FlavorName) error {
	if seg == "" {
		return fmt.Errorf("empty disk segment")
	}

	count := 1
	// Look for an `x` or `×` separator, but only if what's before it is digits.
	if i := strings.IndexAny(seg, "x×"); i > 0 {
		head := seg[:i]
		if n, err := strconv.Atoi(head); err == nil && n > 0 {
			count = n
			// Skip the multiplier rune (× is 2 bytes in UTF-8, x is 1).
			if seg[i] == 'x' {
				seg = seg[i+1:]
			} else {
				seg = seg[i+2:]
			}
		}
	}

	// Trailing letter (if any) is the disk-type.
	diskType := DiskTypeUnspecified
	if len(seg) > 0 {
		last := seg[len(seg)-1]
		if last < '0' || last > '9' {
			switch last {
			case 'n':
				diskType = DiskTypeNetwork
			case 'h':
				diskType = DiskTypeHDD
			case 's':
				diskType = DiskTypeSSD
			case 'p':
				diskType = DiskTypeNVMe
			default:
				return fmt.Errorf("unknown disk type letter %q (want n/h/s/p)", string(last))
			}
			seg = seg[:len(seg)-1]
		}
	}

	if seg == "" {
		return fmt.Errorf("missing disk size")
	}
	gb, err := strconv.Atoi(seg)
	if err != nil {
		return fmt.Errorf("invalid disk size %q: %w", seg, err)
	}
	if gb <= 0 {
		return fmt.Errorf("disk size must be > 0, got %d", gb)
	}

	fn.HasDisk = true
	fn.DiskCount = count
	fn.DiskGB = gb
	fn.DiskType = diskType
	return nil
}

// parseExtensions handles the underscore-prefixed extension chain. Today we
// only surface the hypervisor; everything else is accepted but ignored.
func parseExtensions(ext string, fn *FlavorName) error {
	// ext begins with "_". Split on "_" and skip the empty leading element.
	parts := strings.Split(ext, "_")
	for _, p := range parts {
		if p == "" {
			continue
		}
		switch p {
		case "kvm", "xen", "chy", "vmw", "hyv", "bms":
			fn.Hypervisor = p
		}
		// Other extensions (CPU arch, GPU, _hwv, _ib) are accepted-but-ignored
		// for now — parsing/validating them is out of scope for this slice.
	}
	return nil
}

// Validate is the public entry point used by API handlers. Names that don't
// start with `SCS-` pass through silently (operators are free to define their
// own non-SCS flavors). SCS-* names must parse cleanly.
func Validate(name string) error {
	if !strings.HasPrefix(name, "SCS-") {
		return nil
	}
	_, err := ParseFlavorName(name)
	return err
}

// ParseFlavorNameOrPassthrough is the convenience form for API handlers: it
// returns (nil, nil) for non-SCS names so the caller can branch on whether
// SCS extra-specs should be mirrored, without duplicating the prefix check
// at every call site.
func ParseFlavorNameOrPassthrough(name string) (*FlavorName, error) {
	if !strings.HasPrefix(name, "SCS-") {
		return nil, nil
	}
	fn, err := ParseFlavorName(name)
	if err != nil {
		return nil, err
	}
	return &fn, nil
}

// ExtraSpecs returns the `scs:*` flavor_extra_specs that mirror the parsed
// name, matching the shape produced by the SCS-0103 seed migration so an
// SCS-aware client sees the same keys regardless of how the flavor was
// created.
func (fn FlavorName) ExtraSpecs() map[string]string {
	specs := map[string]string{
		"scs:cpu-type": string(fn.CPUType),
	}
	if fn.HasDisk && fn.DiskType != DiskTypeUnspecified {
		specs["scs:disk0-type"] = string(fn.DiskType)
	}
	return specs
}
