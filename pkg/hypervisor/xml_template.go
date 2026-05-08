package hypervisor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// VMSpec defines VM specifications
type VMSpec struct {
	UUID        string
	Name        string
	VCPUs       int
	MemoryMB    int
	DiskGB      int
	ImagePath   string   // Path to image (RBD or local)
	Networks    []NetworkConfig
	Volumes     []VolumeConfig
	CloudInit   *CloudInitConfig
}

// NetworkConfig defines network interface configuration
type NetworkConfig struct {
	PortID     string
	MACAddress string
	BridgeName string
	IPAddress  string
	NetworkID  string
}

// VolumeConfig defines volume attachment configuration
type VolumeConfig struct {
	VolumeID  string
	RBDPool   string
	RBDImage  string
	Device    string // vda, vdb, etc.
}

// CloudInitConfig defines cloud-init configuration
type CloudInitConfig struct {
	MetaData string
	UserData string
}

// GenerateVMXML generates libvirt XML for a VM
func GenerateVMXML(spec VMSpec) string {
	var sb strings.Builder

	// Domain header
	sb.WriteString(fmt.Sprintf(`<domain type='kvm'>
  <name>%s</name>
  <uuid>%s</uuid>
  <memory unit='MiB'>%d</memory>
  <currentMemory unit='MiB'>%d</currentMemory>
  <vcpu placement='static'>%d</vcpu>

  <os>
    <type arch='x86_64' machine='pc-i440fx-2.12'>hvm</type>
    <boot dev='hd'/>
  </os>

  <features>
    <acpi/>
    <apic/>
    <pae/>
  </features>

  <cpu mode='host-model'>
    <model fallback='allow'/>
  </cpu>

  <clock offset='utc'>
    <timer name='rtc' tickpolicy='catchup'/>
    <timer name='pit' tickpolicy='delay'/>
    <timer name='hpet' present='no'/>
  </clock>

  <on_poweroff>destroy</on_poweroff>
  <on_reboot>restart</on_reboot>
  <on_crash>restart</on_crash>

  <devices>
    <emulator>/usr/bin/qemu-system-x86_64</emulator>
`,
		xmlEscape(spec.Name), xmlEscape(spec.UUID), spec.MemoryMB, spec.MemoryMB, spec.VCPUs))

	// Boot disk (RBD-backed or local)
	if strings.HasPrefix(spec.ImagePath, "rbd:") {
		// RBD image format: rbd:pool/image
		parts := strings.Split(strings.TrimPrefix(spec.ImagePath, "rbd:"), "/")
		pool := parts[0]
		image := parts[1]

		sb.WriteString(fmt.Sprintf(`
    <disk type='network' device='disk'>
      <driver name='qemu' type='qcow2' cache='writeback'/>
      <source protocol='rbd' name='%s/%s'>
        <host name='127.0.0.1' port='6789'/>
      </source>
      <target dev='vda' bus='virtio'/>
    </disk>
`, xmlEscape(pool), xmlEscape(image)))
	} else {
		// Local file
		sb.WriteString(fmt.Sprintf(`
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2' cache='writeback'/>
      <source file='%s'/>
      <target dev='vda' bus='virtio'/>
    </disk>
`, xmlEscape(spec.ImagePath)))
	}

	// Attached volumes
	for i, vol := range spec.Volumes {
		device := vol.Device
		if device == "" {
			device = fmt.Sprintf("vd%c", 'b'+i) // vdb, vdc, etc.
		}

		sb.WriteString(fmt.Sprintf(`
    <disk type='network' device='disk'>
      <driver name='qemu' type='raw'/>
      <source protocol='rbd' name='%s/%s'>
        <host name='127.0.0.1' port='6789'/>
      </source>
      <target dev='%s' bus='virtio'/>
    </disk>
`, xmlEscape(vol.RBDPool), xmlEscape(vol.RBDImage), xmlEscape(device)))
	}

	// Network interfaces
	for _, net := range spec.Networks {
		sb.WriteString(fmt.Sprintf(`
    <interface type='bridge'>
      <mac address='%s'/>
      <source bridge='%s'/>
      <model type='virtio'/>
    </interface>
`, xmlEscape(net.MACAddress), xmlEscape(net.BridgeName)))
	}

	// Serial console
	sb.WriteString(`
    <serial type='pty'>
      <target port='0'/>
    </serial>
    <console type='pty'>
      <target type='serial' port='0'/>
    </console>
`)

	// VNC graphics
	sb.WriteString(`
    <graphics type='vnc' port='-1' autoport='yes' listen='0.0.0.0'>
      <listen type='address' address='0.0.0.0'/>
    </graphics>
`)

	// Cloud-init (if provided)
	if spec.CloudInit != nil {
		sb.WriteString(fmt.Sprintf(`
    <disk type='file' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <source file='/var/lib/o3k/cloud-init/%s.iso'/>
      <target dev='hdc' bus='ide'/>
      <readonly/>
    </disk>
`, spec.UUID))
	}

	// Close devices and domain
	sb.WriteString(`  </devices>
</domain>
`)

	return sb.String()
}

