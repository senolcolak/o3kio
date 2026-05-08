package common

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpfile, err := os.CreateTemp("", "o3k-test-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	configContent := `
database:
  url: "postgres://test:test@localhost/test"
  max_connections: 10

keystone:
  port: 5000
  jwt_secret: "test-secret"
  token_ttl: 24h
  admin_user: "admin"
  admin_password: "secret"

nova:
  port: 8774
  libvirt_uri: "qemu:///system"
  default_flavor: "m1.small"

neutron:
  port: 9696
  dhcp_lease_time: 24h
  iptables_enabled: true

cinder:
  port: 8776
  ceph_pool: "volumes"
  ceph_conf: "/etc/ceph/ceph.conf"

glance:
  port: 9292
  ceph_pool: "images"
  ceph_conf: "/etc/ceph/ceph.conf"

logging:
  level: "info"
  format: "json"
`

	if _, err := tmpfile.Write([]byte(configContent)); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpfile.Close()

	config, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify database config
	if config.Database.URL != "postgres://test:test@localhost/test" {
		t.Errorf("Expected database URL postgres://test:test@localhost/test, got %s", config.Database.URL)
	}

	if config.Database.MaxConnections != 10 {
		t.Errorf("Expected max connections 10, got %d", config.Database.MaxConnections)
	}

	// Verify keystone config
	if config.Keystone.Port != 5000 {
		t.Errorf("Expected keystone port 5000, got %d", config.Keystone.Port)
	}

	if config.Keystone.JWTSecret != "test-secret" {
		t.Errorf("Expected JWT secret test-secret, got %s", config.Keystone.JWTSecret)
	}

	if config.Keystone.AdminUser != "admin" {
		t.Errorf("Expected admin user admin, got %s", config.Keystone.AdminUser)
	}

	// Verify nova config
	if config.Nova.Port != 8774 {
		t.Errorf("Expected nova port 8774, got %d", config.Nova.Port)
	}

	if config.Nova.LibvirtURI != "qemu:///system" {
		t.Errorf("Expected libvirt URI qemu:///system, got %s", config.Nova.LibvirtURI)
	}

	// Verify neutron config
	if config.Neutron.Port != 9696 {
		t.Errorf("Expected neutron port 9696, got %d", config.Neutron.Port)
	}

	if !config.Neutron.IPTablesEnabled {
		t.Error("Expected iptables enabled")
	}

	// Verify cinder config
	if config.Cinder.Port != 8776 {
		t.Errorf("Expected cinder port 8776, got %d", config.Cinder.Port)
	}

	if config.Cinder.CephPool != "volumes" {
		t.Errorf("Expected ceph pool volumes, got %s", config.Cinder.CephPool)
	}

	// Verify glance config
	if config.Glance.Port != 9292 {
		t.Errorf("Expected glance port 9292, got %d", config.Glance.Port)
	}

	if config.Glance.CephPool != "images" {
		t.Errorf("Expected ceph pool images, got %s", config.Glance.CephPool)
	}

	// Verify logging config
	if config.Logging.Level != "info" {
		t.Errorf("Expected log level info, got %s", config.Logging.Level)
	}

	if config.Logging.Format != "json" {
		t.Errorf("Expected log format json, got %s", config.Logging.Format)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	// A missing config file is the zero-config case — LoadConfig returns
	// an empty Config with no error so the caller can apply bootstrap defaults.
	cfg, err := LoadConfig("/nonexistent/config.yaml")
	if err != nil {
		t.Errorf("Expected nil error for missing config file (zero-config mode), got: %v", err)
	}
	if cfg == nil {
		t.Error("Expected non-nil Config for missing config file")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "o3k-invalid-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	invalidYAML := `
database:
  url: "postgres://test@localhost/test"
  invalid yaml structure
    - broken
`

	if _, err := tmpfile.Write([]byte(invalidYAML)); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpfile.Close()

	_, err = LoadConfig(tmpfile.Name())
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}
