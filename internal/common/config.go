package common

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Database DatabaseConfig `yaml:"database"`
	Keystone KeystoneConfig `yaml:"keystone"`
	Nova     NovaConfig     `yaml:"nova"`
	Neutron  NeutronConfig  `yaml:"neutron"`
	Cinder   CinderConfig   `yaml:"cinder"`
	Glance   GlanceConfig   `yaml:"glance"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type DatabaseConfig struct {
	URL            string `yaml:"url"`
	MaxConnections int    `yaml:"max_connections"`
}

type KeystoneConfig struct {
	Port          int           `yaml:"port"`
	JWTSecret     string        `yaml:"jwt_secret"`
	TokenTTL      time.Duration `yaml:"token_ttl"`
	AdminUser     string        `yaml:"admin_user"`
	AdminPassword string        `yaml:"admin_password"`
}

type NovaConfig struct {
	Port           int    `yaml:"port"`
	LibvirtURI     string `yaml:"libvirt_uri"`
	DefaultFlavor  string `yaml:"default_flavor"`
	LibvirtMode    string `yaml:"libvirt_mode"` // "stub" or "real"
}

type NeutronConfig struct {
	Port             int           `yaml:"port"`
	DHCPLeaseTime    time.Duration `yaml:"dhcp_lease_time"`
	IPTablesEnabled  bool          `yaml:"iptables_enabled"`
}

type CinderConfig struct {
	Port     int    `yaml:"port"`
	CephPool string `yaml:"ceph_pool"`
	CephConf string `yaml:"ceph_conf"`
}

type GlanceConfig struct {
	Port     int    `yaml:"port"`
	CephPool string `yaml:"ceph_pool"`
	CephConf string `yaml:"ceph_conf"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
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

	// Environment variable overrides
	if dbURL := os.Getenv("O3K_DB_URL"); dbURL != "" {
		cfg.Database.URL = dbURL
	}
	if jwtSecret := os.Getenv("O3K_JWT_SECRET"); jwtSecret != "" {
		cfg.Keystone.JWTSecret = jwtSecret
	}

	// Warn if using default JWT secret
	if cfg.Keystone.JWTSecret == "change-me-in-production" {
		fmt.Fprintln(os.Stderr, "WARNING: Using default JWT secret! Set O3K_JWT_SECRET environment variable in production.")
	}

	return &cfg, nil
}
