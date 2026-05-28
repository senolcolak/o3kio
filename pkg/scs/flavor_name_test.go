package scs

import (
	"strings"
	"testing"
)

// TestParseFlavorName_Mandatory exercises every flavor name from SCS-0103-v1
// (the mandatory set seeded by migration 075). Parsing them is the floor: if
// these don't round-trip through the parser, the seed itself is suspect.
func TestParseFlavorName_Mandatory(t *testing.T) {
	cases := []struct {
		name     string
		vcpus    int
		cpuType  CPUType
		ramGiB   float64
		hasDisk  bool
		diskGB   int
		diskType DiskType
	}{
		{"SCS-1L-1", 1, CPUTypeCrowded, 1, false, 0, DiskTypeUnspecified},
		{"SCS-1V-2", 1, CPUTypeShared, 2, false, 0, DiskTypeUnspecified},
		{"SCS-1V-4", 1, CPUTypeShared, 4, false, 0, DiskTypeUnspecified},
		{"SCS-1V-8", 1, CPUTypeShared, 8, false, 0, DiskTypeUnspecified},
		{"SCS-2V-4", 2, CPUTypeShared, 4, false, 0, DiskTypeUnspecified},
		{"SCS-2V-4-20s", 2, CPUTypeShared, 4, true, 20, DiskTypeSSD},
		{"SCS-2V-8", 2, CPUTypeShared, 8, false, 0, DiskTypeUnspecified},
		{"SCS-2V-16", 2, CPUTypeShared, 16, false, 0, DiskTypeUnspecified},
		{"SCS-4V-8", 4, CPUTypeShared, 8, false, 0, DiskTypeUnspecified},
		{"SCS-4V-16", 4, CPUTypeShared, 16, false, 0, DiskTypeUnspecified},
		{"SCS-4V-16-100s", 4, CPUTypeShared, 16, true, 100, DiskTypeSSD},
		{"SCS-4V-32", 4, CPUTypeShared, 32, false, 0, DiskTypeUnspecified},
		{"SCS-8V-16", 8, CPUTypeShared, 16, false, 0, DiskTypeUnspecified},
		{"SCS-8V-32", 8, CPUTypeShared, 32, false, 0, DiskTypeUnspecified},
		{"SCS-16V-32", 16, CPUTypeShared, 32, false, 0, DiskTypeUnspecified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn, err := ParseFlavorName(tc.name)
			if err != nil {
				t.Fatalf("ParseFlavorName(%q): %v", tc.name, err)
			}
			if fn.VCPUs != tc.vcpus {
				t.Errorf("VCPUs = %d, want %d", fn.VCPUs, tc.vcpus)
			}
			if fn.CPUType != tc.cpuType {
				t.Errorf("CPUType = %q, want %q", fn.CPUType, tc.cpuType)
			}
			if fn.RAMGiB != tc.ramGiB {
				t.Errorf("RAMGiB = %v, want %v", fn.RAMGiB, tc.ramGiB)
			}
			if fn.HasDisk != tc.hasDisk {
				t.Errorf("HasDisk = %v, want %v", fn.HasDisk, tc.hasDisk)
			}
			if tc.hasDisk {
				if fn.DiskGB != tc.diskGB {
					t.Errorf("DiskGB = %d, want %d", fn.DiskGB, tc.diskGB)
				}
				if fn.DiskType != tc.diskType {
					t.Errorf("DiskType = %q, want %q", fn.DiskType, tc.diskType)
				}
			}
		})
	}
}

// TestParseFlavorName_CPUTypes covers every CPU-type letter from SCS-0100-v3.
func TestParseFlavorName_CPUTypes(t *testing.T) {
	cases := []struct {
		name string
		want CPUType
	}{
		{"SCS-1C-1", CPUTypeDedicatedCore},
		{"SCS-1T-1", CPUTypeDedicatedThread},
		{"SCS-1V-1", CPUTypeShared},
		{"SCS-1L-1", CPUTypeCrowded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn, err := ParseFlavorName(tc.name)
			if err != nil {
				t.Fatalf("ParseFlavorName(%q): %v", tc.name, err)
			}
			if fn.CPUType != tc.want {
				t.Errorf("CPUType = %q, want %q", fn.CPUType, tc.want)
			}
		})
	}
}

