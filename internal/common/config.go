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

// LoadConfig loads configuration from file and applies environment variable overrides
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
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

	// Refuse to start with default JWT secret in production
	if cfg.Keystone.JWTSecret == "" || cfg.Keystone.JWTSecret == "change-me-in-production" {
		env := os.Getenv("O3K_ENV")
		if env != "development" && env != "test" {
			fmt.Fprintln(os.Stderr, "FATAL: JWT secret is set to the insecure default. Set O3K_JWT_SECRET or set O3K_ENV=development to allow default in dev mode.")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "WARNING: Using default JWT secret (O3K_ENV="+env+"). Do NOT use this in production.")
	}

	return &cfg, nil
}