// GenerateCloudInitISO generates cloud-init ISO content
func GenerateCloudInitISO(uuid string, config *CloudInitConfig) (string, error) {
	if config == nil {
		return "", nil // No cloud-init requested
	}

	isoDir := "/var/lib/o3k/cloud-init"
	isoPath := fmt.Sprintf("%s/%s.iso", isoDir, uuid)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(isoDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cloud-init directory: %w", err)
	}

	// Create temporary directory for cloud-init files
	tmpDir, err := os.MkdirTemp("", "cloud-init-"+uuid)
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Write meta-data file
	metaDataPath := filepath.Join(tmpDir, "meta-data")
	if err := os.WriteFile(metaDataPath, []byte(config.MetaData), 0644); err != nil {
		return "", fmt.Errorf("failed to write meta-data: %w", err)
	}

	// Write user-data file
	userDataPath := filepath.Join(tmpDir, "user-data")
	if err := os.WriteFile(userDataPath, []byte(config.UserData), 0644); err != nil {
		return "", fmt.Errorf("failed to write user-data: %w", err)
	}

	// Generate ISO using genisoimage (or mkisofs as fallback)
	// Try genisoimage first (Debian/Ubuntu)
	cmd := exec.Command("genisoimage",
		"-output", isoPath,
		"-volid", "cidata",
		"-joliet",
		"-rock",
		metaDataPath,
		userDataPath,
	)

	var output []byte
	var err2 error
	_, err2 = cmd.CombinedOutput()
	if err2 != nil {
		// Try mkisofs as fallback (older systems)
		cmd = exec.Command("mkisofs",
			"-output", isoPath,
			"-volid", "cidata",
			"-joliet",
			"-rock",
			metaDataPath,
			userDataPath,
		)
		output, err2 = cmd.CombinedOutput()
		if err2 != nil {
			return "", fmt.Errorf("failed to create ISO (genisoimage/mkisofs not available): %w, output: %s", err2, output)
		}
	}

	return isoPath, nil
}

// DefaultCloudInitConfig returns default cloud-init configuration
func DefaultCloudInitConfig(hostname, sshKey string) *CloudInitConfig {
	safeHostname := sanitizeHostname(hostname)

	metaData := fmt.Sprintf(`instance-id: %s
local-hostname: %s
`, safeHostname, safeHostname)

	userData := `#cloud-config
packages:
  - curl
  - vim
runcmd:
  - echo "O3K VM booted successfully" > /var/log/o3k.log
`

	if sshKey != "" && !strings.ContainsAny(sshKey, "\n\r") {
		userData += fmt.Sprintf(`
ssh_authorized_keys:
  - %s
`, sshKey)
	}

	return &CloudInitConfig{
		MetaData: metaData,
		UserData: userData,
	}
}

var validHostnameRe = regexp.MustCompile(`[^a-zA-Z0-9\-.]`)

func sanitizeHostname(h string) string {
	h = strings.ReplaceAll(h, "\n", "")
	h = strings.ReplaceAll(h, "\r", "")
	h = validHostnameRe.ReplaceAllString(h, "-")
	if len(h) > 63 {
		h = h[:63]
	}
	return h
}

// DiskSpec defines disk device configuration
type DiskSpec struct {
	Device   string // e.g., /dev/vdb, /dev/vdc
	Type     string // "network" for RBD, "file" for local
	Source   string // RBD: "pool/image", File: "/path/to/file.qcow2"
	Protocol string // "rbd" for Ceph RBD
}

// GenerateDiskXML generates libvirt XML for a disk device
func GenerateDiskXML(spec DiskSpec) string {
	var sb strings.Builder

	// Extract device letter from path (e.g., "/dev/vdb" -> "vdb")
	device := strings.TrimPrefix(spec.Device, "/dev/")

	if spec.Type == "network" && spec.Protocol == "rbd" {
		// RBD network disk
		sb.WriteString(fmt.Sprintf(`<disk type='network' device='disk'>
  <driver name='qemu' type='raw' cache='writeback'/>
  <source protocol='rbd' name='%s'>
    <host name='127.0.0.1' port='6789'/>
  </source>
  <target dev='%s' bus='virtio'/>
</disk>`, xmlEscape(spec.Source), xmlEscape(device)))
	} else {
		// Local file disk
		sb.WriteString(fmt.Sprintf(`<disk type='file' device='disk'>
  <driver name='qemu' type='qcow2' cache='writeback'/>
  <source file='%s'/>
  <target dev='%s' bus='virtio'/>
</disk>`, xmlEscape(spec.Source), xmlEscape(device)))
	}

	return sb.String()
}