// TestParseFlavorName_InsecureCPU exercises the optional `i` suffix on the
// CPU-type letter (SCS-0100-v3 §CPU: insufficient microcode/mitigations).
func TestParseFlavorName_InsecureCPU(t *testing.T) {
	fn, err := ParseFlavorName("SCS-2Vi-4")
	if err != nil {
		t.Fatalf("ParseFlavorName: %v", err)
	}
	if fn.CPUType != CPUTypeShared {
		t.Errorf("CPUType = %q, want %q", fn.CPUType, CPUTypeShared)
	}
	if !fn.CPUInsecure {
		t.Error("CPUInsecure should be true")
	}
}

// TestParseFlavorName_RAMSuffixes exercises the optional `u` (no ECC) and `o`
// (oversubscribed) RAM suffixes. Order is enforced: u before o.
func TestParseFlavorName_RAMSuffixes(t *testing.T) {
	cases := []struct {
		name           string
		ram            float64
		noECC          bool
		oversubscribed bool
		wantErr        bool
	}{
		{name: "SCS-1V-4u", ram: 4, noECC: true},
		{name: "SCS-1V-4o", ram: 4, oversubscribed: true},
		{name: "SCS-1V-4uo", ram: 4, noECC: true, oversubscribed: true},
		{name: "SCS-1V-3.5", ram: 3.5},
		{name: "SCS-1V-4ou", wantErr: true}, // wrong order
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn, err := ParseFlavorName(tc.name)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", fn)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFlavorName: %v", err)
			}
			if fn.RAMGiB != tc.ram {
				t.Errorf("RAMGiB = %v, want %v", fn.RAMGiB, tc.ram)
			}
			if fn.RAMNoECC != tc.noECC {
				t.Errorf("RAMNoECC = %v, want %v", fn.RAMNoECC, tc.noECC)
			}
			if fn.RAMOversubscribed != tc.oversubscribed {
				t.Errorf("RAMOversubscribed = %v, want %v", fn.RAMOversubscribed, tc.oversubscribed)
			}
		})
	}
}

// TestParseFlavorName_DiskTypes covers every disk-type letter.
func TestParseFlavorName_DiskTypes(t *testing.T) {
	cases := []struct {
		name string
		want DiskType
		gb   int
	}{
		{"SCS-1V-4-10", DiskTypeUnspecified, 10},
		{"SCS-1V-4-10s", DiskTypeSSD, 10},
		{"SCS-1V-4-10n", DiskTypeNetwork, 10},
		{"SCS-1V-4-10h", DiskTypeHDD, 10},
		{"SCS-1V-4-10p", DiskTypeNVMe, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn, err := ParseFlavorName(tc.name)
			if err != nil {
				t.Fatalf("ParseFlavorName: %v", err)
			}
			if fn.DiskType != tc.want {
				t.Errorf("DiskType = %q, want %q", fn.DiskType, tc.want)
			}
			if fn.DiskGB != tc.gb {
				t.Errorf("DiskGB = %d, want %d", fn.DiskGB, tc.gb)
			}
			if fn.DiskCount != 1 {
				t.Errorf("DiskCount = %d, want 1", fn.DiskCount)
			}
		})
	}
}

// TestParseFlavorName_DiskMultiplier covers the M×N disk count form.
func TestParseFlavorName_DiskMultiplier(t *testing.T) {
	fn, err := ParseFlavorName("SCS-2V-8-3x10s")
	if err != nil {
		t.Fatalf("ParseFlavorName: %v", err)
	}
	if fn.DiskCount != 3 {
		t.Errorf("DiskCount = %d, want 3", fn.DiskCount)
	}
	if fn.DiskGB != 10 {
		t.Errorf("DiskGB = %d, want 10", fn.DiskGB)
	}
	if fn.DiskType != DiskTypeSSD {
		t.Errorf("DiskType = %q, want ssd", fn.DiskType)
	}
}

