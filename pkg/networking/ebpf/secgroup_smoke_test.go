//go:build ebpf_integration
// +build ebpf_integration

// Smoke test that verifies the compiled eBPF object can be loaded by cilium/ebpf
// and exposes the program + maps the Go code expects. Requires:
//   - Linux with BPF support (CONFIG_BPF=y, CONFIG_BPF_SYSCALL=y)
//   - CAP_BPF / CAP_SYS_ADMIN (run as root or with sudo)
//   - The compiled object at pkg/networking/ebpf/secgroup.o (run `make build-ebpf` first)
//
// Run with:
//   sudo -E env "PATH=$PATH" go test -tags ebpf_integration ./pkg/networking/ebpf/ -run TestEBPFLoad -v

package ebpf

import (
	"path/filepath"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
)

const ebpfObjectRelPath = "secgroup.o"

func TestEBPFLoad_CollectionSpec(t *testing.T) {
	objPath, err := filepath.Abs(ebpfObjectRelPath)
	if err != nil {
		t.Fatalf("resolve object path: %v", err)
	}

	spec, err := ebpf.LoadCollectionSpec(objPath)
	if err != nil {
		t.Fatalf("LoadCollectionSpec(%s): %v", objPath, err)
	}

	if _, ok := spec.Programs["xdp_security_group_filter"]; !ok {
		t.Errorf("program 'xdp_security_group_filter' missing from spec; have %v", programNames(spec))
	}
	for _, mapName := range []string{"sg_rules", "sg_statistics"} {
		if _, ok := spec.Maps[mapName]; !ok {
			t.Errorf("map %q missing from spec; have %v", mapName, mapNames(spec))
		}
	}
}

func TestEBPFLoad_NewCollection(t *testing.T) {
	if err := rlimit.RemoveMemlock(); err != nil {
		t.Fatalf("RemoveMemlock (need CAP_SYS_RESOURCE / root): %v", err)
	}

	objPath, err := filepath.Abs(ebpfObjectRelPath)
	if err != nil {
		t.Fatalf("resolve object path: %v", err)
	}

	spec, err := ebpf.LoadCollectionSpec(objPath)
	if err != nil {
		t.Fatalf("LoadCollectionSpec: %v", err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		t.Fatalf("NewCollection (need CAP_BPF / root): %v", err)
	}
	defer coll.Close()

	prog, ok := coll.Programs["xdp_security_group_filter"]
	if !ok || prog == nil {
		t.Fatalf("loaded collection missing program 'xdp_security_group_filter'")
	}
	if got := prog.Type(); got != ebpf.XDP {
		t.Errorf("program type = %v, want XDP", got)
	}

	sgRules, ok := coll.Maps["sg_rules"]
	if !ok || sgRules == nil {
		t.Fatalf("loaded collection missing map 'sg_rules'")
	}
	if got := sgRules.Type(); got != ebpf.Hash {
		t.Errorf("sg_rules type = %v, want Hash", got)
	}

	sgStats, ok := coll.Maps["sg_statistics"]
	if !ok || sgStats == nil {
		t.Fatalf("loaded collection missing map 'sg_statistics'")
	}
}

func TestEBPFLoad_NewSecurityGroupManager(t *testing.T) {
	if err := rlimit.RemoveMemlock(); err != nil {
		t.Fatalf("RemoveMemlock: %v", err)
	}

	objPath, err := filepath.Abs(ebpfObjectRelPath)
	if err != nil {
		t.Fatalf("resolve object path: %v", err)
	}

	mgr, err := NewSecurityGroupManager(objPath)
	if err != nil {
		t.Fatalf("NewSecurityGroupManager: %v", err)
	}
	defer mgr.Close()

	if mgr.prog == nil || mgr.sgRules == nil || mgr.sgStats == nil {
		t.Fatal("manager has nil program or maps after construction")
	}
}

func programNames(spec *ebpf.CollectionSpec) []string {
	out := make([]string, 0, len(spec.Programs))
	for name := range spec.Programs {
		out = append(out, name)
	}
	return out
}

func mapNames(spec *ebpf.CollectionSpec) []string {
	out := make([]string, 0, len(spec.Maps))
	for name := range spec.Maps {
		out = append(out, name)
	}
	return out
}
