package networking

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"text/template"
)

// DHCPManager manages DHCP servers (dnsmasq)
type DHCPManager struct {
	mode        string // "stub", "iptables", or "ebpf"
	leasePath   string
	configPath  string
	pidPath     string
	mu          sync.Mutex
	runningPIDs map[string]int // networkID -> PID
	stubDHCP    map[string]bool // For stub mode
}

// NewDHCPManager creates a new DHCP manager
func NewDHCPManager(mode string) *DHCPManager {
	mgr := &DHCPManager{
		mode:        mode,
		runningPIDs: make(map[string]int),
		stubDHCP:    make(map[string]bool),
	}

	if mode != "stub" {
		// Both iptables and eBPF use real DHCP
		baseDir := "/var/lib/o3k/dhcp"
		os.MkdirAll(baseDir, 0755)
		os.MkdirAll(filepath.Join(baseDir, "leases"), 0755)
		os.MkdirAll(filepath.Join(baseDir, "configs"), 0755)
		os.MkdirAll(filepath.Join(baseDir, "pids"), 0755)

		mgr.leasePath = filepath.Join(baseDir, "leases")
		mgr.configPath = filepath.Join(baseDir, "configs")
		mgr.pidPath = filepath.Join(baseDir, "pids")
	}

	return mgr
}

// DHCPConfig represents DHCP configuration
type DHCPConfig struct {
	NetworkID      string
	BridgeName     string
	SubnetCIDR     string
	GatewayIP      string
	DNSServers     []string
	LeaseFile      string
	PIDFile        string
	DHCPRangeStart string
	DHCPRangeEnd   string
	LeaseTime      string
}

// StartDHCP starts a DHCP server for a network
func (m *DHCPManager) StartDHCP(config DHCPConfig, nsName string) error {
	if m.mode == "stub" {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.stubDHCP[config.NetworkID] = true
		return nil
	}

	leaseFile := filepath.Join(m.leasePath, config.NetworkID+".leases")
	pidFile := filepath.Join(m.pidPath, config.NetworkID+".pid")
	configFile := filepath.Join(m.configPath, config.NetworkID+".conf")

	config.LeaseFile = leaseFile
	config.PIDFile = pidFile

	// Create dnsmasq config file
	if err := m.writeDNSMasqConfig(configFile, config); err != nil {
		return err
	}

	// Build dnsmasq command
	args := []string{
		"--interface=" + config.BridgeName,
		"--bind-interfaces",
		"--dhcp-range=" + config.DHCPRangeStart + "," + config.DHCPRangeEnd + "," + config.LeaseTime,
		"--dhcp-leasefile=" + leaseFile,
		"--pid-file=" + pidFile,
		"--no-daemon",
		"--log-facility=-", // Log to stderr
	}

	if config.GatewayIP != "" {
		args = append(args, "--dhcp-option=3,"+config.GatewayIP) // Gateway
	}

	if len(config.DNSServers) > 0 {
		for _, dns := range config.DNSServers {
			args = append(args, "--dhcp-option=6,"+dns) // DNS
		}
	}

	// Start dnsmasq in namespace
	var cmd *exec.Cmd
	if nsName != "" {
		fullArgs := append([]string{"netns", "exec", nsName, "dnsmasq"}, args...)
		cmd = exec.Command("ip", fullArgs...)
	} else {
		cmd = exec.Command("dnsmasq", args...)
	}

	// Start in background
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start dnsmasq for network %s: %w", config.NetworkID, err)
	}

	m.runningPIDs[config.NetworkID] = cmd.Process.Pid
	return nil
}

// StopDHCP stops a DHCP server
func (m *DHCPManager) StopDHCP(networkID string) error {
	pidFile := filepath.Join(m.pidPath, networkID+".pid")

	// Read PID from file
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return nil // Already stopped
	}

	var pid int
	fmt.Sscanf(string(pidData), "%d", &pid)

	// Kill process
	if pid > 0 {
		proc, err := os.FindProcess(pid)
		if err == nil {
			proc.Kill()
		}
	}

	// Clean up files
	os.Remove(pidFile)
	os.Remove(filepath.Join(m.leasePath, networkID+".leases"))
	os.Remove(filepath.Join(m.configPath, networkID+".conf"))

	delete(m.runningPIDs, networkID)
	return nil
}

// writeDNSMasqConfig writes dnsmasq configuration file
func (m *DHCPManager) writeDNSMasqConfig(configFile string, config DHCPConfig) error {
	tmpl := `# dnsmasq configuration for network {{ .NetworkID }}
interface={{ .BridgeName }}
bind-interfaces
dhcp-range={{ .DHCPRangeStart }},{{ .DHCPRangeEnd }},{{ .LeaseTime }}
dhcp-leasefile={{ .LeaseFile }}
pid-file={{ .PIDFile }}
{{ if .GatewayIP }}dhcp-option=3,{{ .GatewayIP }}{{ end }}
{{ range .DNSServers }}dhcp-option=6,{{ . }}
{{ end }}
`

	t, err := template.New("dnsmasq").Parse(tmpl)
	if err != nil {
		return err
	}

	f, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer f.Close()

	return t.Execute(f, config)
}

// IsRunning checks if DHCP server is running for a network
func (m *DHCPManager) IsRunning(networkID string) bool {
	pid, exists := m.runningPIDs[networkID]
	if !exists {
		return false
	}

	// Check if process is still running
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = proc.Signal(os.Signal(nil))
	return err == nil
}