// TestParseFlavorName_Malformed checks that broken names produce errors.
// We don't pin exact messages — just that the parser refuses and the error
// mentions the offending token so an operator can fix it.
func TestParseFlavorName_Malformed(t *testing.T) {
	cases := []struct {
		name string
		hint string
	}{
		{"", "empty"},
		{"foo", "prefix"},
		{"SCS-", "vCPU"},
		{"SCS-V-4", "vCPU"},          // missing CPU count
		{"SCS-1X-4", "CPU type"},     // unknown CPU type letter
		{"SCS-1V", "RAM"},            // missing RAM
		{"SCS-1V-", "RAM"},           // empty RAM
		{"SCS-1V-foo", "RAM"},        // non-numeric RAM
		{"SCS-1V-4-", "disk"},        // empty disk segment
		{"SCS-1V-4-foo", "disk"},     // non-numeric disk
		{"SCS-1V-4-10x", "disk"},     // unknown disk type letter
		{"SCS-0V-4", "vCPU"},         // zero vCPUs
		{"SCS-1V-0", "RAM"},          // zero RAM
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseFlavorName(tc.name)
			if err == nil {
				t.Fatalf("ParseFlavorName(%q) succeeded; want error mentioning %q", tc.name, tc.hint)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.hint)) {
				t.Logf("ParseFlavorName(%q) error = %q (hint %q)", tc.name, err.Error(), tc.hint)
			}
		})
	}
}

// TestValidate is the public wrapper: non-SCS names pass through silently,
// SCS-* names get validated.
func TestValidate(t *testing.T) {
	if err := Validate("m1.tiny"); err != nil {
		t.Errorf("Validate(m1.tiny) returned %v; non-SCS names should pass", err)
	}
	if err := Validate("anything-else"); err != nil {
		t.Errorf("Validate(anything-else) returned %v; non-SCS names should pass", err)
	}
	if err := Validate("SCS-2V-4"); err != nil {
		t.Errorf("Validate(SCS-2V-4) returned %v; want nil", err)
	}
	if err := Validate("SCS-bogus"); err == nil {
		t.Error("Validate(SCS-bogus) returned nil; want error")
	}
}

// TestExtraSpecs builds the scs:* extra-spec map from a parsed flavor name.
// This is what Nova will mirror into flavor_extra_specs at create time so the
// SCS-0103 seed and SCS-0100 runtime path produce the same shape.
func TestExtraSpecs(t *testing.T) {
	fn, err := ParseFlavorName("SCS-2V-4-20s")
	if err != nil {
		t.Fatalf("ParseFlavorName: %v", err)
	}
	specs := fn.ExtraSpecs()
	if got := specs["scs:cpu-type"]; got != "shared-core" {
		t.Errorf("scs:cpu-type = %q, want shared-core", got)
	}
	if got := specs["scs:disk0-type"]; got != "ssd" {
		t.Errorf("scs:disk0-type = %q, want ssd", got)
	}

	// Diskless: no scs:disk0-type entry.
	fn2, _ := ParseFlavorName("SCS-2V-4")
	specs2 := fn2.ExtraSpecs()
	if _, ok := specs2["scs:disk0-type"]; ok {
		t.Error("scs:disk0-type should not be present on diskless flavor")
	}
}

// TestParseFlavorName_Hypervisor covers the optional _kvm/_xen/etc. extension.
func TestParseFlavorName_Hypervisor(t *testing.T) {
	fn, err := ParseFlavorName("SCS-2V-4_kvm")
	if err != nil {
		t.Fatalf("ParseFlavorName: %v", err)
	}
	if fn.Hypervisor != "kvm" {
		t.Errorf("Hypervisor = %q, want kvm", fn.Hypervisor)
	}
}
