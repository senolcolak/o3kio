// Package server provides zero-config bootstrap logic for starting O3K
// without a config file or external database.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const defaultDataDir = "/var/lib/o3k"

// DataDir returns the O3K data directory.
// Uses /var/lib/o3k for root, ~/.o3k for non-root.
// O3K_DATA_DIR overrides both.
func DataDir() string {
	if dir := os.Getenv("O3K_DATA_DIR"); dir != "" {
		return dir
	}
	u, err := user.Current()
	if err != nil || u.Uid == "0" {
		return defaultDataDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultDataDir
	}
	return filepath.Join(home, ".o3k")
}

// BootstrapResult holds the values produced by Bootstrap.
type BootstrapResult struct {
	DataDir       string
	DBPath        string
	JWTSecret     string
	AdminPassword string
	AgentToken    string
	// FirstRun is true when the database file did not exist before Bootstrap ran.
	FirstRun bool
}

// Bootstrap ensures the data directory and required secret files exist,
// generating new secrets on first run and loading existing ones on restart.
// It is idempotent: repeated calls return the same values.
func Bootstrap() (*BootstrapResult, error) {
	dataDir := DataDir()
	dbDir := filepath.Join(dataDir, "db")

	if err := os.MkdirAll(dbDir, 0750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	result := &BootstrapResult{
		DataDir: dataDir,
		DBPath:  filepath.Join(dbDir, "state.db"),
	}

	if _, err := os.Stat(result.DBPath); os.IsNotExist(err) {
		result.FirstRun = true
	}

	var err error
	result.JWTSecret, err = loadOrGenerate(filepath.Join(dataDir, "jwt-secret"))
	if err != nil {
		return nil, fmt.Errorf("jwt secret: %w", err)
	}

	result.AdminPassword, err = loadOrGenerate(filepath.Join(dataDir, "initial-password"))
	if err != nil {
		return nil, fmt.Errorf("admin password: %w", err)
	}

	result.AgentToken, err = loadOrGenerate(filepath.Join(dataDir, "agent-token"))
	if err != nil {
		return nil, fmt.Errorf("agent token: %w", err)
	}

	return result, nil
}

// loadOrGenerate reads the secret from path if it exists, or generates a new
// one and writes it to path with mode 0600.
func loadOrGenerate(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		return strings.TrimSpace(string(data)), nil
	}
	secret, err := generateSecret(32)
	if err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	if err := os.WriteFile(path, []byte(secret+"\n"), 0600); err != nil {
		return "", fmt.Errorf("write secret file %s: %w", path, err)
	}
	return secret, nil
}

// generateSecret produces n random bytes encoded as a lowercase hex string.
func generateSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
