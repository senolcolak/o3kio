package common

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Database DatabaseConfig `yaml:"database"`
	Compute  ComputeConfig  `yaml:"compute"`
	Keystone KeystoneConfig `yaml:"keystone"`
	Nova     NovaConfig     `yaml:"nova"`
	Neutron  NeutronConfig  `yaml:"neutron"`
	Cinder   CinderConfig   `yaml:"cinder"`
	Glance   GlanceConfig   `yaml:"glance"`
	Logging  LoggingConfig  `yaml:"logging"`
	Cache    CacheConfig    `yaml:"cache"`
	Server   ServerConfig   `yaml:"server"`
	Tunnel   TunnelConfig   `yaml:"tunnel"`
	Tasks    TaskConfig     `yaml:"tasks"`
}

// TaskConfig holds tuning knobs for the async task worker pool and reconciler.
type TaskConfig struct {
	MaxWorkers         int `yaml:"max_workers"`
	ReconcilerInterval int `yaml:"reconciler_interval_sec"`
}

type ServerConfig struct {
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"`
	BindHost           string   `yaml:"bind_host"` // Default bind address for all services; defaults to "127.0.0.1"
}

type DatabaseConfig struct {
	URL               string        `yaml:"url"`
	Datastore         string        `yaml:"datastore"`          // "sqlite:///path/to/db" or "postgres://..." — overrides URL if set
	MaxConnections    int           `yaml:"max_connections"`
	MinConnections    int           `yaml:"min_connections"`
	MaxConnLifetime   time.Duration `yaml:"max_conn_lifetime"`
	MaxConnIdleTime   time.Duration `yaml:"max_conn_idle_time"`
	HealthCheckPeriod time.Duration `yaml:"health_check_period"`
}

type ComputeConfig struct {
	NodeID            string        `yaml:"node_id"`
	TunnelIP          string        `yaml:"tunnel_ip"`
	VXLANPort         int           `yaml:"vxlan_port"`
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
}

type KeystoneConfig struct {
	Port          int           `yaml:"port"`
	JWTSecret     string        `yaml:"jwt_secret"`
	TokenTTL      time.Duration `yaml:"token_ttl"`
	AdminUser     string        `yaml:"admin_user"`
	AdminPassword string        `yaml:"admin_password"`
}

type NovaConfig struct {
	Port          int    `yaml:"port"`
	LibvirtURI    string `yaml:"libvirt_uri"`
	DefaultFlavor string `yaml:"default_flavor"`
	LibvirtMode   string `yaml:"libvirt_mode"` // "stub" or "real"
	AsyncCompute  bool   `yaml:"async_compute"`
}

// TunnelConfig holds configuration for the agent tunnel / join-token system.
type TunnelConfig struct {
	Port        int    `yaml:"port"`
	TokenSecret string `yaml:"token_secret"`
	TokenFile   string `yaml:"token_file"`
}

type NeutronConfig struct {
	Port                     int           `yaml:"port"`
	DHCPLeaseTime            time.Duration `yaml:"dhcp_lease_time"`
	IPTablesEnabled          bool          `yaml:"iptables_enabled"`
	NetworkingMode           string        `yaml:"networking_mode"` // "stub", "iptables", or "ebpf"
	VXLANEnabled             bool          `yaml:"vxlan_enabled"`
	VNIRangeStart            int           `yaml:"vni_range_start"`
	VNIRangeEnd              int           `yaml:"vni_range_end"`
	CoordinationPollInterval time.Duration `yaml:"coordination_poll_interval"`
	VXLANMTU                 int           `yaml:"vxlan_mtu"`
}

type CinderConfig struct {
	Port        int    `yaml:"port"`
	CephPool    string `yaml:"ceph_pool"`
	CephConf    string `yaml:"ceph_conf"`
	StorageMode string `yaml:"storage_mode"` // "stub" or "real"
}

type GlanceConfig struct {
	Port        int    `yaml:"port"`
	CephPool    string `yaml:"ceph_pool"`
	CephConf    string `yaml:"ceph_conf"`
	StorageMode string `yaml:"storage_mode"` // "stub", "local", "rbd", "s3", or combinations
	S3Bucket    string `yaml:"s3_bucket"`    // S3 bucket name
	S3Region    string `yaml:"s3_region"`    // S3 region
	S3Endpoint  string `yaml:"s3_endpoint"`  // Custom S3 endpoint (for MinIO, Ceph RGW, etc.)
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type CacheConfig struct {
	Enabled    bool                     `yaml:"enabled"`
	RedisURL   string                   `yaml:"redis_url"`
	KeyPrefix  string                   `yaml:"key_prefix"`
	DefaultTTL time.Duration            `yaml:"default_ttl"`
	TTL        map[string]time.Duration `yaml:"ttl"` // Per-resource TTL overrides
}

// expandEnvWithDefault expands environment variable references in the form
// ${VAR_NAME:default_value}. If VAR_NAME is set, its value is used; otherwise
// default_value is used.
func expandEnvWithDefault(s string) string {
	// Handle ${VAR:default} pattern
	result := s
	for {
		start := -1
		for i := range len(result) - 1 {
			if result[i] == '$' && result[i+1] == '{' {
				start = i
				break
			}
		}
		if start == -1 {
			break
		}
		end := -1
		for i := start + 2; i < len(result); i++ {
			if result[i] == '}' {
				end = i
				break
			}
		}
		if end == -1 {
			break
		}
		expr := result[start+2 : end]
		var name, defaultVal string
		if colonIdx := -1; true {
			for i := range len(expr) {
				if expr[i] == ':' {
					colonIdx = i
					break
				}
			}
			if colonIdx >= 0 {
				name = expr[:colonIdx]
				defaultVal = expr[colonIdx+1:]
			} else {
				name = expr
			}
		}
		val := os.Getenv(name)
		if val == "" {
			val = defaultVal
		}
		result = result[:start] + val + result[end+1:]
	}
	return result
}

// LoadConfig loads configuration from file and applies environment variable overrides.
// If path does not exist, an empty Config is returned — zero-config mode.
func LoadConfig(path string) (*Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// File not found — proceed with zero-config empty struct.
	} else if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Expand environment variable references in database URL
	cfg.Database.URL = expandEnvWithDefault(cfg.Database.URL)
	cfg.Database.Datastore = expandEnvWithDefault(cfg.Database.Datastore)

	// Environment variable overrides
	if dbURL := os.Getenv("O3K_DB_URL"); dbURL != "" {
		cfg.Database.URL = dbURL
	}
	if ds := os.Getenv("O3K_DATASTORE"); ds != "" {
		cfg.Database.Datastore = ds
	}
	if jwtSecret := os.Getenv("O3K_JWT_SECRET"); jwtSecret != "" {
		cfg.Keystone.JWTSecret = jwtSecret
	}
	if s3Bucket := os.Getenv("O3K_S3_BUCKET"); s3Bucket != "" {
		cfg.Glance.S3Bucket = s3Bucket
	}
	if s3Region := os.Getenv("O3K_S3_REGION"); s3Region != "" {
		cfg.Glance.S3Region = s3Region
	}
	if s3Endpoint := os.Getenv("O3K_S3_ENDPOINT"); s3Endpoint != "" {
		cfg.Glance.S3Endpoint = s3Endpoint
	}

	// Refuse to start with default JWT secret in production.
	// Skip the check when no secret is set at all — zero-config mode will
	// supply a bootstrap-generated secret before starting any service.
	if cfg.Keystone.JWTSecret == "change-me-in-production" {
		env := os.Getenv("O3K_ENV")
		if env != "development" && env != "test" {
			fmt.Fprintln(os.Stderr, "FATAL: JWT secret is set to the insecure default. Set O3K_JWT_SECRET or set O3K_ENV=development to allow default in dev mode.")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "WARNING: Using default JWT secret (O3K_ENV="+env+"). Do NOT use this in production.")
	}

	return &cfg, nil
}

// ValidateConfig checks required fields and allowed enum values.
// It does not enforce JWT secret strength — that check lives in main.go
// because zero-config mode may supply the secret after LoadConfig returns.
func ValidateConfig(cfg *Config) error {
	// Database: at least one of URL or Datastore must be set (zero-config
	// mode sets them after this call, so only validate when both are present).
	if cfg.Database.URL != "" || cfg.Database.Datastore != "" {
		ds := cfg.Database.Datastore
		if ds == "" {
			ds = cfg.Database.URL
		}
		if ds == "" {
			return fmt.Errorf("database.url or database.datastore must not be empty")
		}
	}

	// Service ports: 0 means "not configured" (zero-config mode uses defaults),
	// so only validate ports that are explicitly set to an out-of-range value.
	ports := map[string]int{
		"keystone.port": cfg.Keystone.Port,
		"nova.port":     cfg.Nova.Port,
		"neutron.port":  cfg.Neutron.Port,
		"cinder.port":   cfg.Cinder.Port,
		"glance.port":   cfg.Glance.Port,
		"tunnel.port":   cfg.Tunnel.Port,
	}
	for name, port := range ports {
		if port != 0 && (port < 1 || port > 65535) {
			return fmt.Errorf("%s: invalid port %d (must be 1-65535)", name, port)
		}
	}

	// Storage mode enum validation.
	allowedStorageModes := map[string]bool{
		"stub": true, "local": true, "rbd": true, "s3": true,
		"local,rbd": true, "local,s3": true, "rbd,s3": true,
		"local,rbd,s3": true,
	}
	if mode := cfg.Cinder.StorageMode; mode != "" && !allowedStorageModes[mode] {
		return fmt.Errorf("cinder.storage_mode: unknown value %q", mode)
	}
	if mode := cfg.Glance.StorageMode; mode != "" && !allowedStorageModes[mode] {
		return fmt.Errorf("glance.storage_mode: unknown value %q", mode)
	}

	// Networking mode enum validation.
	allowedNetworkingModes := map[string]bool{
		"stub": true, "iptables": true, "ebpf": true, "real": true,
	}
	if mode := cfg.Neutron.NetworkingMode; mode != "" && !allowedNetworkingModes[mode] {
		return fmt.Errorf("neutron.networking_mode: unknown value %q", mode)
	}

	// Real libvirt mode requires a URI.
	if cfg.Nova.LibvirtMode == "real" && cfg.Nova.LibvirtURI == "" {
		return fmt.Errorf("nova.libvirt_uri must be set when nova.libvirt_mode is \"real\"")
	}

	return nil
}

// BindAddress returns "host:port", using cfg.Server.BindHost as the host
// component. When BindHost is empty it defaults to "127.0.0.1" (loopback
// only). Pass "0.0.0.0" in the config to listen on all interfaces.
func BindAddress(cfg *Config, port int) string {
	host := cfg.Server.BindHost
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%d", host, port)
}
